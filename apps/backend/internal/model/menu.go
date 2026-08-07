package model

// Menu 对应 sys_menu 表：目录/菜单/按钮三合一的权限树。
//
// 该表无软删除列（与 SQL 保持一致），删除为物理删除。
type Menu struct {
	ID       uint64 `gorm:"primaryKey;column:id" json:"id"`
	ParentID uint64 `gorm:"column:parent_id;not null;default:0;index:idx_parent_id" json:"parentId"`
	Name     string `gorm:"column:name;size:64;not null" json:"name"`
	// Type 取值见 MenuType* 常量：1 目录 2 菜单 3 按钮。
	Type      int8   `gorm:"column:type;not null;default:2" json:"type"`
	Path      string `gorm:"column:path;size:200" json:"path"`
	Component string `gorm:"column:component;size:255" json:"component"`
	// Perms 是权限标识，命名约定「模块:资源:操作」，如 system:user:add。
	Perms   string `gorm:"column:perms;size:100" json:"perms"`
	Icon    string `gorm:"column:icon;size:100" json:"icon"`
	Sort    int    `gorm:"column:sort;not null;default:0" json:"sort"`
	Visible int8   `gorm:"column:visible;not null;default:1" json:"visible"`
	Status  int8   `gorm:"column:status;not null;default:1" json:"status"`
	IsFrame int8   `gorm:"column:is_frame;not null;default:0" json:"isFrame"`

	Audit
	Timestamps

	// Children 由 treeutil 在内存中组装，不映射数据库列。
	Children []*Menu `gorm:"-" json:"children,omitempty"`
}

func (Menu) TableName() string { return "sys_menu" }

// 以下三个方法实现 treeutil.Node[*Menu]，使 *Menu 可直接建树。
func (m *Menu) NodeID() uint64         { return m.ID }
func (m *Menu) ParentIDValue() uint64  { return m.ParentID }
func (m *Menu) SetChildren(cs []*Menu) { m.Children = cs }

// IsButton 判断是否为按钮类型（无路由，仅承载 perms）。
func (m *Menu) IsButton() bool { return m.Type == MenuTypeButton }

// IsVisible 判断菜单是否应在侧边栏显示。
func (m *Menu) IsVisible() bool { return m.Visible == StatusEnabled }
