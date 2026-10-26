package httpapi

import (
	"net/http"

	"wafadmin/internal/service"
	"wafadmin/pkg/response"
)

// AIHandler 处理 AI 配置与日志智能分析。
type AIHandler struct {
	AI     *service.AIService
	Writer response.Writer
	Responder
}

// Register 注册 AI 相关路由。
func (h *AIHandler) Register(mux *http.ServeMux, routes Routes) {
	mux.HandleFunc("GET /api/ai/config", routes.Private(h.config))
	mux.HandleFunc("PUT /api/ai/config", routes.Private(h.saveConfig))
	mux.HandleFunc("POST /api/ai/test", routes.Private(h.test))
	mux.HandleFunc("POST /api/ai/analyze", routes.Private(h.analyze))
}

// config 返回 AI 配置（密钥脱敏）。
func (h *AIHandler) config(w http.ResponseWriter, r *http.Request) {
	status, err := h.AI.Status(r.Context())
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, status)
}

// saveConfig 保存 AI 配置。
func (h *AIHandler) saveConfig(w http.ResponseWriter, r *http.Request) {
	payload := service.AIConfig{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}
	saved, err := h.AI.SaveConfig(r.Context(), payload)
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, map[string]any{"config": saved.Masked(), "message": "配置已保存"})
}

// test 测试大模型接口连通性。
func (h *AIHandler) test(w http.ResponseWriter, r *http.Request) {
	result, err := h.AI.Test(r.Context())
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, result)
}

// analyze 对指定日志进行 AI 分析。
func (h *AIHandler) analyze(w http.ResponseWriter, r *http.Request) {
	payload := service.AnalyzeLogsRequest{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}
	result, err := h.AI.AnalyzeLogs(r.Context(), payload)
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, result)
}
