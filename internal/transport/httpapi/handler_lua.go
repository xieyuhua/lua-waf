package httpapi

import (
	"net/http"

	"wafadmin/internal/service"
	"wafadmin/pkg/response"
)

// LuaConfigHandler 处理 lua-waf config.lua 的查看与修改。
type LuaConfigHandler struct {
	Lua    *service.LuaConfigService
	Writer response.Writer
	Responder
}

// Register 注册 lua 配置相关路由。
func (h *LuaConfigHandler) Register(mux *http.ServeMux, routes Routes) {
	mux.HandleFunc("GET /api/lua/config", routes.Private(h.view))
	mux.HandleFunc("PUT /api/lua/config", routes.Private(h.update))
	mux.HandleFunc("GET /api/lua/config/raw", routes.Private(h.raw))
	mux.HandleFunc("POST /api/lua/config/test", routes.Private(h.testConnections))
	mux.HandleFunc("POST /api/lua/reload", routes.Private(h.reload))
}

// view 返回配置项（密钥脱敏）。
func (h *LuaConfigHandler) view(w http.ResponseWriter, r *http.Request) {
	result, err := h.Lua.View(r.Context())
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, result)
}

// update 保存配置项并返回最新视图。
func (h *LuaConfigHandler) update(w http.ResponseWriter, r *http.Request) {
	// 前端 el-input-number 提交数字、el-switch 提交 "on"/"off"，
	// 这里用 any 接住再统一转字符串，避免数字导致「请求参数格式不正确」
	payload := struct {
		Values map[string]any `json:"values"`
	}{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}

	result, err := h.Lua.Update(r.Context(), service.CoerceLuaValues(payload.Values))
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, result)
}

// raw 返回 config.lua 原文。
func (h *LuaConfigHandler) raw(w http.ResponseWriter, r *http.Request) {
	content, err := h.Lua.Raw(r.Context())
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, map[string]string{"content": content})
}

// reload 让修改生效（nginx -t && nginx -s reload）。
func (h *LuaConfigHandler) reload(w http.ResponseWriter, r *http.Request) {
	output, err := h.Lua.Reload(r.Context())
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, map[string]any{"message": "配置已生效", "output": output})
}

// testConnections 用表单当前值测试 Redis / MySQL 连通性（保存前自检）。
func (h *LuaConfigHandler) testConnections(w http.ResponseWriter, r *http.Request) {
	payload := struct {
		Values map[string]any `json:"values"`
	}{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}

	result, err := h.Lua.TestConnections(r.Context(), service.CoerceLuaValues(payload.Values))
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, result)
}
