package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/dto"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/repository"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/vo"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/errs"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/logger"
)

// RoleService 负责角色的增删改查、菜单授权与数据权限配置。
//
// 重要约定：任何改动 sys_role 或 sys_role_menu 的操作，都必须在事务提交后
// 调用 permissions.ReloadPolicies——Casbin 策略由这两张表生成，不刷新则
// 权限变更不会生效（设计文档「技术难点提示 4」）。
// 刷新必须放在事务之外：放在事务内一旦回滚，策略已按未落库的数据重建，
// 内存与数据库就此不一致。
type RoleService struct {
	roles       *repository.RoleRepository
	menus       *repository.MenuRepository
	permissions *PermissionService
}

func NewRoleService(
	roles *repository.RoleRepository,
	menus *repository.MenuRepository,
	permissions *PermissionService,
) *RoleService {
	return &RoleService{roles: roles, menus: menus, permissions: permissions}
}

// Page 分页查询角色列表。
func (s *RoleService) Page(ctx context.Context, query *dto.RoleQuery) ([]*vo.RoleItem, int64, error) {
	query.Normalize()

	roles, total, err := s.roles.Page(ctx, query)
	if err != nil {
		return nil, 0, errs.Internal("查询角色列表失败").WithCause(err)
	}
	return vo.NewRoleItems(roles), total, nil
}

// ListAll 查询全部角色，供用户分配角色的下拉框使用。
func (s *RoleService) ListAll(ctx context.Context) ([]*vo.RoleItem, error) {
	roles, err := s.roles.FindAll(ctx)
	if err != nil {
		return nil, errs.Internal("查询角色失败").WithCause(err)
	}
	return vo.NewRoleItems(roles), nil
}

// Get 查询角色详情，附带已分配的菜单与自定义数据范围部门。
func (s *RoleService) Get(ctx context.Context, id uint64) (*vo.RoleDetail, error) {
	role, err := s.roles.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, errs.ErrRoleNotFound
		}
		return nil, err
	}

	menuIDs, err := s.roles.FindMenuIDs(ctx, id)
	if err != nil {
		return nil, errs.Internal("查询角色菜单失败").WithCause(err)
	}
	deptIDs, err := s.roles.FindDeptIDs(ctx, id)
	if err != nil {
		return nil, errs.Internal("查询角色部门失败").WithCause(err)
	}
	return vo.NewRoleDetail(role, menuIDs, deptIDs), nil
}

// MenuIDs 查询角色已分配的菜单 ID，用于权限树回显。
func (s *RoleService) MenuIDs(ctx context.Context, roleID uint64) ([]uint64, error) {
	if _, err := s.roles.FindByID(ctx, roleID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, errs.ErrRoleNotFound
		}
		return nil, err
	}

	ids, err := s.roles.FindMenuIDs(ctx, roleID)
	if err != nil {
		return nil, errs.Internal("查询角色菜单失败").WithCause(err)
	}
	// 保证返回 [] 而非 null，前端权限树会直接遍历该结果。
	if ids == nil {
		return []uint64{}, nil
	}
	return ids, nil
}

// Create 新增角色，可同时分配菜单权限与自定义数据范围。
func (s *RoleService) Create(ctx context.Context, operatorID uint64, req *dto.CreateRoleRequest) error {
	exists, err := s.roles.ExistsCode(ctx, req.Code, 0)
	if err != nil {
		return errs.Internal("校验角色标识失败").WithCause(err)
	}
	if exists {
		return errs.ErrRoleCodeExists
	}
	// 不允许新建标识为 admin 的角色：鉴权中间件对该标识直接放行，
	// 等于凭空造出一个不受任何权限约束的角色。
	if req.Code == model.SuperAdminRoleCode {
		return errs.BadRequest("角色标识 admin 为超级管理员保留，请更换")
	}

	if err := s.validateMenuIDs(ctx, req.MenuIDs); err != nil {
		return err
	}

	dataScope := req.DataScope
	if dataScope == 0 {
		dataScope = model.DataScopeDept
	}
	status := model.StatusEnabled
	if req.Status != nil {
		status = *req.Status
	}

	role := &model.Role{
		Name:      req.Name,
		Code:      req.Code,
		Sort:      req.Sort,
		DataScope: dataScope,
		Status:    status,
		Remark:    req.Remark,
	}
	role.CreatedBy = &operatorID
	role.UpdatedBy = &operatorID

	err = s.roles.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Create(role).Error; err != nil {
			return errs.Internal("创建角色失败").WithCause(err)
		}
		if len(req.MenuIDs) > 0 {
			if err := s.roles.ReplaceMenus(ctx, tx, role.ID, req.MenuIDs); err != nil {
				return errs.Internal("分配角色菜单失败").WithCause(err)
			}
		}
		// 仅自定义数据范围才需要落部门关联，其余范围由算法推导。
		if dataScope == model.DataScopeCustom && len(req.DeptIDs) > 0 {
			if err := s.roles.ReplaceDepts(ctx, tx, role.ID, req.DeptIDs); err != nil {
				return errs.Internal("分配角色部门失败").WithCause(err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	s.reloadPolicies(ctx, "新增角色")
	return nil
}

// Update 修改角色。
//
// Code 不在可改字段内（UpdateRoleRequest 未提供）：已签发的 JWT 中带有角色标识，
// 改标识会让这些 Token 的权限判定瞬间失效或错位。
func (s *RoleService) Update(ctx context.Context, operatorID, id uint64, req *dto.UpdateRoleRequest) error {
	role, err := s.roles.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errs.ErrRoleNotFound
		}
		return err
	}

	// 超级管理员角色不允许停用或改权限，避免把系统锁死到无人可管理。
	if role.IsSuperAdmin() {
		if req.Status != nil && *req.Status == model.StatusDisabled {
			return errs.Forbidden("超级管理员角色不允许停用")
		}
		if req.MenuIDs != nil {
			return errs.Forbidden("超级管理员角色的权限不允许修改")
		}
	}

	if err := s.validateMenuIDs(ctx, req.MenuIDs); err != nil {
		return err
	}

	if req.Name != nil {
		role.Name = *req.Name
	}
	if req.Sort != nil {
		role.Sort = *req.Sort
	}
	if req.DataScope != nil {
		role.DataScope = *req.DataScope
	}
	if req.Status != nil {
		role.Status = *req.Status
	}
	if req.Remark != nil {
		role.Remark = *req.Remark
	}
	role.UpdatedBy = &operatorID

	err = s.roles.DB().Transaction(func(tx *gorm.DB) error {
		// Omit 关联：Role 上的 Menus/Depts 是多对多，交给 Save 会整表重写，
		// 绕过 ReplaceMenus/ReplaceDepts 这套显式维护逻辑。
		if err := tx.WithContext(ctx).Omit("Menus", "Depts").Save(role).Error; err != nil {
			return errs.Internal("保存角色失败").WithCause(err)
		}
		// nil 表示不改动权限；空切片表示清空全部权限。
		if req.MenuIDs != nil {
			if err := s.roles.ReplaceMenus(ctx, tx, id, req.MenuIDs); err != nil {
				return errs.Internal("分配角色菜单失败").WithCause(err)
			}
		}
		// 数据范围改为非自定义后，残留的部门关联会在下次切回自定义时
		// 意外复活，故一并清理。
		if req.DataScope != nil && *req.DataScope != model.DataScopeCustom {
			if err := s.roles.ReplaceDepts(ctx, tx, id, nil); err != nil {
				return errs.Internal("清理角色部门失败").WithCause(err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	s.reloadPolicies(ctx, "修改角色")
	return nil
}

// Delete 删除角色。
func (s *RoleService) Delete(ctx context.Context, id uint64) error {
	role, err := s.roles.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errs.ErrRoleNotFound
		}
		return err
	}
	if role.IsSuperAdmin() {
		return errs.Forbidden("超级管理员角色不允许删除")
	}

	count, err := s.roles.CountUsers(ctx, id)
	if err != nil {
		return errs.Internal("统计角色使用数失败").WithCause(err)
	}
	if count > 0 {
		return errs.BadRequest("该角色已分配给用户，请先解除分配后再删除")
	}

	// sys_role 是软删除，但关联表不是；不清理会残留脏关联，
	// 且新角色若复用了同一自增 ID 会直接继承这些权限。
	err = s.roles.DB().Transaction(func(tx *gorm.DB) error {
		if err := s.roles.ReplaceMenus(ctx, tx, id, nil); err != nil {
			return errs.Internal("清理角色菜单失败").WithCause(err)
		}
		if err := s.roles.ReplaceDepts(ctx, tx, id, nil); err != nil {
			return errs.Internal("清理角色部门失败").WithCause(err)
		}
		if err := tx.WithContext(ctx).Delete(&model.Role{}, id).Error; err != nil {
			return errs.Internal("删除角色失败").WithCause(err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	s.reloadPolicies(ctx, "删除角色")
	return nil
}

// AssignMenus 覆盖角色的菜单权限。
func (s *RoleService) AssignMenus(ctx context.Context, roleID uint64, menuIDs []uint64) error {
	role, err := s.roles.FindByID(ctx, roleID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errs.ErrRoleNotFound
		}
		return err
	}
	if role.IsSuperAdmin() {
		return errs.Forbidden("超级管理员角色的权限不允许修改")
	}

	if err := s.validateMenuIDs(ctx, menuIDs); err != nil {
		return err
	}

	err = s.roles.DB().Transaction(func(tx *gorm.DB) error {
		if err := s.roles.ReplaceMenus(ctx, tx, roleID, menuIDs); err != nil {
			return errs.Internal("分配角色菜单失败").WithCause(err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	s.reloadPolicies(ctx, "分配菜单权限")
	return nil
}

// SetDataScope 设置角色的数据权限范围。
func (s *RoleService) SetDataScope(ctx context.Context, roleID uint64, req *dto.DataScopeRequest) error {
	role, err := s.roles.FindByID(ctx, roleID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errs.ErrRoleNotFound
		}
		return err
	}

	if req.DataScope == model.DataScopeCustom && len(req.DeptIDs) == 0 {
		return errs.BadRequest("自定义数据范围需至少选择一个部门")
	}

	role.DataScope = req.DataScope

	return s.roles.DB().Transaction(func(tx *gorm.DB) error {
		// 同 Update：关联由 ReplaceDepts 显式维护，不交给 Save。
		if err := tx.WithContext(ctx).Omit("Menus", "Depts").Save(role).Error; err != nil {
			return errs.Internal("保存角色失败").WithCause(err)
		}
		// 非自定义范围时清空部门关联，语义与 Update 保持一致。
		deptIDs := req.DeptIDs
		if req.DataScope != model.DataScopeCustom {
			deptIDs = nil
		}
		if err := s.roles.ReplaceDepts(ctx, tx, roleID, deptIDs); err != nil {
			return errs.Internal("分配角色部门失败").WithCause(err)
		}
		return nil
	})
	// 数据权限范围不参与 Casbin 策略（策略只管接口级权限点），
	// 故此处无需 ReloadPolicies。
}

// validateMenuIDs 校验菜单 ID 均真实存在。
//
// 不校验会把不存在的 menu_id 写进 sys_role_menu：这类脏关联在权限树上
// 完全不可见（树只渲染真实菜单），却会长期滞留在库里，
// 一旦某个新建菜单复用了同一自增 ID，该角色就凭空获得了新权限。
func (s *RoleService) validateMenuIDs(ctx context.Context, menuIDs []uint64) error {
	if len(menuIDs) == 0 {
		return nil
	}

	all, err := s.menus.FindAll(ctx)
	if err != nil {
		return errs.Internal("查询菜单失败").WithCause(err)
	}
	valid := make(map[uint64]struct{}, len(all))
	for _, menu := range all {
		valid[menu.ID] = struct{}{}
	}
	for _, id := range menuIDs {
		if _, ok := valid[id]; !ok {
			return errs.BadRequest("包含不存在的菜单，请刷新后重试")
		}
	}
	return nil
}

// reloadPolicies 重建 Casbin 策略。
//
// 失败只记日志不返回错误：业务数据已提交成功，此时对调用方报错会让前端
// 以为操作失败而重试，反而造成困惑。策略在下次任意变更或重启时会自愈。
func (s *RoleService) reloadPolicies(ctx context.Context, action string) {
	if err := s.permissions.ReloadPolicies(ctx); err != nil {
		logger.Warnf("%s后刷新 Casbin 策略失败，权限变更可能延迟生效: %v", action, err)
	}
}
