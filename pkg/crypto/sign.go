package crypto

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

// md5Sum 是 md5 计算的内部入口。
func md5Sum(src []byte) []byte {
	sum := md5.Sum(src)
	return sum[:]
}

// randomHex 生成 size 字节的随机数据的十六进制表示。
func randomHex(size int) string {
	if size <= 0 {
		size = 16
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

// Signer 生成与校验多台 WAF 之间的同步接口签名。
// 签名算法与 1.x 版本保持一致：md5(APIKey + md5(timestamp + APIKey))。
type Signer struct {
	APIKey string
	// Window 为允许的时间戳偏差，默认 10 分钟。
	Window time.Duration
}

// Sign 计算指定时间戳的签名。
func (s Signer) Sign(timestamp int64) string {
	inner := (Hasher{}).MD5(fmt.Sprintf("%d%s", timestamp, s.APIKey))
	outer := md5Sum([]byte(s.APIKey + inner))
	return hex.EncodeToString(outer)
}

// Verify 校验时间戳与签名是否匹配，并检查时间戳新鲜度。
func (s Signer) Verify(rawTimestamp, hash string) bool {
	if s.APIKey == "" || rawTimestamp == "" || hash == "" {
		return false
	}
	timestamp, err := strconv.ParseInt(rawTimestamp, 10, 64)
	if err != nil {
		return false
	}

	window := s.Window
	if window <= 0 {
		window = 10 * time.Minute
	}
	drift := time.Since(time.Unix(timestamp, 0))
	if drift > window || drift < -window {
		return false
	}

	expected := s.Sign(timestamp)
	if len(expected) != len(hash) {
		return false
	}
	return hmac.Equal([]byte(expected), []byte(hash))
}

// HMAC 计算基于 sha256 的消息认证码，用于会话 Cookie 签名。
func (s Signer) HMAC(secret []byte, payload []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return mac.Sum(nil)
}
