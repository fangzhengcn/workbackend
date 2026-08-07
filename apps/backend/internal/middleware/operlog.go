package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/model"
	"github.com/fangzhengcn/workbackend/apps/backend/internal/repository"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/crypto"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/logger"
)

// maxLoggedBodySize 限制记录的请求体大小，避免大文件上传把日志表撑爆。
const maxLoggedBodySize = 4 * 1024

// sensitiveKeys 列出需要在日志中脱敏的字段名。
//
// 密码类字段整体替换为 ***；手机号/邮箱做部分掩码。
// 漏掉字段就意味着明文入库，新增敏感入参时必须同步此处。
var (
	redactKeys = map[string]struct{}{
		"password":    {},
		"oldPassword": {},
		"newPassword": {},
		"captchaCode": {},
	}
	maskPhoneKeys = map[string]struct{}{
		"phone": {},
	}
	maskEmailKeys = map[string]struct{}{
		"email": {},
	}
)

// bodyWriter 包装 ResponseWriter 以便同时捕获响应内容。
type bodyWriter struct {
	gin.ResponseWriter
	buffer *bytes.Buffer
}

func (w bodyWriter) Write(b []byte) (int, error) {
	/*
	 * 只捕获 JSON 响应。
	 *
	 * 文件下载类接口（如 CSV 导出）的响应体就是数据本身，
	 * 记进日志表等于把导出内容又存了一份副本：既让日志表随导出次数膨胀，
	 * 也把已脱敏的业务数据复制到另一张表里扩大了暴露面。
	 * 判定放在 Write 而非 OperLog 末尾，是因为此时 Content-Type 已确定。
	 */
	if w.captureBody() && w.buffer.Len() < maxLoggedBodySize {
		w.buffer.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

// captureBody 判断当前响应是否值得记入日志。
func (w bodyWriter) captureBody() bool {
	return strings.Contains(w.Header().Get("Content-Type"), "application/json")
}

// isMultipart 判断请求是否为文件上传。
//
// 这类请求体是二进制内容，既不能被截断读取（会破坏请求），
// 记进日志表也没有价值。
func isMultipart(c *gin.Context) bool {
	return strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data")
}

// describeUpload 用文件名与大小概括一次上传，替代无法记录的二进制请求体。
//
// 在 c.Next() 之后调用：此时 handler 已解析过表单，可直接读 MultipartForm 缓存，
// 不会二次消费请求体。
func describeUpload(c *gin.Context) string {
	form := c.Request.MultipartForm
	if form == nil {
		return `{"upload":"multipart（未解析）"}`
	}

	files := make([]string, 0, 2)
	for field, headers := range form.File {
		for _, fh := range headers {
			files = append(files, fmt.Sprintf(`{"field":%q,"filename":%q,"size":%d}`,
				field, fh.Filename, fh.Size))
		}
	}
	if len(files) == 0 {
		return `{"upload":"无文件字段"}`
	}
	return `{"files":[` + strings.Join(files, ",") + `]}`
}

// OperLog 记录写操作日志。
//
// title 为操作模块名（如「用户管理」），businessType 见 model.BusinessType* 常量。
// 只对写操作挂载本中间件：查询量大且价值低，全量记录会迅速膨胀。
func OperLog(logs *repository.LogRepository, title string, businessType int8) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		/*
		 * 读出请求体用于记日志，读完必须还原，否则后续 ShouldBindJSON 读到空内容。
		 *
		 * 文件上传（multipart）必须整体跳过：下面用 LimitReader 只读前 4KB
		 * 再把 Body 替换成这 4KB，对一张 1.8MB 的图片而言等于把请求截断，
		 * 后端解析 multipart 时报「unexpected EOF」，且报错位置离真正的原因很远。
		 * 上传请求体是二进制文件内容，记进日志表本身也没有价值。
		 */
		var requestBody []byte
		if c.Request.Body != nil && !isMultipart(c) {
			limited := io.LimitReader(c.Request.Body, maxLoggedBodySize)
			requestBody, _ = io.ReadAll(limited)
			c.Request.Body = io.NopCloser(bytes.NewReader(requestBody))
		}

		writer := bodyWriter{ResponseWriter: c.Writer, buffer: &bytes.Buffer{}}
		c.Writer = writer

		c.Next()

		status := model.StatusEnabled
		errorMsg := ""
		if c.Writer.Status() != http.StatusOK {
			status = model.StatusDisabled
			errorMsg = c.Errors.String()
		}

		var operUserID *uint64
		if id := CurrentUserID(c); id > 0 {
			operUserID = &id
		}

		// 文件上传的请求体是二进制，不记内容；改记文件名与大小，
		// 这样审计时仍能回答「谁上传了什么文件」。
		requestParam := sanitizeBody(requestBody)
		if isMultipart(c) {
			requestParam = describeUpload(c)
		}

		entry := &model.OperLog{
			Title:        title,
			BusinessType: businessType,
			Method:       c.Request.Method,
			RequestURL:   c.Request.URL.Path,
			OperUserID:   operUserID,
			OperName:     CurrentUsername(c),
			OperIP:       c.ClientIP(),
			RequestParam: requestParam,
			JSONResult:   writer.buffer.String(),
			Status:       status,
			ErrorMsg:     truncate(errorMsg, 2000),
			CostTime:     int(time.Since(start).Milliseconds()),
			CreatedAt:    time.Now(),
		}

		// 日志写入失败不应影响已完成的业务操作，仅告警。
		if err := logs.CreateOperLog(c.Request.Context(), entry); err != nil {
			logger.Warnf("写入操作日志失败: %v", err)
		}
	}
}

// sanitizeBody 对请求体中的敏感字段脱敏。
//
// 解析失败时返回占位符而非原文——宁可丢失排查信息，
// 也不能把可能含明文密码的内容原样入库。
func sanitizeBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "[非 JSON 或解析失败，已省略以防泄露敏感信息]"
	}
	redact(payload)
	sanitized, err := json.Marshal(payload)
	if err != nil {
		return "[序列化失败]"
	}
	return truncate(string(sanitized), maxLoggedBodySize)
}

// redact 递归脱敏 map 中的敏感字段。
func redact(payload map[string]any) {
	for key, value := range payload {
		if _, ok := redactKeys[key]; ok {
			payload[key] = "***"
			continue
		}
		if str, isString := value.(string); isString {
			if _, ok := maskPhoneKeys[key]; ok {
				payload[key] = crypto.MaskPhone(str)
				continue
			}
			if _, ok := maskEmailKeys[key]; ok {
				payload[key] = crypto.MaskEmail(str)
				continue
			}
		}
		// 嵌套对象与数组同样需要处理。
		switch nested := value.(type) {
		case map[string]any:
			redact(nested)
		case []any:
			for _, item := range nested {
				if obj, ok := item.(map[string]any); ok {
					redact(obj)
				}
			}
		}
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
