package app

// 本文件是组合根里的「基础设施装配」：按运行模式选仓储、构造 nginx 管理器与会话管理器，
// 以及进程退出时的资源释放。业务服务装配见 wire_services.go。

import (
	"fmt"

	"wafadmin/internal/config"

	"wafadmin/internal/nginx"
	"wafadmin/internal/repository"
	"wafadmin/internal/repository/mysql"
	"wafadmin/internal/service"

	"wafadmin/pkg/session"
	"wafadmin/pkg/sqlwatch"
)

type builder struct {
	cfg     *config.Config
	closers []func() error
	// retention 为日志归档服务（保留天数支持后台热更新）。
	retention *service.RetentionService

	// alert 为告警推送服务（命中勾选规则时提醒）。
	alert *service.AlertService

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
