# workbackend

开发工作后台，包含前后端。前后端分离的 **RBAC 权限管理后台**，monorepo 组织。

- 后端：Golang 1.21 + Gin + GORM + MySQL 8 + Redis + Casbin
- 前端：Vue 3 + Vite + TypeScript + ant-design-vue + Pinia
- 权限：菜单级 / 按钮级 / 接口级 / 数据级 四层控制
- 安全：邮箱与手机号 AES-256-GCM 加密存储 + HMAC 盲索引

## 一键部署

只需装好 Docker，然后：

```bash
make deploy
```

脚本会自动生成随机密钥、构建镜像、按依赖顺序启动 MySQL / Redis / 后端 / Nginx，
并在 MySQL 首次启动时导入表结构与种子数据。完成后访问 <http://localhost>，
用 `admin` / `123456` 登录（**登录后请立即修改密码**）。

| 命令 | 说明 |
| --- | --- |
| `make deploy` | 构建并启动全部服务 |
| `make deploy-rebuild` | 强制重建镜像后启动 |
| `make deploy-logs` | 查看日志 |
| `make deploy-ps` | 查看服务状态 |
| `make deploy-down` | 停止（保留数据） |
| `make deploy-destroy` | 停止并清空数据库 |

> `deploy/.env` 内含自动生成的密钥，**请务必备份**：AES/HMAC 密钥丢失后，
> 已加密的邮箱/手机号将无法解密。

## 本地开发

```bash
make init      # 安装前端依赖 + go mod tidy
make keygen    # 生成密钥，填入 .env（参考 .env.example）
make db-init   # 导入建表脚本

make dev-api   # 后端 :8080（development 环境）
make dev-web   # 前端 :5173
```

`make help` 查看全部命令。

## 目录结构

```
├── apps/
│   ├── backend/            Go 服务（cmd / internal / pkg / config）
│   └── web/                Vue3 前端
├── packages/
│   └── shared/             前后端共享的 TS 类型与权限码常量
├── deploy/                 Dockerfile / compose / nginx / deploy.sh
├── docs/                   设计文档与建表脚本
├── CLAUDE.md               开发约定与关键约束
└── Makefile
```

配置按环境分目录、目录内按关注点分文件：

```
apps/backend/config/{development,testing,production}/
    app.yaml  database.yaml  redis.yaml  log.yaml
```

由 `APP_ENV` 选择环境（缺省 `development`）；所有密钥字段留空，由环境变量注入。

## 文档

- [设计文档](docs/权限管理后台设计文档.md) —— 架构、RBAC 模型、加密方案、接口规范
- [建表脚本](docs/权限系统数据库.sql) —— 表结构与初始化数据
- [CLAUDE.md](CLAUDE.md) —— **改代码前必读**：分层约定、加密字段注意事项、权限同步规则

## 当前完成度

阶段一（基础框架）与阶段二（认证鉴权）已完成，登录鉴权全链路可跑通；
用户管理已实现，角色/菜单/部门目前仅提供只读接口。详见 [CLAUDE.md](CLAUDE.md)。
