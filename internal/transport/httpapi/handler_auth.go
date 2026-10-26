package httpapi

import (
	"net/http"

	"wafadmin/internal/model"
	"wafadmin/internal/service"
	"wafadmin/pkg/response"
	"wafadmin/pkg/session"
)

// AuthHandler 处理登录、注销、验证码与当前账号信息。
type AuthHandler struct {
	Auth    *service.AuthService
	Session *session.Manager
	Writer  response.Writer
	Responder
}

// Register 注册认证相关路由。
func (h *AuthHandler) Register(mux *http.ServeMux, routes Routes) {
	mux.HandleFunc("GET /api/auth/captcha", routes.Public(h.captcha))
	mux.HandleFunc("POST /api/auth/login", routes.Public(h.login))
	mux.HandleFunc("POST /api/auth/logout", routes.Private(h.logout))
	mux.HandleFunc("GET /api/auth/profile", routes.Private(h.profile))
}

// captcha 生成图形验证码。
func (h *AuthHandler) captcha(w http.ResponseWriter, r *http.Request) {
	challenge, err := h.Auth.NewCaptcha()
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, challenge)
}

// login 校验账号口令并写入会话。
func (h *AuthHandler) login(w http.ResponseWriter, r *http.Request) {
	payload := service.LoginRequest{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}
	payload.ClientIp = clientIP(r)

	user, err := h.Auth.Login(r.Context(), payload)
	if err != nil {
		h.Error(w, err)
		return
	}
	if err := h.Session.Issue(w, user.Id, user.UserName); err != nil {
		h.Writer.Fail(w, http.StatusInternalServerError, response.CodeInternal, "签发会话失败")
		return
	}
	h.Writer.OK(w, map[string]any{
		"user":     user.Display(),
		"redirect": "/admin/dashboard",
	})
}

// logout 清除会话。
func (h *AuthHandler) logout(w http.ResponseWriter, r *http.Request) {
	h.Session.Clear(w)
	h.Message(w, "已退出登录")
}

// profile 返回当前登录账号信息。
func (h *AuthHandler) profile(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.Profile(r.Context(), CurrentUID(r))
	if err != nil {
		h.Session.Clear(w)
		h.Error(w, err)
		return
	}
	h.Data(w, userView(user))
}

// userView 返回脱敏后的账号信息。
func userView(user *model.User) model.UserView {
	return user.Display()
}
