# CLAUDE.md

本文件为 Claude Code 在本仓库工作时的指引。

## 项目概述

前后端分离的 **RBAC 权限管理后台**，monorepo 组织：

- `apps/backend` —— Golang + Gin + GORM + MySQL，提供无状态 RESTful API
- `apps/web` —— Vue3 + Vite + ant-design-vue + Pinia
- `packages/shared` —— 前后端共享的 TS 类型与权限码常量
- `docs/` —— 设计文档与建表脚本（**改表结构时必须同步这里**）
- `deploy/` —— Dockerfile、docker-compose、Nginx 配置、一键部署脚本 `deploy.sh`

设计依据：`docs/权限管理后台设计文档.md`、`docs/权限系统数据库.sql`。
两者是本项目的事实标准，实现与文档冲突时应先确认哪边需要更新，不要单方面改代码。

## 常用命令

### 一键部署（Docker，推荐）

```bash
make deploy          # 构建并启动全部服务，首次自动生成 deploy/.env 密钥
make deploy-logs     # 跟踪日志
make deploy-ps       # 服务状态
make deploy-down     # 停止（保留数据卷）
make deploy-destroy  # 停止并删除数据卷（清空数据库）
```

等价于 `cd deploy && ./deploy.sh up`。脚本会自动：
检查 Docker → 生成随机密钥写入 `deploy/.env`（权限 600）→ 构建镜像 →
按 healthcheck 顺序启动（MySQL/Redis 就绪后才起 backend）→ 轮询 `/healthz` →
打印访问地址与初始账号。

数据库表结构在 MySQL 容器首次启动时由 `docs/权限系统数据库.sql` 自动导入
（仅数据卷为空时执行，重复 `up` 不会覆盖数据）。

### 本地开发

```bash
make init        # 安装前端依赖 + go mod tidy
make keygen      # 生成 AES/HMAC/JWT 随机密钥
make db-init     # 导入 docs/权限系统数据库.sql

make dev-api      # 启动后端 :8080（development 环境）
make dev-api-test # 以 testing 环境启动后端
make dev-web      # 启动前端 :5173（已代理 /api 到后端）

make test / vet / fmt / typecheck
```

## 配置管理

**按环境分目录，环境内按关注点分文件**：

```
apps/backend/config/
├── development/    app.yaml  database.yaml  redis.yaml  log.yaml
├── testing/        （同上四个文件）
├── production/     （同上四个文件）
└── rbac_model.conf  Casbin 模型定义
```

- 环境由 `APP_ENV` 决定（`development` / `testing` / `production`，缺省 development），
  也可用 `-env` 参数指定。兼容 `dev` / `test` / `prod` 简写。
- 加载时把该环境目录下**四个文件全部合并**，缺任何一个直接报错——
  显式列出而非通配扫描，避免漏放文件却到运行时才发现配置为零值（`config.configFiles`）。
- 优先级：`config/{env}/*.yaml` < 环境变量 `APP_*`。
- 环境变量命名：配置路径大写、点换下划线，前缀 `APP_`。
  例：`jwt.secret` → `APP_JWT_SECRET`，`mysql.password` → `APP_MYSQL_PASSWORD`。
  可覆盖的键在 `config.envBoundKeys` 中显式列出（Viper 的 AutomaticEnv 对嵌套
  struct 的 Unmarshal 不可靠，故用 BindEnv）。

**新增配置项时**：改对应 `xxx.yaml`（三个环境都要）+ `config.go` 的 struct
+ 若需环境变量覆盖则加入 `envBoundKeys`。

### 密钥绝不入库

`yaml` 中所有 `secret` / `aesKey` / `hmacKey` / `password` 字段一律留空，
必须由环境变量注入。启动时 `validate()` 会校验：

- AES/HMAC 密钥必须是 64 位 hex（32 字节），且两者不能相同
- JWT secret 不能为空
- **生产环境额外要求**：MySQL 密码非空、`app.mode` 为 release、
  CORS 白名单不含 `*` 与 localhost

校验失败直接启动失败，而非用弱默认值跑起来。

## 首次运行（本地非 Docker）

1. `cp .env.example .env`，`make keygen` 把三个密钥填进去。
2. `make db-init` 导入表结构与种子数据。
3. `make dev-api` + `make dev-web`。
4. 默认账号 `admin` / `123456`（种子数据中的公开示例哈希，**上线前必须重置**）。

> **种子密码哈希改动后务必跑 `go test ./internal/service/`**。
> bcrypt 哈希不可读，肉眼看不出它对应哪个明文。曾出现脚本注释写着
> 「密码 '123456' 的 bcrypt 值」，但那串哈希是网上流传的示例值、实际对应
> `admin123`——照文档登录必然 401，且报错只说「密码错误」，
> 排查方向被完全误导。`auth_seed_test.go` 直接从 SQL 解析哈希并断言匹配，
> 改错任一边都会立刻失败。
>
> 另注意：**建表脚本只在数据卷为空时导入**。改了种子文件后，
> 已存在的库不会更新，需 `./deploy.sh destroy && ./deploy.sh up` 重建
> （会清空数据）或手动 UPDATE。

## 架构与分层约定

请求链路：`Router → Middleware → Controller → Service → Repository → MySQL/Redis`

各层职责边界（**不要越层**）：

- **Controller** —— 只做 HTTP 相关：绑定校验参数、调用 Service、返回统一响应。不写业务逻辑，不碰数据库。
- **Service** —— 业务逻辑与事务边界。权限判断、数据组装在这里。
- **Repository** —— 纯数据访问，屏蔽 GORM 细节。返回 `repository.ErrNotFound` 而非 `gorm.ErrRecordNotFound`，避免上层依赖 ORM。
- **Model** —— GORM 实体，与 `docs/权限系统数据库.sql` 一一对应。
- **DTO / VO** —— 入参与出参，与实体解耦。**敏感字段脱敏统一在 VO 构造函数里做**。

## 关键约束（改代码前必读）

### 1. 前端权限控制不是安全边界

前端的菜单显隐、`v-permission` 指令只是体验优化。攻击者可绕过前端直接调接口。

**每个写接口都必须在路由上挂 `middleware.RequirePerm(...)`**，见 `internal/router/router.go`。
新增接口时若忘记挂权限中间件，等于开了一个未授权入口。

### 2. 敏感字段加密与盲索引

`sys_user` 的 `nickname` / `email` / `phone` 以 AES-256-GCM 密文存储：

- 业务代码给 `model.EncryptedString` 字段**赋明文即可**，读写自动加解密（`internal/model/encrypted.go`）。
- `email_hash` / `phone_hash` 是 HMAC 盲索引，由 `User.BeforeSave` 钩子自动维护，**不要手动赋值**。
- **密文列无法 `WHERE phone = ?` 查询，也无法 LIKE 模糊搜索**。按手机号/邮箱查询必须先
  `model.Cipher().BlindIndex(明文)` 算出 hash 再查 `phone_hash` 列。参考 `UserRepository.FindByPhoneHash`。
- **不要用 `Updates(map)` 更新加密字段**：会绕过 `EncryptedString.Value()` 与 `BeforeSave`，
  导致明文落库或 hash 与密文不一致。用 `Save(&user)`。非加密列（如 `last_login_ip`）用 map 是安全的。
- 空值存 NULL 而非空串——唯一索引下 NULL 可重复，空串不可，多个未填手机号的用户否则会冲突。

### 3. 日志与响应必须脱敏

- 写日志涉及手机号/邮箱时用 `crypto.MaskPhone` / `crypto.MaskEmail`。
- 操作日志中间件已对 `password` / `phone` / `email` 等字段脱敏，
  **新增敏感入参字段时要同步 `internal/middleware/operlog.go` 的 `redactKeys` 等映射**，否则明文入库。
- VO 返回给前端的手机号/邮箱是脱敏值（`vo.NewUserInfo` / `vo.NewUserItem`）。

### 4. 权限变更需刷新 Casbin 策略

Casbin 策略从 `sys_role_menu` 生成。改完角色权限后必须调
`PermissionService.ReloadPolicies(ctx)`，否则变更不生效。

策略语义是 `p = 角色标识, 权限标识(perms), *`，**obj 是权限点而非请求路径**
（`config/rbac_model.conf`）。路由到权限点的映射在 `router.go` 里显式声明。

**必须在事务提交之后刷新**，不能放在事务内：放在事务内一旦回滚，
策略已按未落库的数据重建，内存与数据库就此不一致。
刷新失败只记警告不返回错误——业务数据已提交成功，此时报错会让前端
以为操作失败而重试；策略在下次任意变更或重启时会自愈。

**`ReloadPolicies` 期间必须关掉 autosave**（`enforcer.EnableAutoSave(false)`）：
`ClearPolicy()` 只清内存、不删 `casbin_rule` 表，而 autosave 会让循环里每条
`AddPolicy` 立即单条 INSERT，撞上表里已有记录的唯一索引报 **1062**。
首次部署能成功只因表是空的，**第二次启动必崩**。
末尾的 `SavePolicy()` 本就整表覆盖，那些单条写入是多余的。

### 5. 超级管理员绕过

`code = "admin"` 的角色在鉴权中间件中直接放行；`id = 1` 的用户禁止删除与停用
（`service.adminUserID`）。目的是避免管理员把自己锁死。

### 6. perms 命名前后端必须一致

格式 `模块:资源:操作`（如 `system:user:add`）。三处需同步：

1. `packages/shared/src/perms.ts` 的常量
2. `apps/backend/internal/router/router.go` 的 `perm*` 常量
3. `sys_menu.perms` 数据库记录

**每个权限点都必须有对应的 `type=3` 按钮记录并授给角色**，否则该权限
**无法被授予任何非 admin 角色**——接口永远 403，而界面上完全看不出问题。

这个缺陷曾长期潜伏：种子里只有用户模块有按钮权限（1001-1005），
其余六个模块一个都没有。**因为 admin 拿的是 `vo.AllPerms`（`*:*:*`）通配符
且鉴权中间件对 admin 角色直接放行，用 admin 测一切正常**，
只有换成普通角色才会暴露。

比对命令（新增权限点后跑一遍）：

```bash
grep -oE "'system:[a-zA-Z]+:[a-zA-Z]+'" packages/shared/src/perms.ts | tr -d "'" | sort -u
grep -oE '"system:[a-zA-Z]+:[a-zA-Z]+"' apps/backend/internal/router/router.go | tr -d '"' | sort -u
grep -oE "'system:[a-zA-Z]+:[a-zA-Z]+'" docs/权限系统数据库.sql | tr -d "'" | sort -u
# 三者取差集应为空；至少「router 有但种子缺」必须为空
```

**用非 admin 角色验证权限**：admin 的通配符会掩盖所有授权缺失。

### 7. 前端动态路由与刷新丢失

菜单树 → 路由的转换在 `apps/web/src/store/permission.ts`。
F5 刷新后 Pinia 清空但 Token 仍在，路由守卫会重新拉取菜单并 `addRoute`，
然后 **`return { ...to, replace: true }` 重新导航**——因为新路由是本次守卫之后才注册的，
当前导航仍按旧路由表匹配会落到 404。改守卫逻辑时注意别破坏这一点（`router/guard.ts`）。

菜单的 `component` 字段（如 `system/user/index`）由 `import.meta.glob` 静态映射到
`views/` 下的组件，**数据库里配的路径必须真实存在**，否则退化为 404 页并打控制台错误。

### 8. 树形结构在内存组装

菜单树、部门树都是查出扁平列表后用 `pkg/treeutil` 递归组装（1 次 SQL），
不要改成递归查库。`treeutil.Build` 已处理孤儿节点与环。

### 9. 数据权限过滤

`service.DataScopeService.Scope()` 返回可叠加的 GORM Scope，
新增需要数据权限过滤的列表查询时复用它，不要手写过滤条件。

多角色取**最宽松**的范围（数值越小范围越大）；无任何角色时返回 `1 = 0`（看不到数据），
而不是不加条件（看到全部）。

### 10. 树形表的 parentId 变更必须重算整棵子树

改 `sys_dept.parent_id` 时，**自身与全部后代的 `ancestors` 都要重算**
（`DeptService.rebuildDescendantAncestors`）。只改自己会让后代的 `ancestors`
仍指向旧路径，而 `FindSubtreeIDs` 用 `FIND_IN_SET(?, ancestors)` 做子树过滤，
失配后数据权限判断静默出错——用户看到不该看的数据，界面上完全察觉不到。

菜单与部门都**禁止把节点挂到自己或自己的后代之下**：那会让整棵子树脱离根节点，
从树上凭空消失且无法再从界面移回来。回溯判定要带步数上限，
否则库里已有脏环时会挂住请求。

### 11. 树接口不能返回 null

`treeutil.Build` 保证返回非 nil 切片。前端对树结果普遍直接 `.map()`，
拿到 `null` 会抛 `Cannot read properties of null`——
且报错信息与真实原因（如「该角色未被授予任何菜单」）毫无关联，极难排查。
新增返回数组的接口时，VO 层同样要把 nil 归一成 `[]`。

### 12. 菜单变更同样要刷新 Casbin

除了第 4 条说的角色权限变更，**改 `sys_menu.perms` 或删除菜单也会影响策略**
（策略由 `sys_role_menu` 关联的 `perms` 生成），同样需要 `ReloadPolicies`。

`sys_menu` 是**物理删除**，删除菜单必须在同一事务内清理 `sys_role_menu`，
否则残留的授权行会在新菜单复用同一自增 ID 时让角色凭空获得权限。

### 13. 用 GORM 的 Save 保存 Preload 过的实体必须 Omit 关联

`Save` 默认「完整保存关联」：它会按实体上已加载的关联对象**反推外键写回**，
把刚赋的新值覆盖成旧值。典型症状是**接口返回 200、数据库却没变**。

`UserRepository.FindByID` 用 `Preload("Dept")` 载入旧部门，
若直接 `Save(user)`，`dept_id` 会被写回旧值——「改部门不生效」正是此因。
多对多（`Roles`/`Menus`/`Depts`）同理会被整表重写，绕过
`ReplaceRoles`/`ReplaceMenus` 的显式维护逻辑。

**约定**：保存这类实体一律 `Omit` 掉关联，
如 `Save(user)` 前 `.Omit("Dept", "Roles")`（repository 层已内置）。
关联表的增删改只走 `Replace*` 方法。

### 14. 路由注册顺序：静态段必须在 `:id` 之前

gin 的路由树里 `/users/export` 与 `/users/:id` 在同一位置冲突时，
后注册的静态段会被 `:id` 匹配走，表现为「导出接口返回 JSON 说 ID 参数非法」
——状态码是 400 而非 404，极易误判成参数问题。

已踩过并已处理的几处：`/roles/all`、`/users/export`、
`/oper-logs/clean`、`/oper-logs/export`、`/dicts/data/type/:type`。
新增此类路径时**注册在 `:id` 之前**，并写一条 HTTP 测试断言它没被吞掉。

补充：`/dicts/data/:type` 这种写法会与 `/dicts/data/:id` **直接冲突导致启动 panic**
（同一位置不能有两个不同名的参数），故用 `/dicts/data/type/:type` 多加一段。

### 15. 状态码正常不代表结果正确

多次遇到「HTTP 200 但内容是错的」，且状态码正常反而更难发现：

- nginx 未代理 `/swagger`、`/uploads` 时，被 SPA 的 `try_files` 回退到
  `index.html`——Swagger 返回 200 却是 Vue 页面，`<img>` 拿到 HTML 后图片裂开。
- GORM 关联覆盖导致「改部门」返回 200 但库里没变。

**排查这类问题要看响应体或直接查库，不能只看状态码。**
反过来，改动关键路径后也应断言内容而非仅断言 2xx。

### 16. 前端弹窗内的下拉/树数据不要缓存

弹窗里的角色、部门、菜单树**每次打开都要重新拉取**。
曾用 `if (list.length === 0)` 只拉一次，结果：在角色管理新建了角色，
回到用户管理时组件实例还在内存里，下拉框永远看不到新角色（除非整页刷新）。

这些数据都在**别的页面**维护，缓存必然导致陈旧。相关接口都很轻
（不分页、无关联查询），每次开弹窗多一次请求完全值得。
多个数据源用 `Promise.all` / `allSettled` 并发，避免串行等待。

同理：`RoleFormModal` 保存后要刷新右侧数据 —— 类型标识可能刚被改过，
而右侧查询条件用的是旧标识，不同步会查出空列表。

### 17. ant-design-vue 的几个类型与行为陷阱

- **`a-tree` 授权树必须用 `check-strictly`**。默认联动模式下父节点因
  「子节点未全选」只进 `halfChecked`、不出现在 `checked` 里，提交时父目录被漏掉
  ——保存后「系统管理」目录授权丢失，菜单直接从侧边栏消失。
  提交时取 `checked.checked ?? checked`。
- **`a-tree` 的 `tree-data` 必须自带 `key`**，`field-names` 只在运行期改读取字段名，
  不满足 `DataNode` 的类型约束。显式映射成 `{key, title, children}`。
  （`a-tree-select` 的类型较松，`field-names` 可用。）
- **`a-range-picker` 的 `v-model` 类型是 `[Dayjs, Dayjs] | undefined`**，
  用 `unknown[]` 过不了类型检查。时间区间的结束日期要补到 `23:59:59`，
  否则当天产生的记录会被区间漏掉。
- **`a-upload` 默认自己发请求**，那条链路不走项目的 axios 实例、拿不到
  `Authorization` 头。用 `before-upload` 返回 `false` 接管，自己调 API。

### 18. useTable 的类型与语义

`composables/useTable.ts` 封装了查询条件 + 分页 + 加载态 + 拉取，
列表页复用它、树形页不用（树一次取全量在内存展开）。

- `rows` 必须标注为 `Ref<T[]>`：泛型经 `ref()` 会被推断成 `UnwrapRef`，
  不标注会导致模板里传给 `a-table` 的 `data-source` 类型不符。
- 删除后调 `refreshAfterRemove()`：当前页被删空且不是第一页时自动回退一页，
  否则用户会停在空列表上误以为数据全没了。
- **导出等需要突破分页上限的场景不能复用 `Service.Page`**：
  `PageQuery.Normalize()` 会把 `Size` 截到 200，那是一种静默截断。

## 环境依赖注意

- **后端需要 Go 1.21+**（`go.mod` 声明）。本机为 Go 1.21.13。
- 前端用 pnpm workspace。若无 pnpm，`corepack enable` 即可。
- **Docker 部署不依赖本机 Go**——镜像内用 `golang:1.21-alpine` 构建。
- **本机容器运行时是 Rancher Desktop**（非 Docker Desktop）。
  `/var/run/docker.sock` 是指向其他用户家目录的失效软链，会报 `permission denied`
  而非 `not found`，容易误判成需要 sudo。每个新 shell 都要先设：
  `export DOCKER_HOST="unix:///Users/admin/.rd/docker.sock"`
- 本机 80 端口被既有 SSH 隧道占用，`deploy/.env` 的 `WEB_PORT` 已改为 **8088**。
- **本地经 localhost 访问必须用 `APP_ENV=testing`**：production 的启动校验禁止
  CORS 白名单含 localhost，而浏览器发的 Origin 就是 `http://localhost:8088`，
  用 production 会被 CORS 拦成 403（空响应体，`cost:0`，极易误判）。

## 排查方法（踩过多次，按此顺序最省时间）

### 先定位「请求死在哪一层」

链路是 `浏览器 → nginx(8088) → backend(8080) → MySQL/Redis`，
**先确认请求走到了哪一层**，再看代码：

| 现象 | 结论 |
| --- | --- |
| 后端日志无该请求记录 | 死在 nginx 或更前（查 `docker logs rbac-web`） |
| 后端有记录但 `cost:0` | 被中间件拦下（CORS/鉴权），未进 handler |
| 后端有记录且 `cost>0` | 进了业务逻辑，看具体报错 |
| 返回 200 但数据没变 | **直接查库**，不要信接口的返回值 |

这条链路每一层都咬过一次：nginx 的 1MB 限制（413）、CORS 白名单（403 空响应体）、
operLog 截断请求体（400 unexpected EOF）、GORM 关联覆盖（200 但没落库）。

### 查库比读代码快

「返回成功但没生效」这类问题，**查库一步就能定位**。
`dept_id` 那个 bug 正是如此：接口 200、代码逐行都对，但库里没变——
立刻排除了前端与参数绑定。

```bash
export DOCKER_HOST="unix:///Users/admin/.rd/docker.sock"
cd deploy && PW=$(grep '^MYSQL_ROOT_PASSWORD=' .env | cut -d= -f2)
docker exec -e MYSQL_PWD="$PW" rbac-mysql mysql -uroot rbac_admin -e "SELECT ..."
```

### 用对照实验分离变量，不要靠推断

「后端有问题还是前端发错了」，用 curl 做 A/B 对照一次就能分清。
定位「请选择要上传的图片」时的三组对照：

```bash
# A. 正确 multipart（curl 自动带 boundary）→ 200，证明后端没问题
curl -F "file=@a.png" -H "Authorization: Bearer $TOKEN" ...
# B. 字段名写错 → 400
curl -F "avatar=@a.png" ...
# C. Content-Type 写死 json → 400（与用户现象一致，锁定前端）
curl -H "Content-Type: application/json" --data-binary "@a.png" ...
```

### 「改了没生效」先确认改动真进了容器

前端改动要经 `docker compose build web` 才进镜像。比对 bundle 指纹或 grep 特征串，
**不要假设**：

```bash
docker exec rbac-web sh -c 'grep -oE "assets/index-[A-Za-z0-9_-]+\.js" /usr/share/nginx/html/index.html'
docker exec rbac-web sh -c 'grep -c "某个特征串" /usr/share/nginx/html/assets/index-XXX.js'
```

反向也要警惕：曾怀疑「容器跑的是旧包」，grep 后确认代码**已在包里**，
问题出在代码本身（axios 的 `delete` 无效）。若草率归因为「没部署上」，
重新部署仍会失败，只会更困惑。

**浏览器缓存同理**：改完前端要让用户 Cmd+Shift+R 硬刷新，否则旧 JS 还在跑。

### 报错信息可能与真实原因毫无关联

| 报错 | 真实原因 |
| --- | --- |
| `Cannot read properties of null (reading 'map')` | 角色未授予任何菜单，接口返回 `null` |
| `no such table: casbin_rule` | 表存在，是 `truncate` 语法 sqlite 不支持 |
| `unexpected EOF` | operLog 中间件把请求体截断了 |
| `permission denied`（docker.sock） | 软链指向别的用户家目录，不是权限不足 |
| 「密码错误」 | 种子哈希对应的明文与文档写的不一致 |

**先验证报错字面所指是否真的成立**（表是否存在、文件是否可读），再往上游找。

## 当前完成度

对应设计文档的阶段划分：

| 范围 | 状态 |
| --- | --- |
| 阶段一 基础框架（配置/GORM/日志/统一响应） | ✅ |
| 阶段二 认证鉴权（登录/JWT/中间件/Casbin/前端守卫） | ✅ |
| 阶段三 RBAC CRUD（用户/角色/菜单/部门） | ✅ 前后端均已完成 |
| 阶段四 字典、日志查询、个人中心 | ✅ 已完成 |
| 阶段五 Swagger 生成、单测补全 | Swagger ✅；单测 + Service 层集成测试已补 |

七个模块（用户/角色/菜单/部门/字典/操作日志/登录日志）加个人中心均为
「Controller → Service → Repository」完整分层，`controller.notImplemented` 已删除。
新增模块可参照：`RoleService`（事务 + Casbin 策略刷新）、
`DeptService`（树形 ancestors 重算）、`LogService`（只读 + 批量删除）。

前端 `views/system/` 下已无占位页。约定：
- 列表页复用 `composables/useTable.ts`（查询条件 + 分页 + 加载态 + 删除后回退页码）
- 树形页（菜单/部门）不用 `useTable`——它们一次取全量在内存展开，不分页
- 弹窗拆成同目录下的独立组件（如 `RoleFormModal.vue`），不内联进列表页

### 个人中心的边界
`PUT /auth/profile` 只开放 `nickname`/`email`/`phone`/`gender` 四个字段。
**不要往里加 status/deptId/roleIds**——那些属于管理员职权，
放进这个接口等于让任何登录用户给自己改部门或提权。
操作对象固定取自 Token（`middleware.CurrentUserID`），不接受前端传 ID。

该路由不挂 `RequirePerm`：任何登录用户都该能改自己的昵称。
`/profile` 前端路由固定注册在 `store/permission.ts` 的 layout children 里
（不走 `sys_menu`），`meta.hidden` 让它不出现在侧边栏。

### Swagger 文档

访问 **http://localhost:8088/swagger/index.html**（经 nginx 代理到后端）。
右上角 Authorize 填 `Bearer <token>` 后可直接在页面上调接口。

重新生成：`make swagger`。三个容易踩的点都已写进 Makefile 注释：

1. **swag CLI 版本必须与 `go.mod` 里的 `swaggo/swag` 一致**（当前 v1.16.3）。
   版本不匹配时生成的 `docs.go` 会引用库中不存在的字段（如 `LeftDelim`）而编译失败。
2. **`--parseDependency --parseInternal` 缺一不可**：注解引用了 `internal/` 下的
   vo/dto/response 类型，不加会报 `cannot find type definition`。
3. **各 controller 里有一行 `_ "internal/vo"` 空导入**，专为 swag 解析注解而存在。
   Controller 编译期并不需要 vo（Service 已返回 VO），但 swag 只在**当前文件的
   import 列表**里找类型定义，删掉它整份文档就生成不出来。改动前先跑 `make swagger`。

**生产环境不注册 `/swagger` 路由**（`router.go` 用 `!cfg.App.IsProduction()` 判定）：
这份文档完整列出全部接口与字段含义，暴露等于主动交出一张攻击面地图。
`config` 包有测试锁住这个判定。

nginx 需显式代理 `/swagger/`，否则会被 SPA 的 `try_files` 回退到 `index.html`
——**表现为返回 200 但看到的是前端页面**，状态码正常反而更难发现。

### 测试约定

`go test ./...` 共 225 个用例，分三层：

**纯逻辑单测** —— 不碰数据库，测算法与判定（如 `validateShape`、环检测、
`ChildAncestors` 拼接、`IsProduction`）。

**Service 层集成测试**（`internal/service/*_integration_test.go`）——
用 **sqlite 内存库**跑真实 SQL，从 Service 方法入口进。

**HTTP 层端到端测试**（`internal/router/http_test.go`）——
用 `httptest` + 真实 gin 引擎 + sqlite + **miniredis**，从 HTTP 请求进，
走完 `Router → Middleware → Controller → Service → DB`。
这一层专门覆盖 Service 测试绕过的三处：**中间件、参数绑定、响应封装**，
而它们恰恰是最容易出安全问题的地方。已验证能抓住：

- 漏挂 `RequirePerm`（删掉一个 → `common` 用户能删用户 → 测试报 403 期望但得 200）
- `redactKeys` 漏字段（删掉 `password` → 操作日志出现明文密码 → 测试失败并打印出问题 `request_param`）

之所以必须有真库那两层：本项目已暴露过三个只在真实 SQL 执行时才现形的缺陷，
共同点是「代码读起来正确、接口也返回 200」，单测与 mock 都拦不住：

1. GORM 的 `Save` 默认「完整保存关联」，按 Preload 出来的旧 `Dept`
   把 `dept_id` 覆盖回旧值（改部门不生效）
2. Casbin autosave 撞 `casbin_rule` 唯一索引（后端重启必崩）
3. `browser` 列宽不足导致登录日志写不进库

写测试时注意：

- **断言必须直接查库或看 HTTP 响应体**，不能复用 service 的返回值——
  后者可能返回内存中已改好的实体，恰好掩盖「没落库」这个事实。
- **状态码不等于内容正确**：曾出现 nginx 把 `/swagger` 回退到 SPA，
  返回 200 但内容是 Vue 页面。断言关键路径时要检查响应体。
- 用 `newTestDB(t)` 建库，**逐表迁移**：`sys_menu` 与 `sys_dept` 都有名为
  `idx_parent_id` 的索引，MySQL 按表隔离索引名而 sqlite 要求全库唯一。
- 测试用户 ID 从 11 起：自增会让第一个用户拿到 `id=1`，那正是受保护的超管 ID。
- HTTP 测试**必须带 `Origin` 头**（`testOrigin`），否则被 CORS 拦掉根本走不到业务逻辑。

**回归测试要验证它真能抓住 bug**：写完后临时把修复改回错误写法，
确认测试失败、且失败信息指向根因，再改回来。否则可能写出一条对着
错误代码也通过的「测试」。

sqlite 覆盖不到的部分（**只能靠真实 MySQL 验证**）：

- **`FIND_IN_SET`** 是 MySQL 方言，故 `DeptRepository.FindSubtreeIDs` 与依赖它的
  `DataScopeDeptTree` 过滤未被覆盖。
- **Casbin 策略持久化到 `casbin_rule`**：`gormadapter.SavePolicy` 按驱动名分支
  执行清表语句，而 `glebarez/sqlite` 上报的驱动名是 `"sqlite"`，既不匹配它认识的
  `sqlite.DriverName` 也不匹配 `"sqlite3"`，落到 default 分支发出 `truncate table`
  —— sqlite 不支持，报出的却是极具误导性的「no such table: casbin_rule」。
  故 HTTP 测试改用无适配器的 enforcer，策略由 `seedCasbinPolicies` 灌进内存
  （与 `ReloadPolicies` 的最终效果一致，中间件读到的策略是真实的）。

**已知遗留**：

- **用户导入未实现**。设计文档 §10 要求「用户管理：…导入导出」，
  导出已完成，导入（解析上传文件并批量建号）未做，
  `system:user:import` 权限点已在种子中预留。

### 文件上传的约定

头像上传 `POST /auth/avatar`，文件存本地磁盘（`upload.dir`，Docker 下是
`/app/uploads` + `backend-uploads` 数据卷），静态访问前缀 `upload.urlPrefix`。

**安全约束（`pkg/upload`，改动前务必理解）**：

- **按内容嗅探类型，绝不信扩展名或 Content-Type**。两者都由客户端提供、
  可任意伪造——把 shell.php 改名 avatar.png 就能绕过扩展名检查，
  而文件落在对外静态可访问的目录下，等于一个 RCE 或存储型 XSS 入口。
- **不接受 SVG**。SVG 是 XML，可内嵌 `<script>`，被当图片直接访问时
  会在同源下执行。
- **文件名完全由服务端生成**（16 字节随机 hex）。用户可控的文件名是路径遍历的
  经典入口，随机名从根上消除该面，顺带避免同名覆盖。
- **`upload.Remove` 拒绝任何带路径分隔符的入参**。它的参数来自数据库里的旧头像名，
  不校验就等于把「换头像」变成任意文件删除。
- **换头像时删旧文件**，否则每次更换都留一份孤儿文件，长期运行后目录里
  绝大多数文件都不再被引用且无从判断哪些能清。数据库更新失败时要回滚已落盘的文件。
- **大小上限双重把关**：先按 `file.Size` 挡一道（避免白读大文件），
  再用 `io.LimitReader` 二次限制（`file.Size` 由客户端声明，不可全信）。

**部署要点**：

- **nginx 必须设 `client_max_body_size`**（现为 4m）。默认只有 **1MB**，
  会在请求到达后端之前直接返回 **413**——后端日志里连一条记录都没有，
  排查时极易误判成接口问题（只能从 nginx 日志的
  `client intended to send too large body` 看出来）。
  该值必须**大于**后端的业务上限（头像 2MB），否则真正的错误提示
  「图片不能超过 2MB」永远没机会返回给用户。留 4m 是因为 multipart 编码有额外开销。
- `Dockerfile.backend` 必须**预建 `/app/uploads` 并 chown 给 app 用户**。
  Docker 挂卷时若容器内路径已存在，卷继承其属主；否则挂载点归 root，
  而进程以 app 身份运行，写头像会 permission denied。
- **nginx 必须显式代理 `/uploads/`**，否则被 SPA 的 `try_files` 回退到
  `index.html`——`<img>` 拿到一份 HTML，表现为图片裂开但状态码是 200。
- **axios 实例不设全局 `Content-Type`**。曾写死 `application/json`，
  它会盖掉浏览器为 multipart 自动生成的含 boundary 的头，
  后端切不开表单，报「请选择要上传的图片」。
  在拦截器里删该头**不可靠**——axios v1 的 `AxiosHeaders` 做了键名归一化，
  原生 `delete` 与 `setContentType(null)` 都试过，症状依旧是
  「代码看着删了、请求里 Content-Type 仍是 json」。
  **正解是不设默认值**：axios 会按 `data` 类型自动推断
  （普通对象→json，FormData→multipart+boundary）。
- 上传失败时后端会带上实际收到的 `Content-Type`（见 `UploadAvatar`）。
  只回一句「请选择要上传的图片」时，用户明明选了文件却收到该提示，
  无法分辨是字段名不对还是 multipart 头缺 boundary。

**头像 URL 不鉴权**：要能直接放进 `<img src>`，带 Token 做不到；
文件名随机不可枚举，泄露风险可接受。

### 导出的约定

三个导出接口：`/users/export`、`/oper-logs/export`、`/login-logs/export`，
均为 **CSV**（标准库 `encoding/csv`，零新增依赖，可流式写出）。

- **必须写 UTF-8 BOM**（`pkg/export` 已处理）。缺 BOM 时 Excel 会按本地代码页
  解码，中文全变乱码——而文件内容其实完全正确，从代码上看不出问题。
- **手机号/邮箱在导出里同样脱敏**，与列表用同一套 VO 构造函数。
  导出文件极易外流且流向不可控，不在此开绕过脱敏的后门。
  确需真实值应走数据库导出并配审批流程。
- **有行数上限**（用户 1 万 / 日志 5 万）。超限时**报错并告知实际条数**，
  而不是静默截断——截断会让用户拿到一份「看起来完整」的残缺数据。
  注意导出不能复用 `Service.Page`：`PageQuery.Normalize()` 会把 Size 截到 200，
  那正是一种静默截断。
- **响应头必须在写入 body 之前设置完毕**。一旦开始写 body，
  状态码与头已发出，此后出错无法再改成 JSON 错误响应，
  用户会下载到一个内容截断的文件而非看到错误提示。
- **导出挂 `operLog`**（businessType=4 查询）：一次带走大量数据，
  事后要能回答「谁在什么时候导出了什么条件的数据」。
- 前端用 `utils/request.ts` 的 `download()`，不能走 `request()`
  （后者取 `response.data.data`，而下载响应体是二进制）。
  走 Axios 而非 `window.open` 是为了带上 `Authorization` 头。

**操作日志与文件上传的两处冲突**（`middleware/operlog.go`）：

1. **不能读走 multipart 的请求体**。中间件为记日志会用 `LimitReader` 只读前
   4KB 再把 `Body` 替换成这 4KB——对一张 1.8MB 的图片等于把请求截断，
   后端解析时报 **`unexpected EOF`**，报错位置离真正原因很远。
   故 `isMultipart` 的请求整体跳过读取，改用 `describeUpload` 记文件名与大小
   （审计仍能回答「谁上传了什么文件」）。
2. **只捕获 JSON 响应体**（`captureBody`）。文件下载的响应体就是数据本身，
   记进日志表等于把导出内容又存一份副本。

**测试用例的体积要跨过阈值**：头像用例最初只传几个字节的图片，
远低于 4KB 截断线，因此完全没覆盖到第 1 条——这个 bug 是用户报上来的，
测试全绿却漏掉了。现有 `TestAvatarUploadSurvivesOperLogMiddleware` 用 64KB
图片守住这条路径（已验证：去掉修复即失败并复现 `unexpected EOF`）。

## 验证状态

已验证通过：

- **后端**：`go build ./...`、`go vet ./...` 均无错误；`gofmt` 干净
- **单元测试**（`go test ./...` 全绿，225 个用例）：
  - `pkg/crypto` 9 项：加解密往返、GCM 随机性、篡改检测、盲索引确定性、脱敏
  - `pkg/treeutil`：空输入返回 `[]` 而非 `null`、建树层级、孤儿丢弃、环不死循环
  - `config`：`IsProduction` 判定（Swagger 暴露开关的依据）、环境简写归一
  - `internal/service` 纯逻辑：种子密码哈希与文档一致、bcrypt salt 生效、
    角色数据范围校验、超管识别、菜单类型字段组合、菜单/部门环检测、
    字典标识字符集、默认项互斥
  - `internal/service` **集成测试（真实 SQL）**：
    **改部门真的落库**（回归已修 bug，已验证能抓住）、留空不覆盖手机号、
    手机号密文与盲索引往返、用户软删除、超管保护、
    **移动部门后孙级 ancestors 同步重算**、禁止挂到后代下、
    删除部门的子部门/用户双重拦截、同级重名、
    菜单授权幂等与清空、删除菜单清理关联行、perms 唯一、按钮不能作父级
  - `pkg/upload`：**PHP/HTML/SVG/ELF/纯文本伪装成 .png 全部被拒**（按内容嗅探）、
    合法图片按真实类型定扩展名、文件名不含用户输入、连续上传不重名、
    超限与空文件被拒且不留残留、`Remove` 拒绝路径穿越且幂等
  - `internal/router` **HTTP 层端到端**（真实 gin + sqlite + miniredis）：
    **36 个接口无 Token 全部 401**（漏挂中间件会在此暴露）、伪造 Token 被拒、
    **按权限点逐个判定 403**（已验证删掉一个 RequirePerm 就失败）、
    超管绕过生效、CORS 白名单拦截与回显、**登出后 Token 立即失效**、
    **操作日志密码脱敏为 `***`、手机号邮箱打码**（已验证删掉 redactKeys 条目就失败）、
    响应体不泄露完整手机号/邮箱/密码哈希、参数校验 400、未知路由统一响应、
    healthz 公开、非生产环境暴露 Swagger
  - `internal/router` **导出**：**导出文件里手机号/邮箱同样脱敏**、
    三个导出都带 UTF-8 BOM、**导出权限独立于列表权限**（有 list 无 export 应 403）、
    未登录 401、`/export` 未被 `/:id` 吃掉、导出遵循查询筛选、
    **导出动作记入操作日志且不把 CSV 内容存进 json_result**（该用例发现了真实缺陷）
  - `internal/router` **头像上传**：伪装文件经完整 HTTP 链路仍被拒且不落盘、
    上传成功后落盘并写回数据库、**换头像删除旧文件**、未登录不能写盘、
    上传后的 URL 无需鉴权即可访问、**静态路由挡住路径穿越**（含 URL 编码变体）
- **Docker 环境端到端可用**：四容器 healthy，`/healthz` 与前端均 200，
  后端在库中已有策略的情况下重启不再报 `1062`
- **路由注册**：全部写接口返回 401（而非 404），证明路由已挂且鉴权生效；
  易冲突的 `/roles/all` vs `/:id`、`/dicts/data/type/:type` vs `/data/:id`、
  `/oper-logs/clean` vs `/:id` 均已确认未被吞掉（gin 路由冲突会启动即 panic）
- **权限点三处同步**：`router.go` 的 perm 常量全部存在于种子 SQL 与
  `perms.ts`，无拼写差异；不存在「路由要求但无法授予」的权限。
  （补充文档时用该比对命令发现 `system:user:export` 漏了种子记录，已补 id=1007。）
- **菜单组件路径**：种子里 7 个 `sys_menu.component` 均对应真实存在的 `.vue` 文件
- **前端**：`pnpm --filter @workbackend/web build` 通过（含 `vue-tsc`）

未验证：

- **登录的完整 HTTP 流程含验证码**。HTTP 层测试关掉了验证码（它属独立关注点），
  Token 直接签发而非走登录接口。命令行验证也受阻：验证码在密码校验之前，
  取 token 需从 Redis 读明文（testing 环境用 **Redis db 1**，
  键名 `auth:captcha:{id}`），而 `docker exec` 进 MySQL/Redis
  频繁被本机权限策略拦截。
- **`DataScopeDeptTree` 的子树过滤**与 **Casbin 策略持久化**：
  依赖 MySQL 方言，sqlite 覆盖不到（原因见「测试约定」）。
- 日志的批量删除与清空未走 HTTP 测试（列表与鉴权已覆盖）。

### 前端构建的一个坑

**本地 `pnpm build` / `vue-tsc` 会给出缓存结果**，已两次出现「本地通过、
Docker 构建报 TS 错误」的情况（`a-tree` 的 `DataNode` 缺 `key`、
`a-range-picker` 的 `unknown[]` 不匹配）。

改完前端后要么先 `rm -rf apps/web/node_modules/.vite apps/web/dist` 再构建，
要么直接跑 `docker compose build web` 验证——后者才是真实的门禁。


