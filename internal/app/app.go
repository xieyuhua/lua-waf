// Package app 是应用的组合根：按「配置 → 仓储 → 服务 → 接入层」的顺序装配依赖，
// 并负责启动、优雅关闭与首次初始化。
package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"wafadmin/internal/config"
	"wafadmin/internal/ingest"
	"wafadmin/internal/model"
	"wafadmin/internal/nginx"
	"wafadmin/internal/repository"
	"wafadmin/internal/repository/mysql"
	"wafadmin/internal/service"
	"wafadmin/internal/transport/httpapi"
	"wafadmin/pkg/captcha"
	"wafadmin/pkg/crypto"
	"wafadmin/pkg/geo"
	"wafadmin/pkg/ratelimit"
	"wafadmin/pkg/redis"
	"wafadmin/pkg/response"
	"wafadmin/pkg/session"
	"wafadmin/pkg/sqlwatch"
)

// Options 是构造 App 的入参。
type Options struct {
	// Config 为配置加载器，为空时使用默认加载器。
	Config *config.Loader
	// Static 为内嵌的前端构建产物（web/dist）。
	Static fs.FS
	// Version 覆盖配置中的版本号。
	Version string
}

// App 是应用运行实例。
type App struct {
	cfg      *config.Config
	repos    repository.Repositories
	server   *http.Server
	consumer *ingest.Consumer
	// cancel 用于停止后台消费协程
	cancel  context.CancelFunc
	closers []func() error
}

// New 装配全部依赖并返回可运行的 App。
func New(opts Options) (*App, error) {
	loader := opts.Config
	if loader == nil {
		loader = &config.Loader{}
	}
	cfg, err := loader.Load()
	if err != nil {
		return nil, err
	}
	if opts.Version != "" {
		cfg.AppVer = opts.Version
	}
	if cfg.EnsureSessionSecret() {
		log.Println("[warn] 未配置 security.SESSION_SECRET，已生成临时密钥，重启后会话将失效")
	}

	builder := &builder{cfg: cfg}
	repos, observer, err := builder.repositories()
	if err != nil {
		return nil, err
	}
	if err := repos.Ensure(); err != nil {
		builder.close()
		return nil, err
	}

	deps, err := builder.services(repos)
	if err != nil {
		builder.close()
		return nil, err
	}
	builder.wireSlowSQL(observer, deps)

	// 会话与 CSRF 需要被热更新引用，因此先于 Server 构造
	sessionManager := builder.sessionManager()
	csrfGuard := &httpapi.CSRFGuard{Secure: cfg.SessionSecure}

	// 运行配置服务：读写 conf/app.ini 并把变更热更新到各服务
	deps.SysConfig = &service.SystemConfigService{
		Current: cfg,
		Loader:  loader,
		Writer:  &config.Writer{Path: configPath(cfg)},
		Applier: &runtimeConfig{
			nginx:   builder.nginxManager(),
			deps:    deps,
			session: sessionManager,
			csrf:    csrfGuard,
			db:      builder.store,
		},
	}

	// 日志入库：lua 只写 Redis，本进程消费后批量写 MySQL（默认关闭，按需在 [log] INGEST_ENABLED 开启）
	consumer := ingest.New(ingest.Options{
		Redis: redis.Options{
			Addr:     cfg.LogIngest.Addr,
			Password: cfg.LogIngest.Password,
			DB:       cfg.LogIngest.DB,
		},
		Source: cfg.LogIngest.ListKey,
		Batch:  cfg.LogIngest.Batch,
		Block:  time.Duration(cfg.LogIngest.BlockMS) * time.Millisecond,
	}, repos.Log)
	consumer.Logf = func(format string, args ...any) { log.Printf(format, args...) }
	deps.Ingest = consumer

	// lua-waf config.lua 在线编辑：保存后可用 nginx reload 生效
	deps.LuaConfig = &service.LuaConfigService{
		Path:     cfg.LuaConfigPath,
		Reloader: builder.nginxManager(),
	}

	server := &httpapi.Server{
		Cfg:     cfg,
		Deps:    *deps,
		Session: sessionManager,
		CSRF:    csrfGuard,
		Writer:  response.Writer{},
		Static:  opts.Static,
	}

	instance := &App{
		cfg:     cfg,
		repos:   repos,
		closers: builder.closers,
		server: &http.Server{
			Addr:              cfg.Addr(),
			Handler:           server.Handler(),
			ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout:       60 * time.Second,
			WriteTimeout:      120 * time.Second,
			IdleTimeout:       90 * time.Second,
		},
	}
	return instance, nil
}

// Run 启动 HTTP 服务并阻塞，直到服务退出。
func (a *App) Run() error {
	if a.consumer != nil && a.cfg.LogIngest.Enabled {
		ctx, cancel := context.WithCancel(context.Background())
		a.cancel = cancel
		log.Printf("[info] 日志入库消费者已启动：source=%s batch=%d", a.consumer.Opts.Source, a.consumer.Opts.Batch)
		go a.consumer.Run(ctx)
	}

	log.Printf("wafadmin %s 启动，运行模式: %s，监听: %s", a.cfg.AppVer, a.cfg.RunMode, a.cfg.Addr())
	if a.cfg.IsDemo() {
		log.Println("[info] 演示模式：使用内存仓储与内置样例数据，不会写入 nginx 配置")
	}
	if a.cfg.Source == "(defaults)" {
		log.Println("[warn] 未找到配置文件，已使用默认配置启动")
	}

	if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown 优雅关闭服务并释放仓储资源。
func (a *App) Shutdown(ctx context.Context) error {
	if a.cancel != nil {
		a.cancel()
	}

	var firstErr error
	if err := a.server.Shutdown(ctx); err != nil {
		firstErr = err
	}
	for _, closer := range a.closers {
		if err := closer(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// builder 按运行模式装配仓储与服务，并记录需要释放的资源。
type builder struct {
	cfg     *config.Config
	closers []func() error
	// nginx 为共享的 nginx 配置管理器（热更新与配置下发共用同一实例）。
	nginx *nginx.Manager
	// store 为 MySQL 仓储实例（演示模式为 nil），用于运行时开关 SQL 日志。
	store *mysql.Store
}

// repositories 按运行模式创建仓储实现。
func (b *builder) repositories() (repository.Repositories, *sqlwatch.Observer, error) {
	store, err := OpenStore(b.cfg)
	if err != nil {
		return repository.Repositories{}, nil, fmt.Errorf("连接数据库失败: %w", err)
	}
	b.store = store.MySQL
	if len(store.closers) > 0 {
		b.closers = append(b.closers, store.closers...)
	}
	return store.Repos, store.Observer, nil
}

// services 装配全部业务服务。
func (b *builder) services(repos repository.Repositories) (*httpapi.Deps, error) {
	hasher := crypto.Hasher{}
	nginxManager := b.nginxManager()
	settings := &service.SettingService{Repo: repos.Setting}

	sites := &service.SiteService{Sites: repos.Site, Nginx: nginxManager, PageSize: b.cfg.PageSize}
	rules := &service.RuleService{Rules: repos.Rule, Catalog: model.Catalog, Nginx: nginxManager}
	users := &service.UserService{Users: repos.User, Hasher: hasher, PageSize: b.cfg.PageSize}
	logs := &service.LogService{Logs: repos.Log, PageSize: b.cfg.PageSize}
	slow := &service.SlowSQLService{
		Repo:         repos.SlowSQL,
		Threshold:    b.cfg.SlowSQLThreshold,
		MaxStatement: 4000,
		PageSize:     b.cfg.PageSize,
	}

	if err := b.bootstrap(context.Background(), users, rules); err != nil {
		return nil, err
	}

	return &httpapi.Deps{
		Auth: &service.AuthService{
			Users:   repos.User,
			Hasher:  hasher,
			Captcha: &captcha.Manager{TTL: b.cfg.CaptchaTTL, Length: 4, Width: 150, Height: 48},
			Limiter: &ratelimit.Limiter{Limit: 10, Window: 5 * time.Minute},
		},
		Sites: sites,
		Rules: rules,
		Users: users,
		Logs:  logs,
		Dashboard: &service.DashboardService{
			Sites: repos.Site, Rules: repos.Rule, Users: repos.User,
			Logs: repos.Log, Slow: repos.SlowSQL, PageSize: b.cfg.PageSize,
		},
		SlowSQL:  slow,
		AI:       &service.AIService{Settings: settings, Logs: repos.Log, SlowSQLs: repos.SlowSQL, Client: &http.Client{Timeout: 120 * time.Second}, Cache: &service.AnalyzeCache{}},
		Map:      &service.AttackMapService{Logs: repos.Log, Sites: repos.Site, Resolver: geo.Default, TargetArea: b.cfg.MapTarget},
		Settings: settings,
		Sync: &service.SyncService{
			Cfg:    b.cfg,
			Sites:  sites,
			Rules:  rules,
			Nginx:  nginxManager,
			Signer: crypto.Signer{APIKey: b.cfg.APIKey, Window: 10 * time.Minute},
			Client: &http.Client{Timeout: 20 * time.Second},
		},
	}, nil
}

// wireSlowSQL 把服务层的慢 SQL 采集器注入仓储观测器。
func (b *builder) wireSlowSQL(observer *sqlwatch.Observer, deps *httpapi.Deps) {
	if observer == nil || deps.SlowSQL == nil {
		return
	}
	observer.Recorder = deps.SlowSQL
	observer.Threshold = b.cfg.SlowSQLThreshold
}

// bootstrap 在空库时写入初始账号与内置规则。
func (b *builder) bootstrap(ctx context.Context, users *service.UserService, rules *service.RuleService) error {
	createdUser, err := users.SeedDefault(ctx)
	if err != nil {
		return err
	}
	if createdUser {
		log.Println("[warn] 已创建初始账号 admin / admin，请登录后立即修改口令")
	}

	createdRules, err := rules.Seed(ctx)
	if err != nil {
		return err
	}
	if createdRules > 0 {
		log.Printf("[info] 已写入 %d 条内置 WAF 规则", createdRules)
	}
	return nil
}

// sessionManager 构造会话管理器。
func (b *builder) sessionManager() *session.Manager {
	return &session.Manager{
		CookieName: "waf_session",
		Secret:     []byte(b.cfg.SessionSecret),
		TTL:        b.cfg.SessionTTL,
		Secure:     b.cfg.SessionSecure,
	}
}

// nginxManager 构造（并缓存）nginx 配置管理器；演示模式只渲染不落盘。
func (b *builder) nginxManager() *nginx.Manager {
	if b.nginx != nil {
		return b.nginx
	}
	b.nginx = &nginx.Manager{
		Bin:       b.cfg.NginxBin,
		VhostsDir: b.cfg.NginxVhosts,
		CertDir:   b.cfg.SSLCertDir,
		RuleDir:   b.cfg.RulePath,
		LogDir:    b.cfg.WafLogDir,
		DryRun:    b.cfg.IsDemo(),
	}
	return b.nginx
}

// close 释放已获取的外部资源。
func (b *builder) close() {
	for _, closer := range b.closers {
		_ = closer()
	}
}
