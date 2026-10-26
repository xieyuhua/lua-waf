package httpapi

import (
	"net/http"

	"wafadmin/internal/model"
	"wafadmin/internal/service"
	"wafadmin/pkg/response"
)

// LogHandler 处理攻击日志的查询、筛选与统计。
type LogHandler struct {
	Logs   *service.LogService
	Sites  *service.SiteService
	Writer response.Writer
	Responder
}

// Register 注册日志相关路由。
func (h *LogHandler) Register(mux *http.ServeMux, routes Routes) {
	mux.HandleFunc("GET /api/logs", routes.Private(h.list))
	mux.HandleFunc("GET /api/logs/overview", routes.Private(h.overview))
	mux.HandleFunc("GET /api/logs/filters", routes.Private(h.filters))
	mux.HandleFunc("GET /api/logs/ips", routes.Private(h.ips))
	mux.HandleFunc("GET /api/logs/types", routes.Private(h.types))
	mux.HandleFunc("GET /api/logs/dates", routes.Private(h.dates))
	mux.HandleFunc("GET /api/logs/{id}", routes.Private(h.detail))
}

// list 按筛选条件分页查询日志。
func (h *LogHandler) list(w http.ResponseWriter, r *http.Request) {
	query := NewQueryParams(r)
	payload := model.LogQuery{
		ListQuery:  query.Page(),
		AttackType: query.String("attack_type"),
		ClientIp:   query.String("client_ip"),
		ServerName: query.String("server_name"),
		Day:        query.String("day"),
		From:       query.String("from"),
		To:         query.String("to"),
	}
	result, err := h.Logs.List(r.Context(), payload)
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, result)
}

// overview 返回日志统计摘要。
func (h *LogHandler) overview(w http.ResponseWriter, r *http.Request) {
	days := NewQueryParams(r).Int("days", 15)
	overview, err := h.Logs.Overview(r.Context(), days)
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, overview)
}

// filters 返回筛选项（攻击类型、站点列表）。
func (h *LogHandler) filters(w http.ResponseWriter, r *http.Request) {
	types, err := h.Logs.AttackTypes(r.Context())
	if err != nil {
		h.Error(w, err)
		return
	}
	sites, err := h.Sites.All(r.Context())
	if err != nil {
		h.Error(w, err)
		return
	}

	names := make([]string, 0, len(sites))
	for _, site := range sites {
		names = append(names, site.SiteName)
	}
	h.Data(w, map[string]any{"attack_types": types, "sites": names})
}

// ips 返回按来源 IP 聚合的统计。
func (h *LogHandler) ips(w http.ResponseWriter, r *http.Request) {
	query := NewQueryParams(r)
	stats, err := h.Logs.IPSources(r.Context(), query.String("day"), query.Int("limit", 100))
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, stats)
}

// types 返回按攻击类型聚合的统计。
func (h *LogHandler) types(w http.ResponseWriter, r *http.Request) {
	stats, err := h.Logs.TypeStats(r.Context(), NewQueryParams(r).String("day"))
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, stats)
}

// dates 返回最近若干天的攻击趋势。
func (h *LogHandler) dates(w http.ResponseWriter, r *http.Request) {
	trend, err := h.Logs.Trend(r.Context(), NewQueryParams(r).Int("days", 15))
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, trend)
}

// detail 返回单条日志详情。
func (h *LogHandler) detail(w http.ResponseWriter, r *http.Request) {
	item, err := h.Logs.Detail(r.Context(), PathParams{Request: r}.Int64("id"))
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, item)
}
