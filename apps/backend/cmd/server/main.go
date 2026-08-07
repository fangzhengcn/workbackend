// Package main 是后端服务入口。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fangzhengcn/workbackend/apps/backend/config"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/bootstrap"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/router"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/logger"
)

// @title       权限管理后台 API
// @version     1.0
// @description 基于 RBAC 的通用后台管理系统接口
// @BasePath    /api/v1
// @securityDefinitions.apikey BearerAuth
// @in          header
// @name        Authorization
func main() {
	configDir := flag.String("config", "config", "配置文件根目录")
	env := flag.String("env", "", "运行环境：development / testing / production（缺省取 APP_ENV，再缺省为 development）")
	flag.Parse()

	if err := run(*configDir, *env); err != nil {
		// 此时 logger 可能尚未初始化，直接写 stderr 保证错误可见。
		fmt.Fprintf(os.Stderr, "启动失败: %v\n", err)
		os.Exit(1)
	}
}

func run(configDir, env string) error {
	cfg, err := config.Load(configDir, env)
	if err != nil {
		return err
	}

	if err := logger.Init(logger.Options{
		Level:      cfg.Log.Level,
		Format:     cfg.Log.Format,
		Dir:        cfg.Log.Dir,
		MaxSize:    cfg.Log.MaxSize,
		MaxBackups: cfg.Log.MaxBackups,
		MaxAge:     cfg.Log.MaxAge,
		Compress:   cfg.Log.Compress,
		Stdout:     cfg.Log.Stdout,
	}); err != nil {
		return err
	}

	// 装配阶段留出超时，避免数据库/Redis 不可用时无限期挂住。
	initCtx, cancelInit := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelInit()

	app, err := bootstrap.New(initCtx, cfg)
	if err != nil {
		return err
	}
	defer app.Close()

	engine := router.Setup(app.Router)
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.App.Port),
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// 在独立 goroutine 中启动，主 goroutine 负责等待退出信号。
	serverErr := make(chan error, 1)
	go func() {
		logger.Infof("服务已启动：http://localhost:%d （env=%s）", cfg.App.Port, cfg.App.Env)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("HTTP 服务异常退出: %w", err)
	case sig := <-quit:
		logger.Infof("收到信号 %s，开始优雅关闭...", sig)
	}

	// 给正在处理的请求留出完成时间，避免直接切断连接。
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("优雅关闭失败: %w", err)
	}

	logger.Infof("服务已退出")
	return nil
}
