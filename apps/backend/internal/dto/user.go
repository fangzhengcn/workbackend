package dto

// UserQuery 用户列表查询条件。
type UserQuery struct {
	PageQuery
	// Username 支持模糊查询（明文列）。
	Username string `form:"username" binding:"omitempty,max=64"`
	// Phone 只能精确匹配：手机号为密文，需转成 HMAC 盲索引后查询。
	Phone  string  `form:"phone" binding:"omitempty,max=32"`
	Status *int8   `form:"status" binding:"omitempty,oneof=0 1"`
	DeptID *uint64 `form:"deptId"`
}

// CreateUserRequest 新增用户。
type CreateUserRequest struct {
	Username string   `json:"username" binding:"required,min=2,max=64"`
	Password string   `json:"password" binding:"required,min=6,max=64"`
	Nickname string   `json:"nickname" binding:"omitempty,max=64"`
	Email    string   `json:"email" binding:"omitempty,email,max=128"`
	Phone    string   `json:"phone" binding:"omitempty,len=11,numeric"`
	Gender   int8     `json:"gender" binding:"omitempty,oneof=0 1 2"`
	DeptID   *uint64  `json:"deptId"`
	Status   *int8    `json:"status" binding:"omitempty,oneof=0 1"`
	Remark   string   `json:"remark" binding:"omitempty,max=255"`
	RoleIDs  []uint64 `json:"roleIds"`
}

// UpdateUserRequest 修改用户。
//
// 用指针区分「未传该字段」与「显式传空值」：nil 表示不修改。
// 不含 Username 与 Password：账号不可改，密码走独立的重置接口。
type UpdateUserRequest struct {
	Nickname *string  `json:"nickname" binding:"omitempty,max=64"`
	Email    *string  `json:"email" binding:"omitempty,email,max=128"`
	Phone    *string  `json:"phone" binding:"omitempty,len=11,numeric"`
	Gender   *int8    `json:"gender" binding:"omitempty,oneof=0 1 2"`
	DeptID   *uint64  `json:"deptId"`
	Status   *int8    `json:"status" binding:"omitempty,oneof=0 1"`
	Remark   *string  `json:"remark" binding:"omitempty,max=255"`
	RoleIDs  []uint64 `json:"roleIds"`
}

// ResetPasswordRequest 管理员重置他人密码。
type ResetPasswordRequest struct {
	Password string `json:"password" binding:"required,min=6,max=64"`
}

// AssignRolesRequest 给用户分配角色。
type AssignRolesRequest struct {
	RoleIDs []uint64 `json:"roleIds" binding:"required"`
}
