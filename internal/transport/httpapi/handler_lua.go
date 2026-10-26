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
	payload := struct {
		Values map[string]string `json:"values"`
	}{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}

	result, err := h.Lua.Update(r.Context(), payload.Values)
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
