package crypto

import "testing"

const (
	testAESKey  = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	testHMACKey = "1f1e1d1c1b1a191817161514131211100f0e0d0c0b0a09080706050403020100"
)

func newTestCipher(t *testing.T) *Cipher {
	t.Helper()
	c, err := NewCipher(testAESKey, testHMACKey, 1)
	if err != nil {
		t.Fatalf("NewCipher 失败: %v", err)
	}
	return c
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c := newTestCipher(t)
	for _, plaintext := range []string{"13800138000", "user@example.com", "张三", ""} {
		encrypted, err := c.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("Encrypt(%q) 失败: %v", plaintext, err)
		}
		got, err := c.Decrypt(encrypted)
		if err != nil {
			t.Fatalf("Decrypt 失败: %v", err)
		}
		if got != plaintext {
			t.Errorf("往返结果不一致: 期望 %q, 实际 %q", plaintext, got)
		}
	}
}

// GCM 的随机 nonce 必须让相同明文产生不同密文，否则会泄露「两人手机号相同」。
func TestEncryptIsNonDeterministic(t *testing.T) {
	c := newTestCipher(t)
	first, err := c.Encrypt("13800138000")
	if err != nil {
		t.Fatalf("Encrypt 失败: %v", err)
	}
	second, err := c.Encrypt("13800138000")
	if err != nil {
		t.Fatalf("Encrypt 失败: %v", err)
	}
	if first == second {
		t.Error("相同明文产生了相同密文，nonce 未随机化")
	}
}

// 空明文不应变成一段密文，否则「未填写」与「填了值」无法区分。
func TestEncryptEmptyStaysEmpty(t *testing.T) {
	c := newTestCipher(t)
	got, err := c.Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt 失败: %v", err)
	}
	if got != "" {
		t.Errorf("空明文应返回空串, 实际 %q", got)
	}
}

func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	c := newTestCipher(t)
	encrypted, err := c.Encrypt("13800138000")
	if err != nil {
		t.Fatalf("Encrypt 失败: %v", err)
	}
	// 翻转最后一个字符，authTag 校验应当失败。
	tampered := []byte(encrypted)
	if tampered[len(tampered)-1] == 'A' {
		tampered[len(tampered)-1] = 'B'
	} else {
		tampered[len(tampered)-1] = 'A'
	}
	if _, err := c.Decrypt(string(tampered)); err == nil {
		t.Error("篡改后的密文应解密失败，但成功了")
	}
}

// 盲索引必须确定性，否则无法用于 WHERE 精确查询与唯一索引。
func TestBlindIndexIsDeterministic(t *testing.T) {
	c := newTestCipher(t)
	first := c.BlindIndex("13800138000")
	second := c.BlindIndex("13800138000")
	if first != second {
		t.Errorf("盲索引不确定: %q != %q", first, second)
	}
	if len(first) != 64 {
		t.Errorf("盲索引应为 64 位 hex, 实际长度 %d", len(first))
	}
	if other := c.BlindIndex("13800138001"); other == first {
		t.Error("不同明文得到了相同盲索引")
	}
	if c.BlindIndex("") != "" {
		t.Error("空明文的盲索引应为空串，以便配合唯一索引存 NULL")
	}
}

// 盲索引必须使用独立的 HMAC 密钥，不能退化成用 AES 密钥。
func TestBlindIndexUsesIndependentKey(t *testing.T) {
	c1 := newTestCipher(t)
	c2, err := NewCipher(testAESKey, testAESKey, 1)
	if err == nil {
		if c2.BlindIndex("13800138000") == c1.BlindIndex("13800138000") {
			t.Error("更换 HMAC 密钥后盲索引未变化")
		}
	}
}

func TestNewCipherRejectsBadKeys(t *testing.T) {
	if _, err := NewCipher("abcd", testHMACKey, 1); err == nil {
		t.Error("过短的 AES 密钥应被拒绝")
	}
	if _, err := NewCipher("zz"+testAESKey[2:], testHMACKey, 1); err == nil {
		t.Error("非 hex 的 AES 密钥应被拒绝")
	}
}

func TestMaskPhone(t *testing.T) {
	cases := map[string]string{
		"13800138000": "138****8000",
		"":            "",
		"12345":       "*****",
	}
	for in, want := range cases {
		if got := MaskPhone(in); got != want {
			t.Errorf("MaskPhone(%q) = %q, 期望 %q", in, got, want)
		}
	}
}

func TestMaskEmail(t *testing.T) {
	cases := map[string]string{
		"user@example.com": "u**r@example.com",
		"ab@example.com":   "a*@example.com",
		"a@example.com":    "*@example.com",
		"notanemail":       "**********",
	}
	for in, want := range cases {
		if got := MaskEmail(in); got != want {
			t.Errorf("MaskEmail(%q) = %q, 期望 %q", in, got, want)
		}
	}
}
