package httpapi

import (
	"net/http"
	"strconv"

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
	mux.HandleFunc("GET /api/ai/results/{logId}", routes.Private(h.cachedResult))
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

// cachedResult 返回某条日志在服务端缓存中的研判结果（供多端查看）。
func (h *AIHandler) cachedResult(w http.ResponseWriter, r *http.Request) {
	logID, err := strconv.ParseInt(r.PathValue("logId"), 10, 64)
	if err != nil || logID <= 0 {
		h.Error(w, service.ErrInvalid.New("日志 ID 不合法"))
		return
	}

	result, err := h.AI.CachedResult(r.Context(), logID)
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, result)
}
