package service

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/dto"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/repository"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/vo"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/errs"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/logger"
)

// MenuService 负责菜单（目录/菜单/按钮）的增删改查。
//
// 与角色一样，菜单变更会影响 Casbin 策略——策略由 sys_role_menu 关联的
// sys_menu.perms 生成，改 perms 或删菜单后必须刷新，否则旧策略继续生效。
// CLAUDE.md 只提到「角色改权限要刷新」，菜单侧同样需要，容易遗漏。
type MenuService struct {
	menus       *repository.MenuRepository
	permissions *PermissionService
}

func NewMenuService(menus *repository.MenuRepository, permissions *PermissionService) *MenuService {
	return &MenuService{menus: menus, permissions: permissions}
}

// Tree 返回完整菜单树（含隐藏与停用项），供菜单管理页与角色授权树使用。
func (s *MenuService) Tree(ctx context.Context) ([]*vo.MenuNode, error) {
	menus, err := s.menus.FindAll(ctx)
	if err != nil {
		return nil, errs.Internal("查询菜单失败").WithCause(err)
	}
	return vo.BuildMenuTree(menus), nil
}

// Create 新增菜单。
func (s *MenuService) Create(ctx context.Context, operatorID uint64, req *dto.CreateMenuRequest) error {
	if err := s.validateParent(ctx, req.ParentID); err != nil {
		return err
	}
	if err := s.validateShape(req.Type, req.Path, req.Component, req.Perms); err != nil {
		return err
	}
	if err := s.checkPermsUnique(ctx, req.Perms, 0); err != nil {
		return err
	}

	menu := &model.Menu{
		ParentID:  req.ParentID,
		Name:      req.Name,
		Type:      req.Type,
		Path:      strings.TrimSpace(req.Path),
		Component: strings.TrimSpace(req.Component),
		Perms:     strings.TrimSpace(req.Perms),
		Icon:      req.Icon,
		Sort:      req.Sort,
		Visible:   defaultInt8(req.Visible, model.StatusEnabled),
		Status:    defaultInt8(req.Status, model.StatusEnabled),
		IsFrame:   defaultInt8(req.IsFrame, 0),
	}
	menu.CreatedBy = &operatorID
	menu.UpdatedBy = &operatorID

	if err := s.menus.Create(ctx, menu); err != nil {
		return errs.Internal("创建菜单失败").WithCause(err)
	}

	// 新菜单尚未授予任何角色，理论上不影响现有策略；
	// 但仍刷新一次，避免「先建按钮再授权」的流程里出现时序空窗。
	s.reloadPolicies(ctx, "新增菜单")
	return nil
}

// Update 修改菜单。
func (s *MenuService) Update(ctx context.Context, operatorID, id uint64, req *dto.UpdateMenuRequest) error {
	menu, err := s.menus.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errs.ErrMenuNotFound
		}
		return err
	}

	if req.ParentID != nil && *req.ParentID != menu.ParentID {
		if err := s.validateParent(ctx, *req.ParentID); err != nil {
			return err
		}
		if err := s.validateNotDescendant(ctx, id, *req.ParentID); err != nil {
			return err
		}
		menu.ParentID = *req.ParentID
	}

	if req.Name != nil {
		menu.Name = *req.Name
	}
	if req.Type != nil {
		menu.Type = *req.Type
	}
	if req.Path != nil {
		menu.Path = strings.TrimSpace(*req.Path)
	}
	if req.Component != nil {
		menu.Component = strings.TrimSpace(*req.Component)
	}
	if req.Perms != nil {
		menu.Perms = strings.TrimSpace(*req.Perms)
	}
	if req.Icon != nil {
		menu.Icon = *req.Icon
	}
	if req.Sort != nil {
		menu.Sort = *req.Sort
	}
	if req.Visible != nil {
		menu.Visible = *req.Visible
	}
	if req.Status != nil {
		menu.Status = *req.Status
	}
	if req.IsFrame != nil {
		menu.IsFrame = *req.IsFrame
	}
	menu.UpdatedBy = &operatorID

	// 类型可能被改动，故按改后的最终形态校验。
	if err := s.validateShape(menu.Type, menu.Path, menu.Component, menu.Perms); err != nil {
		return err
	}
	if err := s.checkPermsUnique(ctx, menu.Perms, id); err != nil {
		return err
	}

	if err := s.menus.Save(ctx, menu); err != nil {
		return errs.Internal("保存菜单失败").WithCause(err)
	}

	s.reloadPolicies(ctx, "修改菜单")
	return nil
}

// Delete 删除菜单。
func (s *MenuService) Delete(ctx context.Context, id uint64) error {
	if _, err := s.menus.FindByID(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errs.ErrMenuNotFound
		}
		return err
	}

	// 有子节点时不允许删除：级联删除整棵子树的破坏力过大，
	// 且用户往往只想删当前这一个节点，误操作代价不可逆（本表是物理删除）。
	hasChildren, err := s.menus.HasChildren(ctx, id)
	if err != nil {
		return errs.Internal("查询子菜单失败").WithCause(err)
	}
	if hasChildren {
		return errs.BadRequest("该菜单下还有子菜单，请先删除子菜单")
	}

	err = s.menus.DB().Transaction(func(tx *gorm.DB) error {
		// 先清关联再删菜单，两者必须同事务，否则会留下悬空授权行。
		if err := s.menus.DeleteRoleLinks(ctx, tx, id); err != nil {
			return errs.Internal("清除菜单的角色关联失败").WithCause(err)
		}
		if err := tx.WithContext(ctx).Delete(&model.Menu{}, id).Error; err != nil {
			return errs.Internal("删除菜单失败").WithCause(err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	s.reloadPolicies(ctx, "删除菜单")
	return nil
}

// validateParent 校验父节点存在且可作为父级。
func (s *MenuService) validateParent(ctx context.Context, parentID uint64) error {
	if parentID == model.RootID {
		return nil // 顶级节点
	}

	parent, err := s.menus.FindByID(ctx, parentID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errs.BadRequest("上级菜单不存在")
		}
		return err
	}
	// 按钮是权限点而非容器，挂在它下面的节点永远不会被渲染。
	if parent.IsButton() {
		return errs.BadRequest("按钮类型不能作为上级菜单")
	}
	return nil
}

// validateNotDescendant 阻止把节点挂到自己或自己的后代之下。
//
// 这类操作会让该子树从根上脱离（既不在根下，也构成 parent 环），
// treeutil 虽有环检测不会崩，但整棵子树会从菜单树里凭空消失，
// 且无法再通过界面移回来——数据已坏，只能改库。
func (s *MenuService) validateNotDescendant(ctx context.Context, id, newParentID uint64) error {
	if id == newParentID {
		return errs.BadRequest("上级菜单不能是自己")
	}

	all, err := s.menus.FindAll(ctx)
	if err != nil {
		return errs.Internal("查询菜单失败").WithCause(err)
	}
	parentOf := make(map[uint64]uint64, len(all))
	for _, menu := range all {
		parentOf[menu.ID] = menu.ParentID
	}

	// 从目标父节点向上回溯，若途中遇到自己，说明目标是自己的后代。
	// 循环上限用节点总数兜底，避免库里已有脏环时无限循环。
	for cursor, steps := newParentID, 0; cursor != model.RootID && steps <= len(all); steps++ {
		if cursor == id {
			return errs.BadRequest("上级菜单不能是自己的子节点")
		}
		next, ok := parentOf[cursor]
		if !ok {
			break
		}
		cursor = next
	}
	return nil
}

// validateShape 按菜单类型校验字段组合。
//
// 目录与菜单必须有 path，否则前端无法生成路由，配好了却点不进去；
// 按钮不需要 path/component（它只承载 perms），但必须有 perms，
// 否则这个节点既不显示也不授权，纯粹是条无意义的数据。
func (s *MenuService) validateShape(menuType int8, path, component, perms string) error {
	switch menuType {
	case model.MenuTypeDir:
		if strings.TrimSpace(path) == "" {
			return errs.BadRequest("目录必须填写路由地址")
		}
	case model.MenuTypeMenu:
		if strings.TrimSpace(path) == "" {
			return errs.BadRequest("菜单必须填写路由地址")
		}
		// 组件路径要与 views/ 下的真实文件对应，否则前端退化成 404 页。
		// 此处只能校验非空，路径是否存在由前端 import.meta.glob 决定。
		if strings.TrimSpace(component) == "" {
			return errs.BadRequest("菜单必须填写组件路径，如 system/user/index")
		}
	case model.MenuTypeButton:
		if strings.TrimSpace(perms) == "" {
			return errs.BadRequest("按钮必须填写权限标识")
		}
	default:
		return errs.BadRequest("菜单类型非法")
	}
	return nil
}

// checkPermsUnique 校验权限标识不重复。
//
// 重复的 perms 会让 Casbin 出现两条等价策略：本身不报错，但撤销其中一个菜单的
// 授权后权限依然生效，排查时完全看不出原因。
func (s *MenuService) checkPermsUnique(ctx context.Context, perms string, excludeID uint64) error {
	perms = strings.TrimSpace(perms)
	if perms == "" {
		return nil // 目录常无 perms，允许为空
	}

	all, err := s.menus.FindAll(ctx)
	if err != nil {
		return errs.Internal("查询菜单失败").WithCause(err)
	}
	for _, menu := range all {
		if menu.ID != excludeID && menu.Perms == perms {
			return errs.BadRequest("权限标识已被「" + menu.Name + "」占用，请更换")
		}
	}
	return nil
}

// reloadPolicies 重建 Casbin 策略；失败只告警，理由同 RoleService。
func (s *MenuService) reloadPolicies(ctx context.Context, action string) {
	if err := s.permissions.ReloadPolicies(ctx); err != nil {
		logger.Warnf("%s后刷新 Casbin 策略失败，权限变更可能延迟生效: %v", action, err)
	}
}

// defaultInt8 在指针为空时取默认值，用于「未传该字段」与「显式传 0」的区分。
func defaultInt8(value *int8, fallback int8) int8 {
	if value == nil {
		return fallback
	}
	return *value
}
