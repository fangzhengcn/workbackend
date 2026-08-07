// Package errs 定义业务错误类型与错误码。
//
// 约定：Service 层返回 *AppError 描述可预期的业务失败（如「用户名已存在」），
// Controller 层统一识别并转成对应 HTTP 响应；非 *AppError 一律视为 500 内部错误，
// 只记日志、不把细节返回给前端。
package errs

import (
	"errors"
	"fmt"
	"net/http"
)

// 业务错误码，与设计文档 §9.1 的统一响应 code 对齐。
const (
	CodeSuccess      = 200
	CodeBadRequest   = 400
	CodeUnauthorized = 401
	CodeForbidden    = 403
	CodeNotFound     = 404
	CodeInternal     = 500
)

// AppError 是可预期的业务错误，携带对外暴露的 code 与 message。
type AppError struct {
	Code    int
	Message string
	// cause 保留底层错误用于日志排查，不会返回给前端。
	cause error
}

func (e *AppError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.cause)
	}
	return e.Message
}

// Unwrap 支持 errors.Is / errors.As 链式判断。
func (e *AppError) Unwrap() error { return e.cause }

// WithCause 附加底层原因，返回副本以避免修改共享的哨兵错误。
func (e *AppError) WithCause(err error) *AppError {
	return &AppError{Code: e.Code, Message: e.Message, cause: err}
}

// HTTPStatus 返回该错误对应的 HTTP 状态码。
func (e *AppError) HTTPStatus() int {
	switch e.Code {
	case CodeBadRequest:
		return http.StatusBadRequest
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeNotFound:
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

// New 构造一个自定义业务错误。
func New(code int, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

// BadRequest 参数错误。
func BadRequest(message string) *AppError { return New(CodeBadRequest, message) }

// Unauthorized 未登录或凭证失效。
func Unauthorized(message string) *AppError { return New(CodeUnauthorized, message) }

// Forbidden 无权限。
func Forbidden(message string) *AppError { return New(CodeForbidden, message) }

// NotFound 资源不存在。
func NotFound(message string) *AppError { return New(CodeNotFound, message) }

// Internal 服务器内部错误。
func Internal(message string) *AppError { return New(CodeInternal, message) }

// As 尝试把任意 error 断言为 *AppError。
func As(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}

// 预定义的常见业务错误。使用时建议 .WithCause(err) 附加底层原因。
var (
	ErrInvalidCredentials = Unauthorized("用户名或密码错误")
	ErrUserDisabled       = Forbidden("账号已停用，请联系管理员")
	ErrTokenInvalid       = Unauthorized("登录状态无效，请重新登录")
	ErrTokenExpired       = Unauthorized("登录已过期，请重新登录")
	ErrPermissionDenied   = Forbidden("无操作权限")
	ErrCaptchaInvalid     = BadRequest("验证码错误或已失效")
	ErrUserNotFound       = NotFound("用户不存在")
	ErrRoleNotFound       = NotFound("角色不存在")
	ErrMenuNotFound       = NotFound("菜单不存在")
	ErrDeptNotFound       = NotFound("部门不存在")
	ErrUsernameExists     = BadRequest("登录账号已存在")
	ErrPhoneExists        = BadRequest("手机号已被使用")
	ErrEmailExists        = BadRequest("邮箱已被使用")
	ErrRoleCodeExists     = BadRequest("角色标识已存在")
	ErrOldPasswordWrong   = BadRequest("原密码不正确")
	// ErrProtectedAdmin 防止误删/停用初始超级管理员，避免把自己锁死。
	ErrProtectedAdmin = Forbidden("超级管理员账号受保护，不允许该操作")
)
