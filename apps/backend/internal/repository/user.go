// Package repository 提供基于 GORM 的数据访问，屏蔽 ORM 细节便于测试与替换。
//
// 约定：Repository 只做数据存取，不含业务规则；跨表事务由 Service 层编排。
package repository

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/dto"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
)

// ErrNotFound 表示记录不存在，由各 Repository 统一返回，避免 Service 依赖 gorm 包。
var ErrNotFound = errors.New("repository: 记录不存在")

// UserRepository 负责 sys_user 的数据访问。
type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// FindByUsername 按登录账号查询，并预加载角色用于鉴权。
func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).
		Preload("Roles").
		Preload("Dept").
		Where("username = ?", username).
		First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}
	return &user, nil
}

// FindByID 按主键查询，预加载角色与部门。
func (r *UserRepository) FindByID(ctx context.Context, id uint64) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).
		Preload("Roles").
		Preload("Dept").
		First(&user, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询用户失败: %w", err)
	}
	return &user, nil
}

// FindByPhoneHash 通过盲索引精确查询手机号对应的用户。
//
// 手机号是 AES 密文，无法用 WHERE phone = ? 查询，
// 必须先用 Cipher().BlindIndex(明文) 算出 hash 再查（设计文档 6.4.3）。
func (r *UserRepository) FindByPhoneHash(ctx context.Context, phoneHash string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("phone_hash = ?", phoneHash).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("按手机号查询用户失败: %w", err)
	}
	return &user, nil
}

// FindByEmailHash 通过盲索引精确查询邮箱对应的用户。
func (r *UserRepository) FindByEmailHash(ctx context.Context, emailHash string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("email_hash = ?", emailHash).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("按邮箱查询用户失败: %w", err)
	}
	return &user, nil
}

// ExistsUsername 判断账号是否已存在；excludeID 用于修改场景排除自身。
func (r *UserRepository) ExistsUsername(ctx context.Context, username string, excludeID uint64) (bool, error) {
	return r.exists(ctx, "username = ?", username, excludeID)
}

// ExistsPhoneHash 判断手机号是否已被占用。
func (r *UserRepository) ExistsPhoneHash(ctx context.Context, phoneHash string, excludeID uint64) (bool, error) {
	return r.exists(ctx, "phone_hash = ?", phoneHash, excludeID)
}

// ExistsEmailHash 判断邮箱是否已被占用。
func (r *UserRepository) ExistsEmailHash(ctx context.Context, emailHash string, excludeID uint64) (bool, error) {
	return r.exists(ctx, "email_hash = ?", emailHash, excludeID)
}

func (r *UserRepository) exists(ctx context.Context, condition string, value any, excludeID uint64) (bool, error) {
	query := r.db.WithContext(ctx).Model(&model.User{}).Where(condition, value)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("唯一性校验失败: %w", err)
	}
	return count > 0, nil
}

// Page 分页查询用户列表。
//
// scopes 用于叠加数据权限过滤（见 service.DataScopeScope），
// 保证「能看哪些行」的规则统一在一处实现。
func (r *UserRepository) Page(
	ctx context.Context, query *dto.UserQuery, phoneHash string, scopes ...func(*gorm.DB) *gorm.DB,
) ([]*model.User, int64, error) {
	db := r.db.WithContext(ctx).Model(&model.User{}).Scopes(scopes...)

	if query.Username != "" {
		db = db.Where("username LIKE ?", "%"+query.Username+"%")
	}
	if phoneHash != "" {
		// 密文列无法 LIKE，只能按盲索引精确匹配。
		db = db.Where("phone_hash = ?", phoneHash)
	}
	if query.Status != nil {
		db = db.Where("status = ?", *query.Status)
	}
	if query.DeptID != nil {
		db = db.Where("dept_id = ?", *query.DeptID)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计用户总数失败: %w", err)
	}
	// 总数为 0 时无需再查列表。
	if total == 0 {
		return []*model.User{}, 0, nil
	}

	var users []*model.User
	err := db.Preload("Roles").Preload("Dept").
		Order("id DESC").
		Offset(query.Offset()).
		Limit(query.Limit()).
		Find(&users).Error
	if err != nil {
		return nil, 0, fmt.Errorf("查询用户列表失败: %w", err)
	}
	return users, total, nil
}

// Create 新增用户。
func (r *UserRepository) Create(ctx context.Context, user *model.User) error {
	if err := r.db.WithContext(ctx).Create(user).Error; err != nil {
		return fmt.Errorf("创建用户失败: %w", err)
	}
	return nil
}

// Save 全量保存用户。
//
// 用 Save 而非 Updates(map)：EncryptedString 的加密与 BeforeSave 里的盲索引
// 重算都依赖结构体字段，用 map 更新会绕过这两者，导致明文落库或 hash 不一致。
//
// Omit 掉关联：调用方传的通常是 Preload 过的实体，而 Save 默认会「完整保存关联」
// ——它按旧的 user.Dept 实体反推 dept_id 写回，把新设的部门覆盖成旧值；
// 多对多的 Roles 则会被整表重写，绕过 ReplaceRoles 的显式维护。
func (r *UserRepository) Save(ctx context.Context, user *model.User) error {
	if err := r.db.WithContext(ctx).Omit("Dept", "Roles").Save(user).Error; err != nil {
		return fmt.Errorf("保存用户失败: %w", err)
	}
	return nil
}

// UpdateLoginInfo 记录最后登录时间与 IP。
//
// 这里用 Updates(map) 是安全的：只涉及非加密列，且能避免整行 Save
// 触发不必要的重新加密。
func (r *UserRepository) UpdateLoginInfo(ctx context.Context, id uint64, ip string) error {
	err := r.db.WithContext(ctx).Model(&model.User{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"last_login_at": gorm.Expr("NOW()"),
			"last_login_ip": ip,
		}).Error
	if err != nil {
		return fmt.Errorf("更新登录信息失败: %w", err)
	}
	return nil
}

// Delete 软删除用户。
func (r *UserRepository) Delete(ctx context.Context, id uint64) error {
	if err := r.db.WithContext(ctx).Delete(&model.User{}, id).Error; err != nil {
		return fmt.Errorf("删除用户失败: %w", err)
	}
	return nil
}

// ReplaceRoles 覆盖用户的角色关联，必须在事务中调用。
func (r *UserRepository) ReplaceRoles(ctx context.Context, tx *gorm.DB, userID uint64, roleIDs []uint64) error {
	db := tx
	if db == nil {
		db = r.db
	}
	db = db.WithContext(ctx)

	// 先清空再插入，避免逐条 diff 的复杂度。
	if err := db.Where("user_id = ?", userID).Delete(&model.UserRole{}).Error; err != nil {
		return fmt.Errorf("清除用户原有角色失败: %w", err)
	}
	if len(roleIDs) == 0 {
		return nil
	}
	links := make([]model.UserRole, 0, len(roleIDs))
	for _, roleID := range roleIDs {
		links = append(links, model.UserRole{UserID: userID, RoleID: roleID})
	}
	if err := db.Create(&links).Error; err != nil {
		return fmt.Errorf("分配用户角色失败: %w", err)
	}
	return nil
}

// DB 暴露底层句柄，供 Service 开启事务。
func (r *UserRepository) DB() *gorm.DB { return r.db }
