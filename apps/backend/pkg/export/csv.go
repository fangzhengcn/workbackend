// Package export 提供 CSV 导出的公共能力。
//
// 选 CSV 而非 xlsx：标准库 encoding/csv 即可，零新增依赖，
// 且能边查边写、内存占用与数据量无关；Excel/WPS/Numbers 都能直接打开。
package export

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// utf8BOM 是 UTF-8 字节序标记。
//
// 必须写在文件开头：Excel（尤其 Windows 版）读无 BOM 的 CSV 时会按本地
// ANSI 代码页解码，中文表头与内容全部变成乱码。这个坑对用户是「导出坏了」，
// 但对程序来说文件内容完全正确，很难从代码上看出问题。
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Writer 是带 BOM 的 CSV 写入器。
type Writer struct {
	csv *csv.Writer
}

// NewWriter 写入 BOM 并返回写入器。
func NewWriter(w io.Writer) (*Writer, error) {
	if _, err := w.Write(utf8BOM); err != nil {
		return nil, fmt.Errorf("写入 BOM 失败: %w", err)
	}
	return &Writer{csv: csv.NewWriter(w)}, nil
}

// WriteRow 写一行。
func (w *Writer) WriteRow(fields []string) error {
	if err := w.csv.Write(fields); err != nil {
		return fmt.Errorf("写入 CSV 行失败: %w", err)
	}
	return nil
}

// Flush 刷出缓冲并返回累积的错误。
func (w *Writer) Flush() error {
	w.csv.Flush()
	if err := w.csv.Error(); err != nil {
		return fmt.Errorf("刷出 CSV 失败: %w", err)
	}
	return nil
}

/*
SetHeaders 设置下载响应头。

文件名同时给 filename 与 filename*：
前者用 ASCII 兜底（老浏览器），后者按 RFC 5987 传 UTF-8 百分号编码，
否则中文文件名在部分浏览器上会变成乱码或被截断。
*/
func SetHeaders(w http.ResponseWriter, filename string) {
	encoded := url.PathEscape(filename)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set(
		"Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, asciiFallback(filename), encoded),
	)
	// 导出内容随查询条件变化，且可能含敏感信息，禁止任何缓存。
	w.Header().Set("Cache-Control", "no-store")
}

// Filename 按「前缀_时间戳.csv」生成文件名。
//
// 带时间戳是为了让用户多次导出的文件不互相覆盖——
// 同名下载在多数浏览器里会变成 xxx(1).csv，反而更难分辨哪份是新的。
func Filename(prefix string, at time.Time) string {
	return fmt.Sprintf("%s_%s.csv", prefix, at.Format("20060102_150405"))
}

// asciiFallback 把非 ASCII 字符替换为下划线，供 filename 参数使用。
func asciiFallback(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 128 {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	return b.String()
}

// FormatTime 统一导出中的时间格式；零值输出空串而非 0001-01-01。
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}
