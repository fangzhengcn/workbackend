package service

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mojocn/base64Captcha"

	"github.com/fangzhengcn/workbackend/apps/backend/internal/vo"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/cache"
	"github.com/fangzhengcn/workbackend/apps/backend/pkg/errs"
)

// captchaTTL 是验证码有效期。过短影响体验，过长便于爆破。
const captchaTTL = 3 * time.Minute

// CaptchaService 生成与校验图形验证码。
//
// 答案存 Redis 而非内存：多实例部署时任一实例都能校验通过。
type CaptchaService struct {
	cache  *cache.Client
	driver base64Captcha.Driver
	// enabled 为 false 时跳过校验，便于本地开发与自动化测试。
	enabled bool
}

func NewCaptchaService(cacheClient *cache.Client, enabled bool) *CaptchaService {
	// 数字算式型验证码：可读性好于扭曲字符，用户输入成本低。
	driver := base64Captcha.NewDriverDigit(48, 130, 4, 0.6, 40)
	return &CaptchaService{cache: cacheClient, driver: driver, enabled: enabled}
}

// Enabled 返回验证码是否启用。
func (s *CaptchaService) Enabled() bool { return s.enabled }

// Generate 生成一张验证码，答案写入 Redis。
func (s *CaptchaService) Generate(ctx context.Context) (*vo.CaptchaResult, error) {
	_, content, answer := s.driver.GenerateIdQuestionAnswer()
	item, err := s.driver.DrawCaptcha(content)
	if err != nil {
		return nil, errs.Internal("生成验证码失败").WithCause(err)
	}

	// 用 uuid 作为 Redis key，避免依赖 driver 内部的 id 生成规则。
	captchaID := uuid.NewString()
	if err := s.cache.SetCaptcha(ctx, captchaID, answer, captchaTTL); err != nil {
		return nil, errs.Internal("保存验证码失败").WithCause(err)
	}

	return &vo.CaptchaResult{
		CaptchaID:   captchaID,
		ImageBase64: item.EncodeB64string(),
	}, nil
}

// Verify 校验验证码。
//
// 验证码一次性消费：无论对错都已从 Redis 删除，
// 避免同一验证码被反复用于撞库。
func (s *CaptchaService) Verify(ctx context.Context, captchaID, code string) error {
	if !s.enabled {
		return nil
	}
	if captchaID == "" || code == "" {
		return errs.ErrCaptchaInvalid
	}

	answer, err := s.cache.ConsumeCaptcha(ctx, captchaID)
	if err != nil {
		return errs.Internal("校验验证码失败").WithCause(err)
	}
	// 已过期或不存在。
	if answer == "" {
		return errs.ErrCaptchaInvalid
	}
	if !strings.EqualFold(strings.TrimSpace(code), answer) {
		return errs.ErrCaptchaInvalid
	}
	return nil
}
