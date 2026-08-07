package router_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
)

/*
 * 鉴权边界。
 *
 * CLAUDE.md 的第一条关键约束是「前端权限控制不是安全边界，每个写接口都必须挂
 * RequirePerm」。漏挂一个中间件等于开一个未授权入口，而这种缺陷在前端界面上
 * 完全看不出来——按钮藏着，接口却是敞开的。
 * 这组用例逐个接口验证「无 Token 拒绝、有 Token 但无权限拒绝」。
 */

// TestUnauthenticatedRequestsAreRejected 验证不带 Token 时全部写接口被拒。
func TestUnauthenticatedRequestsAreRejected(t *testing.T) {
	env := newTestEnv(t)

	// 覆盖每个模块的代表性写接口；漏挂中间件的接口会在这里返回 2xx。
	endpoints := []struct{ method, path string }{
		{"GET", "/users"},
		{"POST", "/users"},
		{"PUT", "/users/11"},
		{"DELETE", "/users/11"},
		{"PUT", "/users/11/password"},
		{"PUT", "/users/11/roles"},
		{"GET", "/roles"},
		{"POST", "/roles"},
		{"PUT", "/roles/2"},
		{"DELETE", "/roles/2"},
		{"PUT", "/roles/2/menus"},
		{"PUT", "/roles/2/data-scope"},
		{"GET", "/menus/tree"},
		{"POST", "/menus"},
		{"PUT", "/menus/100"},
		{"DELETE", "/menus/100"},
		{"GET", "/depts/tree"},
		{"POST", "/depts"},
		{"PUT", "/depts/2"},
		{"DELETE", "/depts/2"},
		{"GET", "/dicts/types"},
		{"POST", "/dicts/types"},
		{"PUT", "/dicts/types/1"},
		{"DELETE", "/dicts/types/1"},
		{"GET", "/dicts/data"},
		{"POST", "/dicts/data"},
		{"GET", "/oper-logs"},
		{"DELETE", "/oper-logs"},
		{"DELETE", "/oper-logs/clean"},
		{"GET", "/login-logs"},
		{"DELETE", "/login-logs"},
		{"DELETE", "/login-logs/clean"},
		{"GET", "/auth/info"},
		{"GET", "/auth/menus"},
		{"PUT", "/auth/password"},
		{"PUT", "/auth/profile"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			status, resp := env.do(t, ep.method, ep.path, "", nil)
			if status != http.StatusUnauthorized {
				t.Errorf("无 Token 访问 %s %s 返回 %d（期望 401），"+
					"该接口可能漏挂了 JWTAuth 中间件", ep.method, ep.path, status)
			}
			if resp.Code != http.StatusUnauthorized {
				t.Errorf("响应体 code=%d，期望 401", resp.Code)
			}
		})
	}
}

// TestInvalidTokenIsRejected 验证伪造与格式错误的 Token 被拒。
func TestInvalidTokenIsRejected(t *testing.T) {
	env := newTestEnv(t)

	cases := map[string]string{
		"完全伪造":     "not-a-jwt-at-all",
		"结构像但签名错":  "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VySWQiOjF9.bad-signature",
		"空字符串当作有头": " ",
	}

	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			status, _ := env.do(t, "GET", "/users", token, nil)
			if status != http.StatusUnauthorized {
				t.Errorf("非法 Token 返回 %d，期望 401", status)
			}
		})
	}
}

/*
 * TestPermissionEnforcedPerEndpoint 是这一层最核心的用例。
 *
 * common 角色只被授予 system:user:list 与 system:user:add，
 * 故它能查列表、能新增，但删除/改密码/角色管理等一律应 403。
 * 这验证的是 RequirePerm 中间件真的按 Casbin 策略逐点判定，
 * 而不是「登录了就放行」。
 */
func TestPermissionEnforcedPerEndpoint(t *testing.T) {
	env := newTestEnv(t)
	token := env.commonToken(t)

	allowed := []struct{ method, path string }{
		{"GET", "/users"}, // system:user:list
	}
	for _, ep := range allowed {
		t.Run("允许 "+ep.method+" "+ep.path, func(t *testing.T) {
			status, _ := env.do(t, ep.method, ep.path, token, nil)
			if status != http.StatusOK {
				t.Errorf("已授权的接口返回 %d，期望 200", status)
			}
		})
	}

	denied := []struct{ method, path string }{
		{"DELETE", "/users/11"},       // 未授予 system:user:remove
		{"PUT", "/users/11/password"}, // 未授予 system:user:resetPwd
		{"GET", "/roles"},             // 未授予 system:role:list
		{"POST", "/roles"},            // 未授予 system:role:add
		{"GET", "/menus/tree"},        // 未授予 system:menu:list
		{"GET", "/dicts/types"},       // 未授予 system:dict:list
		{"GET", "/oper-logs"},         // 未授予 system:operlog:list
		{"GET", "/login-logs"},        // 未授予 system:loginlog:list
	}
	for _, ep := range denied {
		t.Run("拒绝 "+ep.method+" "+ep.path, func(t *testing.T) {
			status, _ := env.do(t, ep.method, ep.path, token, nil)
			if status != http.StatusForbidden {
				t.Errorf("未授权的接口返回 %d（期望 403），"+
					"权限点 %s 可能被错误放行", status, ep.path)
			}
		})
	}
}

// TestSuperAdminBypassesPermissionCheck 验证超管绕过权限校验。
//
// 设计文档 §4.3：code = "admin" 的角色在鉴权中间件中直接放行，
// 目的是避免管理员把自己锁死。这条捷径必须真的生效，
// 否则一旦误删 admin 角色的某个授权就再也改不回来。
func TestSuperAdminBypassesPermissionCheck(t *testing.T) {
	env := newTestEnv(t)
	token := env.adminToken(t)

	// admin 角色在种子里没有被授予任何 sys_role_menu，
	// 全靠中间件的超管放行——正好验证这条捷径。
	endpoints := []struct{ method, path string }{
		{"GET", "/users"},
		{"GET", "/roles"},
		{"GET", "/menus/tree"},
		{"GET", "/depts/tree"},
		{"GET", "/dicts/types"},
		{"GET", "/oper-logs"},
		{"GET", "/login-logs"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			status, resp := env.do(t, ep.method, ep.path, token, nil)
			if status != http.StatusOK {
				t.Errorf("超管访问 %s 返回 %d（期望 200），超管放行失效", ep.path, status)
			}
			if resp.Code != http.StatusOK {
				t.Errorf("响应体 code=%d，期望 200", resp.Code)
			}
		})
	}
}

// TestCORSRejectsUnknownOrigin 验证白名单外的来源被拦。
//
// 这条曾是一个真实故障：用 production 配置跑本地，浏览器发的
// Origin 是 http://localhost:8088 而白名单里只有真实域名，
// 请求被拦成 403 且响应体为空，极难判断原因。
func TestCORSRejectsUnknownOrigin(t *testing.T) {
	env := newTestEnv(t)
	token := env.adminToken(t)

	req := newRequest(t, "GET", "/api/v1/users", token, nil)
	req.Header.Set("Origin", "http://evil.example.com")
	rec := serve(env, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("白名单外的 Origin 返回 %d，期望 403", rec.Code)
	}

	// 白名单内的来源应正常放行，并回显 Allow-Origin 头。
	req2 := newRequest(t, "GET", "/api/v1/users", token, nil)
	req2.Header.Set("Origin", testOrigin)
	rec2 := serve(env, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("白名单内的 Origin 返回 %d，期望 200", rec2.Code)
	}
	if got := rec2.Header().Get("Access-Control-Allow-Origin"); got != testOrigin {
		t.Errorf("Access-Control-Allow-Origin = %q，期望 %q", got, testOrigin)
	}
}

// TestLogoutBlacklistsToken 验证登出后原 Token 立即失效。
//
// 登出把 jti 写入 Redis 黑名单，JWTAuth 每次请求都要查。
// 若这一步失效，「退出登录」只是前端清了本地存储，
// 泄露的 Token 在有效期内仍可畅通无阻。
func TestLogoutBlacklistsToken(t *testing.T) {
	env := newTestEnv(t)
	token := env.adminToken(t)

	// 登出前可用
	if status, _ := env.do(t, "GET", "/auth/info", token, nil); status != http.StatusOK {
		t.Fatalf("登出前访问 /auth/info 返回 %d", status)
	}

	if status, _ := env.do(t, "POST", "/auth/logout", token, nil); status != http.StatusOK {
		t.Fatalf("登出失败，状态码 %d", status)
	}

	// 登出后同一个 Token 应被拒绝
	status, _ := env.do(t, "GET", "/auth/info", token, nil)
	if status != http.StatusUnauthorized {
		t.Errorf("登出后原 Token 仍可用（返回 %d），Token 黑名单未生效——"+
			"泄露的 Token 在有效期内将无法吊销", status)
	}
}

/*
 * TestOperLogRedactsPassword 验证操作日志把密码脱敏后再入库。
 *
 * CLAUDE.md 第 3 条：新增敏感入参字段时要同步 middleware/operlog.go 的
 * redactKeys，否则明文入库。这条约束只有走完整 HTTP 链路才能验证——
 * 中间件读的是原始请求体，Service 层测试根本不经过它。
 */
func TestOperLogRedactsPassword(t *testing.T) {
	env := newTestEnv(t)
	token := env.adminToken(t)

	const rawPassword = "PlainTextSecret123"
	body := map[string]any{
		"username": "newbie",
		"password": rawPassword,
		"phone":    "13800001111",
		"email":    "newbie@example.com",
		"deptId":   2,
	}
	status, resp := env.do(t, "POST", "/users", token, body)
	if status != http.StatusOK {
		t.Fatalf("新增用户失败：%d %s", status, resp.Message)
	}

	var logs []model.OperLog
	if err := env.db.Order("id DESC").Find(&logs).Error; err != nil {
		t.Fatalf("读取操作日志失败: %v", err)
	}
	if len(logs) == 0 {
		t.Fatal("未写入操作日志，operLog 中间件可能没生效")
	}

	param := logs[0].RequestParam
	if strings.Contains(param, rawPassword) {
		t.Errorf("操作日志里出现了明文密码！\nrequest_param = %s\n"+
			"检查 middleware/operlog.go 的 redactKeys", param)
	}
	if !strings.Contains(param, "***") {
		t.Errorf("密码字段未被替换为 ***：%s", param)
	}
	// 手机号与邮箱应被打码而非原样留存。
	if strings.Contains(param, "13800001111") {
		t.Errorf("操作日志里出现了完整手机号：%s", param)
	}
	if strings.Contains(param, "newbie@example.com") {
		t.Errorf("操作日志里出现了完整邮箱：%s", param)
	}
}

// TestUserListResponseMasksSensitiveFields 验证列表响应里的手机号/邮箱已脱敏。
//
// 脱敏在 VO 构造函数里做（vo.NewUserItem）。这条从 HTTP 响应体断言，
// 确保没有哪一层把原始实体直接序列化出去。
func TestUserListResponseMasksSensitiveFields(t *testing.T) {
	env := newTestEnv(t)
	token := env.adminToken(t)

	// 先建一个带手机号邮箱的用户
	body := map[string]any{
		"username": "masked",
		"password": "123456",
		"phone":    "13912340000",
		"email":    "masked@example.com",
		"deptId":   2,
	}
	if status, resp := env.do(t, "POST", "/users", token, body); status != http.StatusOK {
		t.Fatalf("新增用户失败：%d %s", status, resp.Message)
	}

	status, resp := env.do(t, "GET", "/users", token, nil)
	if status != http.StatusOK {
		t.Fatalf("查询用户列表失败：%d", status)
	}

	raw := string(resp.Data)
	if strings.Contains(raw, "13912340000") {
		t.Errorf("响应体包含完整手机号，VO 脱敏未生效：%s", raw)
	}
	if strings.Contains(raw, "masked@example.com") {
		t.Errorf("响应体包含完整邮箱，VO 脱敏未生效：%s", raw)
	}
	// 密码哈希永远不该出现在响应里。
	if strings.Contains(raw, "$2a$") {
		t.Errorf("响应体泄露了密码哈希：%s", raw)
	}
}

// TestBadRequestBodyReturns400 验证参数校验生效且不泄露内部细节。
func TestBadRequestBodyReturns400(t *testing.T) {
	env := newTestEnv(t)
	token := env.adminToken(t)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"缺少必填的用户名", map[string]any{"password": "123456"}},
		{"密码过短", map[string]any{"username": "shortpwd", "password": "1"}},
		{"邮箱格式错误", map[string]any{"username": "bademail", "password": "123456", "email": "not-an-email"}},
		{"手机号位数不对", map[string]any{"username": "badphone", "password": "123456", "phone": "123"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, resp := env.do(t, "POST", "/users", token, tc.body)
			if status != http.StatusBadRequest {
				t.Errorf("非法参数返回 %d，期望 400", status)
			}
			// 不应把 Go 结构体名等内部细节直接抛给调用方之外的信息，
			// 但需要有可读的中文提示。
			if resp.Message == "" {
				t.Error("400 响应缺少提示信息")
			}
		})
	}
}

// TestNotFoundRouteReturnsUnifiedBody 验证未知路由走统一响应而非 gin 默认 404。
func TestNotFoundRouteReturnsUnifiedBody(t *testing.T) {
	env := newTestEnv(t)

	status, resp := env.do(t, "GET", "/no-such-endpoint", env.adminToken(t), nil)
	if status != http.StatusNotFound {
		t.Errorf("未知路由返回 %d，期望 404", status)
	}
	if resp.Message != "接口不存在" {
		t.Errorf("未知路由的提示为 %q，期望统一响应的「接口不存在」", resp.Message)
	}
}

// TestHealthzIsPublic 验证健康检查不需要鉴权（供容器探针使用）。
func TestHealthzIsPublic(t *testing.T) {
	env := newTestEnv(t)

	req := newRequest(t, "GET", "/healthz", "", nil)
	rec := serve(env, req)
	if rec.Code != http.StatusOK {
		t.Errorf("/healthz 返回 %d，期望 200——容器探针会因此判定服务不健康", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("healthz 返回 %v，期望 status=ok", body)
	}
}

// TestSwaggerExposedInNonProduction 验证非生产环境注册了 Swagger 路由。
//
// 与之对应的「生产不暴露」由 config 包的测试锁住判定条件；
// 这里确认非生产时确实可访问，否则那个开关就成了永远关闭。
func TestSwaggerExposedInNonProduction(t *testing.T) {
	env := newTestEnv(t)

	req := newRequest(t, "GET", "/swagger/index.html", "", nil)
	rec := serve(env, req)
	// Swagger UI 会返回 200 或重定向；只要不是 404 就说明路由已注册。
	if rec.Code == http.StatusNotFound {
		t.Error("非生产环境未注册 /swagger 路由")
	}
}
