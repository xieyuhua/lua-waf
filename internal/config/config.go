// Package config 负责装配、校验 wafadmin 的运行配置。
// 配置来源优先级：默认值 < conf/app.ini < 环境变量。
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/ini.v1"
)

// 运行模式。demo 模式使用内存仓储并载入演示数据，无需 MySQL。
const (
	ModeDev  = "dev"
	ModeDemo = "demo"
	ModeProd = "prod"
)

// 会话密钥来源。写回配置文件时只有 file 来源才会覆盖该键，
// 避免把环境变量或启动时随机生成的密钥落盘。
const (
	SecretSourceFile      = "file"
	SecretSourceEnv       = "env"
	SecretSourceGenerated = "generated"
	SecretSourceEmpty     = "empty"
)

// Database 描述 MySQL 连接参数。
type Database struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	// AutoMigrate 为 true 时，仅当表不存在才自动建表；已存在的表不做任何结构同步。
	AutoMigrate bool
	// ShowSQL 为 true 时打印每条执行的 SQL（含参数与耗时），仅用于排障。
	ShowSQL bool
	// AddMissingColumns 为 true 时，对**已存在**的表只补充缺失的列（不做修改/删除）。
	// 用于从 1.x 升级（旧库缺列）时平滑兼容；默认关闭。
	AddMissingColumns bool
}

// DSN 组装 go-sql-driver/mysql 连接串。
func (d Database) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8&parseTime=true&loc=Local&timeout=5s",
		d.User, d.Password, d.Host, d.Port, d.Name)
}

// Config 是应用的全部可配置项。

// LogIngestConfig 是日志 Redis 消费者配置（lua 写 Redis，Go 批量写 MySQL）。
type LogIngestConfig struct {
	// Enabled 表示是否启动消费者。
	Enabled bool
	// Addr 为 Redis 地址（host:port）。
	Addr string
	// Password 为 Redis 密码，可为空。
	Password string
	// DB 为库号。
	DB int
	// ListKey 为 lua 写入的列表键（需与 config.lua 的 config_redis_list_key 一致）。
	ListKey string
	// Batch 为单批入库条数。
	Batch int
	// BlockMS 为阻塞等待毫秒数。
	BlockMS int
}
type Config struct {
	// Source 记录配置文件的加载来源，（defaults）表示全部使用默认值。
	Source  string
	RunMode string
	AppVer  string

	HTTPPort   int
	APIKey     string
	APIServers []string

	NginxBin    string
	NginxVhosts string
	RulePath    string
	SSLCertDir  string
	WafLogDir   string
	// MapTarget 为攻击地图的「防护目标」位置（地区文本，如 中国-北京），用于绘制攻击箭头。
	MapTarget string
	// LuaConfigPath 为 lua-waf 的 config.lua 路径（后台可在线修改）。
	LuaConfigPath string
	// RuleImportDir 为「从规则文件同步」的候选目录：
	// 当后台与 nginx 不在同一台机器（RULE_PATH 不可读）时，用它指向本机的规则目录。
	RuleImportDir string
	// LogIngest 为「lua 写 Redis、Go 消费入库」的消费端配置。
	LogIngest LogIngestConfig

	PageSize int

	SessionSecret string
	SessionTTL    time.Duration
	SessionSecure bool
	CaptchaTTL    time.Duration

	// SessionSecretSource 记录会话密钥来源（file / env / generated / empty）。
	SessionSecretSource string

	// SlowSQLThreshold 为后台自身慢 SQL 的采集阈值。
	SlowSQLThreshold time.Duration

	Database Database
}

// IsDemo 表示是否处于演示模式（内存仓储）。
func (c *Config) IsDemo() bool { return c.RunMode == ModeDemo }

// IsProd 表示是否处于生产模式。
func (c *Config) IsProd() bool { return c.RunMode == ModeProd }

// Addr 返回 HTTP 监听地址。
func (c *Config) Addr() string { return fmt.Sprintf(":%d", c.HTTPPort) }

// EnsureSessionSecret 在未配置会话密钥时生成一次性随机密钥。
// 返回 true 表示本次为随机生成，重启后既有会话将失效。
func (c *Config) EnsureSessionSecret() bool {
	if strings.TrimSpace(c.SessionSecret) != "" {
		if c.SessionSecretSource == "" {
			c.SessionSecretSource = SecretSourceFile
		}
		return false
	}
	buf := make([]byte, 32)
	c.SessionSecretSource = SecretSourceGenerated
	if _, err := rand.Read(buf); err != nil {
		c.SessionSecret = fmt.Sprintf("wafadmin-fallback-%d", time.Now().UnixNano())
		return true
	}
	c.SessionSecret = hex.EncodeToString(buf)
	return true
}

// Loader 负责把 ini 文件与环境变量合并为 Config。
// 零值即可使用：Path 为空时取 WAF_CONFIG 或 conf/app.ini。
type Loader struct {
	// Path 为 ini 文件路径。
	Path string
	// Env 用于读取环境变量，为空时使用 os.Getenv。
	Env func(string) string
}

// Load 读取并校验配置。
func (l *Loader) Load() (*Config, error) {
	cfg := l.defaults()

	path := l.Path
	if strings.TrimSpace(path) == "" {
		path = l.env("WAF_CONFIG")
	}
	if strings.TrimSpace(path) == "" {
		path = filepath.Join("conf", "app.ini")
	}

	switch file, err := ini.LoadSources(ini.LoadOptions{Loose: true}, path); {
	case err == nil:
		cfg.Source = path
		l.applyINI(cfg, file)
	case os.IsNotExist(err):
		// 缺失配置文件时以默认值启动，便于演示与容器化部署。
		cfg.Source = "(defaults)"
	default:
		return nil, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
	}

	l.applyEnv(cfg)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// defaults 返回内置默认配置。
func (l *Loader) defaults() *Config {
	return &Config{
		RunMode:       ModeDev,
		AppVer:        "2.0",
		HTTPPort:      5000,
		APIServers:    []string{"127.0.0.1"},
		NginxBin:      "/usr/local/openresty/nginx/sbin/nginx",
		NginxVhosts:   "/usr/local/openresty/nginx/conf/vhosts/",
		RulePath:      "/usr/local/openresty/nginx/conf/waf/rules/",
		SSLCertDir:    "/www/server/panel/vhost/cert",
		WafLogDir:     "/www/wwwlogs/waf",
		MapTarget:     "中国-北京",
		LuaConfigPath: "/www/server/nginx/x-waf/config.lua",
		RuleImportDir: "lua-waf/rules",

		LogIngest: LogIngestConfig{
			Enabled: false,
			Addr:    "127.0.0.1:6379",
			DB:      0,
			ListKey: "waf:logs",
			Batch:   200,
			BlockMS: 2000,
		},
		PageSize:         10,
		SessionTTL:       12 * time.Hour,
		CaptchaTTL:       5 * time.Minute,
		SlowSQLThreshold: 200 * time.Millisecond,
		Database: Database{
			Host:        "127.0.0.1",
			Port:        3306,
			User:        "waf",
			Name:        "waf",
			AutoMigrate: true,
		},
	}
}

// applyINI 把 ini 内容叠加到配置上。
func (l *Loader) applyINI(cfg *Config, file *ini.File) {
	cfg.RunMode = strings.ToLower(file.Section("").Key("RUN_MODE").MustString(cfg.RunMode))

	server := file.Section("server")
	cfg.HTTPPort = server.Key("HTTP_PORT").MustInt(cfg.HTTPPort)
	cfg.APIKey = server.Key("API_KEY").MustString(cfg.APIKey)
	cfg.NginxBin = server.Key("NGINX_BIN").MustString(cfg.NginxBin)
	cfg.NginxVhosts = ensureTrailingSlash(server.Key("NGINX_VHOSTS").MustString(cfg.NginxVhosts))
	cfg.SSLCertDir = server.Key("SSL_CERT_DIR").MustString(cfg.SSLCertDir)
	cfg.APIServers = splitList(server.Key("API_SERVERS").MustString(strings.Join(cfg.APIServers, ",")), ",")

	waf := file.Section("waf")
	cfg.RulePath = ensureTrailingSlash(waf.Key("RULE_PATH").MustString(cfg.RulePath))
	cfg.RuleImportDir = strings.TrimSpace(waf.Key("RULE_IMPORT_DIR").MustString(cfg.RuleImportDir))
	cfg.WafLogDir = waf.Key("LOG_DIR").MustString(cfg.WafLogDir)
	cfg.MapTarget = waf.Key("MAP_TARGET").MustString(cfg.MapTarget)
	cfg.LuaConfigPath = waf.Key("LUA_CONFIG").MustString(cfg.LuaConfigPath)

	// 日志入库：lua 写 Redis，本进程消费后批量写 MySQL
	logSection := file.Section("log")

	cfg.LogIngest.Enabled = parseToggle(logSection.Key("INGEST_ENABLED").MustString(boolText(cfg.LogIngest.Enabled)))
	cfg.LogIngest.Addr = logSection.Key("REDIS_ADDR").MustString(cfg.LogIngest.Addr)
	cfg.LogIngest.Password = logSection.Key("REDIS_PASSWORD").MustString(cfg.LogIngest.Password)
	cfg.LogIngest.DB = logSection.Key("REDIS_DB").MustInt(cfg.LogIngest.DB)
	cfg.LogIngest.ListKey = logSection.Key("REDIS_LIST_KEY").MustString(cfg.LogIngest.ListKey)
	cfg.LogIngest.Batch = logSection.Key("INGEST_BATCH").MustInt(cfg.LogIngest.Batch)
	cfg.LogIngest.BlockMS = logSection.Key("INGEST_BLOCK_MS").MustInt(cfg.LogIngest.BlockMS)

	if ms := file.Section("monitor").Key("SLOW_SQL_MS").MustInt(int(cfg.SlowSQLThreshold.Milliseconds())); ms > 0 {
		cfg.SlowSQLThreshold = time.Duration(ms) * time.Millisecond
	}
	cfg.PageSize = file.Section("pager").Key("PAGEMax").MustInt(cfg.PageSize)

	security := file.Section("security")
	cfg.SessionSecret = security.Key("SESSION_SECRET").MustString(cfg.SessionSecret)
	cfg.SessionSecretSource = SecretSourceEmpty
	if strings.TrimSpace(cfg.SessionSecret) != "" {
		cfg.SessionSecretSource = SecretSourceFile
	}
	cfg.SessionSecure = security.Key("SESSION_SECURE").MustBool(false)
	if ttl, err := time.ParseDuration(security.Key("SESSION_TTL").MustString(cfg.SessionTTL.String())); err == nil {
		cfg.SessionTTL = ttl
	}
	if ttl, err := time.ParseDuration(security.Key("CAPTCHA_TTL").MustString(cfg.CaptchaTTL.String())); err == nil {
		cfg.CaptchaTTL = ttl
	}

	db := file.Section("database")
	cfg.Database.User = db.Key("USER").MustString(cfg.Database.User)
	cfg.Database.Password = db.Key("PASSWD").MustString(cfg.Database.Password)
	cfg.Database.Name = db.Key("NAME").MustString(cfg.Database.Name)
	host, port := splitHostPort(db.Key("HOST").MustString(""), cfg.Database.Port)
	cfg.Database.Host = host
	cfg.Database.Port = db.Key("PORT").MustInt(port)
	cfg.Database.AutoMigrate = db.Key("AUTO_MIGRATE").MustBool(true)
	// 默认：dev 模式打印 SQL，prod 不打印（可用 SHOW_SQL 显式覆盖）
	cfg.Database.ShowSQL = db.Key("SHOW_SQL").MustBool(cfg.RunMode == ModeDev)
	cfg.Database.AddMissingColumns = db.Key("ADD_MISSING_COLUMNS").MustBool(false)
}

// applyEnv 用环境变量覆盖配置，便于容器化部署。
func (l *Loader) applyEnv(cfg *Config) {
	if v := l.env("WAF_RUN_MODE"); v != "" {
		cfg.RunMode = strings.ToLower(v)
	}
	if v := l.env("WAF_HTTP_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.HTTPPort = port
		}
	}
	if v := l.env("WAF_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := l.env("WAF_SESSION_SECRET"); v != "" {
		cfg.SessionSecret = v
		cfg.SessionSecretSource = SecretSourceEnv
	}
	if v := l.env("WAF_DB_HOST"); v != "" {
		host, port := splitHostPort(v, cfg.Database.Port)
		cfg.Database.Host, cfg.Database.Port = host, port
	}
	if v := l.env("WAF_DB_USER"); v != "" {
		cfg.Database.User = v
	}
	if v := l.env("WAF_DB_PASSWD"); v != "" {
		cfg.Database.Password = v
	}
	if v := l.env("WAF_DB_NAME"); v != "" {
		cfg.Database.Name = v
	}
	if v := l.env("WAF_MAP_TARGET"); v != "" {
		cfg.MapTarget = v
	}
	if v := l.env("WAF_RULE_IMPORT_DIR"); v != "" {
		cfg.RuleImportDir = v
	}
	if v := l.env("WAF_LUA_CONFIG"); v != "" {
		cfg.LuaConfigPath = v
	}

	if v := l.env("WAF_LOG_INGEST"); v != "" {
		cfg.LogIngest.Enabled = parseToggle(v)
	}
	if v := l.env("WAF_REDIS_ADDR"); v != "" {
		cfg.LogIngest.Addr = v
	}
	if v := l.env("WAF_REDIS_PASSWORD"); v != "" {
		cfg.LogIngest.Password = v
	}
	if v := l.env("WAF_DB_SHOW_SQL"); v != "" {
		cfg.Database.ShowSQL = parseToggle(v)
	}
	if v := l.env("WAF_DB_ADD_MISSING_COLUMNS"); v != "" {
		cfg.Database.AddMissingColumns = parseToggle(v)
	}
}

// parseToggle 解析 1/0、true/false、on/off 形式的开关值。
func parseToggle(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "on", "yes", "y":
		return true
	default:
		return false
	}
}

// env 读取环境变量，未配置读取器时回落到 os.Getenv。
func (l *Loader) env(key string) string {
	if l.Env != nil {
		return strings.TrimSpace(l.Env(key))
	}
	return strings.TrimSpace(os.Getenv(key))
}

// Validate 校验关键配置项。
func (c *Config) Validate() error {
	switch c.RunMode {
	case ModeDev, ModeDemo, ModeProd:
	default:
		return fmt.Errorf("不支持的 RUN_MODE: %q（可选 dev/demo/prod）", c.RunMode)
	}
	if c.HTTPPort <= 0 || c.HTTPPort > 65535 {
		return fmt.Errorf("HTTP_PORT 非法: %d", c.HTTPPort)
	}
	if c.PageSize <= 0 {
		c.PageSize = 10
	}
	if c.SessionTTL <= 0 {
		c.SessionTTL = 12 * time.Hour
	}
	if c.CaptchaTTL <= 0 {
		c.CaptchaTTL = 5 * time.Minute
	}
	if len(c.APIServers) == 0 {
		c.APIServers = []string{"127.0.0.1"}
	}
	if !c.IsDemo() {
		if strings.TrimSpace(c.Database.Host) == "" || strings.TrimSpace(c.Database.Name) == "" {
			return fmt.Errorf("database 配置不完整：需要 HOST 与 NAME")
		}
		if net.ParseIP(strings.TrimSpace(c.Database.Host)) == nil && strings.TrimSpace(c.Database.Host) == "" {
			return fmt.Errorf("database.HOST 非法")
		}
	}
	return nil
}

// ensureTrailingSlash 保证目录型配置以分隔符结尾。
func ensureTrailingSlash(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || strings.HasSuffix(p, "/") || strings.HasSuffix(p, "\\") {
		return p
	}
	return p + "/"
}

// splitList 按分隔符切分并剔除空项。
func splitList(raw, sep string) []string {
	out := make([]string, 0, 4)
	for _, item := range strings.Split(raw, sep) {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// splitHostPort 兼容 "1.2.3.4:3306" 与 "1.2.3.4" 两种写法。
func splitHostPort(raw string, fallbackPort int) (string, int) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fallbackPort
	}
	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		return raw, fallbackPort
	}
	if p, convErr := strconv.Atoi(port); convErr == nil && p > 0 {
		return host, p
	}
	return host, fallbackPort
}

// boolText 把布尔值转换为配置文件的 on/off 文本。
func boolText(value bool) string {
	if value {
		return "on"
	}
	return "off"
}
