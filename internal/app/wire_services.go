package app

// 本文件是组合根里的「业务服务装配」：把仓储注入各 service，并完成启动时的初始化
// （默认管理员、内置规则、慢 SQL 观察器等）。基础设施装配见 wire_repos.go。

import (
	"context"

	"log"
	"net/http"
	"time"

	"wafadmin/internal/model"

	"wafadmin/internal/repository"

	"wafadmin/internal/service"
	"wafadmin/internal/transport/httpapi"
	"wafadmin/pkg/captcha"
	"wafadmin/pkg/crypto"
	"wafadmin/pkg/geo"
	"wafadmin/pkg/ratelimit"

	"wafadmin/pkg/sqlwatch"
)

func (b *builder) services(repos repository.Repositories) (*httpapi.Deps, error) {
	// 日志保留策略：RETENTION_DAYS > 0 时每天清理超期日志（默认 0 = 不清理）
	if b.cfg.LogIngest.RetentionDays > 0 {
		retention := &service.RetentionService{Logs: repos.Log, Days: b.cfg.LogIngest.RetentionDays}
		go retention.Run(context.Background())
	} else {
		log.Printf("[info] 日志保留策略未启用（[log] RETENTION_DAYS = 0）")
	}

	hasher := crypto.Hasher{}
	nginxManager := b.nginxManager()
	settings := &service.SettingService{Repo: repos.Setting}

	sites := &service.SiteService{Sites: repos.Site, Nginx: nginxManager, PageSize: b.cfg.PageSize}
	rules := &service.RuleService{Rules: repos.Rule, Catalog: model.Catalog, Nginx: nginxManager, Logs: repos.Log}
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
			Logs: repos.Log,
			Slow: repos.SlowSQL, PageSize: b.cfg.PageSize,
		},
		SlowSQL:   slow,
		AI:        &service.AIService{Settings: settings, Logs: repos.Log, SlowSQLs: repos.SlowSQL, Client: &http.Client{Timeout: 120 * time.Second}, Cache: &service.AnalyzeCache{}},
		Map:       &service.AttackMapService{Logs: repos.Log, Sites: repos.Site, Resolver: geo.Default, TargetArea: b.cfg.MapTarget},
		Settings:  settings,
		RateLimit: &service.RateLimitService{Rules: repos.RateLimit, RuleDir: b.cfg.RulePath, Reloader: nginxManager},
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
