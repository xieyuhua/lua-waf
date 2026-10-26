package httpapi

import (
	"crypto/hmac"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// CSRF Cookie 与请求头的默认名称。
const (
	defaultCSRFCookie = "waf_csrf"
	defaultCSRFHeader = "X-CSRF-Token"
)

// CSRFGuard 实现双提交 Cookie 的 CSRF 防护。
// 前端从 Cookie 读取 token 并以请求头回传，服务端比对两者是否一致。
type CSRFGuard struct {
	CookieName string
	HeaderName string
	Secure     bool
}

// Ensure 返回请求携带的 token，不存在时下发一个新的。
func (g *CSRFGuard) Ensure(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(g.cookieName()); err == nil && cookie.Value != "" {
		return cookie.Value
	}

	token := randomToken()
	http.SetCookie(w, &http.Cookie{
		Name:     g.cookieName(),
		Value:    token,
		Path:     "/",
		HttpOnly: false, // 前端需要读取后放入请求头
		Secure:   g.Secure,
		SameSite: http.SameSiteLaxMode,
	})
	return token
}

// Check 校验非安全请求是否携带了正确的 token。
func (g *CSRFGuard) Check(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}

	cookie, err := r.Cookie(g.cookieName())
	if err != nil || cookie.Value == "" {
		return false
	}
	header := r.Header.Get(g.headerName())
	if header == "" {
		return false
	}
	if len(header) != len(cookie.Value) {
		return false
	}
	return hmac.Equal([]byte(header), []byte(cookie.Value))
}

// cookieName 返回生效的 Cookie 名称。
func (g *CSRFGuard) cookieName() string {
	if g.CookieName == "" {
		return defaultCSRFCookie
	}
	return g.CookieName
}

// headerName 返回生效的请求头名称。
func (g *CSRFGuard) headerName() string {
	if g.HeaderName == "" {
		return defaultCSRFHeader
	}
	return g.HeaderName
}

// randomToken 生成 32 位十六进制随机串。
func randomToken() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "wafadmin-fallback-token"
	}
	return hex.EncodeToString(buf)
}
