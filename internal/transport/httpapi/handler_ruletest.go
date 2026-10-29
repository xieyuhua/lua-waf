package httpapi

import (
	"net/http"

	"wafadmin/internal/ruletest"
	"wafadmin/internal/service"
	"wafadmin/pkg/response"
)

// RuleTestHandler 提供「规则能否命中」的自检接口。
type RuleTestHandler struct {
	// Rules 为规则服务（读取规则集合）。
	Rules *service.RuleService
	// Writer 解析请求体。
	Writer response.Writer
	// Responder 统一响应。
	Responder
}

// Register 注册规则自检路由。
func (h *RuleTestHandler) Register(mux *http.ServeMux, routes Routes) {
	mux.HandleFunc("POST /api/rules/selftest", routes.Private(h.selftest))
}

// selftestPayload 是自检入参。
type selftestPayload struct {
	// Payload 非空时为「载荷验证」模式：判断这条参数/URL 会命中哪些规则。
	Payload string `json:"payload"`
	// Type 只检查某一类规则。
	Type string `json:"type"`
	// All 为 true 时包含已停用的规则。
	All bool `json:"all"`
}

// selftest 执行规则自检并返回报告。
func (h *RuleTestHandler) selftest(w http.ResponseWriter, r *http.Request) {
	payload := selftestPayload{}
	// 允许空 body（等价于全量自检）
	_ = h.Writer.Bind(r, &payload)

	rules, err := h.Rules.List(r.Context(), "", "")
	if err != nil {
		h.Error(w, err)
		return
	}

	report := ruletest.Run(rules, "数据库 rules 表", ruletest.Options{
		Payload:         payload.Payload,
		Type:            payload.Type,
		IncludeDisabled: payload.All,
	})
	h.Data(w, report)
}
