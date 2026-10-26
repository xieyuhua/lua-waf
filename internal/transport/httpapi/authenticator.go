package httpapi

import (
	"net/http"

	"wafadmin/pkg/response"
	"wafadmin/pkg/session"
)

// Authenticator 负责登录校验与 CSRF 校验，并统一把身份写入请求上下文。
type Authenticator struct {
	Session *session.Manager
	CSRF    *CSRFGuard
	Writer  response.Writer
}

// Private 包装需要登录的接口。
func (a *Authenticator) Private(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := a.Session.Read(r)
		if !ok {
			a.Writer.Fail(w, http.StatusUnauthorized, response.CodeUnauthorized, "登录状态已失效，请重新登录")
			return
		}
		if !a.CSRF.Check(r) {
			a.Writer.Fail(w, http.StatusForbidden, response.CodeForbidden, "CSRF 校验失败，请刷新页面后重试")
			return
		}
		a.CSRF.Ensure(w, r)
		next(w, withClaims(r, claims))
	}
}

// Public 包装无需登录的接口，但仍做 CSRF 校验。
func (a *Authenticator) Public(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.CSRF.Check(r) {
			a.Writer.Fail(w, http.StatusForbidden, response.CodeForbidden, "CSRF 校验失败，请刷新页面后重试")
			return
		}
		a.CSRF.Ensure(w, r)
		next(w, r)
	}
}

// Middleware 为所有请求下发 CSRF Cookie，保证前端首次加载即可取到 token。
func (g *CSRFGuard) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.Ensure(w, r)
		next.ServeHTTP(w, r)
	})
}
