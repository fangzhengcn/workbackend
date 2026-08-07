// Package upload 处理文件上传的校验与落盘。
//
// 目前只服务头像场景，但校验逻辑（类型嗅探、大小限制、安全文件名）
// 与业务无关，后续其他上传可直接复用。
package upload

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// 头像相关约束。
const (
	// MaxAvatarSize 限制头像体积。
	//
	// 2MB 足够一张清晰的头像，同时挡住「上传 50MB 图片撑爆磁盘」这类滥用。
	// 校验必须在读取全部内容之前生效，否则限制形同虚设。
	MaxAvatarSize = 2 << 20 // 2MB

	// sniffLen 是类型嗅探需要读取的字节数。
	// 512 是 http.DetectContentType 的约定长度。
	sniffLen = 512
)

/*
allowedImageTypes 是允许的图片类型及其落盘扩展名。

只认这四种：都是浏览器原生支持、且能被 DetectContentType 可靠识别的静态图片格式。
刻意不含 SVG——SVG 是 XML，可内嵌 <script>，被当作图片直接访问时会在
同源下执行，等于一个存储型 XSS 入口。
*/
var allowedImageTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// SaveAvatar 校验并保存头像，返回相对存储目录的文件名。
//
// 返回的是文件名而非完整路径：调用方据此拼出对外 URL，
// 而磁盘路径属于实现细节，不该泄露到数据库或响应里。
func SaveAvatar(file *multipart.FileHeader, dir string) (string, error) {
	if file.Size <= 0 {
		return "", fmt.Errorf("文件为空")
	}
	// 先按声明的大小挡一道，避免白读一遍大文件。
	// 真实大小在下面的 LimitReader 处二次把关（声明值可被伪造）。
	if file.Size > MaxAvatarSize {
		return "", fmt.Errorf("图片不能超过 %dMB", MaxAvatarSize>>20)
	}

	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("打开上传文件失败: %w", err)
	}
	defer src.Close()

	/*
	 * 按内容嗅探真实类型，而不是相信扩展名或 Content-Type。
	 *
	 * 两者都由客户端提供、可任意伪造：把 shell.php 改名为 avatar.png
	 * 就能绕过扩展名检查。嗅探读的是文件头部的魔数，伪造成本高得多。
	 */
	head := make([]byte, sniffLen)
	n, err := io.ReadFull(src, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}
	head = head[:n]

	contentType := http.DetectContentType(head)
	// DetectContentType 可能返回带参数的形式（如 "text/plain; charset=utf-8"）。
	if idx := strings.IndexByte(contentType, ';'); idx >= 0 {
		contentType = strings.TrimSpace(contentType[:idx])
	}
	ext, ok := allowedImageTypes[contentType]
	if !ok {
		return "", fmt.Errorf("只支持 JPG / PNG / GIF / WEBP 图片，当前文件类型为 %s", contentType)
	}

	// 回到开头，之后要完整写盘。
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("重置文件读取位置失败: %w", err)
	}

	/*
	 * 文件名完全由服务端生成，不使用上传方提供的任何片段。
	 *
	 * 用户可控的文件名是路径遍历的经典入口（../../etc/passwd），
	 * 即便做了清洗也容易漏掉编码变体；随机名从根上消除这个面，
	 * 顺带避免同名覆盖与中文名在不同文件系统上的兼容问题。
	 */
	name, err := randomName(ext)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建上传目录失败: %w", err)
	}

	dstPath := filepath.Join(dir, name)
	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", fmt.Errorf("创建文件失败: %w", err)
	}
	defer dst.Close()

	// LimitReader 限制实际写入量：file.Size 来自客户端声明，不可全信。
	// 多给 1 字节用于判断是否超限。
	written, err := io.Copy(dst, io.LimitReader(src, MaxAvatarSize+1))
	if err != nil {
		_ = os.Remove(dstPath)
		return "", fmt.Errorf("写入文件失败: %w", err)
	}
	if written > MaxAvatarSize {
		// 超限文件必须删掉，否则磁盘上会留下垃圾。
		_ = os.Remove(dstPath)
		return "", fmt.Errorf("图片不能超过 %dMB", MaxAvatarSize>>20)
	}

	return name, nil
}

// Remove 删除存储目录下的指定文件。
//
// 只接受文件名，拒绝任何带路径分隔符的输入：
// 该函数会被「换头像时删旧文件」调用，而旧文件名来自数据库——
// 若数据库被写入 ../../ 之类的值，不校验就成了任意文件删除。
func Remove(dir, name string) error {
	if name == "" {
		return nil
	}
	if name != filepath.Base(name) {
		return fmt.Errorf("非法文件名: %s", name)
	}

	err := os.Remove(filepath.Join(dir, name))
	// 文件本就不存在时视为成功：调用方的目的是「确保它没了」。
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除文件失败: %w", err)
	}
	return nil
}

// randomName 生成随机文件名。
func randomName(ext string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成文件名失败: %w", err)
	}
	return hex.EncodeToString(buf) + ext, nil
}
