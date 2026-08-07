// Package service 承载业务逻辑与事务编排，是权限判断与数据组装的主战场。
package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/dto"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/repository"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/vo"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/cache"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/errs"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/jwt"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/logger"
)

// bcryptCost 为密码哈希代价因子。10 是安全性与登录耗时的平衡点。
const bcryptCost = 10

// AuthService 负责登录、登出、Token 续期与当前用户信息查询。
type AuthService struct {
	users    *repository.UserRepository
	roles    *repository.RoleRepository
	menus    *repository.MenuRepository
	logs     *repository.LogRepository
	jwt      *jwt.Manager
	cache    *cache.Client
	captchas *CaptchaService
}

func NewAuthService(
	users *repository.UserRepository,
	roles *repository.RoleRepository,
	menus *repository.MenuRepository,
	logs *repository.LogRepository,
	jwtManager *jwt.Manager,
	cacheClient *cache.Client,
	captchas *CaptchaService,
) *AuthService {
	return &AuthService{
		users:    users,
		roles:    roles,
		menus:    menus,
		logs:     logs,
		jwt:      jwtManager,
		cache:    cacheClient,
		captchas: captchas,
	}
}

// LoginContext 携带请求上下文信息，用于写登录日志。
type LoginContext struct {
	IP      string
	Browser string
	OS      string
}

// Login 校验凭证并签发令牌。
func (s *AuthService) Login(ctx context.Context, req *dto.LoginRequest, meta LoginContext) (*vo.LoginResult, error) {
	// 图形验证码先行校验，减少无谓的密码比对开销。
	if err := s.captchas.Verify(ctx, req.CaptchaID, req.CaptchaCode); err != nil {
		s.recordLogin(ctx, req.Username, meta, false, "验证码错误")
		return nil, err
	}

	user, err := s.users.FindByUsername(ctx, req.Username)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			/*
			 * 用户不存在与密码错误返回同一个提示。
			 * 若区分开来，攻击者可据此枚举出系统中存在哪些账号。
			 */
			s.recordLogin(ctx, req.Username, meta, false, "用户不存在")
			return nil, errs.ErrInvalidCredentials
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		s.recordLogin(ctx, req.Username, meta, false, "密码错误")
		return nil, errs.ErrInvalidCredentials
	}

	if !user.IsEnabled() {
		s.recordLogin(ctx, req.Username, meta, false, "账号已停用")
		return nil, errs.ErrUserDisabled
	}

	result, err := s.issueTokens(user)
	if err != nil {
		return nil, err
	}

	// 登录信息与日志属于旁路记录，失败不应阻断登录。
	if err := s.users.UpdateLoginInfo(ctx, user.ID, meta.IP); err != nil {
		logger.WithField("userId", user.ID).Warnf("更新登录信息失败: %v", err)
	}
	s.recordLogin(ctx, req.Username, meta, true, "登录成功")

	return result, nil
}

// issueTokens 为用户签发 Access 与 Refresh Token。
func (s *AuthService) issueTokens(user *model.User) (*vo.LoginResult, error) {
	roleCodes := user.RoleCodes()
	// jti 用于登出时精确拉黑本次会话。
	jti := uuid.NewString()

	accessToken, expiresAt, err := s.jwt.GenerateAccess(
		user.ID, user.Username, user.DeptIDValue(), roleCodes, jti,
	)
	if err != nil {
		return nil, errs.Internal("签发令牌失败").WithCause(err)
	}
	refreshToken, _, err := s.jwt.GenerateRefresh(
		user.ID, user.Username, user.DeptIDValue(), roleCodes, jti,
	)
	if err != nil {
		return nil, errs.Internal("签发刷新令牌失败").WithCause(err)
	}

	return &vo.LoginResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(time.Until(expiresAt).Seconds()),
		TokenType:    "Bearer",
	}, nil
}

// Logout 将当前 Token 加入黑名单直至其自然过期。
func (s *AuthService) Logout(ctx context.Context, claims *jwt.Claims) error {
	if claims == nil || claims.ExpiresAt == nil {
		return nil
	}
	ttl := time.Until(claims.ExpiresAt.Time)
	if err := s.cache.BlacklistToken(ctx, claims.ID, ttl); err != nil {
		return errs.Internal("登出失败").WithCause(err)
	}
	return nil
}

// Refresh 用 Refresh Token 换取新的令牌对。
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*vo.LoginResult, error) {
	claims, err := s.jwt.ParseRefresh(refreshToken)
	if err != nil {
		return nil, errs.ErrTokenInvalid.WithCause(err)
	}

	// Refresh Token 同样受黑名单约束，否则登出后仍可续期。
	blacklisted, err := s.cache.IsTokenBlacklisted(ctx, claims.ID)
	if err != nil {
		return nil, errs.Internal("校验令牌状态失败").WithCause(err)
	}
	if blacklisted {
		return nil, errs.ErrTokenInvalid
	}

	// 重新查库而非直接信任 Token 内容：用户可能已被停用或角色已变更。
	user, err := s.users.FindByID(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, errs.ErrTokenInvalid
		}
		return nil, err
	}
	if !user.IsEnabled() {
		return nil, errs.ErrUserDisabled
	}

	// 旧 Token 立即失效，防止一个 Refresh Token 被反复使用。
	if claims.ExpiresAt != nil {
		if err := s.cache.BlacklistToken(ctx, claims.ID, time.Until(claims.ExpiresAt.Time)); err != nil {
			logger.Warnf("拉黑旧刷新令牌失败: %v", err)
		}
	}

	return s.issueTokens(user)
}

// Info 返回当前用户信息与权限集合。
func (s *AuthService) Info(ctx context.Context, userID uint64) (*vo.UserInfo, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, errs.ErrUserNotFound
		}
		return nil, err
	}

	roleCodes := user.RoleCodes()
	perms, err := s.permsOf(ctx, user)
	if err != nil {
		return nil, err
	}
	return vo.NewUserInfo(user, roleCodes, perms), nil
}

// permsOf 计算用户的权限标识集合。
func (s *AuthService) permsOf(ctx context.Context, user *model.User) ([]string, error) {
	// 超级管理员用通配符表示拥有全部权限，前端据此直接放开所有按钮。
	if user.IsSuperAdmin() {
		return []string{vo.AllPerms}, nil
	}
	roleIDs := enabledRoleIDs(user.Roles)
	if len(roleIDs) == 0 {
		return []string{}, nil
	}
	perms, err := s.menus.FindPermsByRoleIDs(ctx, roleIDs)
	if err != nil {
		return nil, errs.Internal("查询用户权限失败").WithCause(err)
	}
	return perms, nil
}

// Menus 返回当前用户可见的菜单树，用于前端生成动态路由。
func (s *AuthService) Menus(ctx context.Context, userID uint64) ([]*vo.MenuNode, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, errs.ErrUserNotFound
		}
		return nil, err
	}

	var menus []*model.Menu
	if user.IsSuperAdmin() {
		// 超级管理员看到全部启用菜单，无需按角色过滤。
		menus, err = s.menus.FindAllEnabled(ctx)
	} else {
		roleIDs := enabledRoleIDs(user.Roles)
		if len(roleIDs) == 0 {
			return []*vo.MenuNode{}, nil
		}
		menus, err = s.menus.FindByRoleIDs(ctx, roleIDs)
	}
	if err != nil {
		return nil, errs.Internal("查询菜单失败").WithCause(err)
	}

	return vo.BuildMenuTree(menus), nil
}

// ChangePassword 修改当前用户自己的密码。
func (s *AuthService) ChangePassword(ctx context.Context, userID uint64, req *dto.ChangePasswordRequest) error {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return errs.ErrUserNotFound
		}
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword)); err != nil {
		return errs.ErrOldPasswordWrong
	}

	hashed, err := HashPassword(req.NewPassword)
	if err != nil {
		return err
	}
	user.Password = hashed
	if err := s.users.Save(ctx, user); err != nil {
		return errs.Internal("修改密码失败").WithCause(err)
	}
	return nil
}

// recordLogin 写入登录日志。失败只记警告，不影响登录主流程。
func (s *AuthService) recordLogin(
	ctx context.Context, username string, meta LoginContext, success bool, msg string,
) {
	status := model.StatusDisabled
	if success {
		status = model.StatusEnabled
	}
	log := &model.LoginLog{
		Username:  username,
		IPAddr:    meta.IP,
		Browser:   meta.Browser,
		OS:        meta.OS,
		Status:    status,
		Msg:       msg,
		LoginTime: time.Now(),
	}
	if err := s.logs.CreateLoginLog(ctx, log); err != nil {
		logger.Warnf("写入登录日志失败: %v", err)
	}
}

// HashPassword 生成 bcrypt 密码哈希。
func HashPassword(plaintext string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcryptCost)
	if err != nil {
		return "", errs.Internal("密码加密失败").WithCause(err)
	}
	return string(hashed), nil
}

// enabledRoleIDs 提取启用状态的角色 ID。
func enabledRoleIDs(roles []model.Role) []uint64 {
	ids := make([]uint64, 0, len(roles))
	for _, role := range roles {
		if role.Status == model.StatusEnabled {
			ids = append(ids, role.ID)
		}
	}
	return ids
}
