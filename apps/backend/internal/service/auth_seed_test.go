package service

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// seedPasswordPlaintext 是种子数据中 admin 账号的约定初始密码。
//
// README、deploy.sh 的部署提示、以及建表脚本的注释三处都向使用者承诺了这个值。
const seedPasswordPlaintext = "123456"

// TestSeedAdminPasswordMatchesDocumentedPlaintext 校验建表脚本里 admin 的 bcrypt
// 哈希确实对应文档承诺的明文。
//
// 为什么值得为一行种子数据写测试：bcrypt 哈希不可读，肉眼无法看出它对应哪个明文。
// 本项目曾经踩过——脚本注释写着「密码 '123456' 的 bcrypt 值」，但那串哈希实际是
// 网上流传的示例值，对应的是 admin123，导致首次部署后照文档登录必然 401，
// 且报错只说「密码错误」，排查方向完全被误导。
// 这个测试把「文档承诺」与「实际哈希」锁在一起，改错任一边都会立刻失败。
func TestSeedAdminPasswordMatchesDocumentedPlaintext(t *testing.T) {
	hash := extractSeedAdminHash(t)

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(seedPasswordPlaintext)); err != nil {
		t.Fatalf("种子数据中 admin 的密码哈希与文档承诺的明文 %q 不匹配：%v\n"+
			"哈希=%s\n"+
			"修法：用 bcrypt.GenerateFromPassword([]byte(%q), 10) 重新生成并更新建表脚本，"+
			"或把 README/deploy.sh/脚本注释统一改成哈希真实对应的明文。",
			seedPasswordPlaintext, err, hash, seedPasswordPlaintext)
	}
}

// TestHashPasswordRoundTrip 确认服务层生成的哈希可被校验通过，且不同调用产生不同哈希。
func TestHashPasswordRoundTrip(t *testing.T) {
	const plaintext = "s3cret-pa55"

	first, err := HashPassword(plaintext)
	if err != nil {
		t.Fatalf("生成哈希失败: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(first), []byte(plaintext)); err != nil {
		t.Fatalf("自身生成的哈希无法校验通过: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(first), []byte(plaintext+"x")); err == nil {
		t.Fatal("错误密码竟然校验通过")
	}

	// bcrypt 每次自带随机 salt，相同明文两次生成的哈希必须不同，
	// 否则说明 salt 没生效，密码可被彩虹表批量破解。
	second, err := HashPassword(plaintext)
	if err != nil {
		t.Fatalf("第二次生成哈希失败: %v", err)
	}
	if first == second {
		t.Fatal("相同明文两次生成了相同哈希，salt 未生效")
	}
}

// extractSeedAdminHash 从建表脚本中取出 admin 用户的密码哈希。
//
// 直接解析 SQL 而非把哈希复制进测试：复制一份就等于多一处需要同步的事实，
// 而这个测试的全部意义正是校验脚本里那一份。
func extractSeedAdminHash(t *testing.T) string {
	t.Helper()

	// 从 internal/service 回到仓库根再进 docs。
	path := filepath.Join("..", "..", "..", "..", "docs", "权限系统数据库.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取建表脚本失败（路径 %s）: %v", path, err)
	}

	// 匹配 sys_user 种子行里 'admin' 之后的 bcrypt 串（$2a$/$2b$ 开头，60 字符）。
	re := regexp.MustCompile(`'admin',\s*'(\$2[aby]\$[^']+)'`)
	match := re.FindSubmatch(content)
	if match == nil {
		t.Fatal("未能在建表脚本中定位 admin 的密码哈希，" +
			"可能种子数据结构已变动，请同步更新本测试的匹配规则")
	}
	return string(match[1])
}
