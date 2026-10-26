// Package crypto 提供口令摘要、随机串与接口签名等安全工具。
package crypto

import (
	"crypto/subtle"
	"fmt"
)

// Hasher 负责口令摘要，保持与历史版本数据兼容。
type Hasher struct{}

// MD5 返回 32 位小写 md5 摘要。
func (h Hasher) MD5(src string) string {
	return sumMD5(src)
}

// Password 生成口令摘要。
// 算法与 1.x 版本一致（md5(md5(raw)[5:10])），确保老库账号仍可登录。
func (h Hasher) Password(raw string) string {
	sum := h.MD5(raw)
	if len(sum) < 10 {
		return h.MD5(sum)
	}
	return h.MD5(sum[5:10])
}

// Verify 校验明文口令与已存摘要是否匹配（常量时间比较）。
func (h Hasher) Verify(raw, stored string) bool {
	actual := h.Password(raw)
	if len(actual) != len(stored) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(actual), []byte(stored)) == 1
}

// Token 生成指定字节长度的十六进制随机串。
func (h Hasher) Token(size int) string {
	return randomHex(size)
}

// sumMD5 计算 md5 摘要。
func sumMD5(src string) string {
	return fmt.Sprintf("%x", md5Sum([]byte(src)))
}
