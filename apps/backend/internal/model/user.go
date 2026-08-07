package model

import (
	"time"

	"gorm.io/gorm"
)

// User 对应 sys_user 表。
//
// 敏感字段说明：
//   - Nickname / Email / Phone 为 EncryptedString，落库自动加密、读取自动解密。
//   - EmailHash / PhoneHash 是 HMAC 盲索引，由 BeforeSave 钩子自动维护，
//     业务代码不要手动赋值；查询时用 Cipher().BlindIndex(明文) 生成条件值。
type User struct {
	ID       uint64          `gorm:"primaryKey;column:id" json:"id"`
	Username string          `gorm:"column:username;size:64;not null;uniqueIndex:uk_username" json:"username"`
	Password string          `gorm:"column:password;size:100;not null" json:"-"`
	Nickname EncryptedString `gorm:"column:nickname;size:255" json:"nickname"`
	Avatar   string          `gorm:"column:avatar;size:255" json:"avatar"`
	Email    EncryptedString `gorm:"column:email;size:255" json:"email"`
	// EmailHash 由钩子维护，json 中不暴露。
	EmailHash *string         `gorm:"column:email_hash;size:64;uniqueIndex:uk_email_hash" json:"-"`
	Phone     EncryptedString `gorm:"column:phone;size:128" json:"phone"`
	PhoneHash *string         `gorm:"column:phone_hash;size:64;uniqueIndex:uk_phone_hash" json:"-"`
	Gender    int8            `gorm:"column:gender;not null;default:0" json:"gender"`
	DeptID    *uint64         `gorm:"column:dept_id;index:idx_dept_id" json:"deptId"`
	Status    int8            `gorm:"column:status;not null;default:1" json:"status"`
	// KeyVersion 记录加密时使用的密钥版本，支持平滑轮换。
	KeyVersion  int        `gorm:"column:key_version;not null;default:1" json:"-"`
	LastLoginAt *time.Time `gorm:"column:last_login_at" json:"lastLoginAt"`
	LastLoginIP string     `gorm:"column:last_login_ip;size:64" json:"lastLoginIp"`
	Remark      string     `gorm:"column:remark;size:255" json:"remark"`

	Audit
	Timestamps
	SoftDelete

	// Roles 为多对多关联，需显式 Preload 才会加载。
	Roles []Role `gorm:"many2many:sys_user_role;foreignKey:ID;joinForeignKey:user_id;References:ID;joinReferences:role_id" json:"roles,omitempty"`
	Dept  *Dept  `gorm:"foreignKey:DeptID;references:ID" json:"dept,omitempty"`
}

func (User) TableName() string { return "sys_user" }

// IsEnabled 判断账号是否可用。
func (u *User) IsEnabled() bool { return u.Status == StatusEnabled }

// RoleCodes 提取角色标识集合，需已 Preload Roles。
func (u *User) RoleCodes() []string {
	codes := make([]string, 0, len(u.Roles))
	for _, role := range u.Roles {
		// 停用的角色不应继续授予权限。
		if role.Status == StatusEnabled {
			codes = append(codes, role.Code)
		}
	}
	return codes
}

// IsSuperAdmin 判断是否拥有超级管理员角色。
func (u *User) IsSuperAdmin() bool {
	for _, role := range u.Roles {
		if role.Code == SuperAdminRoleCode && role.Status == StatusEnabled {
			return true
		}
	}
	return false
}

// DeptIDValue 返回部门 ID，未分配部门时返回 0。
func (u *User) DeptIDValue() uint64 {
	if u.DeptID == nil {
		return 0
	}
	return *u.DeptID
}

// BeforeSave 在写库前根据明文重算盲索引，保证 hash 列与密文列始终一致。
//
// 放在 BeforeSave（而非 BeforeCreate/BeforeUpdate 各写一份）是因为它对
// 创建与更新都会触发，避免漏掉某条路径导致 hash 与实际值不符。
//
// 注意：本钩子对 Session 级的批量 Updates(map) 不生效——用 map 更新
// email/phone 时必须自行同步 *_hash，否则唯一约束会失效。
func (u *User) BeforeSave(tx *gorm.DB) error {
	emailHash, err := blindIndexOf(u.Email)
	if err != nil {
		return err
	}
	phoneHash, err := blindIndexOf(u.Phone)
	if err != nil {
		return err
	}
	u.EmailHash = emailHash
	u.PhoneHash = phoneHash

	// 记录本次加密所用的密钥版本。
	if c, err := Cipher(); err == nil {
		u.KeyVersion = c.KeyVersion()
	}
	return nil
}
