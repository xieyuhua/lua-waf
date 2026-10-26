package httpapi

import (
	"net/http"

	"wafadmin/internal/service"
	"wafadmin/pkg/response"
)

// SlowSQLHandler 处理慢 SQL 统计列表与清理。
type SlowSQLHandler struct {
	SlowSQL *service.SlowSQLService
	Writer  response.Writer
	Responder
}

// Register 注册慢 SQL 相关路由。
func (h *SlowSQLHandler) Register(mux *http.ServeMux, routes Routes) {
	mux.HandleFunc("GET /api/slow-sql", routes.Private(h.list))
	mux.HandleFunc("GET /api/slow-sql/stats", routes.Private(h.stats))
	mux.HandleFunc("POST /api/slow-sql/purge", routes.Private(h.purge))
}

// list 按时间范围分页查询慢 SQL。
func (h *SlowSQLHandler) list(w http.ResponseWriter, r *http.Request) {
	result, err := h.SlowSQL.List(r.Context(), h.queryPayload(r))
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, result)
}

// stats 返回慢 SQL 统计指标。
func (h *SlowSQLHandler) stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.SlowSQL.Stats(r.Context(), h.queryPayload(r))
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, stats)
}

// purge 清理指定天数之前的慢 SQL 记录。
func (h *SlowSQLHandler) purge(w http.ResponseWriter, r *http.Request) {
	days := NewQueryParams(r).Int("days", 30)
	removed, err := h.SlowSQL.Purge(r.Context(), days)
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, map[string]any{"removed": removed, "days": days})
}

// queryPayload 从查询参数构造慢 SQL 检索条件。
func (h *SlowSQLHandler) queryPayload(r *http.Request) service.SlowSQLQueryPayload {
	query := NewQueryParams(r)
	page := query.Page()
	return service.SlowSQLQueryPayload{
		Page:    page.Page,
		Size:    page.Size,
		Keyword: page.Keyword,
		From:    query.String("from"),
		To:      query.String("to"),
		Source:  query.String("source"),
		MinMs:   query.Int64("min_ms", 0),
		SortBy:  query.String("sort_by"),
		Order:   query.String("order"),
	}
}
