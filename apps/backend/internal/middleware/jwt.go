// Package middleware 提供 Gin 中间件：鉴权、授权、日志、CORS 等。
package middleware

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/fangzhengcn/workbackend/apps/backend/pkg/cache"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/errs"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/jwt"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/logger"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/response"
)

// gin.Context 中存放登录态的键名。
const (
	ctxKeyClaims   = "auth_claims"
	ctxKeyUserID   = "auth_user_id"
	ctxKeyUsername = "auth_username"
)

// JWTAuth 校验 Access Token 并把登录态写入 Context。
//
// 校验链路：取 Header → 验签与过期 → 查 Redis 黑名单 → 查是否被踢下线。
func JWTAuth(manager *jwt.Manager, cacheClient *cache.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			response.AbortWithCode(c, errs.CodeUnauthorized, "未提供登录凭证，请先登录")
			return
		}

		claims, err := manager.ParseAccess(token)
		if err != nil {
			switch {
			case errors.Is(err, jwt.ErrExpired):
				response.AbortWithCode(c, errs.CodeUnauthorized, "登录已过期，请重新登录")
			case errors.Is(err, jwt.ErrWrongType):
				// 用 Refresh Token 直接访问业务接口，属于非预期用法。
				response.AbortWithCode(c, errs.CodeUnauthorized, "令牌类型不正确")
			default:
				response.AbortWithCode(c, errs.CodeUnauthorized, "登录状态无效，请重新登录")
			}
			return
		}

		// 已登出的 Token 仍未到期，需靠黑名单拦截。
		blacklisted, err := cacheClient.IsTokenBlacklisted(c.Request.Context(), claims.ID)
		if err != nil {
			/*
			 * Redis 故障时选择「放行」而非「全站 401」。
			 * 权衡：已登出 Token 在其剩余有效期内可能被继续使用（风险有限，
			 * 因为 Token 本身仍在有效期内且签名合法）；若选择拒绝，
			 * 则 Redis 抖动会导致所有用户瞬间掉线，可用性代价更大。
			 * 生产环境应对该日志配置告警。
			 */
			logger.Errorf("查询 Token 黑名单失败，本次请求放行: %v", err)
		} else if blacklisted {
			response.AbortWithCode(c, errs.CodeUnauthorized, "登录状态已失效，请重新登录")
			return
		}

		// 被管理员踢下线的用户，其此前签发的所有 Token 一并失效。
		if kickedAt, err := cacheClient.KickedAt(c.Request.Context(), claims.UserID); err != nil {
			logger.Errorf("查询下线标记失败，本次请求放行: %v", err)
		} else if !kickedAt.IsZero() && claims.IssuedAt != nil &&
			claims.IssuedAt.Time.Before(kickedAt) {
			response.AbortWithCode(c, errs.CodeUnauthorized, "账号已被强制下线，请重新登录")
			return
		}

		c.Set(ctxKeyClaims, claims)
		c.Set(ctxKeyUserID, claims.UserID)
		c.Set(ctxKeyUsername, claims.Username)
		c.Next()
	}
}

// extractToken 从 Authorization 头中取出 Bearer Token。
func extractToken(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	if header == "" {
		return ""
	}
	// 大小写不敏感地匹配 "Bearer " 前缀。
	const prefix = "bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}
	return ""
}

// CurrentClaims 从 Context 取出当前登录态；未登录时返回 nil。
func CurrentClaims(c *gin.Context) *jwt.Claims {
	value, ok := c.Get(ctxKeyClaims)
	if !ok {
		return nil
	}
	claims, ok := value.(*jwt.Claims)
	if !ok {
		return nil
	}
	return claims
}

// CurrentUserID 返回当前登录用户 ID；未登录时返回 0。
func CurrentUserID(c *gin.Context) uint64 {
	value, ok := c.Get(ctxKeyUserID)
	if !ok {
		return 0
	}
	id, ok := value.(uint64)
	if !ok {
		return 0
	}
	return id
}

// CurrentUsername 返回当前登录账号。
func CurrentUsername(c *gin.Context) string {
	value, ok := c.Get(ctxKeyUsername)
	if !ok {
		return ""
	}
	name, _ := value.(string)
	return name
}
