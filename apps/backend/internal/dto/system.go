package dto

// RoleQuery 角色列表查询条件。
type RoleQuery struct {
	PageQuery
	Name   string `form:"name" binding:"omitempty,max=64"`
	Code   string `form:"code" binding:"omitempty,max=64"`
	Status *int8  `form:"status" binding:"omitempty,oneof=0 1"`
}

// CreateRoleRequest 新增角色。
type CreateRoleRequest struct {
	Name string `json:"name" binding:"required,max=64"`
	// Code 为权限判定依据，只允许字母/数字/下划线/冒号，避免与 Casbin 策略解析冲突。
	Code      string   `json:"code" binding:"required,max=64,alphanumunicode"`
	Sort      int      `json:"sort"`
	DataScope int8     `json:"dataScope" binding:"omitempty,oneof=1 2 3 4 5"`
	Status    *int8    `json:"status" binding:"omitempty,oneof=0 1"`
	Remark    string   `json:"remark" binding:"omitempty,max=255"`
	MenuIDs   []uint64 `json:"menuIds"`
	DeptIDs   []uint64 `json:"deptIds"`
}

// UpdateRoleRequest 修改角色。Code 不可修改，避免已签发 Token 的角色标识失效。
type UpdateRoleRequest struct {
	Name      *string  `json:"name" binding:"omitempty,max=64"`
	Sort      *int     `json:"sort"`
	DataScope *int8    `json:"dataScope" binding:"omitempty,oneof=1 2 3 4 5"`
	Status    *int8    `json:"status" binding:"omitempty,oneof=0 1"`
	Remark    *string  `json:"remark" binding:"omitempty,max=255"`
	MenuIDs   []uint64 `json:"menuIds"`
}

// AssignMenusRequest 分配角色菜单权限。
type AssignMenusRequest struct {
	MenuIDs []uint64 `json:"menuIds" binding:"required"`
}

// DataScopeRequest 设置角色数据权限范围。
type DataScopeRequest struct {
	DataScope int8 `json:"dataScope" binding:"required,oneof=1 2 3 4 5"`
	// DeptIDs 仅在 DataScope=2（自定义）时生效。
	DeptIDs []uint64 `json:"deptIds"`
}

// CreateMenuRequest 新增菜单。
type CreateMenuRequest struct {
	ParentID  uint64 `json:"parentId"`
	Name      string `json:"name" binding:"required,max=64"`
	Type      int8   `json:"type" binding:"required,oneof=1 2 3"`
	Path      string `json:"path" binding:"omitempty,max=200"`
	Component string `json:"component" binding:"omitempty,max=255"`
	Perms     string `json:"perms" binding:"omitempty,max=100"`
	Icon      string `json:"icon" binding:"omitempty,max=100"`
	Sort      int    `json:"sort"`
	Visible   *int8  `json:"visible" binding:"omitempty,oneof=0 1"`
	Status    *int8  `json:"status" binding:"omitempty,oneof=0 1"`
	IsFrame   *int8  `json:"isFrame" binding:"omitempty,oneof=0 1"`
}

// UpdateMenuRequest 修改菜单。
type UpdateMenuRequest struct {
	ParentID  *uint64 `json:"parentId"`
	Name      *string `json:"name" binding:"omitempty,max=64"`
	Type      *int8   `json:"type" binding:"omitempty,oneof=1 2 3"`
	Path      *string `json:"path" binding:"omitempty,max=200"`
	Component *string `json:"component" binding:"omitempty,max=255"`
	Perms     *string `json:"perms" binding:"omitempty,max=100"`
	Icon      *string `json:"icon" binding:"omitempty,max=100"`
	Sort      *int    `json:"sort"`
	Visible   *int8   `json:"visible" binding:"omitempty,oneof=0 1"`
	Status    *int8   `json:"status" binding:"omitempty,oneof=0 1"`
	IsFrame   *int8   `json:"isFrame" binding:"omitempty,oneof=0 1"`
}

// CreateDeptRequest 新增部门。
type CreateDeptRequest struct {
	ParentID uint64 `json:"parentId"`
	Name     string `json:"name" binding:"required,max=64"`
	Sort     int    `json:"sort"`
	Leader   string `json:"leader" binding:"omitempty,max=64"`
	Phone    string `json:"phone" binding:"omitempty,max=32"`
	Status   *int8  `json:"status" binding:"omitempty,oneof=0 1"`
}

// UpdateDeptRequest 修改部门。
type UpdateDeptRequest struct {
	ParentID *uint64 `json:"parentId"`
	Name     *string `json:"name" binding:"omitempty,max=64"`
	Sort     *int    `json:"sort"`
	Leader   *string `json:"leader" binding:"omitempty,max=64"`
	Phone    *string `json:"phone" binding:"omitempty,max=32"`
	Status   *int8   `json:"status" binding:"omitempty,oneof=0 1"`
}
