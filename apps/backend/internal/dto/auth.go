// Package dto 定义请求入参结构，并承载参数校验规则。
//
// 与 model 解耦：入参不直接绑定实体，避免前端传入 id/status 等字段造成越权修改。
package dto

// LoginRequest 登录请求。
type LoginRequest struct {
	Username string `json:"username" binding:"required,min=2,max=64"`
	Password string `json:"password" binding:"required,min=6,max=64"`
	// CaptchaID 与 CaptchaCode 配对校验；未启用验证码时可为空。
	CaptchaID   string `json:"captchaId"`
	CaptchaCode string `json:"captchaCode"`
}

// RefreshRequest 用 Refresh Token 换取新 Access Token。
type RefreshRequest struct {
	RefreshToken string `json:"refreshToken" binding:"required"`
}

// ChangePasswordRequest 修改自己的密码。
type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword" binding:"required,min=6,max=64"`
	NewPassword string `json:"newPassword" binding:"required,min=6,max=64,nefield=OldPassword"`
}

// UpdateProfileRequest 修改自己的个人资料。
//
// 刻意只开放这四个字段：status/deptId/roleIds 属于管理员职权，
// 若放进本接口，任何用户都能给自己改部门或提权。
// 与 UpdateUserRequest 一样用指针区分「未传」与「显式清空」。
type UpdateProfileRequest struct {
	Nickname *string `json:"nickname" binding:"omitempty,max=64"`
	Email    *string `json:"email" binding:"omitempty,email,max=128"`
	Phone    *string `json:"phone" binding:"omitempty,len=11,numeric"`
	Gender   *int8   `json:"gender" binding:"omitempty,oneof=0 1 2"`
}

// PageQuery 是分页查询的公共参数。
type PageQuery struct {
	Page int `form:"page,default=1" binding:"omitempty,min=1"`
	Size int `form:"size,default=10" binding:"omitempty,min=1,max=200"`
}

// Normalize 修正非法的分页参数，避免 LIMIT 出现负数或过大值。
func (q *PageQuery) Normalize() {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.Size < 1 {
		q.Size = 10
	}
	if q.Size > 200 {
		q.Size = 200
	}
}

// Offset 返回 SQL OFFSET。
func (q PageQuery) Offset() int { return (q.Page - 1) * q.Size }

// Limit 返回 SQL LIMIT。
func (q PageQuery) Limit() int { return q.Size }
