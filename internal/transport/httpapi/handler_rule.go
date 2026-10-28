package httpapi

import (
	"net/http"

	"wafadmin/internal/service"
	"wafadmin/pkg/response"
)

// RuleHandler 处理 WAF 规则的增删改查与下发。
type RuleHandler struct {
	Rules *service.RuleService
	Sync  *service.SyncService
	// RuleSync 负责规则文件与数据库取并集。
	RuleSync *service.RuleFileSync
	// RuleDir 为规则目录（nginx 侧）。
	RuleDir string
	// RuleImportDir 为可选的导入目录（后台与 nginx 不同机时使用）。
	RuleImportDir string
	// Pipeline 提供检测顺序说明。
	Pipeline *service.RulePipelineService
	Writer   response.Writer
	Responder
}

// Register 注册规则相关路由。
func (h *RuleHandler) Register(mux *http.ServeMux, routes Routes) {
	mux.HandleFunc("GET /api/rules/types", routes.Private(h.types))
	mux.HandleFunc("GET /api/rules", routes.Private(h.list))
	mux.HandleFunc("GET /api/rules/pipeline", routes.Private(h.pipeline))
	mux.HandleFunc("GET /api/rules/stats", routes.Private(h.stats))
	mux.HandleFunc("GET /api/rules/stats/samples", routes.Private(h.samples))
	mux.HandleFunc("PUT /api/rules/{id}/enabled", routes.Private(h.setEnabled))
	mux.HandleFunc("POST /api/rules/sync-files", routes.Private(h.syncFiles))
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

// syncFiles 把规则目录与数据库规则取并集：缺少的新增，已有的保持不动。
func (h *RuleHandler) syncFiles(w http.ResponseWriter, r *http.Request) {
	if h.RuleSync == nil {
		h.Error(w, service.ErrInvalid.New("规则文件同步未启用：服务端缺少规则同步组件"))
		return
	}
	result, err := h.RuleSync.SyncFromDirs(r.Context(), []string{h.RuleDir, h.RuleImportDir})
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, result)
}

// pipeline 返回 WAF 检测与拦截顺序（只读说明）。
func (h *RuleHandler) pipeline(w http.ResponseWriter, r *http.Request) {
	if h.Pipeline == nil {
		h.Error(w, service.ErrInternal.New("检测顺序说明未启用"))
		return
	}
	result, err := h.Pipeline.Pipeline(r.Context())
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, result)
}

// stats 返回规则命中统计（默认最近 7 天）。
func (h *RuleHandler) stats(w http.ResponseWriter, r *http.Request) {
	query := NewQueryParams(r)
	days := query.Int("days", 7)
	limit := query.Int("limit", 20)

	items, err := h.Rules.TagStats(r.Context(), days, limit)
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, map[string]any{"days": days, "items": items})
}

// setEnabled 启用或停用一条规则。
func (h *RuleHandler) setEnabled(w http.ResponseWriter, r *http.Request) {
	payload := struct {
		Enabled bool `json:"enabled"`
	}{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}

	rule, err := h.Rules.SetEnabled(r.Context(), PathParams{Request: r}.Int64("id"), payload.Enabled)
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, rule)
}

// samples 返回某条命中规则的日志样例（用于判断是否误杀）。
func (h *RuleHandler) samples(w http.ResponseWriter, r *http.Request) {
	query := NewQueryParams(r)
	view, err := h.Rules.TagSamples(r.Context(), query.String("rule_tag"), query.Int("days", 7), query.Int("limit", 5))
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, view)
}
