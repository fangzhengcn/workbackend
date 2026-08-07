package service

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"path"

	"gorm.io/gorm"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/dto"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/repository"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/vo"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/cache"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/errs"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/logger"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/upload"
)

// adminUserID 是初始超级管理员的 ID，受保护不允许删除或停用。
const adminUserID uint64 = 1

// UserService 负责用户的增删改查与角色分配。
type UserService struct {
	users     *repository.UserRepository
	roles     *repository.RoleRepository
	dataScope *DataScopeService
	cache     *cache.Client
}

func NewUserService(
	users *repository.UserRepository,
	roles *repository.RoleRepository,
	dataScope *DataScopeService,
	cacheClient *cache.Client,
) *UserService {
	return &UserService{users: users, roles: roles, dataScope: dataScope, cache: cacheClient}
}

// maxExportRows 限制单次导出的行数。
//
// 不设上限时，一次导出全表会把所有行连同关联的角色、部门一起读进内存再序列化，
// 数据量大时足以打爆进程；而这个接口只需一个导出权限就能反复触发。
// 超限时明确提示用户收窄筛选条件，而不是悄悄截断——
// 截断会让用户拿到一份「看起来完整」的残缺数据，比报错危险得多。
const maxExportRows = 10000

// Export 按查询条件导出用户，复用列表的筛选与数据权限。
//
// 返回的 VO 与列表接口同一套构造函数，因此手机号/邮箱同样是脱敏值：
// 导出文件极易外流且流向不可控，不能在这里开一个绕过脱敏的后门。
func (s *UserService) Export(ctx context.Context, operatorID uint64, query *dto.UserQuery) ([]*vo.UserItem, error) {
	/*
	 * 不能走 Page：PageQuery.Normalize() 会把 Size 截到 200，
	 * 导出便会静默地只给出前 200 条——用户拿到一份「看起来完整」的残缺数据，
	 * 这比直接报错危险得多。故这里自己组装分页参数，绕开 Normalize。
	 */
	operator, err := s.users.FindByID(ctx, operatorID)
	if err != nil {
		return nil, err
	}
	scope, err := s.dataScope.Scope(ctx, operator, "sys_user")
	if err != nil {
		return nil, err
	}

	var phoneHash string
	if query.Phone != "" {
		cipher, err := model.Cipher()
		if err != nil {
			return nil, errs.Internal("加密器未就绪").WithCause(err)
		}
		phoneHash = cipher.BlindIndex(query.Phone)
	}

	// 多取一条用于判断是否超限：正好等于上限时不该报错，
	// 超出一条才说明数据确实更多。
	query.Page = 1
	query.Size = maxExportRows + 1

	users, total, err := s.users.Page(ctx, query, phoneHash, scope)
	if err != nil {
		return nil, errs.Internal("查询用户失败").WithCause(err)
	}
	if total > maxExportRows {
		return nil, errs.BadRequest(fmt.Sprintf(
			"符合条件的数据有 %d 条，超过单次导出上限 %d 条，请收窄筛选条件后重试",
			total, maxExportRows))
	}
	return vo.NewUserItems(users), nil
}

// Page 分页查询用户列表，自动叠加当前操作人的数据权限范围。
func (s *UserService) Page(ctx context.Context, operatorID uint64, query *dto.UserQuery) ([]*vo.UserItem, int64, error) {
	query.Normalize()

	operator, err := s.users.FindByID(ctx, operatorID)
	if err != nil {
		return nil, 0, err
	}
	scope, err := s.dataScope.Scope(ctx, operator, "sys_user")
	if err != nil {
		return nil, 0, err
	}

	// 手机号是密文，只能按盲索引精确匹配。
	var phoneHash string
	if query.Phone != "" {
		cipher, err := model.Cipher()
		if err != nil {
			return nil, 0, errs.Internal("加密器未就绪").WithCause(err)
		}
		phoneHash = cipher.BlindIndex(query.Phone)
	}

	users, total, err := s.users.Page(ctx, query, phoneHash, scope)
	if err != nil {
		return nil, 0, errs.Internal("查询用户列表失败").WithCause(err)
	}
	return vo.NewUserItems(users), total, nil
}

// Get 查询用户详情。
func (s *UserService) Get(ctx context.Context, id uint64) (*vo.UserItem, error) {
	user, err := s.users.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, errs.ErrUserNotFound
		}
		return nil, err
	}
	return vo.NewUserItem(user), nil
}

// Create 新增用户。
func (s *UserService) Create(ctx context.Context, operatorID uint64, req *dto.CreateUserRequest) error {
	if err := s.checkUnique(ctx, req.Username, req.Phone, req.Email, 0); err != nil {
		return err
	}

	hashed, err := HashPassword(req.Password)
	if err != nil {
		return err
	}

	status := model.StatusEnabled
	if req.Status != nil {
		status = *req.Status
	}

	user := &model.User{
		Username: req.Username,
		Password: hashed,
		// 赋值明文即可，EncryptedString 会在写库时自动加密，
		// 盲索引由 BeforeSave 钩子自动回填。
		Nickname: model.EncryptedString(req.Nickname),
		Email:    model.EncryptedString(req.Email),
		Phone:    model.EncryptedString(req.Phone),
		Gender:   req.Gender,
		DeptID:   req.DeptID,
		Status:   status,
		Remark:   req.Remark,
	}
	user.CreatedBy = &operatorID
	user.UpdatedBy = &operatorID

	// 用户与其角色关联必须同时成功，否则会出现「建了用户但没角色」。
	return s.users.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Create(user).Error; err != nil {
			return errs.Internal("创建用户失败").WithCause(err)
		}
		if len(req.RoleIDs) > 0 {
			if err := s.users.ReplaceRoles(ctx, tx, user.ID, req.RoleIDs); err != nil {
				return errs.Internal("分配角色失败").WithCause(err)
			}
		}
		return nil
	})
}

// Update 修改用户。
func (s *UserService) Update(ctx context.Context, operatorID, id uint64, req *dto.UpdateUserRequest) error {
	user, err := s.users.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errs.ErrUserNotFound
		}
		return err
	}

	// 禁止停用初始超级管理员，避免系统失去唯一的管理入口。
	if id == adminUserID && req.Status != nil && *req.Status == model.StatusDisabled {
		return errs.ErrProtectedAdmin
	}

	phone, email := "", ""
	if req.Phone != nil {
		phone = *req.Phone
	}
	if req.Email != nil {
		email = *req.Email
	}
	if err := s.checkUnique(ctx, "", phone, email, id); err != nil {
		return err
	}

	// 仅覆盖显式传入的字段，nil 表示不修改。
	if req.Nickname != nil {
		user.Nickname = model.EncryptedString(*req.Nickname)
	}
	if req.Email != nil {
		user.Email = model.EncryptedString(*req.Email)
	}
	if req.Phone != nil {
		user.Phone = model.EncryptedString(*req.Phone)
	}
	if req.Gender != nil {
		user.Gender = *req.Gender
	}
	if req.DeptID != nil {
		user.DeptID = req.DeptID
	}
	if req.Status != nil {
		user.Status = *req.Status
	}
	if req.Remark != nil {
		user.Remark = *req.Remark
	}
	user.UpdatedBy = &operatorID

	err = s.users.DB().Transaction(func(tx *gorm.DB) error {
		/*
		 * 必须 Omit 掉关联，否则改不动 dept_id。
		 *
		 * FindByID 用 Preload("Dept") 载入了旧部门实体，GORM 的 Save 默认会
		 * 「完整保存关联」：它按 user.Dept 这个旧实体反推 dept_id 写回，
		 * 把上面刚赋的新值覆盖成旧值——接口返回 200，数据库却没变。
		 * Roles 同理（多对多会被整表重写），而角色关系由 ReplaceRoles 显式维护，
		 * 交给 Save 处理反而会绕过那套逻辑。
		 */
		if err := tx.WithContext(ctx).Omit("Dept", "Roles").Save(user).Error; err != nil {
			return errs.Internal("保存用户失败").WithCause(err)
		}
		// RoleIDs 为 nil 表示不调整角色；非 nil（含空切片）表示覆盖。
		if req.RoleIDs != nil {
			if err := s.users.ReplaceRoles(ctx, tx, id, req.RoleIDs); err != nil {
				return errs.Internal("分配角色失败").WithCause(err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// 角色或状态变更后清理权限缓存，避免旧权限继续生效。
	if err := s.cache.InvalidateUserPerms(ctx, id); err != nil {
		logger.Warnf("清理用户权限缓存失败: %v", err)
	}
	return nil
}

// Delete 软删除用户。
func (s *UserService) Delete(ctx context.Context, id uint64) error {
	if id == adminUserID {
		return errs.ErrProtectedAdmin
	}
	if _, err := s.users.FindByID(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errs.ErrUserNotFound
		}
		return err
	}
	if err := s.users.Delete(ctx, id); err != nil {
		return errs.Internal("删除用户失败").WithCause(err)
	}
	return nil
}

// UpdateAvatar 保存新头像并更新用户记录，返回可访问的 URL。
//
// 换头像时删掉旧文件：不删会让每次更换都在磁盘留一份孤儿文件，
// 长期运行后目录里绝大多数文件都不再被任何用户引用，且无从判断哪些能清。
// 删除失败只告警——新头像已生效，为一个残留文件让整个操作失败不值得。
func (s *UserService) UpdateAvatar(
	ctx context.Context, userID uint64, file *multipart.FileHeader, dir, urlPrefix string,
) (string, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", errs.ErrUserNotFound
		}
		return "", err
	}

	name, err := upload.SaveAvatar(file, dir)
	if err != nil {
		// 校验类错误直接透出给用户（如「只支持 JPG/PNG」），便于自行修正。
		return "", errs.BadRequest(err.Error())
	}

	oldName := path.Base(user.Avatar)
	user.Avatar = urlPrefix + "/" + name
	user.UpdatedBy = &userID

	if err := s.users.Save(ctx, user); err != nil {
		// 数据库没更新成功，刚落盘的文件就是垃圾，回滚掉。
		if removeErr := upload.Remove(dir, name); removeErr != nil {
			logger.Warnf("回滚头像文件失败: %v", removeErr)
		}
		return "", errs.Internal("保存头像失败").WithCause(err)
	}

	// 数据库已指向新文件，此后旧文件不再被引用，可安全删除。
	if user.Avatar != "" && oldName != "." && oldName != "/" {
		if err := upload.Remove(dir, oldName); err != nil {
			logger.Warnf("删除旧头像失败: %v", err)
		}
	}

	return user.Avatar, nil
}

// UpdateProfile 修改当前登录用户自己的资料。
//
// 与 Update 分开而非复用：Update 接受 status/deptId/roleIds，
// 若让用户自助调用，任何人都能给自己改部门或提权。
// 这里只处理四个无权限含义的字段，操作对象固定为调用者自己。
func (s *UserService) UpdateProfile(ctx context.Context, userID uint64, req *dto.UpdateProfileRequest) error {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errs.ErrUserNotFound
		}
		return err
	}

	// 手机号/邮箱有唯一索引（盲索引列），自助修改同样要查重，
	// 否则会撞上数据库唯一约束而抛出无法解读的 SQL 错误。
	phone, email := "", ""
	if req.Phone != nil {
		phone = *req.Phone
	}
	if req.Email != nil {
		email = *req.Email
	}
	if err := s.checkUnique(ctx, "", phone, email, userID); err != nil {
		return err
	}

	if req.Nickname != nil {
		user.Nickname = model.EncryptedString(*req.Nickname)
	}
	if req.Email != nil {
		user.Email = model.EncryptedString(*req.Email)
	}
	if req.Phone != nil {
		user.Phone = model.EncryptedString(*req.Phone)
	}
	if req.Gender != nil {
		user.Gender = *req.Gender
	}
	// 自助修改，操作人就是本人。
	user.UpdatedBy = &userID

	// 走 repository.Save：它已 Omit 掉 Dept/Roles 关联，
	// 否则 Preload 出来的旧部门会把 dept_id 覆盖回去。
	if err := s.users.Save(ctx, user); err != nil {
		return errs.Internal("保存个人资料失败").WithCause(err)
	}
	return nil
}

// ResetPassword 管理员重置指定用户的密码。
func (s *UserService) ResetPassword(ctx context.Context, id uint64, password string) error {
	user, err := s.users.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errs.ErrUserNotFound
		}
		return err
	}

	hashed, err := HashPassword(password)
	if err != nil {
		return err
	}
	user.Password = hashed
	if err := s.users.Save(ctx, user); err != nil {
		return errs.Internal("重置密码失败").WithCause(err)
	}
	return nil
}

// AssignRoles 覆盖用户的角色。
func (s *UserService) AssignRoles(ctx context.Context, id uint64, roleIDs []uint64) error {
	if _, err := s.users.FindByID(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errs.ErrUserNotFound
		}
		return err
	}

	err := s.users.DB().Transaction(func(tx *gorm.DB) error {
		return s.users.ReplaceRoles(ctx, tx, id, roleIDs)
	})
	if err != nil {
		return errs.Internal("分配角色失败").WithCause(err)
	}

	if err := s.cache.InvalidateUserPerms(ctx, id); err != nil {
		logger.Warnf("清理用户权限缓存失败: %v", err)
	}
	return nil
}

// checkUnique 校验账号、手机号、邮箱的唯一性。
//
// 手机号与邮箱是密文，唯一性只能通过盲索引列判断（设计文档 6.4.3）。
func (s *UserService) checkUnique(ctx context.Context, username, phone, email string, excludeID uint64) error {
	if username != "" {
		exists, err := s.users.ExistsUsername(ctx, username, excludeID)
		if err != nil {
			return errs.Internal("校验账号唯一性失败").WithCause(err)
		}
		if exists {
			return errs.ErrUsernameExists
		}
	}

	if phone == "" && email == "" {
		return nil
	}
	cipher, err := model.Cipher()
	if err != nil {
		return errs.Internal("加密器未就绪").WithCause(err)
	}

	if phone != "" {
		exists, err := s.users.ExistsPhoneHash(ctx, cipher.BlindIndex(phone), excludeID)
		if err != nil {
			return errs.Internal("校验手机号唯一性失败").WithCause(err)
		}
		if exists {
			return errs.ErrPhoneExists
		}
	}
	if email != "" {
		exists, err := s.users.ExistsEmailHash(ctx, cipher.BlindIndex(email), excludeID)
		if err != nil {
			return errs.Internal("校验邮箱唯一性失败").WithCause(err)
		}
		if exists {
			return errs.ErrEmailExists
		}
	}
	return nil
}
