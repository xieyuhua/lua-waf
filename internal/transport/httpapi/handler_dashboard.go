package httpapi

import (
	"net/http"

	"wafadmin/internal/service"
)

// DashboardHandler 处理首页看板数据。
type DashboardHandler struct {
	Dashboard *service.DashboardService
	Responder
}

// Register 注册看板路由。
func (h *DashboardHandler) Register(mux *http.ServeMux, routes Routes) {
	mux.HandleFunc("GET /api/dashboard/summary", routes.Private(h.summary))
}

// summary 返回看板汇总数据。
func (h *DashboardHandler) summary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.Dashboard.Summary(r.Context())
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, summary)
}
