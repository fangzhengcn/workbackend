// Package bootstrap 负责应用的依赖装配与生命周期管理。
package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/casbin/casbin/v2"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/fangzhengcn/workbackend/apps/backend/config"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/controller"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/repository"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/router"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/service"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/cache"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/crypto"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/jwt"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/logger"
)

// App 持有应用运行期需要的资源，便于优雅关闭时统一释放。
type App struct {
	Config *config.Config
	DB     *gorm.DB
	Cache  *cache.Client
	Router *router.Dependencies
}

// New 完成全部依赖装配。
func New(ctx context.Context, cfg *config.Config) (*App, error) {
	// 1. 加密器必须最先初始化：任何用户表读写都依赖它，
	//    晚于数据库初始化会导致启动期查询直接失败。
	cipher, err := crypto.NewCipher(cfg.Crypto.AESKey, cfg.Crypto.HMACKey, cfg.Crypto.KeyVersion)
	if err != nil {
		return nil, fmt.Errorf("初始化加密器失败: %w", err)
	}
	model.SetCipher(cipher)

	// 2. 数据库
	db, err := newDB(cfg)
	if err != nil {
		return nil, err
	}

	// 3. Redis
	cacheClient, err := cache.New(ctx, cache.Options{
		Addr:     cfg.Redis.Addr(),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
		PoolSize: cfg.Redis.PoolSize,
	})
	if err != nil {
		return nil, err
	}

	// 4. Repository
	users := repository.NewUserRepository(db)
	roles := repository.NewRoleRepository(db)
	menus := repository.NewMenuRepository(db)
	depts := repository.NewDeptRepository(db)
	logs := repository.NewLogRepository(db)
	dicts := repository.NewDictRepository(db)

	// 5. Casbin
	enforcer, err := newEnforcer(db, cfg.Casbin.ModelPath)
	if err != nil {
		return nil, err
	}

	// 6. Service
	permissions := service.NewPermissionService(enforcer, roles)
	// 启动时全量重建策略，保证与 sys_role_menu 一致
	// （否则库里改过权限但未重启的实例会继续用旧策略）。
	if err := permissions.ReloadPolicies(ctx); err != nil {
		return nil, fmt.Errorf("加载权限策略失败: %w", err)
	}

	jwtManager := jwt.NewManager(
		cfg.JWT.Secret, cfg.JWT.Issuer, cfg.JWT.ExpireHours, cfg.JWT.RefreshExpireHours,
	)
	// 验证码开关由配置决定：开发环境默认关闭，便于用 curl 直接调试登录。
	captchas := service.NewCaptchaService(cacheClient, cfg.App.Captcha)
	dataScope := service.NewDataScopeService(roles, depts)

	authService := service.NewAuthService(users, roles, menus, logs, jwtManager, cacheClient, captchas)
	userService := service.NewUserService(users, roles, dataScope, cacheClient)
	roleService := service.NewRoleService(roles, menus, permissions)
	menuService := service.NewMenuService(menus, permissions)
	deptService := service.NewDeptService(depts)
	dictService := service.NewDictService(dicts)
	logService := service.NewLogService(logs)

	// 7. Controller
	deps := &router.Dependencies{
		Config:      cfg,
		JWT:         jwtManager,
		Cache:       cacheClient,
		Permissions: permissions,
		Logs:        logs,

		Auth: controller.NewAuthController(authService, captchas),
		User: controller.NewUserController(userService, cfg.Upload),
		Role: controller.NewRoleController(roleService),
		Menu: controller.NewMenuController(menuService),
		Dept: controller.NewDeptController(deptService),
		Dict: controller.NewDictController(dictService),
		Log:  controller.NewLogController(logService),
	}

	return &App{Config: cfg, DB: db, Cache: cacheClient, Router: deps}, nil
}

// Close 释放数据库与 Redis 连接。
func (a *App) Close() {
	if a.Cache != nil {
		if err := a.Cache.Close(); err != nil {
			logger.Warnf("关闭 Redis 连接失败: %v", err)
		}
	}
	if a.DB != nil {
		if sqlDB, err := a.DB.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				logger.Warnf("关闭数据库连接失败: %v", err)
			}
		}
	}
}

// newDB 建立 GORM 连接并配置连接池。
func newDB(cfg *config.Config) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN()), &gorm.Config{
		Logger: gormlogger.Default.LogMode(parseGormLogLevel(cfg.MySQL.LogLevel)),
		// 表名已由各 model 的 TableName() 指定，禁用复数化避免意外改名。
		NamingStrategy: nil,
	})
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库句柄失败: %w", err)
	}
	sqlDB.SetMaxIdleConns(cfg.MySQL.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MySQL.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.MySQL.ConnMaxLifetime) * time.Minute)

	// 尽早探活，避免服务起来后第一个请求才发现连不上库。
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("数据库探活失败: %w", err)
	}
	return db, nil
}

func parseGormLogLevel(level string) gormlogger.LogLevel {
	switch level {
	case "silent":
		return gormlogger.Silent
	case "error":
		return gormlogger.Error
	case "warn":
		return gormlogger.Warn
	default:
		return gormlogger.Info
	}
}

// newEnforcer 基于 GORM adapter 构建 Casbin 执行器。
func newEnforcer(db *gorm.DB, modelPath string) (*casbin.Enforcer, error) {
	adapter, err := gormadapter.NewAdapterByDB(db)
	if err != nil {
		return nil, fmt.Errorf("初始化 Casbin adapter 失败: %w", err)
	}
	enforcer, err := casbin.NewEnforcer(modelPath, adapter)
	if err != nil {
		return nil, fmt.Errorf("初始化 Casbin enforcer 失败（模型文件: %s）: %w", modelPath, err)
	}
	if err := enforcer.LoadPolicy(); err != nil {
		return nil, fmt.Errorf("加载 Casbin 策略失败: %w", err)
	}
	return enforcer, nil
}
