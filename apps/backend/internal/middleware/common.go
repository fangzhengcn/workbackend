package middleware

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/fangzhengcn/workbackend/apps/backend/pkg/errs"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/logger"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/response"
)

// CORS 跨域配置。
//
// 显式列出允许的来源而非用 *：携带 Authorization 头的请求
// 在 AllowAllOrigins 下会被浏览器拒绝，且白名单本身也更安全。
func CORS(allowOrigins []string) gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOrigins:     allowOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length", "Content-Disposition"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})
}

// Logger 记录访问日志。
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		fields := map[string]any{
			"status": c.Writer.Status(),
			"method": c.Request.Method,
			"path":   path,
			"ip":     c.ClientIP(),
			"cost":   time.Since(start).Milliseconds(),
		}
		if query != "" {
			fields["query"] = query
		}

		entry := logger.WithFields(fields)
		// Controller 通过 c.Error 挂上的内部错误在此统一记录完整原因。
		if len(c.Errors) > 0 {
			entry.Errorf("请求处理出错: %s", c.Errors.String())
			return
		}
		switch {
		case c.Writer.Status() >= 500:
			entry.Error("服务器错误")
		case c.Writer.Status() >= 400:
			entry.Warn("请求异常")
		default:
			entry.Info("请求完成")
		}
	}
}

// Recovery 捕获 panic，避免单个请求的崩溃导致整个进程退出。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				logger.WithFields(map[string]any{
					"path":   c.Request.URL.Path,
					"method": c.Request.Method,
					"ip":     c.ClientIP(),
				}).Errorf("请求发生 panic: %v", err)

				// 不把 panic 细节返回给前端，避免泄露内部实现。
				response.AbortWithCode(c, errs.CodeInternal, "服务器内部错误")
			}
		}()
		c.Next()
	}
}
