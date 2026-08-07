package model

import "time"

// 操作日志业务类型（sys_oper_log.business_type）。
const (
	BusinessTypeOther  int8 = 0
	BusinessTypeInsert int8 = 1
	BusinessTypeUpdate int8 = 2
	BusinessTypeDelete int8 = 3
	BusinessTypeQuery  int8 = 4
)

// OperLog 对应 sys_oper_log 表：记录增删改等写操作。
//
// RequestParam 写入前必须脱敏（密码、手机号、邮箱等），
// 否则明文敏感信息会通过日志表泄露。
type OperLog struct {
	ID           uint64  `gorm:"primaryKey;column:id" json:"id"`
	Title        string  `gorm:"column:title;size:64" json:"title"`
	BusinessType int8    `gorm:"column:business_type;not null;default:0" json:"businessType"`
	Method       string  `gorm:"column:method;size:100" json:"method"`
	RequestURL   string  `gorm:"column:request_url;size:255" json:"requestUrl"`
	OperUserID   *uint64 `gorm:"column:oper_user_id;index:idx_oper_user_id" json:"operUserId"`
	OperName     string  `gorm:"column:oper_name;size:64" json:"operName"`
	OperIP       string  `gorm:"column:oper_ip;size:64" json:"operIp"`
	RequestParam string  `gorm:"column:request_param;type:text" json:"requestParam"`
	JSONResult   string  `gorm:"column:json_result;type:text" json:"jsonResult"`
	Status       int8    `gorm:"column:status;not null;default:1" json:"status"`
	ErrorMsg     string  `gorm:"column:error_msg;size:2000" json:"errorMsg"`
	// CostTime 单位毫秒。
	CostTime  int       `gorm:"column:cost_time;not null;default:0" json:"costTime"`
	CreatedAt time.Time `gorm:"column:created_at;index:idx_created_at" json:"createdAt"`
}

func (OperLog) TableName() string { return "sys_oper_log" }

// LoginLog 对应 sys_login_log 表：记录登录行为（含失败）。
type LoginLog struct {
	ID       uint64 `gorm:"primaryKey;column:id" json:"id"`
	Username string `gorm:"column:username;size:64;index:idx_username" json:"username"`
	IPAddr   string `gorm:"column:ipaddr;size:64" json:"ipaddr"`
	Location string `gorm:"column:location;size:128" json:"location"`
	Browser  string `gorm:"column:browser;size:256" json:"browser"`
	OS       string `gorm:"column:os;size:64" json:"os"`
	Status   int8   `gorm:"column:status;not null;default:1" json:"status"`
	Msg      string `gorm:"column:msg;size:255" json:"msg"`
	// LoginTime 使用独立列名，非通用 created_at。
	LoginTime time.Time `gorm:"column:login_time;index:idx_login_time" json:"loginTime"`
}

func (LoginLog) TableName() string { return "sys_login_log" }
