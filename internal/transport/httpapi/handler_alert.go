package httpapi

import (
	"net/http"

	"wafadmin/internal/service"
	"wafadmin/pkg/response"
)

// AlertHandler 提供「命中规则 → 企业微信机器人提醒」的配置与测试接口。
type AlertHandler struct {
	// Alert 为告警服务。
	Alert *service.AlertService
	// Writer 负责解析请求体。
	Writer response.Writer
	// Responder 负责统一响应。
	Responder
}

// Register 注册告警相关路由。
func (h *AlertHandler) Register(mux *http.ServeMux, routes Routes) {
	mux.HandleFunc("GET /api/alert/config", routes.Private(h.view))
	mux.HandleFunc("PUT /api/alert/config", routes.Private(h.save))
	mux.HandleFunc("POST /api/alert/test", routes.Private(h.test))
}

// view 返回告警配置与运行状态。
func (h *AlertHandler) view(w http.ResponseWriter, r *http.Request) {
	status, err := h.Alert.View(r.Context())
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, status)
}

// save 保存告警配置。
func (h *AlertHandler) save(w http.ResponseWriter, r *http.Request) {
	payload := service.AlertConfig{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}

	status, err := h.Alert.SaveConfig(r.Context(), payload)
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, status)
}

// test 发送一条测试消息（用请求体里的配置，未传则用已保存的配置）。
func (h *AlertHandler) test(w http.ResponseWriter, r *http.Request) {
	payload := service.AlertConfig{}
	// 允许空 body：此时用已保存的配置
	_ = h.Writer.Bind(r, &payload)

	if err := h.Alert.Test(r.Context(), payload); err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, map[string]any{"message": "测试消息已发送，请查看企业微信群"})
}
