// Package session 提供基于签名 Cookie 的无状态会话管理。
package session

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Claims 是会话中保存的身份信息。
type Claims struct {
	UID  int64  `json:"uid"`
	Name string `json:"name"`
	Exp  int64  `json:"exp"`
}

// Manager 负责签发、校验与清除会话 Cookie。
type Manager struct {
	// CookieName 为会话 Cookie 名称。
	CookieName string
	// Secret 为 HMAC 签名密钥。
	Secret []byte
	// TTL 为会话有效期。
	TTL time.Duration
	// Secure 为 true 时仅在 HTTPS 下发送 Cookie。
	Secure bool
}

// Issue 签发会话 Cookie。
func (m *Manager) Issue(w http.ResponseWriter, uid int64, name string) error {
	ttl := m.ttl()
	claims := Claims{UID: uid, Name: name, Exp: time.Now().Add(ttl).Unix()}
	payload, err := json.Marshal(claims)
	if err != nil {
		return err
	}

	encoded := base64.RawURLEncoding.EncodeToString(payload)
	token := encoded + "." + base64.RawURLEncoding.EncodeToString(m.sign([]byte(encoded)))

	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName(),
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   m.Secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
	return nil
}

// Read 解析并校验会话 Cookie。
func (m *Manager) Read(r *http.Request) (*Claims, bool) {
	cookie, err := r.Cookie(m.cookieName())
	if err != nil || cookie.Value == "" {
		return nil, false
	}

	parts := strings.SplitN(cookie.Value, ".", 2)
	if len(parts) != 2 {
		return nil, false
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	if !hmac.Equal(signature, m.sign([]byte(parts[0]))) {
		return nil, false
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, false
	}
	claims := new(Claims)
	if err := json.Unmarshal(payload, claims); err != nil {
		return nil, false
	}
	if claims.UID <= 0 || time.Now().Unix() > claims.Exp {
		return nil, false
	}
	return claims, true
}

// Clear 清除会话 Cookie。
func (m *Manager) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName(),
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   m.Secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// ttl 返回生效的会话有效期。
func (m *Manager) ttl() time.Duration {
	if m.TTL <= 0 {
		return 12 * time.Hour
	}
	return m.TTL
}

// cookieName 返回生效的 Cookie 名称。
func (m *Manager) cookieName() string {
	if m.CookieName == "" {
		return "waf_session"
	}
	return m.CookieName
}

// sign 对载荷做 HMAC-SHA256 签名。
func (m *Manager) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, m.Secret)
	mac.Write(payload)
	return mac.Sum(nil)
}
