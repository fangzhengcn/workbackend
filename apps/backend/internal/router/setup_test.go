// Package router_test 用 httptest 打完整 HTTP 链路。
//
// 与 service 层集成测试的区别：那一层从 Service 方法入口进，
// 绕过了中间件、参数绑定与响应封装——而这三处恰恰是最容易出安全问题的地方
// （忘挂 RequirePerm 等于开一个未授权入口，操作日志漏脱敏等于明文泄露）。
// 这里从真实的 HTTP 请求进，走完 Router → Middleware → Controller → Service → DB
// 的全链路，断言的是「外部调用者实际看到什么」。
package router_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
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

const (
	testAESKey   = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	testHMACKey  = "ffeeddccbbaa99887766554433221100ffeeddccbbaa99887766554433221100"
	testJWTKey   = "test-jwt-secret-for-http-layer-tests"
	testOrigin   = "http://localhost:8088"
	adminUser    = "admin"
	commonUser   = "fz"
	testPassword = "123456"
)

// testEnv 是一次 HTTP 测试所需的全部上下文。
type testEnv struct {
	engine *gin.Engine
	db     *gorm.DB
	jwt    *jwt.Manager
	logs   *repository.LogRepository
	// uploadDir 供头像用例断言文件是否真的落盘/被清理。
	uploadDir string
}

// newTestEnv 组装一个可发真实 HTTP 请求的服务端。
//
// 依赖全部指向进程内实现：sqlite 内存库 + miniredis，
// 因此无需任何外部服务即可运行（含 CI）。
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	gin.SetMode(gin.TestMode)
	// 中间件里的 logger.Warnf 等调用需要已初始化的 logger，否则会 nil panic。
	if err := logger.Init(logger.Options{Level: "panic", Format: "text"}); err != nil {
		t.Fatalf("初始化日志失败: %v", err)
	}

	cipher, err := crypto.NewCipher(testAESKey, testHMACKey, 1)
	if err != nil {
		t.Fatalf("构造加密器失败: %v", err)
	}
	model.SetCipher(cipher)

	db := newTestDB(t)
	seedFixtures(t, db)

	// miniredis 提供真实的 Redis 协议实现：JWT 中间件要查 Token 黑名单与
	// 强制下线标记，这些是安全检查，不能用「nil 就放行」的空实现糊过去。
	mr := miniredis.RunT(t)
	cacheClient, err := cache.New(context.Background(), cache.Options{Addr: mr.Addr()})
	if err != nil {
		t.Fatalf("连接 miniredis 失败: %v", err)
	}
	t.Cleanup(func() { _ = cacheClient.Close() })

	users := repository.NewUserRepository(db)
	roles := repository.NewRoleRepository(db)
	menus := repository.NewMenuRepository(db)
	depts := repository.NewDeptRepository(db)
	logs := repository.NewLogRepository(db)
	dicts := repository.NewDictRepository(db)

	enforcer, err := newTestEnforcer(t, db)
	if err != nil {
		t.Fatalf("初始化 Casbin 失败: %v", err)
	}
	permissions := service.NewPermissionService(enforcer, roles)
	seedCasbinPolicies(t, enforcer, db)

	jwtManager := jwt.NewManager(testJWTKey, "rbac-admin-test", 2, 168)
	// 验证码关闭：本文件测的是鉴权与业务链路，验证码属于独立关注点，
	// 开启会让每个用例都要先取码，噪音大于收益。
	captchas := service.NewCaptchaService(cacheClient, false)
	dataScope := service.NewDataScopeService(roles, depts)

	// 上传目录用 t.TempDir()：测试结束自动清理，
	// 且各用例互不干扰（否则头像文件会在仓库里堆积）。
	uploadCfg := config.UploadConfig{Dir: t.TempDir(), URLPrefix: "/uploads"}

	deps := &router.Dependencies{
		Config: &config.Config{
			App:    config.AppConfig{Env: config.EnvTesting, Mode: gin.TestMode},
			CORS:   config.CORSConfig{AllowOrigins: []string{testOrigin}},
			Upload: uploadCfg,
		},
		JWT:         jwtManager,
		Cache:       cacheClient,
		Permissions: permissions,
		Logs:        logs,

		Auth: controller.NewAuthController(
			service.NewAuthService(users, roles, menus, logs, jwtManager, cacheClient, captchas),
			captchas,
		),
		User: controller.NewUserController(service.NewUserService(users, roles, dataScope, cacheClient), uploadCfg),
		Role: controller.NewRoleController(service.NewRoleService(roles, menus, permissions)),
		Menu: controller.NewMenuController(service.NewMenuService(menus, permissions)),
		Dept: controller.NewDeptController(service.NewDeptService(depts)),
		Dict: controller.NewDictController(service.NewDictService(dicts)),
		Log:  controller.NewLogController(service.NewLogService(logs)),
	}

	return &testEnv{
		engine:    router.Setup(deps),
		db:        db,
		jwt:       jwtManager,
		logs:      logs,
		uploadDir: uploadCfg.Dir,
	}
}

// newTestEnforcer 用内存模型 + 无持久化适配器构造 enforcer。
//
// 不接 gormadapter：它的 SavePolicy 会先按驱动名分支执行清表语句，
// 而 glebarez/sqlite 上报的驱动名是 "sqlite"，既不匹配它认识的
// sqlite.DriverName 也不匹配 "sqlite3"，于是落到 default 分支发出
// `truncate table` —— sqlite 不支持该语法，报出的却是极具误导性的
// 「no such table: casbin_rule」。这是库与驱动的兼容问题，不该在测试里绕。
//
// 改用无适配器的 enforcer：策略由 seedCasbinPolicies 按 sys_role_menu
// 直接灌进内存，这与 PermissionService.ReloadPolicies 的最终效果一致
// （它也是先 ClearPolicy 再逐条 AddPolicy），中间件读到的策略因此是真实的。
// 代价是覆盖不到「策略持久化到 casbin_rule」这一段，已在 CLAUDE.md 标注。
func newTestEnforcer(t *testing.T, _ *gorm.DB) (*casbin.Enforcer, error) {
	t.Helper()

	const modelText = `
[request_definition]
r = sub, obj, act
[policy_definition]
p = sub, obj, act
[policy_effect]
e = some(where (p.eft == allow))
[matchers]
m = r.sub == p.sub && r.obj == p.obj && (p.act == "*" || r.act == p.act)
`
	path := t.TempDir() + "/rbac_model.conf"
	if err := os.WriteFile(path, []byte(modelText), 0o600); err != nil {
		return nil, fmt.Errorf("写入模型文件失败: %w", err)
	}
	return casbin.NewEnforcer(path)
}

// seedCasbinPolicies 依据 sys_role_menu 把策略灌进 enforcer。
//
// 逻辑与 RoleRepository.FindRoleMenuPerms + ReloadPolicies 等价：
// 「角色标识, 权限标识, *」。
func seedCasbinPolicies(t *testing.T, enforcer *casbin.Enforcer, db *gorm.DB) {
	t.Helper()

	type row struct {
		Code  string
		Perms string
	}
	var rows []row
	err := db.Table("sys_role_menu AS rm").
		Select("r.code AS code, m.perms AS perms").
		Joins("JOIN sys_role r ON r.id = rm.role_id").
		Joins("JOIN sys_menu m ON m.id = rm.menu_id").
		Where("m.perms IS NOT NULL AND m.perms <> ''").
		Scan(&rows).Error
	if err != nil {
		t.Fatalf("查询角色权限失败: %v", err)
	}

	for _, r := range rows {
		if _, err := enforcer.AddPolicy(r.Code, r.Perms, "*"); err != nil {
			t.Fatalf("添加策略失败 (%s, %s): %v", r.Code, r.Perms, err)
		}
	}
}

// newTestDB 建迁移完毕的内存库。
//
// 逐表迁移的原因与 service 层测试相同：sys_menu 与 sys_dept 都有
// idx_parent_id，sqlite 要求索引名全库唯一。
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}

	models := []any{
		&model.User{}, &model.Role{}, &model.Menu{}, &model.Dept{},
		&model.UserRole{}, &model.RoleMenu{}, &model.RoleDept{},
		&model.DictType{}, &model.DictData{},
		&model.OperLog{}, &model.LoginLog{},
	}
	for _, m := range models {
		if err := db.AutoMigrate(m); err != nil && !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("迁移 %T 失败: %v", m, err)
		}
	}
	return db
}

/*
 * seedFixtures 写入贴近建表脚本的最小数据集：
 *
 *   部门 1 总公司 → 2 研发部
 *   角色 1 admin（超管）、2 common（仅授予「用户管理」查看与新增）
 *   菜单 1 系统管理 → 100 用户管理 → 1002 用户新增按钮
 *   用户 1 admin（admin 角色）、11 fz（common 角色）
 */
func seedFixtures(t *testing.T, db *gorm.DB) {
	t.Helper()

	if err := db.Create(&[]*model.Dept{
		{ID: 1, ParentID: 0, Ancestors: "0", Name: "总公司", Status: model.StatusEnabled},
		{ID: 2, ParentID: 1, Ancestors: "0,1", Name: "研发部", Status: model.StatusEnabled},
	}).Error; err != nil {
		t.Fatalf("写入部门失败: %v", err)
	}

	if err := db.Create(&[]*model.Role{
		{ID: 1, Name: "超级管理员", Code: model.SuperAdminRoleCode, DataScope: model.DataScopeAll, Status: model.StatusEnabled},
		{ID: 2, Name: "普通角色", Code: "common", DataScope: model.DataScopeAll, Status: model.StatusEnabled},
	}).Error; err != nil {
		t.Fatalf("写入角色失败: %v", err)
	}

	if err := db.Create(&[]*model.Menu{
		{ID: 1, ParentID: 0, Name: "系统管理", Type: model.MenuTypeDir, Path: "/system", Status: model.StatusEnabled, Visible: model.StatusEnabled},
		{ID: 100, ParentID: 1, Name: "用户管理", Type: model.MenuTypeMenu, Path: "user", Component: "system/user/index", Perms: "system:user:list", Status: model.StatusEnabled, Visible: model.StatusEnabled},
		{ID: 1002, ParentID: 100, Name: "用户新增", Type: model.MenuTypeButton, Perms: "system:user:add", Status: model.StatusEnabled, Visible: model.StatusEnabled},
	}).Error; err != nil {
		t.Fatalf("写入菜单失败: %v", err)
	}

	// common 角色只拿到「查看用户列表」与「新增用户」，
	// 故意不给 system:user:remove——用于验证 403。
	if err := db.Create(&[]*model.RoleMenu{
		{RoleID: 2, MenuID: 1}, {RoleID: 2, MenuID: 100}, {RoleID: 2, MenuID: 1002},
	}).Error; err != nil {
		t.Fatalf("写入角色菜单失败: %v", err)
	}

	hashed, err := service.HashPassword(testPassword)
	if err != nil {
		t.Fatalf("生成密码哈希失败: %v", err)
	}
	deptID := uint64(2)
	if err := db.Create(&[]*model.User{
		{ID: 1, Username: adminUser, Password: hashed, Nickname: model.EncryptedString("超管"), DeptID: &deptID, Status: model.StatusEnabled},
		{ID: 11, Username: commonUser, Password: hashed, Nickname: model.EncryptedString("普通"), DeptID: &deptID, Status: model.StatusEnabled},
	}).Error; err != nil {
		t.Fatalf("写入用户失败: %v", err)
	}

	if err := db.Create(&[]*model.UserRole{
		{UserID: 1, RoleID: 1}, {UserID: 11, RoleID: 2},
	}).Error; err != nil {
		t.Fatalf("写入用户角色失败: %v", err)
	}
}

// newRequest 构造一个请求；path 需含完整前缀（如 /api/v1/users 或 /healthz）。
//
// 与 do 的分工：do 面向「调 /api/v1 下的业务接口」这一主流场景并解析响应体；
// 需要自定义头（如换 Origin）或访问非 /api/v1 路径时用这两个更底层的函数。
func newRequest(t *testing.T, method, path, token string, body any) *http.Request {
	t.Helper()

	payload := ""
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("序列化请求体失败: %v", err)
		}
		payload = string(raw)
	}

	req := httptest.NewRequest(method, path, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

// serve 把请求送进引擎并返回记录器。
func serve(e *testEnv, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.engine.ServeHTTP(rec, req)
	return rec
}

// ---- 请求辅助 ----

// apiResp 是统一响应结构，data 延迟解析以适配不同接口。
type apiResp struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// do 发一个请求并返回状态码与解析后的响应体。
//
// token 为空时不带 Authorization 头，用于验证未登录拦截。
func (e *testEnv) do(t *testing.T, method, path, token string, body any) (int, apiResp) {
	t.Helper()

	var reader *strings.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("序列化请求体失败: %v", err)
		}
		reader = strings.NewReader(string(raw))
	} else {
		reader = strings.NewReader("")
	}

	req := httptest.NewRequest(method, "/api/v1"+path, reader)
	req.Header.Set("Content-Type", "application/json")
	// 带上 Origin：CORS 中间件对白名单外的来源会直接 403，
	// 不带则走不到业务逻辑（这本身也是一个曾经踩过的坑）。
	req.Header.Set("Origin", testOrigin)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	e.engine.ServeHTTP(rec, req)

	var parsed apiResp
	if rec.Body.Len() > 0 {
		// 忽略解析错误：部分响应（如 CORS 拦截）没有 JSON 体，
		// 此时调用方只关心状态码。
		_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	}
	return rec.Code, parsed
}

// tokenFor 直接签发 Token，跳过登录流程。
//
// 登录本身另有用例覆盖；此处只为拿到一个可用凭证，
// 走登录会让每个用例都耦合验证码与密码校验。
func (e *testEnv) tokenFor(t *testing.T, userID uint64, username string, roles []string) string {
	t.Helper()

	token, _, err := e.jwt.GenerateAccess(userID, username, 2, roles, uuid.NewString())
	if err != nil {
		t.Fatalf("签发 Token 失败: %v", err)
	}
	return token
}

func (e *testEnv) adminToken(t *testing.T) string {
	return e.tokenFor(t, 1, adminUser, []string{model.SuperAdminRoleCode})
}

func (e *testEnv) commonToken(t *testing.T) string {
	return e.tokenFor(t, 11, commonUser, []string{"common"})
}
