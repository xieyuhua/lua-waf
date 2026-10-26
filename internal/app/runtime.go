package app

import (
	"path/filepath"

	"wafadmin/internal/config"
	"wafadmin/internal/nginx"
	"wafadmin/internal/repository/mysql"
	"wafadmin/internal/transport/httpapi"
	"wafadmin/pkg/session"
)

// runtimeConfig 把最新配置应用到已装配的组件（热更新）。
// 需要重启进程才生效的字段（运行模式、监听端口、数据库、会话密钥）不在此处理，
// 由 service.SystemConfigService 在响应中提示。
type runtimeConfig struct {
	nginx   *nginx.Manager
	deps    *httpapi.Deps
	session *session.Manager
	csrf    *httpapi.CSRFGuard
	db      *mysql.Store
}

// Apply 实现 service.ConfigApplier。
func (r *runtimeConfig) Apply(cfg *config.Config) {
	// nginx 相关路径与演示开关
	r.nginx.Bin = cfg.NginxBin
	r.nginx.VhostsDir = cfg.NginxVhosts
	r.nginx.CertDir = cfg.SSLCertDir
	r.nginx.RuleDir = cfg.RulePath
	r.nginx.LogDir = cfg.WafLogDir
	r.nginx.DryRun = cfg.IsDemo()

	// 分页大小与慢 SQL 阈值
	r.deps.Sites.PageSize = cfg.PageSize
	r.deps.Users.PageSize = cfg.PageSize
	r.deps.Logs.PageSize = cfg.PageSize
	r.deps.SlowSQL.PageSize = cfg.PageSize
	r.deps.SlowSQL.Threshold = cfg.SlowSQLThreshold
	r.deps.Dashboard.PageSize = cfg.PageSize

	// 节点同步密钥与验证码有效期
	r.deps.Sync.Signer.APIKey = cfg.APIKey
	r.deps.Auth.Captcha.TTL = cfg.CaptchaTTL

	// 会话与 CSRF（密钥变更会使既有会话失效）
	r.session.TTL = cfg.SessionTTL
	r.session.Secret = []byte(cfg.SessionSecret)
	r.session.Secure = cfg.SessionSecure
	r.csrf.Secure = cfg.SessionSecure

	// SQL 日志开关（演示模式无 MySQL，db 为 nil）
	if r.db != nil {
		r.db.ApplyShowSQL(cfg.Database.ShowSQL)
	}
}

// configPath 返回可写的配置文件路径（未找到配置文件时落到 conf/app.ini）。
func configPath(cfg *config.Config) string {
	if cfg.Source != "" && cfg.Source != "(defaults)" {
		return cfg.Source
	}
	return filepath.Join("conf", "app.ini")
}
