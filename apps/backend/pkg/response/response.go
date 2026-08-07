// Package response 提供统一的 HTTP 响应封装。
//
// 所有接口返回体固定为 {code, message, data}（设计文档 §9.1）。
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fangzhengcn/workbackend/apps/backend/pkg/errs"
)

// Body 是统一响应结构。
type Body struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// PageData 是分页列表的标准返回结构。
type PageData struct {
	List  any   `json:"list"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Size  int   `json:"size"`
}

// OK 返回成功响应。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{Code: errs.CodeSuccess, Message: "success", Data: data})
}

// OKMessage 返回仅带提示语的成功响应，用于新增/修改/删除等无返回体的操作。
func OKMessage(c *gin.Context, message string) {
	c.JSON(http.StatusOK, Body{Code: errs.CodeSuccess, Message: message})
}

// Page 返回分页数据。
func Page(c *gin.Context, list any, total int64, page, size int) {
	OK(c, PageData{List: list, Total: total, Page: page, Size: size})
}

// Fail 按错误类型返回失败响应。
//
// 只有 *errs.AppError 的 message 会原样返回给前端；其余错误统一返回
// 「服务器内部错误」，防止把 SQL、堆栈等内部细节泄露出去。
func Fail(c *gin.Context, err error) {
	if appErr, ok := errs.As(err); ok {
		c.JSON(appErr.HTTPStatus(), Body{Code: appErr.Code, Message: appErr.Message})
		return
	}
	// 附加到 gin 的错误链，由 logger 中间件统一记录完整原因。
	_ = c.Error(err)
	c.JSON(http.StatusInternalServerError, Body{Code: errs.CodeInternal, Message: "服务器内部错误"})
}

// FailWithCode 直接指定 code 与 message 返回失败，供中间件使用。
func FailWithCode(c *gin.Context, code int, message string) {
	appErr := errs.New(code, message)
	c.JSON(appErr.HTTPStatus(), Body{Code: code, Message: message})
}

// AbortWithCode 返回失败并终止后续 handler，供鉴权类中间件使用。
func AbortWithCode(c *gin.Context, code int, message string) {
	FailWithCode(c, code, message)
	c.Abort()
}
