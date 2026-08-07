package crypto

import "strings"

// MaskPhone 将手机号脱敏为 138****8000，用于日志与前端展示。
// 长度不足 7 位时整体打码，避免暴露过多信息。
func MaskPhone(phone string) string {
	n := len(phone)
	if n == 0 {
		return ""
	}
	if n < 7 {
		return strings.Repeat("*", n)
	}
	return phone[:3] + "****" + phone[n-4:]
}

// MaskEmail 将邮箱脱敏为 u***r@example.com。
func MaskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		// 不是合法邮箱形态，整体打码而不是原样返回。
		return strings.Repeat("*", len(email))
	}
	name, domain := email[:at], email[at:]
	switch len(name) {
	case 1:
		return "*" + domain
	case 2:
		return name[:1] + "*" + domain
	default:
		return name[:1] + strings.Repeat("*", len(name)-2) + name[len(name)-1:] + domain
	}
}
