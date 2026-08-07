package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
)

// MenuRepository 负责 sys_menu 的数据访问。
type MenuRepository struct {
	db *gorm.DB
}

func NewMenuRepository(db *gorm.DB) *MenuRepository {
	return &MenuRepository{db: db}
}

// FindAll 查出全部菜单（含停用与隐藏），供管理页展示完整树。
func (r *MenuRepository) FindAll(ctx context.Context) ([]*model.Menu, error) {
	var menus []*model.Menu
	err := r.db.WithContext(ctx).
		Order("parent_id ASC, sort ASC, id ASC").
		Find(&menus).Error
	if err != nil {
		return nil, fmt.Errorf("查询菜单列表失败: %w", err)
	}
	return menus, nil
}

// FindByID 按主键查询菜单。
func (r *MenuRepository) FindByID(ctx context.Context, id uint64) (*model.Menu, error) {
	var menu model.Menu
	err := r.db.WithContext(ctx).First(&menu, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询菜单失败: %w", err)
	}
	return &menu, nil
}

// FindAllEnabled 查出所有启用的菜单，供超级管理员获取完整菜单树。
func (r *MenuRepository) FindAllEnabled(ctx context.Context) ([]*model.Menu, error) {
	var menus []*model.Menu
	err := r.db.WithContext(ctx).
		Where("status = ?", model.StatusEnabled).
		Order("parent_id ASC, sort ASC, id ASC").
		Find(&menus).Error
	if err != nil {
		return nil, fmt.Errorf("查询启用菜单失败: %w", err)
	}
	return menus, nil
}

// FindByRoleIDs 查询指定角色集合拥有的启用菜单，结果去重。
func (r *MenuRepository) FindByRoleIDs(ctx context.Context, roleIDs []uint64) ([]*model.Menu, error) {
	if len(roleIDs) == 0 {
		return []*model.Menu{}, nil
	}
	var menus []*model.Menu
	err := r.db.WithContext(ctx).
		Distinct("sys_menu.*").
		Joins("JOIN sys_role_menu ON sys_role_menu.menu_id = sys_menu.id").
		Where("sys_role_menu.role_id IN ?", roleIDs).
		Where("sys_menu.status = ?", model.StatusEnabled).
		Order("sys_menu.parent_id ASC, sys_menu.sort ASC, sys_menu.id ASC").
		Find(&menus).Error
	if err != nil {
		return nil, fmt.Errorf("查询角色菜单失败: %w", err)
	}
	return menus, nil
}

// FindPermsByRoleIDs 查询角色集合对应的权限标识集合。
func (r *MenuRepository) FindPermsByRoleIDs(ctx context.Context, roleIDs []uint64) ([]string, error) {
	if len(roleIDs) == 0 {
		return []string{}, nil
	}
	var perms []string
	err := r.db.WithContext(ctx).
		Model(&model.Menu{}).
		Distinct().
		Joins("JOIN sys_role_menu ON sys_role_menu.menu_id = sys_menu.id").
		Where("sys_role_menu.role_id IN ?", roleIDs).
		Where("sys_menu.status = ?", model.StatusEnabled).
		// 目录节点没有 perms，排除空值避免污染权限集合。
		Where("sys_menu.perms <> ''").
		Pluck("sys_menu.perms", &perms).Error
	if err != nil {
		return nil, fmt.Errorf("查询角色权限标识失败: %w", err)
	}
	return perms, nil
}

// FindAllPerms 查询全部启用菜单的权限标识，用于同步 Casbin 策略。
func (r *MenuRepository) FindAllPerms(ctx context.Context) ([]string, error) {
	var perms []string
	err := r.db.WithContext(ctx).
		Model(&model.Menu{}).
		Distinct().
		Where("status = ? AND perms <> ''", model.StatusEnabled).
		Pluck("perms", &perms).Error
	if err != nil {
		return nil, fmt.Errorf("查询全部权限标识失败: %w", err)
	}
	return perms, nil
}

// HasChildren 判断菜单是否还有子节点，删除前校验。
func (r *MenuRepository) HasChildren(ctx context.Context, id uint64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Menu{}).Where("parent_id = ?", id).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("查询子菜单失败: %w", err)
	}
	return count > 0, nil
}

func (r *MenuRepository) Create(ctx context.Context, menu *model.Menu) error {
	if err := r.db.WithContext(ctx).Create(menu).Error; err != nil {
		return fmt.Errorf("创建菜单失败: %w", err)
	}
	return nil
}

func (r *MenuRepository) Save(ctx context.Context, menu *model.Menu) error {
	if err := r.db.WithContext(ctx).Save(menu).Error; err != nil {
		return fmt.Errorf("保存菜单失败: %w", err)
	}
	return nil
}

// Delete 物理删除菜单（sys_menu 无软删除列）。
func (r *MenuRepository) Delete(ctx context.Context, id uint64) error {
	if err := r.db.WithContext(ctx).Delete(&model.Menu{}, id).Error; err != nil {
		return fmt.Errorf("删除菜单失败: %w", err)
	}
	return nil
}

// DeleteRoleLinks 清除指向该菜单的角色授权行。
//
// sys_menu 是物理删除，若不清理 sys_role_menu 会留下悬空关联：
// 这些行在权限树上不可见（树只渲染真实菜单），却会在新菜单复用同一自增 ID 时
// 让角色凭空获得新权限。必须与菜单删除在同一事务内完成。
func (r *MenuRepository) DeleteRoleLinks(ctx context.Context, tx *gorm.DB, menuID uint64) error {
	db := tx
	if db == nil {
		db = r.db
	}
	err := db.WithContext(ctx).Where("menu_id = ?", menuID).Delete(&model.RoleMenu{}).Error
	if err != nil {
		return fmt.Errorf("清除菜单的角色关联失败: %w", err)
	}
	return nil
}

// DB 暴露底层句柄，供 Service 开启跨表事务。
func (r *MenuRepository) DB() *gorm.DB { return r.db }
