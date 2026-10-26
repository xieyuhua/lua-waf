package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"wafadmin/internal/config"
)

// ConfigApplier 由组合根实现：把最新配置应用到运行中的服务实例。
type ConfigApplier interface {
	Apply(cfg *config.Config)
}

// DatabasePayload 是数据库配置的编辑入参。
type DatabasePayload struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// ConfigPayload 是运行配置的编辑入参。
// 字符串字段留空表示保持原值；密钥字段传脱敏串（含 ****）同样表示保持原值。
type ConfigPayload struct {
	RunMode       string           `json:"run_mode"`
	HTTPPort      int              `json:"http_port"`
	APIKey        string           `json:"api_key"`
	APIServers    []string         `json:"api_servers"`
	NginxBin      string           `json:"nginx_bin"`
	NginxVhosts   string           `json:"nginx_vhosts"`
	SSLCertDir    string           `json:"ssl_cert_dir"`
	RulePath      string           `json:"rule_path"`
	WafLogDir     string           `json:"waf_log_dir"`
	PageSize      int              `json:"page_size"`
	SessionSecret string           `json:"session_secret"`
	SessionTTL    string           `json:"session_ttl"`
	SessionSecure bool             `json:"session_secure"`
	CaptchaTTL    string           `json:"captcha_ttl"`
	SlowSQLMS     int64            `json:"slow_sql_ms"`
	Database      *DatabasePayload `json:"database,omitempty"`
}

// ConfigView 是运行配置的展示结果。
type ConfigView struct {
	// Source 为配置来源（"(defaults)" 表示未找到配置文件）。
	Source string `json:"source"`
	// File 为实际写入的 ini 路径。
	File string `json:"file"`
	// Writable 表示当前进程是否有权限写该文件。
	Writable bool `json:"writable"`
	// Config 为脱敏后的配置内容。
	Config ConfigPayload `json:"config"`
	// RestartRequired 列出需要重启进程才生效的字段。
	RestartRequired []string `json:"restart_required"`
}

// SystemConfigService 负责运行配置的读取、校验、落盘与热更新。
type SystemConfigService struct {
	// Current 为运行中的配置对象（与各层共享同一指针）。
	Current *config.Config
	// Loader 用于从磁盘重新加载配置。
	Loader *config.Loader
	// Writer 负责写回 ini 文件。
	Writer *config.Writer
	// Applier 负责把最新配置应用给运行中的服务。
	Applier ConfigApplier

	mu sync.Mutex
}

// View 返回当前运行配置（密钥脱敏）。
func (s *SystemConfigService) View(ctx context.Context) (*ConfigView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.view(s.Current, nil), nil
}

// Update 校验并保存配置，随后立即热更新到运行中的服务。
func (s *SystemConfigService) Update(ctx context.Context, payload ConfigPayload) (*ConfigView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	candidate := *s.Current
	candidate.APIServers = append([]string{}, s.Current.APIServers...)
	if err := s.merge(&candidate, payload); err != nil {
		return nil, err
	}
	if err := candidate.Validate(); err != nil {
		return nil, ErrInvalid.New("%s", err.Error())
	}
	if strings.TrimSpace(candidate.SessionSecret) == "" {
		return nil, ErrInvalid.New("会话密钥不能为空，请填写不少于 16 位的随机字符串")
	}

	if err := s.Writer.Save(&candidate); err != nil {
		return nil, ErrInternal.Wrap(err, "保存配置文件失败")
	}
	if candidate.Source == "" || candidate.Source == "(defaults)" {
		candidate.Source = s.Writer.Target()
	}

	restart := s.restartRequired(s.Current, &candidate)
	*s.Current = candidate
	s.apply()
	return s.view(s.Current, restart), nil
}

// Reload 从磁盘重新加载配置并应用，用于手工改过 ini 后同步到运行进程。
func (s *SystemConfigService) Reload(ctx context.Context) (*ConfigView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	loaded, err := s.Loader.Load()
	if err != nil {
		return nil, ErrInvalid.New("重新加载配置失败: %s", err.Error())
	}
	loaded.EnsureSessionSecret()

	restart := s.restartRequired(s.Current, loaded)
	*s.Current = *loaded
	s.apply()
	return s.view(s.Current, restart), nil
}

// Raw 返回配置文件原文，便于排障（不含脱敏）。
func (s *SystemConfigService) Raw(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	path := s.Writer.Target()
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound.New("配置文件 %s 不存在", path)
		}
		return "", ErrInternal.Wrap(err, "读取配置文件失败")
	}
	return string(content), nil
}

// merge 把入参合并到配置副本。
func (s *SystemConfigService) merge(cfg *config.Config, payload ConfigPayload) error {
	if value := strings.ToLower(strings.TrimSpace(payload.RunMode)); value != "" {
		cfg.RunMode = value
	}
	if payload.HTTPPort > 0 {
		cfg.HTTPPort = payload.HTTPPort
	}
	if value := strings.TrimSpace(payload.APIKey); value != "" && !isMasked(value) {
		cfg.APIKey = value
	}
	if payload.APIServers != nil {
		servers := make([]string, 0, len(payload.APIServers))
		for _, item := range payload.APIServers {
			if item = strings.TrimSpace(item); item != "" {
				servers = append(servers, item)
			}
		}
		if len(servers) == 0 {
			return ErrInvalid.New("同步节点列表不能为空")
		}
		cfg.APIServers = servers
	}
	if value := strings.TrimSpace(payload.NginxBin); value != "" {
		cfg.NginxBin = value
	}
	if value := strings.TrimSpace(payload.NginxVhosts); value != "" {
		cfg.NginxVhosts = trailingSlash(value)
	}
	if value := strings.TrimSpace(payload.SSLCertDir); value != "" {
		cfg.SSLCertDir = strings.TrimRight(value, "/\\")
	}
	if value := strings.TrimSpace(payload.RulePath); value != "" {
		cfg.RulePath = trailingSlash(value)
	}
	if value := strings.TrimSpace(payload.WafLogDir); value != "" {
		cfg.WafLogDir = strings.TrimRight(value, "/\\")
	}
	if payload.PageSize > 0 {
		if payload.PageSize > 200 {
			return ErrInvalid.New("分页大小不能超过 200")
		}
		cfg.PageSize = payload.PageSize
	}
	if value := strings.TrimSpace(payload.SessionSecret); value != "" && !isMasked(value) {
		if len(value) < 16 {
			return ErrInvalid.New("会话密钥长度不足，请填写不少于 16 位")
		}
		cfg.SessionSecret = value
		// 明确由后台写入的密钥，标记来源为 file 以便落盘
		cfg.SessionSecretSource = config.SecretSourceFile
	}
	if value := strings.TrimSpace(payload.SessionTTL); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed <= 0 || parsed > 720*time.Hour {
			return ErrInvalid.New("会话有效期格式不正确，示例：12h / 30m")
		}
		cfg.SessionTTL = parsed
	}
	cfg.SessionSecure = payload.SessionSecure
	if value := strings.TrimSpace(payload.CaptchaTTL); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed <= 0 || parsed > time.Hour {
			return ErrInvalid.New("验证码有效期格式不正确，示例：5m")
		}
		cfg.CaptchaTTL = parsed
	}
	if payload.SlowSQLMS > 0 {
		if payload.SlowSQLMS > 60000 {
			return ErrInvalid.New("慢 SQL 阈值不能超过 60000 毫秒")
		}
		cfg.SlowSQLThreshold = time.Duration(payload.SlowSQLMS) * time.Millisecond
	}
	if payload.Database != nil {
		if err := mergeDatabase(&cfg.Database, payload.Database); err != nil {
			return err
		}
	}
	return nil
}

// mergeDatabase 合并数据库配置。
func mergeDatabase(db *config.Database, payload *DatabasePayload) error {
	if value := strings.TrimSpace(payload.Host); value != "" {
		db.Host = value
	}
	if payload.Port > 0 {
		db.Port = payload.Port
	}
	if value := strings.TrimSpace(payload.User); value != "" {
		db.User = value
	}
	if value := strings.TrimSpace(payload.Name); value != "" {
		db.Name = value
	}
	if value := strings.TrimSpace(payload.Password); value != "" && !isMasked(value) {
		db.Password = value
	}
	if db.Port <= 0 || db.Port > 65535 {
		return ErrInvalid.New("数据库端口不合法")
	}
	return nil
}

// restartRequired 比较新旧配置，返回需要重启才生效的字段。
func (s *SystemConfigService) restartRequired(old, current *config.Config) []string {
	fields := make([]string, 0, 4)
	if old.RunMode != current.RunMode {
		fields = append(fields, "RUN_MODE（运行模式）")
	}
	if old.HTTPPort != current.HTTPPort {
		fields = append(fields, "HTTP_PORT（监听端口）")
	}
	if old.Database != current.Database {
		fields = append(fields, "database（数据库连接）")
	}
	if old.SessionSecret != current.SessionSecret {
		fields = append(fields, "security.SESSION_SECRET（会话密钥）")
	}
	return fields
}

// view 组装脱敏后的配置视图。
func (s *SystemConfigService) view(cfg *config.Config, restart []string) *ConfigView {
	if restart == nil {
		restart = make([]string, 0, 2)
	}
	path := s.Writer.Target()

	payload := ConfigPayload{
		RunMode:       cfg.RunMode,
		HTTPPort:      cfg.HTTPPort,
		APIKey:        mask(cfg.APIKey),
		APIServers:    append([]string{}, cfg.APIServers...),
		NginxBin:      cfg.NginxBin,
		NginxVhosts:   cfg.NginxVhosts,
		SSLCertDir:    cfg.SSLCertDir,
		RulePath:      cfg.RulePath,
		WafLogDir:     cfg.WafLogDir,
		PageSize:      cfg.PageSize,
		SessionSecret: mask(cfg.SessionSecret),
		SessionTTL:    cfg.SessionTTL.String(),
		SessionSecure: cfg.SessionSecure,
		CaptchaTTL:    cfg.CaptchaTTL.String(),
		SlowSQLMS:     cfg.SlowSQLThreshold.Milliseconds(),
		Database: &DatabasePayload{
			Host:     cfg.Database.Host,
			Port:     cfg.Database.Port,
			User:     cfg.Database.User,
			Password: mask(cfg.Database.Password),
			Name:     cfg.Database.Name,
		},
	}

	return &ConfigView{
		Source:          cfg.Source,
		File:            path,
		Writable:        writable(path),
		Config:          payload,
		RestartRequired: restart,
	}
}

// apply 触发组合根的热更新回调。
func (s *SystemConfigService) apply() {
	if s.Applier != nil {
		s.Applier.Apply(s.Current)
	}
}

// mask 对密钥做脱敏处理。
func mask(value string) string {
	trimmed := strings.TrimSpace(value)
	switch {
	case trimmed == "":
		return ""
	case len(trimmed) <= 8:
		return "********"
	default:
		return trimmed[:4] + "********" + trimmed[len(trimmed)-4:]
	}
}

// isMasked 判断入参是否为脱敏占位串。
func isMasked(value string) bool { return strings.Contains(value, "****") }

// trailingSlash 保证目录型配置以分隔符结尾。
func trailingSlash(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || strings.HasSuffix(path, "/") || strings.HasSuffix(path, "\\") {
		return path
	}
	return path + "/"
}

// writable 探测目标文件所在目录是否可写。
func writable(path string) bool {
	dir := filepath.Dir(path)
	probe, err := os.CreateTemp(dir, ".wafadmin-probe-*")
	if err != nil {
		return false
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return true
}

// DescribeConfigFile 返回便于展示的配置位置说明。
func DescribeConfigFile(view *ConfigView) string {
	return fmt.Sprintf("%s（来源 %s）", view.File, view.Source)
}
