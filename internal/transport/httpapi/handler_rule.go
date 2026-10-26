package httpapi

import (
	"net/http"

	"wafadmin/internal/service"
	"wafadmin/pkg/response"
)

// RuleHandler 处理 WAF 规则的增删改查与下发。
type RuleHandler struct {
	Rules  *service.RuleService
	Sync   *service.SyncService
	Writer response.Writer
	Responder
}

// Register 注册规则相关路由。
func (h *RuleHandler) Register(mux *http.ServeMux, routes Routes) {
	mux.HandleFunc("GET /api/rules/types", routes.Private(h.types))
	mux.HandleFunc("GET /api/rules", routes.Private(h.list))
	mux.HandleFunc("POST /api/rules", routes.Private(h.create))
	mux.HandleFunc("PUT /api/rules/{id}", routes.Private(h.update))
	mux.HandleFunc("DELETE /api/rules/{id}", routes.Private(h.remove))
	mux.HandleFunc("POST /api/rules/validate", routes.Private(h.validate))
	mux.HandleFunc("POST /api/rules/sync", routes.Private(h.syncAll))
}

// types 返回规则类型概览（用于子菜单与数量徽标）。
func (h *RuleHandler) types(w http.ResponseWriter, r *http.Request) {
	views, err := h.Rules.Overview(r.Context())
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, views)
}

// list 按类型与关键词查询规则。
func (h *RuleHandler) list(w http.ResponseWriter, r *http.Request) {
	query := NewQueryParams(r)
	items, err := h.Rules.List(r.Context(), query.String("type"), query.String("keyword"))
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, map[string]any{
		"items": items,
		"total": len(items),
		"type":  query.String("type"),
	})
}

// create 新增规则。
func (h *RuleHandler) create(w http.ResponseWriter, r *http.Request) {
	payload := service.RulePayload{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}
	rule, err := h.Rules.Create(r.Context(), payload)
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, rule)
}

// update 修改规则。
func (h *RuleHandler) update(w http.ResponseWriter, r *http.Request) {
	payload := service.RulePayload{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}
	rule, err := h.Rules.Update(r.Context(), PathParams{Request: r}.Int64("id"), payload.RuleItem)
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, rule)
}

// remove 删除规则。
func (h *RuleHandler) remove(w http.ResponseWriter, r *http.Request) {
	if err := h.Rules.Delete(r.Context(), PathParams{Request: r}.Int64("id")); err != nil {
		h.Error(w, err)
		return
	}
	h.Message(w, "规则已删除")
}

// validate 校验规则内容并返回格式告警。
func (h *RuleHandler) validate(w http.ResponseWriter, r *http.Request) {
	payload := service.RulePayload{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}
	warning, err := h.Rules.Validate(payload.RuleItem)
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, map[string]string{"warning": warning})
}

// syncAll 下发全部规则到本机与集群节点。
func (h *RuleHandler) syncAll(w http.ResponseWriter, r *http.Request) {
	written, err := h.Sync.ApplyRules(r.Context())
	if err != nil {
		h.Error(w, err)
		return
	}
	nodes, pushErr := h.Sync.PushRules(r.Context())

	payload := syncPayload{Applied: written, Nodes: nodes, Message: "规则文件已生成"}
	if pushErr != nil {
		payload.Message = pushErr.Error()
	}
	h.Data(w, payload)
}
