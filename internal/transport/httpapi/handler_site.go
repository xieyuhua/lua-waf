package httpapi

import (
	"net/http"

	"wafadmin/internal/service"
	"wafadmin/pkg/response"
)

// SiteHandler 处理受保护站点的增删改查与配置下发。
type SiteHandler struct {
	Sites  *service.SiteService
	Sync   *service.SyncService
	Writer response.Writer
	Responder
}

// Register 注册站点相关路由。
func (h *SiteHandler) Register(mux *http.ServeMux, routes Routes) {
	mux.HandleFunc("GET /api/sites", routes.Private(h.list))
	mux.HandleFunc("GET /api/sites/all", routes.Private(h.all))
	mux.HandleFunc("POST /api/sites", routes.Private(h.create))
	mux.HandleFunc("GET /api/sites/{id}", routes.Private(h.get))
	mux.HandleFunc("PUT /api/sites/{id}", routes.Private(h.update))
	mux.HandleFunc("DELETE /api/sites/{id}", routes.Private(h.remove))
	mux.HandleFunc("GET /api/sites/{id}/preview", routes.Private(h.preview))
	mux.HandleFunc("POST /api/sites/sync", routes.Private(h.syncAll))
	mux.HandleFunc("POST /api/sites/{id}/sync", routes.Private(h.syncOne))
}

// list 分页查询站点。
func (h *SiteHandler) list(w http.ResponseWriter, r *http.Request) {
	result, err := h.Sites.List(r.Context(), NewQueryParams(r).Page())
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, result)
}

// all 返回全部站点（用于下拉选择）。
func (h *SiteHandler) all(w http.ResponseWriter, r *http.Request) {
	items, err := h.Sites.All(r.Context())
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, items)
}

// get 查询单个站点。
func (h *SiteHandler) get(w http.ResponseWriter, r *http.Request) {
	site, err := h.Sites.Get(r.Context(), PathParams{Request: r}.Int64("id"))
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, site)
}

// create 新增站点。
func (h *SiteHandler) create(w http.ResponseWriter, r *http.Request) {
	payload := service.SitePayload{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}
	site, err := h.Sites.Create(r.Context(), payload)
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, site)
}

// update 修改站点。
func (h *SiteHandler) update(w http.ResponseWriter, r *http.Request) {
	payload := service.SitePayload{}
	if err := h.Writer.Bind(r, &payload); err != nil {
		h.BindError(w)
		return
	}
	site, err := h.Sites.Update(r.Context(), PathParams{Request: r}.Int64("id"), payload)
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, site)
}

// remove 删除站点。
func (h *SiteHandler) remove(w http.ResponseWriter, r *http.Request) {
	if err := h.Sites.Delete(r.Context(), PathParams{Request: r}.Int64("id")); err != nil {
		h.Error(w, err)
		return
	}
	h.Message(w, "站点已删除")
}

// preview 返回渲染后的 nginx 配置。
func (h *SiteHandler) preview(w http.ResponseWriter, r *http.Request) {
	content, err := h.Sites.Preview(r.Context(), PathParams{Request: r}.Int64("id"))
	if err != nil {
		h.Error(w, err)
		return
	}
	h.Data(w, map[string]string{"content": content})
}

// syncAll 同步全部站点配置。
func (h *SiteHandler) syncAll(w http.ResponseWriter, r *http.Request) {
	h.doSync(w, r, 0)
}

// syncOne 同步单个站点配置。
func (h *SiteHandler) syncOne(w http.ResponseWriter, r *http.Request) {
	h.doSync(w, r, PathParams{Request: r}.Int64("id"))
}

// doSync 先在本机应用配置，再推送到集群中的其他节点。
func (h *SiteHandler) doSync(w http.ResponseWriter, r *http.Request, siteID int64) {
	applied, err := h.Sync.ApplySites(r.Context(), siteID)
	if err != nil {
		h.Error(w, err)
		return
	}
	nodes, pushErr := h.Sync.PushSites(r.Context(), siteID)

	payload := syncPayload{Applied: applied, Nodes: nodes}
	if pushErr != nil {
		payload.Message = pushErr.Error()
	} else {
		payload.Message = "本机配置已生效"
	}
	h.Data(w, payload)
}
