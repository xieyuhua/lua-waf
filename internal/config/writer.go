package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/ini.v1"
)

// Writer 负责把 Config 写回 ini 文件。
// 写入时基于原文件进行增量更新，保留其它段落与注释，并在写入前留一份 .bak 备份。
type Writer struct {
	// Path 为目标 ini 文件路径，为空时写入 conf/app.ini。
	Path string
}

// Target 返回实际写入的路径。
func (w *Writer) Target() string {
	if strings.TrimSpace(w.Path) == "" || w.Path == "(defaults)" {
		return filepath.Join("conf", "app.ini")
	}
	return w.Path
}

// Save 把配置写回文件（先写临时文件再 rename，避免写坏原文件）。
func (w *Writer) Save(cfg *Config) error {
	target := w.Target()

	file, err := ini.LoadSources(ini.LoadOptions{Loose: true, AllowShadows: true}, target)
	switch {
	case err == nil:
		// 复用已存在的文件，保留注释与顺序
	case os.IsNotExist(err):
		file = ini.Empty()
	default:
		return fmt.Errorf("读取配置文件 %s 失败: %w", target, err)
	}

	w.apply(file, cfg)

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	if _, statErr := os.Stat(target); statErr == nil {
		if backupErr := os.WriteFile(target+".bak", mustRead(target), 0o600); backupErr != nil {
			return fmt.Errorf("写入配置备份失败: %w", backupErr)
		}
	}

	temp := target + ".tmp"
	if err := file.SaveTo(temp); err != nil {
		return fmt.Errorf("写入临时配置失败: %w", err)
	}
	if err := os.Rename(temp, target); err != nil {
		return fmt.Errorf("替换配置文件失败: %w", err)
	}
	return nil
}

// apply 把配置项写入 ini 结构。
func (w *Writer) apply(file *ini.File, cfg *Config) {
	set := func(section, key, value string) {
		file.Section(section).Key(key).SetValue(value)
	}

	set("", "RUN_MODE", cfg.RunMode)

	set("server", "HTTP_PORT", strconv.Itoa(cfg.HTTPPort))
	set("server", "API_KEY", cfg.APIKey)
	set("server", "NGINX_BIN", cfg.NginxBin)
	set("server", "NGINX_VHOSTS", cfg.NginxVhosts)
	set("server", "SSL_CERT_DIR", cfg.SSLCertDir)
	set("server", "API_SERVERS", strings.Join(cfg.APIServers, ","))

	set("database", "USER", cfg.Database.User)
	set("database", "PASSWD", cfg.Database.Password)
	set("database", "HOST", cfg.Database.Host)
	set("database", "PORT", strconv.Itoa(cfg.Database.Port))
	set("database", "NAME", cfg.Database.Name)
	set("database", "AUTO_MIGRATE", strconv.FormatBool(cfg.Database.AutoMigrate))
	set("database", "SHOW_SQL", strconv.FormatBool(cfg.Database.ShowSQL))
	set("database", "ADD_MISSING_COLUMNS", strconv.FormatBool(cfg.Database.AddMissingColumns))

	set("log", "INGEST_ENABLED", boolText(cfg.LogIngest.Enabled))
	set("waf", "MAP_TARGET", cfg.MapTarget)
	set("waf", "LUA_CONFIG", cfg.LuaConfigPath)
	set("waf", "RULE_PATH", cfg.RulePath)
	set("waf", "LOG_DIR", cfg.WafLogDir)

	set("pager", "PAGEMax", strconv.Itoa(cfg.PageSize))

	// 只有来自配置文件的密钥才写回；环境变量或随机生成的密钥不落盘，
	// 避免把环境变量里的密钥写进文件（此时保留文件中原有取值）。
	if cfg.SessionSecretSource == SecretSourceFile || cfg.SessionSecretSource == "" {
		set("security", "SESSION_SECRET", cfg.SessionSecret)
	}
	set("security", "SESSION_TTL", formatDuration(cfg.SessionTTL))
	set("security", "SESSION_SECURE", strconv.FormatBool(cfg.SessionSecure))
	set("security", "CAPTCHA_TTL", formatDuration(cfg.CaptchaTTL))

	set("monitor", "SLOW_SQL_MS", strconv.FormatInt(cfg.SlowSQLThreshold.Milliseconds(), 10))
}

// formatDuration 把时长格式化为简洁写法（12h / 30m / 45s），避免出现 12h0m0s。
func formatDuration(value time.Duration) string {
	if value <= 0 {
		return "0s"
	}
	switch {
	case value%time.Hour == 0:
		return strconv.FormatInt(int64(value/time.Hour), 10) + "h"
	case value%time.Minute == 0:
		return strconv.FormatInt(int64(value/time.Minute), 10) + "m"
	case value%time.Second == 0:
		return strconv.FormatInt(int64(value/time.Second), 10) + "s"
	default:
		return value.String()
	}
}

// mustRead 读取文件内容，失败时返回空切片（备份失败不应阻断主流程）。
func mustRead(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		return []byte{}
	}
	return data
}
