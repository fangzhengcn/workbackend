package model

import "strings"

// Dept 对应 sys_dept 表：树形组织架构。
type Dept struct {
	ID       uint64 `gorm:"primaryKey;column:id" json:"id"`
	ParentID uint64 `gorm:"column:parent_id;not null;default:0;index:idx_parent_id" json:"parentId"`
	// Ancestors 是逗号分隔的祖级 ID 列表（如 "0,1,2"），
	// 用于 DataScopeDeptTree 时一条 LIKE 查出整棵子树，避免递归查库。
	Ancestors string `gorm:"column:ancestors;size:255;not null;default:''" json:"ancestors"`
	Name      string `gorm:"column:name;size:64;not null" json:"name"`
	Sort      int    `gorm:"column:sort;not null;default:0" json:"sort"`
	Leader    string `gorm:"column:leader;size:64" json:"leader"`
	Phone     string `gorm:"column:phone;size:32" json:"phone"`
	Status    int8   `gorm:"column:status;not null;default:1" json:"status"`

	Audit
	Timestamps
	SoftDelete

	Children []*Dept `gorm:"-" json:"children,omitempty"`
}

func (Dept) TableName() string { return "sys_dept" }

// 实现 treeutil.Node[*Dept]。
func (d *Dept) NodeID() uint64         { return d.ID }
func (d *Dept) ParentIDValue() uint64  { return d.ParentID }
func (d *Dept) SetChildren(cs []*Dept) { d.Children = cs }

// AncestorIDs 解析 Ancestors 为字符串切片。
func (d *Dept) AncestorIDs() []string {
	if d.Ancestors == "" {
		return nil
	}
	return strings.Split(d.Ancestors, ",")
}

// ChildAncestors 返回其子部门应写入的 ancestors 值。
func (d *Dept) ChildAncestors() string {
	if d.Ancestors == "" {
		return formatUint(d.ID)
	}
	return d.Ancestors + "," + formatUint(d.ID)
}
