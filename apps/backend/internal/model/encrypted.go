package model

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"sync"

	"github.com/fangzhengcn/workbackend/apps/backend/pkg/crypto"
)

// 敏感字段加解密采用设计文档 6.4.4 的「方式 A：自定义字段类型」。
//
// 为什么用包级 cipher：database/sql 的 Valuer.Value() 与 Scanner.Scan() 接口
// 签名固定，无法额外传入 cipher 实例。因此在进程启动时通过 SetCipher 注册一次，
// 之后全局只读。代价是不便于在单个进程内使用多套密钥；换来的好处是业务代码
// 完全无感知加解密，不会因为某处忘记加密而明文落库。
var (
	cipherMu     sync.RWMutex
	activeCipher *crypto.Cipher
)

// ErrCipherNotReady 表示尚未调用 SetCipher。
// 出现该错误说明启动流程有问题，宁可显式失败也不能明文落库。
var ErrCipherNotReady = errors.New("model: 加密器未初始化，敏感字段无法读写")

// SetCipher 注册全局加密器，应在 main 启动阶段、任何数据库操作之前调用一次。
func SetCipher(c *crypto.Cipher) {
	cipherMu.Lock()
	defer cipherMu.Unlock()
	activeCipher = c
}

// Cipher 返回全局加密器。
func Cipher() (*crypto.Cipher, error) {
	cipherMu.RLock()
	defer cipherMu.RUnlock()
	if activeCipher == nil {
		return nil, ErrCipherNotReady
	}
	return activeCipher, nil
}

// EncryptedString 是自动加解密的字符串字段。
//
// 写库时 Value() 加密为 Base64(nonce‖ciphertext‖tag)，
// 读库时 Scan() 解密还原为明文，业务代码始终看到明文。
//
// 该类型无法用于 WHERE 精确匹配或建唯一索引（每次密文都不同），
// 需要检索的字段请配合盲索引列（见 BlindIndex 与 sys_user 的 *_hash 列）。
type EncryptedString string

// String 返回明文内容。
func (s EncryptedString) String() string { return string(s) }

// IsZero 判断是否为空。
func (s EncryptedString) IsZero() bool { return s == "" }

// Value 实现 driver.Valuer：加密后写入数据库。
func (s EncryptedString) Value() (driver.Value, error) {
	if s == "" {
		// 空值存 NULL 而不是空串的密文，配合唯一索引可允许多行为空。
		return nil, nil
	}
	c, err := Cipher()
	if err != nil {
		return nil, err
	}
	encrypted, err := c.Encrypt(string(s))
	if err != nil {
		return nil, err
	}
	return encrypted, nil
}

// Scan 实现 sql.Scanner：从数据库读出后解密。
func (s *EncryptedString) Scan(value any) error {
	if value == nil {
		*s = ""
		return nil
	}
	var encoded string
	switch v := value.(type) {
	case string:
		encoded = v
	case []byte:
		encoded = string(v)
	default:
		return fmt.Errorf("model: EncryptedString 无法解析类型 %T", value)
	}
	if encoded == "" {
		*s = ""
		return nil
	}
	c, err := Cipher()
	if err != nil {
		return err
	}
	plaintext, err := c.Decrypt(encoded)
	if err != nil {
		return err
	}
	*s = EncryptedString(plaintext)
	return nil
}

// GormDataType 告知 GORM 该字段在数据库中的存储类型。
func (EncryptedString) GormDataType() string { return "varchar(255)" }

// blindIndexOf 计算明文的 HMAC 盲索引，空值返回 nil 以便存 NULL。
//
// 返回 *string 而非 string：唯一索引下 NULL 可重复、空串不可重复，
// 未填手机号的多个用户必须都存 NULL 才不会互相冲突。
func blindIndexOf(plaintext EncryptedString) (*string, error) {
	if plaintext == "" {
		return nil, nil
	}
	c, err := Cipher()
	if err != nil {
		return nil, err
	}
	hash := c.BlindIndex(string(plaintext))
	return &hash, nil
}
