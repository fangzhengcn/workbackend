// Package logger 封装 Logrus，提供文件+控制台双输出与日志切割。
package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Options 是日志初始化参数，字段含义见 config.LogConfig。
type Options struct {
	Level      string
	Format     string
	Dir        string
	MaxSize    int
	MaxBackups int
	MaxAge     int
	Compress   bool
	Stdout     bool
}

// log 是包级默认 logger，Init 之前也可安全使用（输出到 stderr）。
var log = logrus.New()

// Init 按 Options 配置全局 logger。
func Init(opts Options) error {
	level, err := logrus.ParseLevel(opts.Level)
	if err != nil {
		return fmt.Errorf("logger: 非法的日志级别 %q: %w", opts.Level, err)
	}
	log.SetLevel(level)

	if opts.Format == "json" {
		log.SetFormatter(&logrus.JSONFormatter{TimestampFormat: "2006-01-02 15:04:05"})
	} else {
		log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		})
	}

	writers := make([]io.Writer, 0, 2)
	if opts.Stdout {
		writers = append(writers, os.Stdout)
	}
	if opts.Dir != "" {
		if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
			return fmt.Errorf("logger: 创建日志目录失败: %w", err)
		}
		writers = append(writers, &lumberjack.Logger{
			Filename:   filepath.Join(opts.Dir, "app.log"),
			MaxSize:    opts.MaxSize,
			MaxBackups: opts.MaxBackups,
			MaxAge:     opts.MaxAge,
			Compress:   opts.Compress,
		})
	}
	switch len(writers) {
	case 0:
		// 两个输出都被关掉时保留 stdout，否则日志会静默丢失。
		log.SetOutput(os.Stdout)
	case 1:
		log.SetOutput(writers[0])
	default:
		log.SetOutput(io.MultiWriter(writers...))
	}
	return nil
}

// L 返回底层 logrus 实例，供需要 Hook 等高级用法的场景使用。
func L() *logrus.Logger { return log }

// WithFields 附加结构化字段。
//
// 注意：涉及手机号/邮箱等敏感信息时，务必先用 crypto.MaskPhone / MaskEmail 脱敏。
func WithFields(fields logrus.Fields) *logrus.Entry { return log.WithFields(fields) }

// WithField 附加单个结构化字段。
func WithField(key string, value any) *logrus.Entry { return log.WithField(key, value) }

func Debugf(format string, args ...any) { log.Debugf(format, args...) }
func Infof(format string, args ...any)  { log.Infof(format, args...) }
func Warnf(format string, args ...any)  { log.Warnf(format, args...) }
func Errorf(format string, args ...any) { log.Errorf(format, args...) }
func Fatalf(format string, args ...any) { log.Fatalf(format, args...) }
