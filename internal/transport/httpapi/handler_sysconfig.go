package httpapi

import (
	"net/http"

	"wafadmin/internal/service"
	"wafadmin/pkg/response"
)

// SysConfigHandler 处理运行配置（conf/app.ini）的查看、修改与重载。
type SysConfigHandler struct {
	SysConfig *service.SystemConfigService
	Writer    response.Writer
	Responder
}

// Register 注册运行配置相关路由。
func (h *SysConfigHandler) Register(mux *http.ServeMux, routes Routes) {
	mux.HandleFunc("GET /api/system/config", routes.Private(h.view))
	mux.HandleFunc("PUT /api/system/config", routes.Private(h.update))
	mux.HandleFunc("GET /api/system/config/raw", routes.Private(h.raw))
	mux.HandleFunc("POST /api/system/config/reload", routes.Private(h.reload))
}

// view 返回当前运行配置（密钥脱敏）。
func (h *SysConfigHandler) view(w http.ResponseWriter, r *http.Request) {
	result, err := h.SysConfig.View(r.Context())
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, result)
}

// update 保存配置并热更新。
func (h *SysConfigHandler) update(w http.ResponseWriter, r *http.Request) {
	payload := service.ConfigPayload{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}
	result, err := h.SysConfig.Update(r.Context(), payload)
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, result)
}

// raw 返回配置文件原文。
func (h *SysConfigHandler) raw(w http.ResponseWriter, r *http.Request) {
	content, err := h.SysConfig.Raw(r.Context())
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, map[string]string{"content": content})
}

// reload 从磁盘重新加载并应用配置。
func (h *SysConfigHandler) reload(w http.ResponseWriter, r *http.Request) {
	result, err := h.SysConfig.Reload(r.Context())
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, result)
}
