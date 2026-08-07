// Package vo 定义响应数据结构。
//
// 与 model 解耦的两个目的：
//  1. 避免 password 等敏感字段意外序列化返回；
//  2. 手机号/邮箱等敏感信息在此统一脱敏（设计文档 6.4.5 第 5 条）。
//
// 字段需与 packages/shared/src/types.ts 保持一致。
package vo

import (
	"time"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/crypto"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/treeutil"
)

// AllPerms 是超级管理员的权限通配符，前端据此放开所有按钮。
const AllPerms = "*:*:*"

// LoginResult 登录成功返回的令牌信息。
type LoginResult struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	// ExpiresIn 为 Access Token 剩余有效秒数。
	ExpiresIn int64  `json:"expiresIn"`
	TokenType string `json:"tokenType"`
}

// CaptchaResult 图形验证码。
type CaptchaResult struct {
	CaptchaID string `json:"captchaId"`
	// ImageBase64 形如 data:image/png;base64,...，可直接作为 img src。
	ImageBase64 string `json:"imageBase64"`
}

// UserInfo 当前登录用户信息。手机号与邮箱均已脱敏。
type UserInfo struct {
	ID          uint64     `json:"id"`
	Username    string     `json:"username"`
	Nickname    string     `json:"nickname"`
	Avatar      string     `json:"avatar"`
	Email       string     `json:"email"`
	Phone       string     `json:"phone"`
	Gender      int8       `json:"gender"`
	DeptID      *uint64    `json:"deptId"`
	DeptName    string     `json:"deptName"`
	Status      int8       `json:"status"`
	Roles       []string   `json:"roles"`
	Perms       []string   `json:"perms"`
	LastLoginAt *time.Time `json:"lastLoginAt"`
}

// NewUserInfo 由实体构造 UserInfo，自动脱敏敏感字段。
func NewUserInfo(user *model.User, roles, perms []string) *UserInfo {
	info := &UserInfo{
		ID:       user.ID,
		Username: user.Username,
		Nickname: user.Nickname.String(),
		Avatar:   user.Avatar,
		// 返回脱敏值，避免完整明文出现在网络请求中。
		Email:       crypto.MaskEmail(user.Email.String()),
		Phone:       crypto.MaskPhone(user.Phone.String()),
		Gender:      user.Gender,
		DeptID:      user.DeptID,
		Status:      user.Status,
		Roles:       roles,
		Perms:       perms,
		LastLoginAt: user.LastLoginAt,
	}
	if user.Dept != nil {
		info.DeptName = user.Dept.Name
	}
	// 保证 JSON 中是 [] 而非 null，前端无需额外判空。
	if info.Roles == nil {
		info.Roles = []string{}
	}
	if info.Perms == nil {
		info.Perms = []string{}
	}
	return info
}

// UserItem 用户列表项。
type UserItem struct {
	ID          uint64      `json:"id"`
	Username    string      `json:"username"`
	Nickname    string      `json:"nickname"`
	Email       string      `json:"email"`
	Phone       string      `json:"phone"`
	Gender      int8        `json:"gender"`
	DeptID      *uint64     `json:"deptId"`
	DeptName    string      `json:"deptName"`
	Status      int8        `json:"status"`
	Remark      string      `json:"remark"`
	Roles       []*RoleItem `json:"roles"`
	LastLoginAt *time.Time  `json:"lastLoginAt"`
	CreatedAt   time.Time   `json:"createdAt"`
}

// NewUserItem 由实体构造列表项，同样脱敏手机号与邮箱。
func NewUserItem(user *model.User) *UserItem {
	item := &UserItem{
		ID:          user.ID,
		Username:    user.Username,
		Nickname:    user.Nickname.String(),
		Email:       crypto.MaskEmail(user.Email.String()),
		Phone:       crypto.MaskPhone(user.Phone.String()),
		Gender:      user.Gender,
		DeptID:      user.DeptID,
		Status:      user.Status,
		Remark:      user.Remark,
		Roles:       make([]*RoleItem, 0, len(user.Roles)),
		LastLoginAt: user.LastLoginAt,
		CreatedAt:   user.CreatedAt,
	}
	if user.Dept != nil {
		item.DeptName = user.Dept.Name
	}
	for i := range user.Roles {
		item.Roles = append(item.Roles, NewRoleItem(&user.Roles[i]))
	}
	return item
}

// NewUserItems 批量转换。
func NewUserItems(users []*model.User) []*UserItem {
	items := make([]*UserItem, 0, len(users))
	for _, user := range users {
		items = append(items, NewUserItem(user))
	}
	return items
}

// RoleItem 角色信息。
type RoleItem struct {
	ID        uint64    `json:"id"`
	Name      string    `json:"name"`
	Code      string    `json:"code"`
	Sort      int       `json:"sort"`
	DataScope int8      `json:"dataScope"`
	Status    int8      `json:"status"`
	Remark    string    `json:"remark"`
	CreatedAt time.Time `json:"createdAt"`
}

// NewRoleItem 由实体构造角色 VO。
func NewRoleItem(role *model.Role) *RoleItem {
	return &RoleItem{
		ID:        role.ID,
		Name:      role.Name,
		Code:      role.Code,
		Sort:      role.Sort,
		DataScope: role.DataScope,
		Status:    role.Status,
		Remark:    role.Remark,
		CreatedAt: role.CreatedAt,
	}
}

// NewRoleItems 批量转换。
func NewRoleItems(roles []*model.Role) []*RoleItem {
	items := make([]*RoleItem, 0, len(roles))
	for _, role := range roles {
		items = append(items, NewRoleItem(role))
	}
	return items
}

// RoleDetail 角色详情，在列表项之上附带已分配的菜单与部门 ID。
//
// 单独一个 VO 而非扩展 RoleItem：列表接口若也带上这两个数组，
// 每行都要额外查两次关联表（N+1），而列表页并不需要它们。
type RoleDetail struct {
	*RoleItem
	MenuIDs []uint64 `json:"menuIds"`
	// DeptIDs 仅在 DataScope 为自定义时有意义。
	DeptIDs []uint64 `json:"deptIds"`
}

// NewRoleDetail 构造角色详情。
func NewRoleDetail(role *model.Role, menuIDs, deptIDs []uint64) *RoleDetail {
	// 统一成空切片，避免前端把 null 传给树组件的 checkedKeys。
	if menuIDs == nil {
		menuIDs = []uint64{}
	}
	if deptIDs == nil {
		deptIDs = []uint64{}
	}
	return &RoleDetail{RoleItem: NewRoleItem(role), MenuIDs: menuIDs, DeptIDs: deptIDs}
}

// MenuNode 菜单树节点。
type MenuNode struct {
	ID        uint64      `json:"id"`
	ParentID  uint64      `json:"parentId"`
	Name      string      `json:"name"`
	Type      int8        `json:"type"`
	Path      string      `json:"path"`
	Component string      `json:"component"`
	Perms     string      `json:"perms"`
	Icon      string      `json:"icon"`
	Sort      int         `json:"sort"`
	Visible   int8        `json:"visible"`
	Status    int8        `json:"status"`
	IsFrame   int8        `json:"isFrame"`
	Children  []*MenuNode `json:"children,omitempty"`
}

// 实现 treeutil.Node[*MenuNode]，使 VO 层也能直接建树。
func (m *MenuNode) NodeID() uint64             { return m.ID }
func (m *MenuNode) ParentIDValue() uint64      { return m.ParentID }
func (m *MenuNode) SetChildren(cs []*MenuNode) { m.Children = cs }

// NewMenuNode 由实体构造菜单节点（不含 children）。
func NewMenuNode(menu *model.Menu) *MenuNode {
	return &MenuNode{
		ID:        menu.ID,
		ParentID:  menu.ParentID,
		Name:      menu.Name,
		Type:      menu.Type,
		Path:      menu.Path,
		Component: menu.Component,
		Perms:     menu.Perms,
		Icon:      menu.Icon,
		Sort:      menu.Sort,
		Visible:   menu.Visible,
		Status:    menu.Status,
		IsFrame:   menu.IsFrame,
	}
}

// BuildMenuTree 把扁平菜单列表组装成树。
// 入参应已按 sort 排好序，输出顺序与之一致。
func BuildMenuTree(menus []*model.Menu) []*MenuNode {
	nodes := make([]*MenuNode, 0, len(menus))
	for _, menu := range menus {
		nodes = append(nodes, NewMenuNode(menu))
	}
	return treeutil.Build(nodes, model.RootID)
}

// DeptNode 部门树节点。
type DeptNode struct {
	ID       uint64      `json:"id"`
	ParentID uint64      `json:"parentId"`
	Name     string      `json:"name"`
	Sort     int         `json:"sort"`
	Leader   string      `json:"leader"`
	Phone    string      `json:"phone"`
	Status   int8        `json:"status"`
	Children []*DeptNode `json:"children,omitempty"`
}

func (d *DeptNode) NodeID() uint64             { return d.ID }
func (d *DeptNode) ParentIDValue() uint64      { return d.ParentID }
func (d *DeptNode) SetChildren(cs []*DeptNode) { d.Children = cs }

// BuildDeptTree 把扁平部门列表组装成树。
func BuildDeptTree(depts []*model.Dept) []*DeptNode {
	nodes := make([]*DeptNode, 0, len(depts))
	for _, dept := range depts {
		nodes = append(nodes, &DeptNode{
			ID:       dept.ID,
			ParentID: dept.ParentID,
			Name:     dept.Name,
			Sort:     dept.Sort,
			Leader:   dept.Leader,
			Phone:    dept.Phone,
			Status:   dept.Status,
		})
	}
	return treeutil.Build(nodes, model.RootID)
}

// DictTypeItem 字典类型列表项。
type DictTypeItem struct {
	ID        uint64    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Status    int8      `json:"status"`
	Remark    string    `json:"remark"`
	CreatedAt time.Time `json:"createdAt"`
}

func NewDictTypeItem(t *model.DictType) *DictTypeItem {
	return &DictTypeItem{
		ID:        t.ID,
		Name:      t.Name,
		Type:      t.Type,
		Status:    t.Status,
		Remark:    t.Remark,
		CreatedAt: t.CreatedAt,
	}
}

func NewDictTypeItems(types []*model.DictType) []*DictTypeItem {
	items := make([]*DictTypeItem, 0, len(types))
	for _, t := range types {
		items = append(items, NewDictTypeItem(t))
	}
	return items
}

// DictDataItem 字典数据项。
type DictDataItem struct {
	ID         uint64    `json:"id"`
	DictTypeID uint64    `json:"dictTypeId"`
	DictType   string    `json:"dictType"`
	Label      string    `json:"label"`
	Value      string    `json:"value"`
	Sort       int       `json:"sort"`
	IsDefault  int8      `json:"isDefault"`
	Status     int8      `json:"status"`
	Remark     string    `json:"remark"`
	CreatedAt  time.Time `json:"createdAt"`
}

func NewDictDataItem(d *model.DictData) *DictDataItem {
	return &DictDataItem{
		ID:         d.ID,
		DictTypeID: d.DictTypeID,
		DictType:   d.DictType,
		Label:      d.Label,
		Value:      d.Value,
		Sort:       d.Sort,
		IsDefault:  d.IsDefault,
		Status:     d.Status,
		Remark:     d.Remark,
		CreatedAt:  d.CreatedAt,
	}
}

func NewDictDataItems(list []*model.DictData) []*DictDataItem {
	items := make([]*DictDataItem, 0, len(list))
	for _, d := range list {
		items = append(items, NewDictDataItem(d))
	}
	return items
}

// OperLogItem 操作日志列表项。
//
// 不含 requestParam / jsonResult：这两个 TEXT 字段只在详情里返回，
// 列表带上会让每页响应体膨胀几十 KB（仓库层也没查这两列）。
type OperLogItem struct {
	ID           uint64    `json:"id"`
	Title        string    `json:"title"`
	BusinessType int8      `json:"businessType"`
	Method       string    `json:"method"`
	RequestURL   string    `json:"requestUrl"`
	OperUserID   *uint64   `json:"operUserId"`
	OperName     string    `json:"operName"`
	OperIP       string    `json:"operIp"`
	Status       int8      `json:"status"`
	ErrorMsg     string    `json:"errorMsg"`
	CostTime     int       `json:"costTime"`
	CreatedAt    time.Time `json:"createdAt"`
}

func NewOperLogItem(log *model.OperLog) *OperLogItem {
	return &OperLogItem{
		ID:           log.ID,
		Title:        log.Title,
		BusinessType: log.BusinessType,
		Method:       log.Method,
		RequestURL:   log.RequestURL,
		OperUserID:   log.OperUserID,
		OperName:     log.OperName,
		OperIP:       log.OperIP,
		Status:       log.Status,
		ErrorMsg:     log.ErrorMsg,
		CostTime:     log.CostTime,
		CreatedAt:    log.CreatedAt,
	}
}

func NewOperLogItems(logs []*model.OperLog) []*OperLogItem {
	items := make([]*OperLogItem, 0, len(logs))
	for _, log := range logs {
		items = append(items, NewOperLogItem(log))
	}
	return items
}

// OperLogDetail 操作日志详情，附带请求参数与响应体。
//
// requestParam 在写入时已由 middleware/operlog.go 脱敏（密码置 ***、
// 手机号邮箱打码），此处直接透出即可，无需二次处理。
// 新增敏感入参字段时要同步那里的 redactKeys，否则明文会经由本接口泄露。
type OperLogDetail struct {
	*OperLogItem
	RequestParam string `json:"requestParam"`
	JSONResult   string `json:"jsonResult"`
}

func NewOperLogDetail(log *model.OperLog) *OperLogDetail {
	return &OperLogDetail{
		OperLogItem:  NewOperLogItem(log),
		RequestParam: log.RequestParam,
		JSONResult:   log.JSONResult,
	}
}

// LoginLogItem 登录日志列表项。
type LoginLogItem struct {
	ID        uint64    `json:"id"`
	Username  string    `json:"username"`
	IPAddr    string    `json:"ipaddr"`
	Location  string    `json:"location"`
	Browser   string    `json:"browser"`
	OS        string    `json:"os"`
	Status    int8      `json:"status"`
	Msg       string    `json:"msg"`
	LoginTime time.Time `json:"loginTime"`
}

func NewLoginLogItem(log *model.LoginLog) *LoginLogItem {
	return &LoginLogItem{
		ID:        log.ID,
		Username:  log.Username,
		IPAddr:    log.IPAddr,
		Location:  log.Location,
		Browser:   log.Browser,
		OS:        log.OS,
		Status:    log.Status,
		Msg:       log.Msg,
		LoginTime: log.LoginTime,
	}
}

func NewLoginLogItems(logs []*model.LoginLog) []*LoginLogItem {
	items := make([]*LoginLogItem, 0, len(logs))
	for _, log := range logs {
		items = append(items, NewLoginLogItem(log))
	}
	return items
}
