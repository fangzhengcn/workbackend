// Package model 定义与数据库表一一对应的 GORM 实体。
//
// 表结构以 docs/权限系统数据库.sql 为准，改动字段时两边必须同步。
package model

import (
	"time"

	"gorm.io/gorm"
)

// 状态值约定：与 SQL 中所有 status TINYINT 字段一致。
const (
	StatusDisabled int8 = 0 // 停用
	StatusEnabled  int8 = 1 // 正常
)

// 菜单类型（sys_menu.type）。
const (
	MenuTypeDir    int8 = 1 // 目录
	MenuTypeMenu   int8 = 2 // 菜单
	MenuTypeButton int8 = 3 // 按钮
)

// 数据权限范围（sys_role.data_scope）。
const (
	DataScopeAll      int8 = 1 // 全部数据
	DataScopeCustom   int8 = 2 // 自定义（按 sys_role_dept）
	DataScopeDept     int8 = 3 // 本部门
	DataScopeDeptTree int8 = 4 // 本部门及子部门
	DataScopeSelf     int8 = 5 // 仅本人
)

// 性别（sys_user.gender）。
const (
	GenderUnknown int8 = 0
	GenderMale    int8 = 1
	GenderFemale  int8 = 2
)

// SuperAdminRoleCode 是超级管理员的角色标识。
// 鉴权中间件识别到该角色即直接放行，避免管理员把自己锁死（设计文档 §4.3）。
const SuperAdminRoleCode = "admin"

// RootID 是菜单树与部门树的虚拟根节点 ID（parent_id = 0 表示顶级）。
const RootID uint64 = 0

// Timestamps 是通用时间戳字段。
type Timestamps struct {
	CreatedAt time.Time `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updatedAt"`
}

// Audit 是审计字段：记录创建人与更新人。
type Audit struct {
	CreatedBy *uint64 `gorm:"column:created_by" json:"createdBy,omitempty"`
	UpdatedBy *uint64 `gorm:"column:updated_by" json:"updatedBy,omitempty"`
}

// SoftDelete 提供软删除能力，GORM 会自动在查询中追加 deleted_at IS NULL。
type SoftDelete struct {
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-"`
}
