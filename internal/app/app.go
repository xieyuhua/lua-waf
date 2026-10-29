// Package app 是应用的组合根：按「配置 → 仓储 → 服务 → 接入层」的顺序装配依赖，
// 并负责启动、优雅关闭与首次初始化。
package app

import (
	"context"
	"errors"

	"io/fs"
	"log"
	"net/http"
	"time"

	"wafadmin/internal/config"
	"wafadmin/internal/ingest"

	"wafadmin/internal/repository"

	"wafadmin/internal/model"
	"wafadmin/internal/service"
	"wafadmin/internal/transport/httpapi"
	"wafadmin/pkg/geo"

	"wafadmin/pkg/redis"
	"wafadmin/pkg/response"
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
			nginx:     builder.nginxManager(),
			deps:      deps,
			session:   sessionManager,
			csrf:      csrfGuard,
			db:        builder.store,
			retention: builder.retention,
		},
	}

	// 日志入库：lua 只写 Redis，本进程消费后批量写 MySQL（默认关闭，按需在 [log] INGEST_ENABLED 开启）
	// 告警推送：与 Deps.Alert 共用同一实例（命中勾选规则时提醒）
	alertNotifier := deps.Alert.Notify

	consumer := ingest.New(ingest.Options{
		Redis: redis.Options{
			Addr:     cfg.LogIngest.Addr,
			Password: cfg.LogIngest.Password,
			DB:       cfg.LogIngest.DB,
		},
		Source: cfg.LogIngest.ListKey,
		Geo:    resolveIP2Region(cfg),
		Notify: alertNotifier,
		Batch:  cfg.LogIngest.Batch,
		Block:  time.Duration(cfg.LogIngest.BlockMS) * time.Millisecond,
	}, repos.Log)
	consumer.Logf = func(format string, args ...any) { log.Printf(format, args...) }
	deps.Ingest = consumer
	if cfg.LogIngest.Enabled {
		log.Printf("[info] 日志入库消费者已启用：%s <- %s（批量 %d）", cfg.LogIngest.Addr, cfg.LogIngest.ListKey, cfg.LogIngest.Batch)
	} else {
		log.Printf("[info] 日志入库消费者未启用（[log] INGEST_ENABLED=off）：lua 直连 MySQL 或 file 模式时属正常")
	}

	// 启动时把规则文件与数据库取并集：文件里有的补进库，库里已有的保持不动
	deps.RuleSync = &service.RuleFileSync{Repo: repos.Rule}
	deps.Pipeline = &service.RulePipelineService{}
	if result, err := deps.RuleSync.SyncFromDirs(context.Background(), []string{cfg.RulePath, cfg.RuleImportDir}); err != nil {
		log.Printf("[warn] 规则文件同步失败: %v", err)
	} else if result.Added > 0 {
		log.Printf("[info] 规则文件同步：新增 %d 条，保留 %d 条（目录 %s）", result.Added, result.Kept, result.Dir)
	} else {
		log.Printf("[info] 规则文件同步：%s（目录 %s，文件 %d 个）", result.Message, result.Dir, result.Files)
	}

	// lua-waf config.lua 在线编辑：保存后可用 nginx reload 生效
	deps.LuaConfig = &service.LuaConfigService{
		Logs:     repos.Log,
		Cfg:      cfg,
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
		cfg:      cfg,
		repos:    repos,
		consumer: consumer,
		closers:  builder.closers,
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
// resolveIP2Region 打开 ip2region 数据文件并自检；不可用时返回 nil（area 仍由 lua 侧解析）。
//
// 这样 lua 不必为每个 worker 加载 12MB 数据文件：只要后端能解析，日志入库时就会补全 area。
func resolveIP2Region(cfg *config.Config) func(ip string) (string, bool) {
	region, err := geo.DefaultIP2Region(cfg.IP2RegionDB)
	if err != nil {
		log.Printf("[warn] IP 归属地解析未启用（%s）：%v；area 仍由 lua 侧解析", cfg.IP2RegionDB, err)
		return nil
	}

	sample, ok := region.Search("8.8.8.8")
	if !ok {
		log.Printf("[warn] IP 归属地解析未启用：%s 自检失败（8.8.8.8 解析不出结果），area 仍由 lua 侧解析", cfg.IP2RegionDB)
		return nil
	}

	log.Printf("[info] IP 归属地解析已启用（后端）：%s，自检 8.8.8.8 = %s", cfg.IP2RegionDB, sample)
	return region.Search
}

// alertNotify 把告警服务包装成消费者回调（服务为空时返回 nil，消费者会自动跳过）。
func alertNotify(alert *service.AlertService) func(ctx context.Context, logs []model.WafLog) {
	if alert == nil {
		return nil
	}
	return alert.Notify
}
