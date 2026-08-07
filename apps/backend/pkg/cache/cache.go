// Package cache 封装 Redis 客户端，承载 Token 黑名单、验证码与权限缓存。
package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis key 前缀，集中定义避免各处硬编码拼错。
const (
	// keyTokenBlacklist 记录已登出但尚未自然过期的 Token（按 jti）。
	keyTokenBlacklist = "auth:blacklist:jti:%s"
	// keyUserKicked 记录被「踢下线」的用户，其在该时间点之前签发的 Token 全部失效。
	keyUserKicked = "auth:kicked:user:%d"
	// keyCaptcha 存图形验证码答案。
	keyCaptcha = "auth:captcha:%s"
	// keyUserPerms 缓存用户权限点集合。
	keyUserPerms = "auth:perms:user:%d"
)

// Client 是 Redis 操作的封装。
type Client struct {
	rdb *redis.Client
}

// Options 是 Redis 连接参数。
type Options struct {
	Addr     string
	Password string
	DB       int
	PoolSize int
}

// New 建立 Redis 连接并做一次 Ping 探活。
func New(ctx context.Context, opts Options) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     opts.Addr,
		Password: opts.Password,
		DB:       opts.DB,
		PoolSize: opts.PoolSize,
	})
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		return nil, fmt.Errorf("cache: 连接 Redis 失败: %w", err)
	}
	return &Client{rdb: rdb}, nil
}

// Raw 返回底层客户端，供需要特殊命令的场景使用。
func (c *Client) Raw() *redis.Client { return c.rdb }

// Close 关闭连接。
func (c *Client) Close() error { return c.rdb.Close() }

// BlacklistToken 将 jti 加入黑名单，TTL 设为该 Token 的剩余有效期。
//
// 用剩余有效期而非固定值作为 TTL：Token 自然过期后黑名单记录也随之清理，
// 不会无限堆积。
func (c *Client) BlacklistToken(ctx context.Context, jti string, ttl time.Duration) error {
	if jti == "" {
		return errors.New("cache: jti 不能为空")
	}
	if ttl <= 0 {
		// 已经过期的 Token 无需入黑名单。
		return nil
	}
	return c.rdb.Set(ctx, fmt.Sprintf(keyTokenBlacklist, jti), 1, ttl).Err()
}

// IsTokenBlacklisted 判断 jti 是否已被登出。
func (c *Client) IsTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	n, err := c.rdb.Exists(ctx, fmt.Sprintf(keyTokenBlacklist, jti)).Result()
	if err != nil {
		return false, fmt.Errorf("cache: 查询 Token 黑名单失败: %w", err)
	}
	return n > 0, nil
}

// KickUser 记录用户被强制下线的时间点，其之前签发的所有 Token 立即失效。
// ttl 应不小于 Access Token 有效期，否则记录过早清理会让旧 Token 复活。
func (c *Client) KickUser(ctx context.Context, userID uint64, at time.Time, ttl time.Duration) error {
	return c.rdb.Set(ctx, fmt.Sprintf(keyUserKicked, userID), at.Unix(), ttl).Err()
}

// KickedAt 返回用户被踢下线的时间点；未被踢过时返回零值。
func (c *Client) KickedAt(ctx context.Context, userID uint64) (time.Time, error) {
	sec, err := c.rdb.Get(ctx, fmt.Sprintf(keyUserKicked, userID)).Int64()
	if errors.Is(err, redis.Nil) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("cache: 查询下线标记失败: %w", err)
	}
	return time.Unix(sec, 0), nil
}

// SetCaptcha 保存验证码答案。
func (c *Client) SetCaptcha(ctx context.Context, id, answer string, ttl time.Duration) error {
	return c.rdb.Set(ctx, fmt.Sprintf(keyCaptcha, id), answer, ttl).Err()
}

// ConsumeCaptcha 取出并立即删除验证码，保证一个验证码只能用一次。
func (c *Client) ConsumeCaptcha(ctx context.Context, id string) (string, error) {
	answer, err := c.rdb.GetDel(ctx, fmt.Sprintf(keyCaptcha, id)).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("cache: 读取验证码失败: %w", err)
	}
	return answer, nil
}

// InvalidateUserPerms 清除用户权限缓存，角色/权限变更后调用。
//
// 容忍 nil 接收者：调用方（UserService）对本方法的失败只记警告、不中断业务，
// 说明缓存清理属于「尽力而为」而非关键路径。让未配置缓存的场景（如集成测试）
// 走空实现，比要求每个调用点先判空更简洁，也避免漏判一处就 panic。
func (c *Client) InvalidateUserPerms(ctx context.Context, userID uint64) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Del(ctx, fmt.Sprintf(keyUserPerms, userID)).Err()
}
