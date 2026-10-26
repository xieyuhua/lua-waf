// Package captcha 生成并校验登录用图形验证码。
package captcha

import (
	"crypto/rand"
	"encoding/hex"
	mrand "math/rand"
	"strings"
	"sync"
	"time"
)

// alphabet 为验证码字符集，剔除了易混淆的 0/O/1/I。
const alphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

// item 是一条待校验的验证码。
type item struct {
	code   string
	expire time.Time
}

// Manager 保存验证码并在校验后立即失效，默认有效期 5 分钟。
type Manager struct {
	TTL    time.Duration
	Length int
	Width  int
	Height int

	mu    sync.Mutex
	items map[string]item

	randMu sync.Mutex
	rnd    *mrand.Rand
}

// New 生成一个新的验证码，返回唯一标识与明文（明文用于渲染图片）。
func (m *Manager) New() (id, code string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.items == nil {
		m.items = make(map[string]item, 32)
	}
	m.gcLocked()

	code = m.newCode()
	id = randomID()
	m.items[id] = item{code: code, expire: time.Now().Add(m.ttl())}
	return id, code
}

// Verify 校验验证码；无论成功与否都会销毁该验证码，防止重放。
func (m *Manager) Verify(id, code string) bool {
	id = strings.TrimSpace(id)
	code = strings.ToUpper(strings.TrimSpace(code))
	if id == "" || code == "" {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.items[id]
	if !ok {
		return false
	}
	delete(m.items, id)
	return entry.code == code && time.Now().Before(entry.expire)
}

// Count 返回当前未过期的验证码数量，便于监控内存占用。
func (m *Manager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.items)
}

// newCode 生成指定长度的验证码明文。
func (m *Manager) newCode() string {
	length := m.Length
	if length <= 0 {
		length = 4
	}
	out := make([]byte, length)
	for i := 0; i < length; i++ {
		out[i] = alphabet[m.randInt(len(alphabet))]
	}
	return string(out)
}

// ttl 返回生效的有效期。
func (m *Manager) ttl() time.Duration {
	if m.TTL <= 0 {
		return 5 * time.Minute
	}
	return m.TTL
}

// gcLocked 清理过期条目，调用方必须已持有锁。
func (m *Manager) gcLocked() {
	now := time.Now()
	for key, entry := range m.items {
		if now.After(entry.expire) {
			delete(m.items, key)
		}
	}
}

// randomID 生成 32 位十六进制随机标识。
func randomID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(buf)
}
