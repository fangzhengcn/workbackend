package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/service"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/errs"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/logger"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/response"
)

// RequirePerm 校验当前用户是否拥有指定权限点，是接口级鉴权的安全兜底。
//
// 前端的菜单/按钮控制只是体验优化，攻击者可绕过前端直接调接口，
// 因此每个写接口都必须挂上本中间件（设计文档 §1.2）。
//
// 用法：r.POST("/users", middleware.RequirePerm(perms.UserAdd), handler)
func RequirePerm(permissions *service.PermissionService, perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := CurrentClaims(c)
		if claims == nil {
			// 正常情况下 JWTAuth 已拦截，此处属防御性校验。
			response.AbortWithCode(c, errs.CodeUnauthorized, "未登录")
			return
		}

		// 超级管理员直接放行，避免管理员把自己锁死（设计文档 §4.3）。
		for _, code := range claims.Roles {
			if code == model.SuperAdminRoleCode {
				c.Next()
				return
			}
		}

		allowed, err := permissions.Enforce(claims.Roles, perm)
		if err != nil {
			logger.WithField("perm", perm).Errorf("权限校验异常: %v", err)
			// 校验出错时拒绝而非放行：授权失败必须 fail-closed。
			response.AbortWithCode(c, errs.CodeForbidden, "权限校验失败")
			return
		}
		if !allowed {
			logger.WithFields(map[string]any{
				"userId": claims.UserID,
				"perm":   perm,
				"path":   c.Request.URL.Path,
			}).Warnf("拒绝越权访问")
			response.AbortWithCode(c, errs.CodeForbidden, "无操作权限")
			return
		}

		c.Next()
	}
}

// RequireRole 要求用户具备指定角色之一，用于少数按角色而非权限点控制的场景。
func RequireRole(codes ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		allowed[code] = struct{}{}
	}

	return func(c *gin.Context) {
		claims := CurrentClaims(c)
		if claims == nil {
			response.AbortWithCode(c, errs.CodeUnauthorized, "未登录")
			return
		}
		for _, code := range claims.Roles {
			if code == model.SuperAdminRoleCode {
				c.Next()
				return
			}
			if _, ok := allowed[code]; ok {
				c.Next()
				return
			}
		}
		response.AbortWithCode(c, errs.CodeForbidden, "无操作权限")
	}
}
