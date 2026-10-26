package httpapi

import (
	"context"
	"net/http"

	"wafadmin/pkg/session"
)

// contextKey 是请求上下文的键类型。
type contextKey string

// claimsKey 用于在上下文中保存会话身份。
const claimsKey contextKey = "wafadmin.claims"

// withClaims 把会话身份写入请求上下文。
func withClaims(r *http.Request, claims *session.Claims) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), claimsKey, claims))
}

// CurrentUser 读取当前登录身份。
func CurrentUser(r *http.Request) (*session.Claims, bool) {
	claims, ok := r.Context().Value(claimsKey).(*session.Claims)
	return claims, ok && claims != nil
}

// CurrentUID 读取当前登录账号 ID，未登录时返回 0。
func CurrentUID(r *http.Request) int64 {
	if claims, ok := CurrentUser(r); ok {
		return claims.UID
	}
	return 0
}

// clientIP 提取来源 IP，优先使用反向代理传递的 X-Real-IP。
func clientIP(r *http.Request) string {
	if value := r.Header.Get("X-Real-IP"); value != "" {
		return value
	}
	if value := r.Header.Get("X-Forwarded-For"); value != "" {
		return value
	}
	host := r.RemoteAddr
	for index := len(host) - 1; index >= 0; index-- {
		if host[index] == ':' {
			return host[:index]
		}
	}
	return host
}
