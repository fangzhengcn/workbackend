// Package config 负责加载与校验应用配置。
//
// 目录结构：按环境分目录，每个环境内按关注点分文件。
//
//	config/
//	├── development/   app.yaml  database.yaml  redis.yaml  log.yaml
//	├── testing/       （同上）
//	├── production/    （同上）
//	└── rbac_model.conf
//
// 加载优先级（后者覆盖前者）：
//  1. config/{env}/*.yaml   —— 按文件名顺序合并
//  2. 环境变量 APP_*        —— 敏感项注入，优先级最高
//
// 环境由 APP_ENV 决定，缺省为 development。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// 支持的运行环境。
const (
	EnvDevelopment = "development"
	EnvTesting     = "testing"
	EnvProduction  = "production"
)

// configFiles 是每个环境目录下需要合并的配置文件。
//
// 显式列出而非通配扫描：顺序确定、缺文件能立即报错，
// 避免漏放一个文件却直到运行时才发现某项配置为零值。
var configFiles = []string{
	"app.yaml",
	"database.yaml",
	"redis.yaml",
	"log.yaml",
}

// Config 是应用的全部配置。
type Config struct {
	App    AppConfig    `mapstructure:"app"`
	MySQL  MySQLConfig  `mapstructure:"mysql"`
	Redis  RedisConfig  `mapstructure:"redis"`
	JWT    JWTConfig    `mapstructure:"jwt"`
	Crypto CryptoConfig `mapstructure:"crypto"`
	Log    LogConfig    `mapstructure:"log"`
	CORS   CORSConfig   `mapstructure:"cors"`
	Casbin CasbinConfig `mapstructure:"casbin"`
	Upload UploadConfig `mapstructure:"upload"`
}

type AppConfig struct {
	Name string `mapstructure:"name"`
	Env  string `mapstructure:"env"`
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
	// Captcha 控制是否启用登录图形验证码。
	Captcha bool `mapstructure:"captcha"`
}

// IsProduction 用于在生产环境启用更严格的校验与行为。
func (a AppConfig) IsProduction() bool { return a.Env == EnvProduction }

// IsDevelopment 判断是否为本地开发环境。
func (a AppConfig) IsDevelopment() bool { return a.Env == EnvDevelopment }

type MySQLConfig struct {
	Host            string `mapstructure:"host"`
	Port            int    `mapstructure:"port"`
	Username        string `mapstructure:"username"`
	Password        string `mapstructure:"password"`
	Database        string `mapstructure:"database"`
	Charset         string `mapstructure:"charset"`
	ParseTime       bool   `mapstructure:"parseTime"`
	Loc             string `mapstructure:"loc"`
	MaxIdleConns    int    `mapstructure:"maxIdleConns"`
	MaxOpenConns    int    `mapstructure:"maxOpenConns"`
	ConnMaxLifetime int    `mapstructure:"connMaxLifetime"`
	LogLevel        string `mapstructure:"logLevel"`
}

// DSN 拼装 GORM 使用的 MySQL 连接串。
func (m MySQLConfig) DSN() string {
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=%t&loc=%s",
		m.Username, m.Password, m.Host, m.Port, m.Database, m.Charset, m.ParseTime, m.Loc,
	)
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"poolSize"`
}

func (r RedisConfig) Addr() string { return fmt.Sprintf("%s:%d", r.Host, r.Port) }

type JWTConfig struct {
	Secret             string `mapstructure:"secret"`
	Issuer             string `mapstructure:"issuer"`
	ExpireHours        int    `mapstructure:"expireHours"`
	RefreshExpireHours int    `mapstructure:"refreshExpireHours"`
}

type CryptoConfig struct {
	// AESKey 为 64 位 hex 字符串，解码后是 32 字节的 AES-256 密钥。
	AESKey string `mapstructure:"aesKey"`
	// HMACKey 为盲索引使用的独立密钥，同为 hex 字符串。
	HMACKey string `mapstructure:"hmacKey"`
	// KeyVersion 写入 sys_user.key_version，支持平滑轮换。
	KeyVersion int `mapstructure:"keyVersion"`
}

type LogConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	Dir        string `mapstructure:"dir"`
	MaxSize    int    `mapstructure:"maxSize"`
	MaxBackups int    `mapstructure:"maxBackups"`
	MaxAge     int    `mapstructure:"maxAge"`
	Compress   bool   `mapstructure:"compress"`
	Stdout     bool   `mapstructure:"stdout"`
}

type CORSConfig struct {
	AllowOrigins []string `mapstructure:"allowOrigins"`
}

type CasbinConfig struct {
	ModelPath string `mapstructure:"modelPath"`
}

// UploadConfig 是文件上传配置。
type UploadConfig struct {
	// Dir 是上传文件的存储根目录。
	// Docker 部署时必须挂成数据卷，否则容器重建后头像全部丢失。
	Dir string `mapstructure:"dir"`
	// URLPrefix 是对外访问前缀，需与静态文件路由一致。
	// 单独配置是为了将来换成 CDN 或对象存储时只改这一处。
	URLPrefix string `mapstructure:"urlPrefix"`
}

// envBoundKeys 列出允许（且敏感项必须）由环境变量注入的配置。
//
// Viper 的 AutomaticEnv 只在 Get 时生效，对 Unmarshal 到嵌套 struct 并不可靠，
// 因此这里显式 BindEnv，确保 APP_* 一定能覆盖 yaml 中的值。
var envBoundKeys = []string{
	"app.env", "app.port", "app.mode", "app.captcha",
	"mysql.host", "mysql.port", "mysql.username", "mysql.password", "mysql.database",
	"mysql.maxIdleConns", "mysql.maxOpenConns", "mysql.logLevel",
	"redis.host", "redis.port", "redis.password", "redis.db",
	"jwt.secret", "jwt.expireHours", "jwt.refreshExpireHours",
	"crypto.aesKey", "crypto.hmacKey", "crypto.keyVersion",
	"log.level", "log.format", "log.dir",
	"casbin.modelPath",
	"upload.dir", "upload.urlPrefix",
}

// ResolveEnv 决定当前运行环境：优先取 APP_ENV，缺省为 development。
func ResolveEnv() string {
	env := strings.TrimSpace(os.Getenv("APP_ENV"))
	if env == "" {
		return EnvDevelopment
	}
	// 兼容常见简写，避免因写成 dev/prod 而静默加载错环境。
	switch env {
	case "dev":
		return EnvDevelopment
	case "test":
		return EnvTesting
	case "prod":
		return EnvProduction
	}
	return env
}

// Load 从 configDir/{env}/ 下合并全部配置文件并完成校验。
//
// env 为空时由 ResolveEnv 决定。
func Load(configDir, env string) (*Config, error) {
	if env == "" {
		env = ResolveEnv()
	}

	envDir := filepath.Join(configDir, env)
	info, err := os.Stat(envDir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf(
			"环境配置目录不存在: %s（可选环境: %s / %s / %s，通过 APP_ENV 或 -env 指定）",
			envDir, EnvDevelopment, EnvTesting, EnvProduction,
		)
	}

	v := viper.New()
	v.SetConfigType("yaml")

	// 环境变量覆盖：jwt.secret -> APP_JWT_SECRET
	replacer := strings.NewReplacer(".", "_")
	for _, key := range envBoundKeys {
		envKey := "APP_" + strings.ToUpper(replacer.Replace(key))
		if err := v.BindEnv(key, envKey); err != nil {
			return nil, fmt.Errorf("绑定环境变量 %s 失败: %w", envKey, err)
		}
	}

	// 依次合并各关注点的配置文件。
	for i, name := range configFiles {
		path := filepath.Join(envDir, name)
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("缺少配置文件 %s: %w", path, err)
		}
		v.SetConfigFile(path)

		// 第一个文件用 Read 建立基线，其余用 Merge 叠加。
		if i == 0 {
			err = v.ReadInConfig()
		} else {
			err = v.MergeInConfig()
		}
		if err != nil {
			return nil, fmt.Errorf("加载配置文件 %s 失败: %w", path, err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	// 以实际加载的目录为准，避免 yaml 里写错 env 导致行为与目录不一致。
	cfg.App.Env = env

	// CORS 白名单支持用逗号分隔的环境变量覆盖。
	// 单独处理是因为 Viper 无法把 "a,b" 自动解析成 []string。
	if raw := strings.TrimSpace(os.Getenv("APP_CORS_ALLOWORIGINS")); raw != "" {
		origins := make([]string, 0, 4)
		for _, item := range strings.Split(raw, ",") {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				origins = append(origins, trimmed)
			}
		}
		if len(origins) > 0 {
			cfg.CORS.AllowOrigins = origins
		}
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// validate 校验关键配置，尽早失败而不是运行到一半才报错。
func (c *Config) validate() error {
	if c.App.Port <= 0 || c.App.Port > 65535 {
		return fmt.Errorf("app.port 非法: %d", c.App.Port)
	}
	if c.MySQL.Database == "" {
		return fmt.Errorf("mysql.database 不能为空")
	}
	if c.JWT.Secret == "" {
		return fmt.Errorf("jwt.secret 未配置，请通过环境变量 APP_JWT_SECRET 注入")
	}
	if c.JWT.ExpireHours <= 0 {
		return fmt.Errorf("jwt.expireHours 必须大于 0")
	}
	// AES/HMAC 密钥为 32 字节，hex 表示即 64 个字符。
	if len(c.Crypto.AESKey) != 64 {
		return fmt.Errorf("crypto.aesKey 必须是 64 位 hex 字符串（32 字节），请通过 APP_CRYPTO_AESKEY 注入；可用 make keygen 生成")
	}
	if len(c.Crypto.HMACKey) != 64 {
		return fmt.Errorf("crypto.hmacKey 必须是 64 位 hex 字符串（32 字节），请通过 APP_CRYPTO_HMACKEY 注入；可用 make keygen 生成")
	}
	if c.Crypto.AESKey == c.Crypto.HMACKey {
		return fmt.Errorf("crypto.aesKey 与 crypto.hmacKey 必须相互独立，不可相同")
	}
	if c.Crypto.KeyVersion <= 0 {
		return fmt.Errorf("crypto.keyVersion 必须大于 0")
	}

	// 生产环境额外收紧：这些配置在开发环境无伤大雅，上线却是真实风险。
	if c.App.IsProduction() {
		if c.MySQL.Password == "" {
			return fmt.Errorf("生产环境 mysql.password 不能为空，请通过 APP_MYSQL_PASSWORD 注入")
		}
		if c.App.Mode != "release" {
			return fmt.Errorf("生产环境 app.mode 必须为 release，当前为 %q", c.App.Mode)
		}
		for _, origin := range c.CORS.AllowOrigins {
			if origin == "*" {
				return fmt.Errorf("生产环境 cors.allowOrigins 不允许使用通配 *")
			}
			if strings.Contains(origin, "localhost") || strings.Contains(origin, "127.0.0.1") {
				return fmt.Errorf("生产环境 cors.allowOrigins 不应包含本地地址: %s", origin)
			}
		}
	}
	return nil
}
