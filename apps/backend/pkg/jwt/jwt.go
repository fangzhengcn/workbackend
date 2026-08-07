// Package jwt 负责 Access/Refresh Token 的签发与解析。
package jwt

import (
	"errors"
	"fmt"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

// TokenType 区分 Access 与 Refresh，防止用 Refresh Token 直接访问业务接口。
type TokenType string

const (
	AccessToken  TokenType = "access"
	RefreshToken TokenType = "refresh"
)

var (
	// ErrExpired 表示 Token 已过期，调用方应提示重新登录或走续期。
	ErrExpired = errors.New("jwt: token 已过期")
	// ErrInvalid 表示签名不合法或结构损坏。
	ErrInvalid = errors.New("jwt: token 无效")
	// ErrWrongType 表示 Token 类型与使用场景不符。
	ErrWrongType = errors.New("jwt: token 类型不匹配")
)

// Claims 是 JWT 载荷。
//
// 只放「变更频率低 + 体积小」的信息。权限点（perms）不放入 Token：
// 一是可能很大，二是改权限后旧 Token 仍会带着过期的权限，存在安全风险。
type Claims struct {
	UserID   uint64    `json:"userId"`
	Username string    `json:"username"`
	DeptID   uint64    `json:"deptId,omitempty"`
	Roles    []string  `json:"roles"`
	Type     TokenType `json:"type"`
	jwtlib.RegisteredClaims
}

// Manager 封装签发与解析所需的密钥与有效期配置。
type Manager struct {
	secret        []byte
	issuer        string
	accessExpire  time.Duration
	refreshExpire time.Duration
}

// NewManager 构造 Manager。
func NewManager(secret, issuer string, accessExpireHours, refreshExpireHours int) *Manager {
	return &Manager{
		secret:        []byte(secret),
		issuer:        issuer,
		accessExpire:  time.Duration(accessExpireHours) * time.Hour,
		refreshExpire: time.Duration(refreshExpireHours) * time.Hour,
	}
}

// AccessExpire 返回 Access Token 有效期，供响应体告知前端。
func (m *Manager) AccessExpire() time.Duration { return m.accessExpire }

// GenerateAccess 签发 Access Token，同时返回其 jti 与过期时间。
// jti 用于登出时把该 Token 精确写入 Redis 黑名单。
func (m *Manager) GenerateAccess(userID uint64, username string, deptID uint64, roles []string, jti string) (string, time.Time, error) {
	return m.generate(userID, username, deptID, roles, AccessToken, jti, m.accessExpire)
}

// GenerateRefresh 签发 Refresh Token。
func (m *Manager) GenerateRefresh(userID uint64, username string, deptID uint64, roles []string, jti string) (string, time.Time, error) {
	return m.generate(userID, username, deptID, roles, RefreshToken, jti, m.refreshExpire)
}

func (m *Manager) generate(
	userID uint64, username string, deptID uint64, roles []string,
	tokenType TokenType, jti string, expire time.Duration,
) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(expire)
	claims := Claims{
		UserID:   userID,
		Username: username,
		DeptID:   deptID,
		Roles:    roles,
		Type:     tokenType,
		RegisteredClaims: jwtlib.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   username,
			ID:        jti,
			IssuedAt:  jwtlib.NewNumericDate(now),
			NotBefore: jwtlib.NewNumericDate(now),
			ExpiresAt: jwtlib.NewNumericDate(expiresAt),
		},
	}
	signed, err := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims).SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("jwt: 签发失败: %w", err)
	}
	return signed, expiresAt, nil
}

// Parse 校验签名与有效期并返回载荷。
func (m *Manager) Parse(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwtlib.ParseWithClaims(tokenString, claims, func(t *jwtlib.Token) (any, error) {
		// 显式校验签名算法，防止 alg 混淆攻击（如伪造 alg=none）。
		if _, ok := t.Method.(*jwtlib.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("jwt: 非预期的签名算法 %v", t.Header["alg"])
		}
		return m.secret, nil
	}, jwtlib.WithIssuer(m.issuer))

	if err != nil {
		if errors.Is(err, jwtlib.ErrTokenExpired) {
			return nil, ErrExpired
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if !token.Valid {
		return nil, ErrInvalid
	}
	return claims, nil
}

// ParseAccess 解析并要求其为 Access Token。
func (m *Manager) ParseAccess(tokenString string) (*Claims, error) {
	claims, err := m.Parse(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.Type != AccessToken {
		return nil, ErrWrongType
	}
	return claims, nil
}

// ParseRefresh 解析并要求其为 Refresh Token。
func (m *Manager) ParseRefresh(tokenString string) (*Claims, error) {
	claims, err := m.Parse(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.Type != RefreshToken {
		return nil, ErrWrongType
	}
	return claims, nil
}
