// Package httpapi 是 wafadmin 的 HTTP 接入层。
// 只负责协议转换、参数绑定与路由编排，业务逻辑全部位于 service 层。
package httpapi

import (
	"io/fs"
	"net/http"

	"wafadmin/internal/config"
	"wafadmin/internal/ingest"
	"wafadmin/internal/service"
	"wafadmin/pkg/response"
	"wafadmin/pkg/session"
)

// Deps 汇总各业务域服务，由 app 层装配后注入。
type Deps struct {
	Auth      *service.AuthService
	Sites     *service.SiteService
	Rules     *service.RuleService
	Users     *service.UserService
	Logs      *service.LogService
	Dashboard *service.DashboardService
	SlowSQL   *service.SlowSQLService
	AI        *service.AIService
	Map       *service.AttackMapService
	Sync      *service.SyncService
	Settings  *service.SettingService
	SysConfig *service.SystemConfigService
	LuaConfig *service.LuaConfigService
	Ingest    *ingest.Consumer
	RuleSync  *service.RuleFileSync
	Pipeline  *service.RulePipelineService
	// RateLimit 为按 URL 的精细化限频规则服务。
	RateLimit *service.RateLimitService
	// Alert 为「命中规则 → 企业微信机器人提醒」服务。
	Alert *service.AlertService
}

// Routes 提供统一的中间件包装器，各 handler 按需使用。
type Routes struct {
	// Public 用于无需登录的接口（登录、验证码）。
	Public func(http.HandlerFunc) http.HandlerFunc
	// Private 用于需要登录的接口。
	Private func(http.HandlerFunc) http.HandlerFunc
}

// Server 负责路由注册与中间件编排。
type Server struct {
	Cfg     *config.Config
	Deps    Deps
	Session *session.Manager
	CSRF    *CSRFGuard
	Writer  response.Writer
	// Static 为内嵌的前端构建产物（web/dist）。
	Static fs.FS
}

// Handler 返回装配好中间件的根处理器。
func (s *Server) Handler() http.Handler {
	if s.CSRF == nil {
		s.CSRF = &CSRFGuard{}
	}

	mux := http.NewServeMux()
	s.register(mux)

	var handler http.Handler = mux
	handler = s.CSRF.Middleware(handler)
	handler = SecurityHeaders{}.Wrap(handler)
	handler = AccessLogger{}.Wrap(handler)
	handler = Recoverer{Writer: s.Writer}.Wrap(handler)
	return handler
}

// register 注册全部路由。
func (s *Server) register(mux *http.ServeMux) {
	authenticator := &Authenticator{Session: s.Session, CSRF: s.CSRF, Writer: s.Writer}
	routes := Routes{Public: authenticator.Public, Private: authenticator.Private}

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		s.Writer.OK(w, map[string]string{"status": "ok"})
	})

	(&AuthHandler{Auth: s.Deps.Auth, Session: s.Session, Writer: s.Writer, Responder: Responder{Writer: s.Writer}}).Register(mux, routes)
	(&SiteHandler{Sites: s.Deps.Sites, Sync: s.Deps.Sync, Responder: Responder{Writer: s.Writer}, Writer: s.Writer}).Register(mux, routes)
	(&RuleHandler{Rules: s.Deps.Rules, Sync: s.Deps.Sync, RuleSync: s.Deps.RuleSync, RuleDir: s.Cfg.RulePath, RuleImportDir: s.Cfg.RuleImportDir, Pipeline: s.Deps.Pipeline, Responder: Responder{Writer: s.Writer}, Writer: s.Writer}).Register(mux, routes)
	(&AlertHandler{Alert: s.Deps.Alert, Responder: Responder{Writer: s.Writer}, Writer: s.Writer}).Register(mux, routes)
	(&RateLimitHandler{Rules: s.Deps.RateLimit, Responder: Responder{Writer: s.Writer}, Writer: s.Writer}).Register(mux, routes)
	(&UserHandler{Users: s.Deps.Users, Responder: Responder{Writer: s.Writer}, Writer: s.Writer}).Register(mux, routes)
	(&LogHandler{Logs: s.Deps.Logs, Sites: s.Deps.Sites, Responder: Responder{Writer: s.Writer}, Writer: s.Writer}).Register(mux, routes)
	(&DashboardHandler{Dashboard: s.Deps.Dashboard, Responder: Responder{Writer: s.Writer}}).Register(mux, routes)
	(&SlowSQLHandler{SlowSQL: s.Deps.SlowSQL, Responder: Responder{Writer: s.Writer}, Writer: s.Writer}).Register(mux, routes)
	(&AIHandler{AI: s.Deps.AI, Responder: Responder{Writer: s.Writer}, Writer: s.Writer}).Register(mux, routes)
	(&MapHandler{Map: s.Deps.Map, Responder: Responder{Writer: s.Writer}}).Register(mux, routes)
	(&SystemHandler{Cfg: s.Cfg, Rules: s.Deps.Rules, Settings: s.Deps.Settings, FrontendVersion: s.frontendVersion(), Ingest: s.Deps.Ingest, Responder: Responder{Writer: s.Writer}}).Register(mux, routes)
	(&SysConfigHandler{SysConfig: s.Deps.SysConfig, Writer: s.Writer, Responder: Responder{Writer: s.Writer}}).Register(mux, routes)
	(&LuaConfigHandler{Lua: s.Deps.LuaConfig, Writer: s.Writer, Responder: Responder{Writer: s.Writer}}).Register(mux, routes)
	(&SyncHandler{Sync: s.Deps.Sync, Sites: s.Deps.Sites, Rules: s.Deps.Rules, Writer: s.Writer}).Register(mux)

	// 前端单页应用：未匹配到 API 的路径全部回落到 index.html
	mux.HandleFunc("/", s.serveSPA)
}
