package httpapi

import (
	"net/http"

	"wafadmin/internal/service"
	"wafadmin/pkg/response"
)

// UserHandler 处理后台账号管理。
type UserHandler struct {
	Users  *service.UserService
	Writer response.Writer
	Responder
}

// Register 注册账号相关路由。
func (h *UserHandler) Register(mux *http.ServeMux, routes Routes) {
	mux.HandleFunc("GET /api/users", routes.Private(h.list))
	mux.HandleFunc("POST /api/users", routes.Private(h.create))
	mux.HandleFunc("PUT /api/users/{id}", routes.Private(h.update))
	mux.HandleFunc("DELETE /api/users/{id}", routes.Private(h.remove))
	mux.HandleFunc("POST /api/users/password", routes.Private(h.changePassword))
}

// list 分页查询账号。
func (h *UserHandler) list(w http.ResponseWriter, r *http.Request) {
	result, err := h.Users.List(r.Context(), NewQueryParams(r).Page())
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, result)
}

// create 新增账号。
func (h *UserHandler) create(w http.ResponseWriter, r *http.Request) {
	payload := service.CreateUserRequest{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}
	user, err := h.Users.Create(r.Context(), payload)
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, user.Display())
}

// update 修改账号。
func (h *UserHandler) update(w http.ResponseWriter, r *http.Request) {
	payload := service.UpdateUserRequest{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}
	user, err := h.Users.Update(r.Context(), PathParams{Request: r}.Int64("id"), payload)
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, user.Display())
}

// remove 删除账号。
func (h *UserHandler) remove(w http.ResponseWriter, r *http.Request) {
	if err := h.Users.Delete(r.Context(), PathParams{Request: r}.Int64("id"), CurrentUID(r)); err != nil {
		h.Error(w, err)
		return
	}
	h.Message(w, "账号已删除")
}

// changePassword 修改当前登录账号的口令。
func (h *UserHandler) changePassword(w http.ResponseWriter, r *http.Request) {
	payload := service.ChangePasswordRequest{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}
	if err := h.Users.ChangePassword(r.Context(), CurrentUID(r), payload); err != nil {
		h.Error(w, err)
		return
	}
	h.Message(w, "口令已更新")
}
