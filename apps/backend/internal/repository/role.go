package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/dto"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
)

// RoleRepository 负责 sys_role 及其关联表的数据访问。
type RoleRepository struct {
	db *gorm.DB
}

func NewRoleRepository(db *gorm.DB) *RoleRepository {
	return &RoleRepository{db: db}
}

func (r *RoleRepository) FindByID(ctx context.Context, id uint64) (*model.Role, error) {
	var role model.Role
	err := r.db.WithContext(ctx).First(&role, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询角色失败: %w", err)
	}
	return &role, nil
}

// FindByUserID 查询用户拥有的启用角色。
func (r *RoleRepository) FindByUserID(ctx context.Context, userID uint64) ([]*model.Role, error) {
	var roles []*model.Role
	err := r.db.WithContext(ctx).
		Joins("JOIN sys_user_role ON sys_user_role.role_id = sys_role.id").
		Where("sys_user_role.user_id = ?", userID).
		Where("sys_role.status = ?", model.StatusEnabled).
		Order("sys_role.sort ASC").
		Find(&roles).Error
	if err != nil {
		return nil, fmt.Errorf("查询用户角色失败: %w", err)
	}
	return roles, nil
}

// FindAll 查询全部启用角色，供下拉选择使用。
func (r *RoleRepository) FindAll(ctx context.Context) ([]*model.Role, error) {
	var roles []*model.Role
	err := r.db.WithContext(ctx).
		Where("status = ?", model.StatusEnabled).
		Order("sort ASC, id ASC").
		Find(&roles).Error
	if err != nil {
		return nil, fmt.Errorf("查询角色列表失败: %w", err)
	}
	return roles, nil
}

// ExistsCode 判断角色标识是否已存在。
func (r *RoleRepository) ExistsCode(ctx context.Context, code string, excludeID uint64) (bool, error) {
	query := r.db.WithContext(ctx).Model(&model.Role{}).Where("code = ?", code)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("角色标识唯一性校验失败: %w", err)
	}
	return count > 0, nil
}

// Page 分页查询角色。
func (r *RoleRepository) Page(ctx context.Context, query *dto.RoleQuery) ([]*model.Role, int64, error) {
	db := r.db.WithContext(ctx).Model(&model.Role{})
	if query.Name != "" {
		db = db.Where("name LIKE ?", "%"+query.Name+"%")
	}
	if query.Code != "" {
		db = db.Where("code LIKE ?", "%"+query.Code+"%")
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计角色总数失败: %w", err)
	}
	if total == 0 {
		return []*model.Role{}, 0, nil
	}

	var roles []*model.Role
	err := db.Order("sort ASC, id ASC").
		Offset(query.Offset()).
		Limit(query.Limit()).
		Find(&roles).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询角色列表失败: %w", err)
	}
	return roles, total, nil
}

// FindMenuIDs 查询角色已分配的菜单 ID，用于前端权限树回显。
func (r *RoleRepository) FindMenuIDs(ctx context.Context, roleID uint64) ([]uint64, error) {
	var ids []uint64
	err := r.db.WithContext(ctx).
		Model(&model.RoleMenu{}).
		Where("role_id = ?", roleID).
		Pluck("menu_id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("查询角色菜单失败: %w", err)
	}
	return ids, nil
}

// FindDeptIDs 查询角色自定义数据范围包含的部门 ID。
func (r *RoleRepository) FindDeptIDs(ctx context.Context, roleID uint64) ([]uint64, error) {
	var ids []uint64
	err := r.db.WithContext(ctx).
		Model(&model.RoleDept{}).
		Where("role_id = ?", roleID).
		Pluck("dept_id", &ids).Error
	if err != nil {
		return nil, fmt.Errorf("查询角色部门失败: %w", err)
	}
	return ids, nil
}

func (r *RoleRepository) Create(ctx context.Context, role *model.Role) error {
	if err := r.db.WithContext(ctx).Create(role).Error; err != nil {
		return fmt.Errorf("创建角色失败: %w", err)
	}
	return nil
}

// Save 保存角色。
//
// Omit 关联：Menus/Depts 是多对多，交给 Save 会整表重写
// sys_role_menu / sys_role_dept，绕过 ReplaceMenus/ReplaceDepts 的显式维护。
func (r *RoleRepository) Save(ctx context.Context, role *model.Role) error {
	if err := r.db.WithContext(ctx).Omit("Menus", "Depts").Save(role).Error; err != nil {
		return fmt.Errorf("保存角色失败: %w", err)
	}
	return nil
}

func (r *RoleRepository) Delete(ctx context.Context, id uint64) error {
	if err := r.db.WithContext(ctx).Delete(&model.Role{}, id).Error; err != nil {
		return fmt.Errorf("删除角色失败: %w", err)
	}
	return nil
}

// CountUsers 统计使用该角色的用户数，删除角色前校验。
func (r *RoleRepository) CountUsers(ctx context.Context, roleID uint64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Table("sys_user_role").
		Joins("JOIN sys_user ON sys_user.id = sys_user_role.user_id AND sys_user.deleted_at IS NULL").
		Where("sys_user_role.role_id = ?", roleID).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("统计角色使用数失败: %w", err)
	}
	return count, nil
}

// ReplaceMenus 覆盖角色的菜单权限，必须在事务中调用。
//
// 调用方在提交事务后需刷新 Casbin 策略，否则权限变更不会立即生效
// （设计文档「技术难点提示 4」）。
func (r *RoleRepository) ReplaceMenus(ctx context.Context, tx *gorm.DB, roleID uint64, menuIDs []uint64) error {
	db := tx
	if db == nil {
		db = r.db
	}
	db = db.WithContext(ctx)

	if err := db.Where("role_id = ?", roleID).Delete(&model.RoleMenu{}).Error; err != nil {
		return fmt.Errorf("清除角色原有菜单失败: %w", err)
	}
	if len(menuIDs) == 0 {
		return nil
	}
	links := make([]model.RoleMenu, 0, len(menuIDs))
	for _, menuID := range menuIDs {
		links = append(links, model.RoleMenu{RoleID: roleID, MenuID: menuID})
	}
	if err := db.Create(&links).Error; err != nil {
		return fmt.Errorf("分配角色菜单失败: %w", err)
	}
	return nil
}

// ReplaceDepts 覆盖角色的自定义数据范围部门。
func (r *RoleRepository) ReplaceDepts(ctx context.Context, tx *gorm.DB, roleID uint64, deptIDs []uint64) error {
	db := tx
	if db == nil {
		db = r.db
	}
	db = db.WithContext(ctx)

	if err := db.Where("role_id = ?", roleID).Delete(&model.RoleDept{}).Error; err != nil {
		return fmt.Errorf("清除角色原有部门失败: %w", err)
	}
	if len(deptIDs) == 0 {
		return nil
	}
	links := make([]model.RoleDept, 0, len(deptIDs))
	for _, deptID := range deptIDs {
		links = append(links, model.RoleDept{RoleID: roleID, DeptID: deptID})
	}
	if err := db.Create(&links).Error; err != nil {
		return fmt.Errorf("分配角色部门失败: %w", err)
	}
	return nil
}

// FindRoleMenuPerms 返回「角色标识 -> 权限标识集合」，用于全量重建 Casbin 策略。
func (r *RoleRepository) FindRoleMenuPerms(ctx context.Context) (map[string][]string, error) {
	type row struct {
		Code  string
		Perms string
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("sys_role").
		Select("sys_role.code AS code, sys_menu.perms AS perms").
		Joins("JOIN sys_role_menu ON sys_role_menu.role_id = sys_role.id").
		Joins("JOIN sys_menu ON sys_menu.id = sys_role_menu.menu_id").
		Where("sys_role.status = ? AND sys_role.deleted_at IS NULL", model.StatusEnabled).
		Where("sys_menu.status = ? AND sys_menu.perms <> ''", model.StatusEnabled).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("查询角色权限映射失败: %w", err)
	}

	result := make(map[string][]string)
	for _, item := range rows {
		result[item.Code] = append(result[item.Code], item.Perms)
	}
	return result, nil
}

func (r *RoleRepository) DB() *gorm.DB { return r.db }
