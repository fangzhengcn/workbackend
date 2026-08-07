.PHONY: help init deploy deploy-rebuild deploy-down deploy-logs deploy-ps deploy-destroy \
        db-init keygen dev-api dev-api-test dev-web build build-api build-web \
        tidy fmt vet test swagger

BACKEND_DIR := apps/backend
MYSQL_USER  ?= root
MYSQL_HOST  ?= 127.0.0.1
MYSQL_PORT  ?= 3306

help: ## 显示所有可用命令
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

# ============ 一键部署（Docker）============

deploy: ## 【一键部署】构建并启动全部服务（自动生成密钥）
	@cd deploy && ./deploy.sh up

deploy-rebuild: ## 强制重新构建镜像并启动
	@cd deploy && ./deploy.sh rebuild

deploy-down: ## 停止并移除容器（保留数据）
	@cd deploy && ./deploy.sh down

deploy-logs: ## 跟踪查看容器日志
	@cd deploy && ./deploy.sh logs

deploy-ps: ## 查看服务状态
	@cd deploy && ./deploy.sh ps

deploy-destroy: ## 停止并删除数据卷（会清空数据库）
	@cd deploy && ./deploy.sh destroy

# ============ 本地开发 ============

init: ## 安装前端依赖 + 拉取 Go 依赖
	pnpm install
	cd $(BACKEND_DIR) && go mod tidy

keygen: ## 生成 AES/HMAC/JWT 随机密钥，填入 .env
	@echo "APP_CRYPTO_AESKEY=$$(openssl rand -hex 32)"
	@echo "APP_CRYPTO_HMACKEY=$$(openssl rand -hex 32)"
	@echo "APP_JWT_SECRET=$$(openssl rand -hex 32)"

db-init: ## 导入建表脚本与初始化数据（会重建 rbac_admin 库的表）
	mysql -h$(MYSQL_HOST) -P$(MYSQL_PORT) -u$(MYSQL_USER) -p < docs/权限系统数据库.sql

dev-api: ## 启动后端（development 环境，默认 :8080）
	cd $(BACKEND_DIR) && APP_ENV=development go run ./cmd/server

dev-api-test: ## 以 testing 环境启动后端
	cd $(BACKEND_DIR) && APP_ENV=testing go run ./cmd/server

dev-web: ## 启动前端（默认 :5173）
	pnpm --filter @workbackend/web dev

# ============ 构建与检查 ============

build: build-api build-web ## 构建前后端

build-api: ## 编译后端为单一二进制
	cd $(BACKEND_DIR) && go build -trimpath -ldflags "-s -w" -o bin/server ./cmd/server

build-web: ## 构建前端静态资源到 apps/web/dist
	pnpm --filter @workbackend/web build

tidy: ## 整理 Go 依赖
	cd $(BACKEND_DIR) && go mod tidy

fmt: ## 格式化 Go 代码
	cd $(BACKEND_DIR) && go fmt ./...

vet: ## Go 静态检查
	cd $(BACKEND_DIR) && go vet ./...

test: ## 运行 Go 单元测试
	cd $(BACKEND_DIR) && go test ./... -count=1

typecheck: ## 前端类型检查
	pnpm -r typecheck

swagger: ## 生成 Swagger 文档（需 swag v1.16.x，见下方注释）
	@# 版本必须与 go.mod 里的 github.com/swaggo/swag 一致，否则生成的 docs.go
	@# 会用到库中不存在的字段（如 LeftDelim）而编译失败：
	@#   go install github.com/swaggo/swag/cmd/swag@v1.16.3
	@# --parseDependency --parseInternal 缺一不可：注解引用了 internal 下的
	@# vo/dto/response 类型，不加会报 cannot find type definition。
	cd $(BACKEND_DIR) && swag init -g cmd/server/main.go -o docs --parseDependency --parseInternal
