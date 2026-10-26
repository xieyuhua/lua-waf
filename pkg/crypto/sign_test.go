package crypto_test

import (
	"testing"
	"time"

	"wafadmin/pkg/crypto"
)

// TestSignerRoundTrip 验证签名可被自身校验通过。
func TestSignerRoundTrip(t *testing.T) {
	signer := crypto.Signer{APIKey: "xie20", Window: 10 * time.Minute}
	now := time.Now().Unix()

	signature := signer.Sign(now)
	if !signer.Verify(itoa(now), signature) {
		t.Fatal("签名校验应通过")
	}
	if signer.Verify(itoa(now), "deadbeef") {
		t.Fatal("错误签名不应通过校验")
	}
	if signer.Verify("not-a-number", signature) {
		t.Fatal("非法时间戳不应通过校验")
	}
}

// TestSignerRejectsExpired 验证过期时间戳会被拒绝。
func TestSignerRejectsExpired(t *testing.T) {
	signer := crypto.Signer{APIKey: "xie20", Window: time.Minute}
	stale := time.Now().Add(-10 * time.Minute).Unix()

	signature := signer.Sign(stale)
	if signer.Verify(itoa(stale), signature) {
		t.Fatal("超过时间窗口的请求不应通过校验")
	}
}

// TestSignerRequiresAPIKey 验证未配置 API_KEY 时拒绝校验。
func TestSignerRequiresAPIKey(t *testing.T) {
	signer := crypto.Signer{}
	now := time.Now().Unix()
	if signer.Verify(itoa(now), signer.Sign(now)) {
		t.Fatal("未配置 API_KEY 时不应通过校验")
	}
}

// itoa 把时间戳转换为十进制字符串。
func itoa(value int64) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	buf := make([]byte, 0, 20)
	for value > 0 {
		buf = append([]byte{byte('0' + value%10)}, buf...)
		value /= 10
	}
	if negative {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}
