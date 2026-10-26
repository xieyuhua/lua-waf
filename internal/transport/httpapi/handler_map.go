package httpapi

import (
	"net/http"

	"wafadmin/internal/service"
)

// MapHandler 处理攻击地图大屏数据。
type MapHandler struct {
	Map *service.AttackMapService
	Responder
}

// Register 注册攻击地图路由。
func (h *MapHandler) Register(mux *http.ServeMux, routes Routes) {
	mux.HandleFunc("GET /api/attack-map", routes.Private(h.snapshot))
}

// snapshot 返回攻击地图快照，range 支持 today / all。
func (h *MapHandler) snapshot(w http.ResponseWriter, r *http.Request) {
	day := NewQueryParams(r).String("range")
	snapshot, err := h.Map.Snapshot(r.Context(), day)
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, snapshot)
}
