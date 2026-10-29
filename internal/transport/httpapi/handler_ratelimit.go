package httpapi

import (
	"net/http"
	"strconv"

	"wafadmin/internal/model"
	"wafadmin/internal/service"
	"wafadmin/pkg/response"
)

// RateLimitHandler 提供「按 URL 限频」规则的接口。
type RateLimitHandler struct {
	// Rules 为限频规则服务。
	Rules *service.RateLimitService
	// Writer 负责解析请求体。
	Writer response.Writer
	// Responder 负责统一响应（含 BindError）。
	Responder
}

// Register 注册限频相关路由。
func (h *RateLimitHandler) Register(mux *http.ServeMux, routes Routes) {
	mux.HandleFunc("GET /api/ratelimit", routes.Private(h.list))
	mux.HandleFunc("POST /api/ratelimit", routes.Private(h.create))
	mux.HandleFunc("PUT /api/ratelimit/{id}", routes.Private(h.update))
	mux.HandleFunc("PUT /api/ratelimit/{id}/enabled", routes.Private(h.setEnabled))
	mux.HandleFunc("DELETE /api/ratelimit/{id}", routes.Private(h.remove))
	mux.HandleFunc("POST /api/ratelimit/sync", routes.Private(h.sync))
}

// list 返回全部规则。
func (h *RateLimitHandler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.Rules.List(r.Context())
	if err != nil {
		h.Responder.Error(w, err)
		return
	}
	h.Responder.Data(w, map[string]any{
		"items":    items,
		"rule_dir": h.Rules.RuleDir,
	})
}

// create 新建规则。
func (h *RateLimitHandler) create(w http.ResponseWriter, r *http.Request) {
	payload := model.RateLimitRule{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}

	item, err := h.Rules.Create(r.Context(), payload)
	if err != nil {
		h.Responder.Error(w, err)
		return
	}
	h.Responder.Data(w, item)
}

// update 更新规则。
func (h *RateLimitHandler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseId(w, r)
	if !ok {
		return
	}

	payload := model.RateLimitRule{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}

	item, err := h.Rules.Update(r.Context(), id, payload)
	if err != nil {
		h.Responder.Error(w, err)
		return
	}
	h.Responder.Data(w, item)
}

// setEnabled 启停规则。
func (h *RateLimitHandler) setEnabled(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseId(w, r)
	if !ok {
		return
	}

	payload := struct {
		Enabled bool `json:"enabled"`
	}{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}

	if err := h.Rules.SetEnabled(r.Context(), id, payload.Enabled); err != nil {
		h.Responder.Error(w, err)
		return
	}
	h.Responder.Data(w, map[string]any{"id": id, "enabled": payload.Enabled})
}

// remove 删除规则。
func (h *RateLimitHandler) remove(w http.ResponseWriter, r *http.Request) {
	id, ok := h.parseId(w, r)
	if !ok {
		return
	}

	if err := h.Rules.Delete(r.Context(), id); err != nil {
		h.Responder.Error(w, err)
		return
	}
	h.Responder.Data(w, map[string]any{"id": id})
}

// sync 手动下发规则文件。
func (h *RateLimitHandler) sync(w http.ResponseWriter, r *http.Request) {
	result, err := h.Rules.Sync(r.Context())
	if err != nil {
		h.Responder.Error(w, err)
		return
	}
	h.Responder.Data(w, result)
}

// parseId 解析路径里的规则 ID。
func (h *RateLimitHandler) parseId(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := r.PathValue("id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		h.Responder.Error(w, service.ErrInvalid.New("规则 ID 不合法"))
		return 0, false
	}
	return id, true
}
