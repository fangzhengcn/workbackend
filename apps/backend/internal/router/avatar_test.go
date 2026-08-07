package router_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
)

var testPNG = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}

// uploadAvatar 以 multipart 表单发一次头像上传。
func uploadAvatar(t *testing.T, env *testEnv, token, filename string, content []byte) (int, apiResp) {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("构造表单失败: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("写入内容失败: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("关闭表单失败: %v", err)
	}

	// 直接构造请求：multipart 的 body 与 Content-Type（含 boundary）必须配对，
	// 复用 newRequest 会把 Content-Type 覆盖成 application/json。
	req := httptest.NewRequest("POST", "/api/v1/auth/avatar", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Origin", testOrigin)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := serve(env, req)

	var parsed apiResp
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	}
	return rec.Code, parsed
}

/*
 * TestAvatarUploadRejectsDisguisedFiles 验证伪装成图片的文件被拒。
 *
 * 这是整个上传功能的安全底线：头像目录是对外静态可访问的，
 * 若把可执行内容或含 <script> 的 HTML/SVG 放进去，
 * 就成了远程代码执行或存储型 XSS 的入口。
 * 扩展名与 Content-Type 都由客户端提供、可任意伪造，只能按内容嗅探。
 */
func TestAvatarUploadRejectsDisguisedFiles(t *testing.T) {
	env := newTestEnv(t)
	token := env.adminToken(t)

	cases := map[string][]byte{
		"PHP 伪装成 png":  []byte("<?php system($_GET['c']); ?>"),
		"HTML 伪装成 png": []byte("<html><script>alert(1)</script></html>"),
		"SVG 伪装成 png":  []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"纯文本伪装成 png":   []byte("not an image at all"),
	}

	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			status, resp := uploadAvatar(t, env, token, "avatar.png", content)
			if status != http.StatusBadRequest {
				t.Errorf("伪装文件返回 %d（期望 400），"+
					"类型校验可能在看扩展名而非文件内容", status)
			}
			if resp.Message == "" {
				t.Error("拒绝时未给出可读的提示")
			}
		})
	}

	// 被拒的上传不该在磁盘留下任何文件。
	entries, err := os.ReadDir(env.uploadDir)
	if err == nil && len(entries) != 0 {
		t.Errorf("校验失败的上传留下了 %d 个文件", len(entries))
	}
}

// TestAvatarUploadSucceedsAndUpdatesUser 验证上传成功后落盘且写回数据库。
func TestAvatarUploadSucceedsAndUpdatesUser(t *testing.T) {
	env := newTestEnv(t)
	token := env.adminToken(t)

	status, resp := uploadAvatar(t, env, token, "me.png", testPNG)
	if status != http.StatusOK {
		t.Fatalf("上传失败：%d %s", status, resp.Message)
	}

	// 响应里应返回可访问的 URL
	url := strings.Trim(string(resp.Data), `"`)
	if !strings.HasPrefix(url, "/uploads/") {
		t.Errorf("返回的头像 URL 为 %q，期望以 /uploads/ 开头", url)
	}

	// 数据库里的 avatar 应指向该 URL
	var user model.User
	if err := env.db.First(&user, 1).Error; err != nil {
		t.Fatalf("读取用户失败: %v", err)
	}
	if user.Avatar != url {
		t.Errorf("数据库里的 avatar 为 %q，期望 %q", user.Avatar, url)
	}

	// 文件真的落盘了
	entries, err := os.ReadDir(env.uploadDir)
	if err != nil {
		t.Fatalf("读取上传目录失败: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("上传目录里有 %d 个文件，期望 1 个", len(entries))
	}
	// 文件名应是服务端生成的随机名，不含上传时的原名
	if strings.Contains(entries[0].Name(), "me") {
		t.Errorf("落盘文件名 %q 沿用了上传方提供的名字，存在路径遍历风险",
			entries[0].Name())
	}
}

/*
 * TestAvatarUploadRemovesOldFile 验证换头像时删掉旧文件。
 *
 * 不删会让每次更换都留一份孤儿文件，长期运行后目录里绝大多数文件
 * 都不再被任何用户引用，且无从判断哪些能清理。
 */
func TestAvatarUploadRemovesOldFile(t *testing.T) {
	env := newTestEnv(t)
	token := env.adminToken(t)

	if status, resp := uploadAvatar(t, env, token, "first.png", testPNG); status != http.StatusOK {
		t.Fatalf("首次上传失败：%d %s", status, resp.Message)
	}
	first, _ := os.ReadDir(env.uploadDir)
	if len(first) != 1 {
		t.Fatalf("首次上传后有 %d 个文件", len(first))
	}
	firstName := first[0].Name()

	if status, resp := uploadAvatar(t, env, token, "second.png", testPNG); status != http.StatusOK {
		t.Fatalf("二次上传失败：%d %s", status, resp.Message)
	}

	after, _ := os.ReadDir(env.uploadDir)
	if len(after) != 1 {
		t.Errorf("换头像后目录里有 %d 个文件，旧文件未被清理", len(after))
	}
	if len(after) == 1 && after[0].Name() == firstName {
		t.Error("新头像未替换旧文件")
	}
}

// TestAvatarUploadRejectsUnauthenticated 验证未登录不能上传。
//
// 若放行，任何人都能往服务器磁盘写文件。
func TestAvatarUploadRejectsUnauthenticated(t *testing.T) {
	env := newTestEnv(t)

	status, _ := uploadAvatar(t, env, "", "avatar.png", testPNG)
	if status != http.StatusUnauthorized {
		t.Errorf("未登录上传返回 %d，期望 401", status)
	}

	entries, err := os.ReadDir(env.uploadDir)
	if err == nil && len(entries) != 0 {
		t.Error("未登录的上传竟然写入了文件")
	}
}

// TestAvatarUploadRejectsMissingFile 验证不带文件时给出可读提示。
func TestAvatarUploadRejectsMissingFile(t *testing.T) {
	env := newTestEnv(t)
	token := env.adminToken(t)

	// 发一个没有 file 字段的普通请求
	status, resp := env.do(t, "POST", "/auth/avatar", token, map[string]any{"foo": "bar"})
	if status != http.StatusBadRequest {
		t.Errorf("缺少文件返回 %d，期望 400", status)
	}
	if resp.Message == "" {
		t.Error("缺少文件时未给出提示")
	}
}

// TestUploadedAvatarIsPubliclyAccessible 验证上传后的头像可直接访问。
//
// 头像要能放进 <img src>，带 Token 的请求做不到这一点，故静态路由不鉴权；
// 文件名是 16 字节随机 hex，不可枚举。
func TestUploadedAvatarIsPubliclyAccessible(t *testing.T) {
	env := newTestEnv(t)
	token := env.adminToken(t)

	status, resp := uploadAvatar(t, env, token, "me.png", testPNG)
	if status != http.StatusOK {
		t.Fatalf("上传失败：%d %s", status, resp.Message)
	}
	url := strings.Trim(string(resp.Data), `"`)

	// 不带 Token 直接访问该 URL
	req := newRequest(t, "GET", url, "", nil)
	rec := serve(env, req)
	if rec.Code != http.StatusOK {
		t.Errorf("头像 URL 返回 %d，期望 200——<img> 标签将无法显示头像", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), testPNG) {
		t.Error("返回的内容与上传的文件不一致")
	}
}

/*
 * TestAvatarUploadSurvivesOperLogMiddleware 验证大于日志截断阈值的上传不被破坏。
 *
 * 真实故障：operLog 中间件为记日志会读走请求体，且只用 LimitReader 读前 4KB
 * 再把 Body 替换成这 4KB——对一张 1.8MB 的图片而言等于把请求截断，
 * 后端解析 multipart 时报「unexpected EOF」，报错位置离真正的原因很远。
 *
 * 此前的头像用例全都只传几个字节的图片，远低于 4KB 阈值，
 * 因此完全没能覆盖这条路径。这里刻意用一张 64KB 的图片。
 */
func TestAvatarUploadSurvivesOperLogMiddleware(t *testing.T) {
	env := newTestEnv(t)
	token := env.adminToken(t)

	// 远超中间件的 4KB 记录上限
	large := append(append([]byte{}, testPNG...), bytes.Repeat([]byte{0x42}, 64*1024)...)

	status, resp := uploadAvatar(t, env, token, "large.png", large)
	if status != http.StatusOK {
		t.Fatalf("大于 4KB 的上传失败：%d %s\n"+
			"若报 unexpected EOF，说明 operLog 中间件截断了请求体",
			status, resp.Message)
	}

	// 文件必须完整落盘，不能只写进前 4KB
	entries, err := os.ReadDir(env.uploadDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("上传目录异常：err=%v 文件数=%d", err, len(entries))
	}
	info, err := entries[0].Info()
	if err != nil {
		t.Fatalf("读取文件信息失败: %v", err)
	}
	if info.Size() != int64(len(large)) {
		t.Errorf("落盘大小为 %d 字节，期望 %d——文件被截断了",
			info.Size(), len(large))
	}
}

// TestAvatarUploadLogsFileMetadata 验证上传的操作日志记文件元信息而非二进制内容。
//
// 请求体是二进制，记进日志表既无价值也会让表膨胀；
// 但审计仍需回答「谁上传了什么文件」，故改记文件名与大小。
func TestAvatarUploadLogsFileMetadata(t *testing.T) {
	env := newTestEnv(t)
	token := env.adminToken(t)

	if status, resp := uploadAvatar(t, env, token, "portrait.png", testPNG); status != http.StatusOK {
		t.Fatalf("上传失败：%d %s", status, resp.Message)
	}

	var logs []model.OperLog
	if err := env.db.Where("request_url = ?", "/api/v1/auth/avatar").Find(&logs).Error; err != nil {
		t.Fatalf("查询操作日志失败: %v", err)
	}
	if len(logs) == 0 {
		t.Fatal("上传未被记入操作日志")
	}

	param := logs[0].RequestParam
	if !strings.Contains(param, "portrait.png") {
		t.Errorf("操作日志未记录上传的文件名：%s", param)
	}
	// 不该出现二进制内容（PNG 魔数）
	if strings.Contains(param, "\x89PNG") {
		t.Errorf("操作日志里存了文件的二进制内容：%q", param)
	}
}

// TestStaticRouteRejectsPathTraversal 验证静态路由挡住路径穿越。
//
// 若能穿越出去，配置文件（含数据库密码所在路径）与二进制都可能被下载。
func TestStaticRouteRejectsPathTraversal(t *testing.T) {
	env := newTestEnv(t)

	attempts := []string{
		"/uploads/../config/rbac_model.conf",
		"/uploads/../../etc/passwd",
		"/uploads/%2e%2e%2fconfig%2frbac_model.conf",
	}

	for _, path := range attempts {
		t.Run(path, func(t *testing.T) {
			req := newRequest(t, "GET", path, "", nil)
			rec := serve(env, req)
			// 只要不是 200 就说明没读到目标文件
			if rec.Code == http.StatusOK && rec.Body.Len() > 0 {
				t.Errorf("路径穿越成功，返回了 %d 字节内容", rec.Body.Len())
			}
		})
	}
}
