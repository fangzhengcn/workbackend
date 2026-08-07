package model

// Role 对应 sys_role 表。
type Role struct {
	ID   uint64 `gorm:"primaryKey;column:id" json:"id"`
	Name string `gorm:"column:name;size:64;not null" json:"name"`
	Code string `gorm:"column:code;size:64;not null;uniqueIndex:uk_code" json:"code"`
	Sort int    `gorm:"column:sort;not null;default:0" json:"sort"`
	// DataScope 决定该角色可见的数据范围，取值见 DataScope* 常量。
	DataScope int8   `gorm:"column:data_scope;not null;default:3" json:"dataScope"`
	Status    int8   `gorm:"column:status;not null;default:1" json:"status"`
	Remark    string `gorm:"column:remark;size:255" json:"remark"`

	Audit
	Timestamps
	SoftDelete

	Menus []Menu `gorm:"many2many:sys_role_menu;foreignKey:ID;joinForeignKey:role_id;References:ID;joinReferences:menu_id" json:"menus,omitempty"`
	Depts []Dept `gorm:"many2many:sys_role_dept;foreignKey:ID;joinForeignKey:role_id;References:ID;joinReferences:dept_id" json:"depts,omitempty"`
}

func (Role) TableName() string { return "sys_role" }

// IsSuperAdmin 判断是否为超级管理员角色。
func (r *Role) IsSuperAdmin() bool { return r.Code == SuperAdminRoleCode }

// UserRole 对应 sys_user_role 关联表。
type UserRole struct {
	UserID uint64 `gorm:"primaryKey;column:user_id" json:"userId"`
	RoleID uint64 `gorm:"primaryKey;column:role_id" json:"roleId"`
}

func (UserRole) TableName() string { return "sys_user_role" }

// RoleMenu 对应 sys_role_menu 关联表。
type RoleMenu struct {
	RoleID uint64 `gorm:"primaryKey;column:role_id" json:"roleId"`
	MenuID uint64 `gorm:"primaryKey;column:menu_id" json:"menuId"`
}

func (RoleMenu) TableName() string { return "sys_role_menu" }

// RoleDept 对应 sys_role_dept 关联表，用于 DataScopeCustom 的自定义数据范围。
type RoleDept struct {
	RoleID uint64 `gorm:"primaryKey;column:role_id" json:"roleId"`
	DeptID uint64 `gorm:"primaryKey;column:dept_id" json:"deptId"`
}

func (RoleDept) TableName() string { return "sys_role_dept" }
