// Package crypto 提供敏感字段的加解密与盲索引能力。
//
// 设计要点（对应设计文档 6.4）：
//   - 敏感字段（nickname/email/phone）使用 AES-256-GCM 加密后落库，
//     存储格式为 Base64(nonce ‖ ciphertext ‖ authTag)。
//   - GCM 每次加密随机生成 12 字节 nonce，因此相同明文的密文不同，
//     不会泄露「两个用户手机号相同」这类信息。
//   - 因密文不可检索，需精确查询/去重的字段（email/phone）额外用
//     HMAC-SHA256 生成确定性盲索引，并使用独立于 AES 的密钥。
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

var (
	// ErrInvalidKeySize 表示密钥长度不符合 AES-256 要求。
	ErrInvalidKeySize = errors.New("crypto: 密钥必须为 32 字节")
	// ErrCiphertextTooShort 表示密文长度不足，无法包含 nonce。
	ErrCiphertextTooShort = errors.New("crypto: 密文长度不足")
)

// Cipher 封装 AES-256-GCM 加解密与 HMAC 盲索引计算。
//
// 并发安全：内部 cipher.AEAD 与 hmacKey 创建后只读，可被多 goroutine 共享。
type Cipher struct {
	aead    cipher.AEAD
	hmacKey []byte
	// keyVersion 会随记录一起写入 sys_user.key_version，便于密钥轮换时识别。
	keyVersion int
}

// NewCipher 用 hex 编码的密钥构造 Cipher。
// aesKeyHex 与 hmacKeyHex 均须为 64 个 hex 字符（对应 32 字节）。
func NewCipher(aesKeyHex, hmacKeyHex string, keyVersion int) (*Cipher, error) {
	aesKey, err := hex.DecodeString(aesKeyHex)
	if err != nil {
		return nil, fmt.Errorf("crypto: 解析 AES 密钥失败: %w", err)
	}
	hmacKey, err := hex.DecodeString(hmacKeyHex)
	if err != nil {
		return nil, fmt.Errorf("crypto: 解析 HMAC 密钥失败: %w", err)
	}
	if len(aesKey) != 32 || len(hmacKey) != 32 {
		return nil, ErrInvalidKeySize
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: 初始化 AES 失败: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: 初始化 GCM 失败: %w", err)
	}

	return &Cipher{aead: aead, hmacKey: hmacKey, keyVersion: keyVersion}, nil
}

// KeyVersion 返回当前密钥版本。
func (c *Cipher) KeyVersion() int { return c.keyVersion }

// Encrypt 将明文加密为 Base64(nonce ‖ ciphertext ‖ authTag)。
// 空字符串原样返回，避免把「未填写」变成一段密文。
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: 生成 nonce 失败: %w", err)
	}
	// Seal 将密文与 authTag 追加在 nonce 之后，形成自包含的存储格式。
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt 解密由 Encrypt 生成的字符串。
// authTag 校验失败（密文被篡改或密钥不匹配）时返回错误。
func (c *Cipher) Decrypt(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("crypto: Base64 解码失败: %w", err)
	}
	nonceSize := c.aead.NonceSize()
	if len(raw) < nonceSize {
		return "", ErrCiphertextTooShort
	}
	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// 不要把原始错误直接暴露给调用方日志，避免密文细节外泄。
		return "", fmt.Errorf("crypto: 解密失败，密文可能被篡改或密钥不匹配")
	}
	return string(plaintext), nil
}

// BlindIndex 计算明文的 HMAC-SHA256 盲索引，返回 64 位小写 hex。
//
// 确定性：相同明文恒得相同结果，因此可用于 WHERE 精确匹配与唯一索引。
// 不可逆：即便该列泄露也无法反推明文。
func (c *Cipher) BlindIndex(plaintext string) string {
	if plaintext == "" {
		return ""
	}
	mac := hmac.New(sha256.New, c.hmacKey)
	mac.Write([]byte(plaintext))
	return hex.EncodeToString(mac.Sum(nil))
}
