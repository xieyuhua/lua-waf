package httpapi

import (
	"net/http"

	"wafadmin/internal/service"
	"wafadmin/pkg/response"
)

// ApisixHandler 处理 APISIX 路由插件（镜像复制 / key 限流）的查询与下发。
type ApisixHandler struct {
	Apisix *service.ApisixService
	Writer response.Writer
	Responder
}

// Register 注册 APISIX 相关路由。
func (h *ApisixHandler) Register(mux *http.ServeMux, routes Routes) {
	mux.HandleFunc("GET /api/apisix/config", routes.Private(h.getConfig))
	mux.HandleFunc("PUT /api/apisix/config", routes.Private(h.saveConfig))
	mux.HandleFunc("GET /api/apisix/status", routes.Private(h.status))
	mux.HandleFunc("GET /api/apisix/routes", routes.Private(h.routes))
	mux.HandleFunc("PUT /api/apisix/routes/{id}/mirror", routes.Private(h.setMirror))
	mux.HandleFunc("DELETE /api/apisix/routes/{id}/mirror", routes.Private(h.clearMirror))
	mux.HandleFunc("PUT /api/apisix/routes/{id}/limit", routes.Private(h.setLimit))
	mux.HandleFunc("DELETE /api/apisix/routes/{id}/limit", routes.Private(h.clearLimit))
}

// status 返回管理接口连通状态。
func (h *ApisixHandler) status(w http.ResponseWriter, r *http.Request) {
	h.Data(w, h.Apisix.Status(r.Context()))
}

// routes 返回路由列表及其镜像/限流配置。
func (h *ApisixHandler) routes(w http.ResponseWriter, r *http.Request) {
	list, err := h.Apisix.Routes(r.Context())
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, list)
}

// setMirror 开启或更新镜像复制。
func (h *ApisixHandler) setMirror(w http.ResponseWriter, r *http.Request) {
	payload := service.ApisixMirrorPayload{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}

	id := PathParams{Request: r}.String("id")
	if err := h.Apisix.SetMirror(r.Context(), id, payload); err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, map[string]any{"message": "镜像复制已生效", "route_id": id})
}

// clearMirror 关闭镜像复制。
func (h *ApisixHandler) clearMirror(w http.ResponseWriter, r *http.Request) {
	id := PathParams{Request: r}.String("id")
	if err := h.Apisix.ClearPlugin(r.Context(), id, "proxy-mirror"); err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, map[string]any{"message": "镜像复制已关闭", "route_id": id})
}

// setLimit 开启或更新 key 限流。
func (h *ApisixHandler) setLimit(w http.ResponseWriter, r *http.Request) {
	payload := service.ApisixLimitPayload{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}

	id := PathParams{Request: r}.String("id")
	if err := h.Apisix.SetLimit(r.Context(), id, payload); err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, map[string]any{"message": "key 限流已生效", "route_id": id})
}

// clearLimit 关闭 key 限流。
func (h *ApisixHandler) clearLimit(w http.ResponseWriter, r *http.Request) {
	id := PathParams{Request: r}.String("id")
	if err := h.Apisix.ClearPlugin(r.Context(), id, "limit-count"); err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, map[string]any{"message": "key 限流已关闭", "route_id": id})
}

// getConfig 返回集成的启用状态与连接参数（密钥脱敏）。
func (h *ApisixHandler) getConfig(w http.ResponseWriter, r *http.Request) {
	h.Data(w, h.Apisix.Config())
}

// saveConfig 启用/关闭集成并保存连接参数（写回 conf/app.ini 并热更新）。
func (h *ApisixHandler) saveConfig(w http.ResponseWriter, r *http.Request) {
	payload := service.ApisixConfigPayload{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}

	view, err := h.Apisix.SaveConfig(r.Context(), payload)
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, view)
}
