package httpapi

import (
	"net/http"

	"wafadmin/internal/config"
	"wafadmin/internal/service"
)

// SystemHandler 提供系统运行信息与前端所需的元数据。
type SystemHandler struct {
	Cfg      *config.Config
	Rules    *service.RuleService
	Settings *service.SettingService
	Responder
}

// Register 注册系统信息路由。
func (h *SystemHandler) Register(mux *http.ServeMux, routes Routes) {
	mux.HandleFunc("GET /api/system/info", routes.Private(h.info))
}

// SystemInfo 是系统运行信息。
type SystemInfo struct {
	Version    string   `json:"version"`
	RunMode    string   `json:"run_mode"`
	Demo       bool     `json:"demo"`
	PageSize   int      `json:"page_size"`
	HTTPPort   int      `json:"http_port"`
	NginxBin   string   `json:"nginx_bin"`
	Vhosts     string   `json:"vhosts"`
	RulePath   string   `json:"rule_path"`
	APIServers []string `json:"api_servers"`
	// NginxDryRun 表示配置只渲染不落盘（演示模式）。
	NginxDryRun bool `json:"nginx_dry_run"`
}

// info 返回系统运行信息。
func (h *SystemHandler) info(w http.ResponseWriter, r *http.Request) {
	h.Data(w, SystemInfo{
		Version:     h.Cfg.AppVer,
		RunMode:     h.Cfg.RunMode,
		Demo:        h.Cfg.IsDemo(),
		PageSize:    h.Cfg.PageSize,
		HTTPPort:    h.Cfg.HTTPPort,
		NginxBin:    h.Cfg.NginxBin,
		Vhosts:      h.Cfg.NginxVhosts,
		RulePath:    h.Cfg.RulePath,
		APIServers:  h.Cfg.APIServers,
		NginxDryRun: h.Cfg.IsDemo(),
	})
}
