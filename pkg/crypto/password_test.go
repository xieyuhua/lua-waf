package crypto_test

import (
	"testing"

	"wafadmin/pkg/crypto"
)

// TestPasswordIsCompatibleWithLegacy 验证口令摘要算法与 1.x 版本一致。
// 老库中的口令是 md5(md5(raw)[5:10])，升级后必须能继续登录。
func TestPasswordIsCompatibleWithLegacy(t *testing.T) {
	hasher := crypto.Hasher{}

	sum := hasher.MD5("admin")
	want := hasher.MD5(sum[5:10])
	if got := hasher.Password("admin"); got != want {
		t.Fatalf("口令摘要不兼容: got %s want %s", got, want)
	}
	if len(want) != 32 {
		t.Fatalf("口令摘要长度应为 32，实际 %d", len(want))
	}
}

// TestVerify 校验口令比对逻辑。
func TestVerify(t *testing.T) {
	hasher := crypto.Hasher{}
	stored := hasher.Password("secret-pass")

	if !hasher.Verify("secret-pass", stored) {
		t.Fatal("正确口令应校验通过")
	}
	if hasher.Verify("wrong-pass", stored) {
		t.Fatal("错误口令不应校验通过")
	}
	if hasher.Verify("secret-pass", "short") {
		t.Fatal("非法摘要不应校验通过")
	}
}

// TestToken 校验随机串长度与唯一性。
func TestToken(t *testing.T) {
	hasher := crypto.Hasher{}
	first := hasher.Token(16)
	second := hasher.Token(16)

	if len(first) != 32 {
		t.Fatalf("16 字节随机串应生成 32 位十六进制，实际 %d", len(first))
	}
	if first == second {
		t.Fatal("两次生成的随机串不应相同")
	}
}
