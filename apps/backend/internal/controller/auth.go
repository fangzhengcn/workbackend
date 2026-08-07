// Package controller 处理 HTTP 层事务：绑定校验参数、调用 Service、组装响应。
//
// 约定：Controller 不写业务逻辑，也不直接访问数据库。
package controller

import (
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/dto"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/middleware"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/service"
	// 空导入 vo 供 swag 解析 @Success 注解里的 vo.* 类型。
	// Controller 编译期确实不需要 vo（Service 已把实体转成 VO 返回），
	// 但 swag 只在当前文件的 import 列表里查找类型定义，缺了它会报
	// 「cannot find type definition: vo.XxxItem」而整份文档生成失败。
	// 删掉这行前请先跑 make swagger 确认仍能生成。
	_ "github.com/fangzhengcn/workbackend/apps/backend/internal/vo"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/errs"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/response"
)

// AuthController 提供登录、登出、当前用户信息与菜单接口。
type AuthController struct {
	auth     *service.AuthService
	captchas *service.CaptchaService
}

func NewAuthController(auth *service.AuthService, captchas *service.CaptchaService) *AuthController {
	return &AuthController{auth: auth, captchas: captchas}
}

// Login 登录换取令牌。
//
// @Summary  用户登录
// @Tags     认证
// @Accept   json
// @Produce  json
// @Param    body body     dto.LoginRequest true "登录参数"
// @Success  200  {object} response.Body{data=vo.LoginResult} "登录成功"
// @Router   /auth/login [post]
func (ctl *AuthController) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}

	meta := service.LoginContext{
		IP: c.ClientIP(),
		// User-Agent 解析未做细分，直接留存原始值便于排查。
		// 必须截断：browser 列为 VARCHAR(256)，而 UA 长度不受控，
		// 超长会让登录日志 INSERT 报 1406 而写入失败。
		Browser: truncate(c.GetHeader("User-Agent"), 256),
		OS:      "",
	}

	result, err := ctl.auth.Login(c.Request.Context(), &req, meta)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

// Logout 登出，将当前 Token 加入黑名单。
//
// @Summary  用户登出
// @Tags     认证
// @Produce  json
// @Success  200 {object} response.Body
// @Router   /auth/logout [post]
func (ctl *AuthController) Logout(c *gin.Context) {
	claims := middleware.CurrentClaims(c)
	if err := ctl.auth.Logout(c.Request.Context(), claims); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "已退出登录")
}

// Captcha 获取图形验证码。
//
// @Summary  获取图形验证码
// @Tags     认证
// @Produce  json
// @Success  200 {object} response.Body{data=vo.CaptchaResult}
// @Router   /auth/captcha [get]
func (ctl *AuthController) Captcha(c *gin.Context) {
	result, err := ctl.captchas.Generate(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

// Refresh 用 Refresh Token 换取新令牌。
//
// @Summary  刷新令牌
// @Tags     认证
// @Accept   json
// @Produce  json
// @Param    body body     dto.RefreshRequest true "刷新参数"
// @Success  200  {object} response.Body{data=vo.LoginResult}
// @Router   /auth/refresh [post]
func (ctl *AuthController) Refresh(c *gin.Context) {
	var req dto.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	result, err := ctl.auth.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

// Info 获取当前用户信息与权限集合。
//
// @Summary  获取当前用户信息
// @Tags     认证
// @Produce  json
// @Success  200 {object} response.Body{data=vo.UserInfo}
// @Router   /auth/info [get]
func (ctl *AuthController) Info(c *gin.Context) {
	info, err := ctl.auth.Info(c.Request.Context(), middleware.CurrentUserID(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, info)
}

// Menus 获取当前用户菜单树。
//
// @Summary  获取当前用户菜单树
// @Tags     认证
// @Produce  json
// @Success  200 {object} response.Body{data=[]vo.MenuNode}
// @Router   /auth/menus [get]
func (ctl *AuthController) Menus(c *gin.Context) {
	menus, err := ctl.auth.Menus(c.Request.Context(), middleware.CurrentUserID(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, menus)
}

// ChangePassword 修改当前用户密码。
//
// @Summary  修改自己的密码
// @Tags     个人中心
// @Accept   json
// @Produce  json
// @Param    body body     dto.ChangePasswordRequest true "密码参数"
// @Success  200  {object} response.Body
// @Router   /auth/password [put]
func (ctl *AuthController) ChangePassword(c *gin.Context) {
	var req dto.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errs.BadRequest("参数错误："+err.Error()))
		return
	}
	if err := ctl.auth.ChangePassword(c.Request.Context(), middleware.CurrentUserID(c), &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OKMessage(c, "密码修改成功，请重新登录")
}

// truncate 把字符串限制在 maxBytes 字节内，且不切断 UTF-8 字符。
//
// 用于写库前兜住长度不受客户端控制的字段（如 User-Agent）：
// 列宽是字节数，直接按 len(rune) 截断仍可能超长或产生非法编码。
func truncate(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// 从上限处回退到最近的字符边界，避免留下半个多字节字符。
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
