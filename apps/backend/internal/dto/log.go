package dto

// OperLogQuery 操作日志列表查询条件。
//
// 时间区间用字符串接收而非 time.Time：前端传的是 "2026-08-06 00:00:00" 这类
// 本地时间字符串，绑定成 time.Time 需要固定格式与时区约定，
// 交给数据库按会话时区比较更简单，也避免多一层格式解析失败。
type OperLogQuery struct {
	PageQuery
	// Title 是操作模块名（如「用户管理」），模糊匹配。
	Title    string `form:"title" binding:"omitempty,max=64"`
	OperName string `form:"operName" binding:"omitempty,max=64"`
	// BusinessType 见 model.BusinessType* 常量：0 其他 1 新增 2 修改 3 删除 4 查询。
	BusinessType *int8  `form:"businessType" binding:"omitempty,oneof=0 1 2 3 4"`
	Status       *int8  `form:"status" binding:"omitempty,oneof=0 1"`
	BeginTime    string `form:"beginTime" binding:"omitempty,max=32"`
	EndTime      string `form:"endTime" binding:"omitempty,max=32"`
}

// LoginLogQuery 登录日志列表查询条件。
type LoginLogQuery struct {
	PageQuery
	Username  string `form:"username" binding:"omitempty,max=64"`
	IPAddr    string `form:"ipaddr" binding:"omitempty,max=64"`
	Status    *int8  `form:"status" binding:"omitempty,oneof=0 1"`
	BeginTime string `form:"beginTime" binding:"omitempty,max=32"`
	EndTime   string `form:"endTime" binding:"omitempty,max=32"`
}

// DeleteLogsRequest 批量删除日志。
type DeleteLogsRequest struct {
	IDs []uint64 `json:"ids" binding:"required,min=1"`
}
