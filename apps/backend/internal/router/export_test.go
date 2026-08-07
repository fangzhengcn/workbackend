package router_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
)

/*
 * 导出功能的 HTTP 层测试。
 *
 * 导出是「一次把大量数据带出系统」的操作，风险点与普通接口不同：
 * 脱敏是否在文件里也生效、权限是否独立于列表权限、是否会被 SPA 之外的路径
 * 意外放行。这些只能从真实响应体断言。
 */

// exportRequest 发一个导出请求并返回状态码与响应体原文。
//
// 不复用 do()：导出返回的是 CSV 而非统一 JSON 结构，
// 用 apiResp 解析会失败并丢掉真正要断言的内容。
func exportRequest(t *testing.T, env *testEnv, path, token string) (int, string, http.Header) {
	t.Helper()

	req := newRequest(t, "GET", "/api/v1"+path, token, nil)
	req.Header.Set("Origin", testOrigin)
	rec := serve(env, req)
	return rec.Code, rec.Body.String(), rec.Header()
}

// TestUserExportMasksSensitiveFields 验证导出文件里的手机号/邮箱已脱敏。
//
// 这是导出功能最关键的一条：导出文件极易外流且流向不可控，
// 若这里返回明文，一个导出权限就等于拿走全量真实联系方式，
// 前面所有的 AES 加密与 VO 脱敏都被绕过。
func TestUserExportMasksSensitiveFields(t *testing.T) {
	env := newTestEnv(t)
	token := env.adminToken(t)

	const rawPhone, rawEmail = "13912340000", "secret@example.com"
	body := map[string]any{
		"username": "exported",
		"password": "123456",
		"phone":    rawPhone,
		"email":    rawEmail,
		"deptId":   2,
	}
	if status, resp := env.do(t, "POST", "/users", token, body); status != http.StatusOK {
		t.Fatalf("新增用户失败：%d %s", status, resp.Message)
	}

	status, csvBody, header := exportRequest(t, env, "/users/export", token)
	if status != http.StatusOK {
		t.Fatalf("导出失败：%d body=%s", status, csvBody)
	}

	if strings.Contains(csvBody, rawPhone) {
		t.Errorf("导出文件包含完整手机号，脱敏被绕过：\n%s", csvBody)
	}
	if strings.Contains(csvBody, rawEmail) {
		t.Errorf("导出文件包含完整邮箱，脱敏被绕过：\n%s", csvBody)
	}
	// 密码哈希绝不该出现在导出里。
	if strings.Contains(csvBody, "$2a$") {
		t.Errorf("导出文件泄露了密码哈希：\n%s", csvBody)
	}
	// 但该用户本身要在文件里，否则等于导出了空数据。
	if !strings.Contains(csvBody, "exported") {
		t.Errorf("导出文件里找不到刚建的用户：\n%s", csvBody)
	}

	if ct := header.Get("Content-Type"); !strings.Contains(ct, "text/csv") {
		t.Errorf("Content-Type = %q，期望 text/csv", ct)
	}
	if cd := header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Errorf("Content-Disposition = %q，期望含 attachment", cd)
	}
}

// TestExportWritesUTF8BOM 验证导出文件带 UTF-8 BOM。
//
// 缺 BOM 时 Excel（尤其 Windows 版）会按本地代码页解码，
// 中文表头与内容全变乱码——对用户就是「导出坏了」，
// 但文件内容其实完全正确，很难从代码上看出问题。
func TestExportWritesUTF8BOM(t *testing.T) {
	env := newTestEnv(t)
	token := env.adminToken(t)

	for _, path := range []string{"/users/export", "/oper-logs/export", "/login-logs/export"} {
		t.Run(path, func(t *testing.T) {
			status, body, _ := exportRequest(t, env, path, token)
			if status != http.StatusOK {
				t.Fatalf("导出失败：%d", status)
			}
			if !strings.HasPrefix(body, "\xEF\xBB\xBF") {
				t.Error("导出内容缺少 UTF-8 BOM，Excel 打开会中文乱码")
			}
			// 去掉 BOM 后应以中文表头开头。
			if trimmed := strings.TrimPrefix(body, "\xEF\xBB\xBF"); trimmed == "" {
				t.Error("导出内容为空，连表头都没有")
			}
		})
	}
}

// TestExportRequiresOwnPermission 验证导出权限独立于列表权限。
//
// common 角色有 system:user:list（能看列表）但没有 system:user:export。
// 若导出复用了列表权限，任何能看列表的人都能拖走全量数据——
// 这正是把导出单独设为一个权限点的意义。
func TestExportRequiresOwnPermission(t *testing.T) {
	env := newTestEnv(t)
	token := env.commonToken(t)

	// 先确认它确实能看列表，排除「本来就没权限」的干扰。
	if status, _ := env.do(t, "GET", "/users", token, nil); status != http.StatusOK {
		t.Fatalf("前置条件不成立：common 角色应能查看用户列表，实际 %d", status)
	}

	for _, path := range []string{"/users/export", "/oper-logs/export", "/login-logs/export"} {
		t.Run(path, func(t *testing.T) {
			status, _, _ := exportRequest(t, env, path, token)
			if status != http.StatusForbidden {
				t.Errorf("无导出权限却返回 %d（期望 403），"+
					"导出可能错误地复用了列表权限", status)
			}
		})
	}
}

// TestExportRejectsUnauthenticated 验证导出接口同样需要登录。
func TestExportRejectsUnauthenticated(t *testing.T) {
	env := newTestEnv(t)

	for _, path := range []string{"/users/export", "/oper-logs/export", "/login-logs/export"} {
		t.Run(path, func(t *testing.T) {
			status, _, _ := exportRequest(t, env, path, "")
			if status != http.StatusUnauthorized {
				t.Errorf("未登录访问导出返回 %d，期望 401", status)
			}
		})
	}
}

// TestExportPathNotShadowedByIDRoute 验证 /export 没被 /:id 吃掉。
//
// gin 里 /users/:id 与 /users/export 注册顺序错了，"export" 会被当成 id，
// 表现为「导出返回一个 JSON 报错说 ID 参数非法」——
// 状态码是 400 而非 404，极易误判成参数问题。
func TestExportPathNotShadowedByIDRoute(t *testing.T) {
	env := newTestEnv(t)
	token := env.adminToken(t)

	status, body, header := exportRequest(t, env, "/users/export", token)
	if status != http.StatusOK {
		t.Fatalf("导出返回 %d，body=%s", status, body)
	}
	// 若被 /:id 匹配走，返回的会是 JSON 而非 CSV。
	if ct := header.Get("Content-Type"); strings.Contains(ct, "json") {
		t.Errorf("导出返回了 JSON（Content-Type=%s），"+
			"/users/export 可能被 /users/:id 匹配走了", ct)
	}
}

// TestExportRespectsQueryFilters 验证导出遵循与列表一致的筛选条件。
//
// 若忽略筛选而永远导全量，用户筛了条件却拿到全表，
// 既不符合预期也放大了数据外流面。
func TestExportRespectsQueryFilters(t *testing.T) {
	env := newTestEnv(t)
	token := env.adminToken(t)

	// 种子里已有 admin 与 fz；按账号精确筛选应只命中 fz。
	status, body, _ := exportRequest(t, env, "/users/export?username=fz", token)
	if status != http.StatusOK {
		t.Fatalf("导出失败：%d", status)
	}

	if !strings.Contains(body, commonUser) {
		t.Errorf("按 username=fz 筛选，导出里却没有 fz：\n%s", body)
	}
	if strings.Contains(body, adminUser) {
		t.Errorf("按 username=fz 筛选，导出里却出现了 admin，筛选条件未生效：\n%s", body)
	}
}

// TestExportIsRecordedInOperLog 验证导出动作本身被记入操作日志。
//
// 导出是一次性带走大量数据，属于最该留痕的操作之一：
// 事后要能回答「谁在什么时候导出了什么条件的数据」。
func TestExportIsRecordedInOperLog(t *testing.T) {
	env := newTestEnv(t)
	token := env.adminToken(t)

	if status, _, _ := exportRequest(t, env, "/users/export", token); status != http.StatusOK {
		t.Fatalf("导出失败")
	}

	var logs []model.OperLog
	if err := env.db.Where("title = ?", "导出用户").Find(&logs).Error; err != nil {
		t.Fatalf("查询操作日志失败: %v", err)
	}
	if len(logs) == 0 {
		t.Fatal("导出未被记入操作日志，事后无法追溯谁导出了数据")
	}
	if logs[0].OperName != adminUser {
		t.Errorf("操作日志里的操作人是 %q，期望 %q", logs[0].OperName, adminUser)
	}
	// 导出的响应体是 CSV，不该被当作 json_result 存进日志表
	// （那会让日志表里出现一份完整的导出副本）。
	if strings.Contains(logs[0].JSONResult, "账号") {
		t.Errorf("操作日志里存了导出文件内容，日志表将随导出次数膨胀：%s", logs[0].JSONResult)
	}
}
