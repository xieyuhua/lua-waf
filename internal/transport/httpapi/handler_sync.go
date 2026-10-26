package httpapi

import (
	"net/http"

	"wafadmin/internal/service"
	"wafadmin/pkg/response"
)

// syncPayload 是后台主动同步接口的响应。
type syncPayload struct {
	// Applied 为本机实际应用的配置条数。
	Applied int `json:"applied"`
	// Message 为结果说明。
	Message string `json:"message"`
	// Nodes 为集群各节点的返回结果。
	Nodes []service.SyncResult `json:"nodes"`
}

// legacySyncResponse 是 1.x 版本的同步响应格式，为兼容老节点保留。
type legacySyncResponse struct {
	Status  int    `json:"Status"`
	Message string `json:"Message"`
}

// SyncHandler 处理节点之间的配置同步。
// 这些接口不使用会话，而是通过 API_KEY 签名校验来源。
type SyncHandler struct {
	Sync   *service.SyncService
	Sites  *service.SiteService
	Rules  *service.RuleService
	Slow   *service.SlowSQLService
	Writer response.Writer
}

// Register 注册节点同步路由（含 1.x 兼容路径）。
func (h *SyncHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/sync/site", h.applySites)
	mux.HandleFunc("GET /api/sync/rule", h.applyRules)
	mux.HandleFunc("POST /api/sync/slow-sql", h.reportSlowSQL)

	// 1.x 兼容：老节点仍会回调这两个地址
	mux.HandleFunc("GET /api/site/sync/", h.applySites)
	mux.HandleFunc("GET /api/rule/sync/", h.applyRules)
}

// applySites 接收其他节点下发的站点同步指令。
func (h *SyncHandler) applySites(w http.ResponseWriter, r *http.Request) {
	query := NewQueryParams(r)
	if !h.Sync.Verify(query.String("timestamp"), query.String("hash")) {
		h.legacy(w, http.StatusForbidden, 2, "invalid hash parameter")
		return
	}

	siteID := query.Int64("id", 0)
	applied, err := h.Sync.ApplySites(r.Context(), siteID)
	if err != nil {
		h.legacy(w, http.StatusOK, 1, err.Error())
		return
	}
	h.legacy(w, http.StatusOK, 0, "successful")
	_ = applied
}

// applyRules 接收其他节点下发的规则同步指令。
func (h *SyncHandler) applyRules(w http.ResponseWriter, r *http.Request) {
	query := NewQueryParams(r)
	if !h.Sync.Verify(query.String("timestamp"), query.String("hash")) {
		h.legacy(w, http.StatusForbidden, 2, "invalid hash parameter")
		return
	}
	if _, err := h.Sync.ApplyRules(r.Context()); err != nil {
		h.legacy(w, http.StatusOK, 1, err.Error())
		return
	}
	h.legacy(w, http.StatusOK, 0, "successful")
}

// reportSlowSQL 接收外部采集器上报的慢 SQL。
func (h *SyncHandler) reportSlowSQL(w http.ResponseWriter, r *http.Request) {
	query := NewQueryParams(r)
	if !h.Sync.Verify(query.String("timestamp"), query.String("hash")) {
		h.legacy(w, http.StatusForbidden, 2, "invalid hash parameter")
		return
	}

	payload := service.SlowSQLReportPayload{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.Writer.Fail(w, http.StatusBadRequest, response.CodeInvalid, "请求参数格式不正确")
		return
	}
	item, err := h.Slow.Report(r.Context(), payload)
	if err != nil {
		if svcErr, ok := service.AsError(err); ok {
			h.Writer.Fail(w, svcErr.Status(), svcErr.Code(), svcErr.Message)
			return
		}
		h.Writer.Fail(w, http.StatusInternalServerError, response.CodeInternal, err.Error())
		return
	}
	h.Writer.OK(w, item)
}

// legacy 输出 1.x 兼容格式的响应。
func (h *SyncHandler) legacy(w http.ResponseWriter, status, code int, message string) {
	if status != http.StatusOK {
		w.WriteHeader(status)
	}
	h.Writer.OK(w, legacySyncResponse{Status: code, Message: message})
}
