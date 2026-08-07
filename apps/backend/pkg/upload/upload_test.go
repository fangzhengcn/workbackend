package upload

import (
	"bytes"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pngHeader 是最小可识别的 PNG 魔数（DetectContentType 只看头部）。
var pngHeader = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}

// gifHeader 是 GIF89a 魔数。
var gifHeader = []byte("GIF89a")

// newFileHeader 构造一个 multipart.FileHeader，模拟真实上传。
//
// 走 multipart.Writer 而非手搓结构体：FileHeader 的 Open() 依赖内部状态，
// 直接构造出来的对象无法读取内容。
func newFileHeader(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("构造 multipart 失败: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("写入内容失败: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("关闭 multipart 失败: %v", err)
	}

	reader := multipart.NewReader(&buf, w.Boundary())
	form, err := reader.ReadForm(int64(len(content)) + 1024)
	if err != nil {
		t.Fatalf("解析 multipart 失败: %v", err)
	}
	return form.File["file"][0]
}

/*
 * TestSaveAvatarRejectsNonImageDespiteExtension 是本包最重要的一条。
 *
 * 攻击者把可执行内容改名为 .png 上传，若只查扩展名或 Content-Type
 * （两者都由客户端提供、可任意伪造）就会被放过；
 * 文件落在能被静态服务访问的目录下，等于一个远程代码执行或存储型 XSS 入口。
 * 这里断言必须按内容嗅探真实类型。
 */
func TestSaveAvatarRejectsNonImageDespiteExtension(t *testing.T) {
	dir := t.TempDir()

	cases := map[string][]byte{
		"PHP 伪装成 png":    []byte("<?php system($_GET['c']); ?>"),
		"HTML 伪装成 png":   []byte("<html><script>alert(1)</script></html>"),
		"SVG 伪装成 png":    []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"ELF 可执行伪装成 png": {0x7F, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00},
		"纯文本伪装成 png":     []byte("just plain text, definitely not an image"),
	}

	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			// 文件名与扩展名都伪装成合法图片
			fh := newFileHeader(t, "avatar.png", content)
			saved, err := SaveAvatar(fh, dir)
			if err == nil {
				t.Errorf("非图片内容被接受了（保存为 %s），"+
					"类型校验可能在看扩展名而非文件内容", saved)
			}
		})
	}

	// 确认没有任何文件被留在磁盘上。
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读取目录失败: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("校验失败的上传在磁盘留下了 %d 个文件", len(entries))
	}
}

// TestSaveAvatarAcceptsRealImages 验证合法图片能保存成功且扩展名由服务端决定。
func TestSaveAvatarAcceptsRealImages(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name    string
		content []byte
		wantExt string
	}{
		{"PNG", pngHeader, ".png"},
		{"GIF", gifHeader, ".gif"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 上传时故意给一个错误的扩展名，验证落盘时按真实类型纠正
			fh := newFileHeader(t, "whatever.txt", tc.content)
			saved, err := SaveAvatar(fh, dir)
			if err != nil {
				t.Fatalf("合法图片被拒绝: %v", err)
			}
			if filepath.Ext(saved) != tc.wantExt {
				t.Errorf("落盘扩展名为 %q，期望 %q（应按真实类型而非上传名决定）",
					filepath.Ext(saved), tc.wantExt)
			}
			if _, err := os.Stat(filepath.Join(dir, saved)); err != nil {
				t.Errorf("文件未真正写入磁盘: %v", err)
			}
		})
	}
}

// TestSaveAvatarGeneratesSafeName 验证文件名完全由服务端生成。
//
// 用户可控的文件名是路径遍历的经典入口；随机名从根上消除该风险，
// 顺带避免同名覆盖。
func TestSaveAvatarGeneratesSafeName(t *testing.T) {
	dir := t.TempDir()

	malicious := []string{
		"../../../../etc/passwd.png",
		"..\\..\\windows\\system32\\evil.png",
		"a/b/c.png",
	}

	for _, filename := range malicious {
		t.Run(filename, func(t *testing.T) {
			fh := newFileHeader(t, filename, pngHeader)
			saved, err := SaveAvatar(fh, dir)
			if err != nil {
				// 部分名字可能在 multipart 层就被处理掉，不算失败
				t.Skipf("该文件名未进入保存流程: %v", err)
			}
			// 返回的必须是纯文件名，不含任何路径成分
			if saved != filepath.Base(saved) {
				t.Errorf("返回的文件名含路径成分: %q", saved)
			}
			if strings.Contains(saved, "..") {
				t.Errorf("返回的文件名含相对路径: %q", saved)
			}
			// 文件必须落在指定目录内
			if _, err := os.Stat(filepath.Join(dir, saved)); err != nil {
				t.Errorf("文件不在预期目录内: %v", err)
			}
		})
	}
}

// TestSaveAvatarUniqueNames 验证连续上传不会互相覆盖。
func TestSaveAvatarUniqueNames(t *testing.T) {
	dir := t.TempDir()
	seen := make(map[string]struct{})

	for i := 0; i < 20; i++ {
		fh := newFileHeader(t, "same.png", pngHeader)
		saved, err := SaveAvatar(fh, dir)
		if err != nil {
			t.Fatalf("第 %d 次保存失败: %v", i, err)
		}
		if _, dup := seen[saved]; dup {
			t.Fatalf("文件名重复: %s，旧头像会被覆盖", saved)
		}
		seen[saved] = struct{}{}
	}
}

// TestSaveAvatarRejectsOversize 验证超出大小限制的文件被拒且不留残留。
func TestSaveAvatarRejectsOversize(t *testing.T) {
	dir := t.TempDir()

	// 构造一个头部合法但整体超限的「图片」
	oversize := append(append([]byte{}, pngHeader...), bytes.Repeat([]byte{0x00}, MaxAvatarSize+1024)...)
	fh := newFileHeader(t, "big.png", oversize)

	if _, err := SaveAvatar(fh, dir); err == nil {
		t.Error("超过大小限制的文件被接受了")
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("被拒的超大文件在磁盘留下了 %d 个残留", len(entries))
	}
}

// TestSaveAvatarRejectsEmpty 验证空文件被拒。
func TestSaveAvatarRejectsEmpty(t *testing.T) {
	dir := t.TempDir()
	fh := newFileHeader(t, "empty.png", []byte{})

	if _, err := SaveAvatar(fh, dir); err == nil {
		t.Error("空文件被接受了")
	}
}

/*
 * TestRemoveRejectsPathTraversal 验证删除时拒绝带路径的文件名。
 *
 * Remove 的入参来自数据库里存的旧头像名。若那个值被写入 ../../ 之类的内容
 * （例如经由别的漏洞），不校验就等于把「换头像」变成任意文件删除。
 */
func TestRemoveRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()

	// 在上层目录放一个「受害文件」，确认它不会被删掉
	victim := filepath.Join(dir, "victim.txt")
	if err := os.WriteFile(victim, []byte("important"), 0o600); err != nil {
		t.Fatalf("准备受害文件失败: %v", err)
	}

	sub := filepath.Join(dir, "avatars")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("建子目录失败: %v", err)
	}

	if err := Remove(sub, "../victim.txt"); err == nil {
		t.Error("带路径的文件名被接受，存在任意文件删除风险")
	}
	if _, err := os.Stat(victim); err != nil {
		t.Error("上层目录的文件被删除了，路径遍历防护失效")
	}
}

// TestRemoveIsIdempotent 验证删除不存在的文件不报错。
//
// 调用方的目的是「确保它没了」；文件本就不存在时报错会让
// 「换头像」这类流程因为一个无关紧要的失败而中断。
func TestRemoveIsIdempotent(t *testing.T) {
	dir := t.TempDir()

	if err := Remove(dir, "never-existed.png"); err != nil {
		t.Errorf("删除不存在的文件应视为成功，实际报错: %v", err)
	}
	if err := Remove(dir, ""); err != nil {
		t.Errorf("空文件名应直接返回成功，实际报错: %v", err)
	}
}
