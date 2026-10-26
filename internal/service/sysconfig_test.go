package service_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"wafadmin/internal/config"
	"wafadmin/internal/service"
)

// applierStub 记录热更新调用。
type applierStub struct {
	applied *config.Config
	calls   int
}

// Apply 实现 service.ConfigApplier。
func (s *applierStub) Apply(cfg *config.Config) {
	s.applied = cfg
	s.calls++
}

// newSystemConfig 构造基于临时 ini 文件的运行配置服务。
func newSystemConfig(t *testing.T) (*service.SystemConfigService, *applierStub, string) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "app.ini")
	initial := `; 测试用配置
RUN_MODE = demo

[server]
HTTP_PORT = 5002
API_KEY = initial-key-1234567890
NGINX_BIN = /usr/sbin/nginx
NGINX_VHOSTS = /tmp/vhosts/
SSL_CERT_DIR = /tmp/cert
API_SERVERS = 127.0.0.1,10.0.0.9

[database]
USER = waf
PASSWD = db-pass-1234567
HOST = 127.0.0.1
PORT = 3306
NAME = waf

[waf]
RULE_PATH = /tmp/rules/
LOG_DIR = /tmp/logs

[pager]
PAGEMax = 10

[security]
SESSION_SECRET = session-secret-1234567890
SESSION_TTL = 12h
SESSION_SECURE = false
CAPTCHA_TTL = 5m

[monitor]
SLOW_SQL_MS = 200
`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatalf("写入初始配置失败: %v", err)
	}

	cfg, err := (&config.Loader{Path: path}).Load()
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	applier := &applierStub{}
	svc := &service.SystemConfigService{
		Current: cfg,
		Loader:  &config.Loader{Path: path},
		Writer:  &config.Writer{Path: path},
		Applier: applier,
	}
	return svc, applier, path
}

// TestSystemConfigViewMasksSecrets 校验读取配置时密钥被脱敏。
func TestSystemConfigViewMasksSecrets(t *testing.T) {
	svc, _, path := newSystemConfig(t)
	view, err := svc.View(context.Background())
	if err != nil {
		t.Fatalf("读取配置失败: %v", err)
	}

	if view.File != path {
		t.Fatalf("配置文件路径异常: %s", view.File)
	}
	if !view.Writable {
		t.Fatal("临时目录应可写")
	}
	if strings.Contains(view.Config.APIKey, "initial-key-1234567890") {
		t.Fatalf("API Key 未脱敏: %s", view.Config.APIKey)
	}
	if !strings.Contains(view.Config.APIKey, "****") {
		t.Fatalf("API Key 应包含掩码: %s", view.Config.APIKey)
	}
	if view.Config.PageSize != 10 || view.Config.SlowSQLMS != 200 {
		t.Fatalf("配置读取不一致: %+v", view.Config)
	}
	if view.Config.Database == nil || view.Config.Database.Password == "" {
		t.Fatal("数据库配置应回显（口令脱敏）")
	}
	if strings.Contains(view.Config.Database.Password, "db-pass-1234567") {
		t.Fatal("数据库口令未脱敏")
	}
}

// TestSystemConfigUpdatePersistsAndApplies 校验保存后落盘并触发热更新。
func TestSystemConfigUpdatePersistsAndApplies(t *testing.T) {
	svc, applier, path := newSystemConfig(t)

	view, err := svc.Update(context.Background(), service.ConfigPayload{
		PageSize:   30,
		SlowSQLMS:  500,
		APIServers: []string{"127.0.0.1", "10.0.0.10"},
	})
	if err != nil {
		t.Fatalf("保存配置失败: %v", err)
	}

	if view.Config.PageSize != 30 || view.Config.SlowSQLMS != 500 {
		t.Fatalf("返回值未更新: %+v", view.Config)
	}
	if len(view.RestartRequired) != 0 {
		t.Fatalf("仅修改热更新项不应要求重启: %v", view.RestartRequired)
	}
	if applier.calls != 1 || applier.applied == nil {
		t.Fatal("保存后应触发一次热更新")
	}
	if applier.applied.PageSize != 30 || applier.applied.SlowSQLThreshold != 500*time.Millisecond {
		t.Fatalf("热更新配置不正确: %+v", applier.applied)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取配置文件失败: %v", err)
	}
	text := string(content)
	if !hasKeyValue(text, "PAGEMax", "30") {
		t.Fatalf("分页大小未写入文件: %s", text)
	}
	if !hasKeyValue(text, "SLOW_SQL_MS", "500") {
		t.Fatalf("慢 SQL 阈值未写入文件: %s", text)
	}
	if !hasKeyValue(text, "API_SERVERS", "127.0.0.1,10.0.0.10") {
		t.Fatalf("节点列表未写入文件: %s", text)
	}
	if !strings.Contains(text, "; 测试用配置") {
		t.Fatal("写入时丢失了原有注释")
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatalf("应生成 .bak 备份: %v", err)
	}
}

// TestSystemConfigUpdateKeepsMaskedSecrets 校验传入脱敏串时保留原密钥。
func TestSystemConfigUpdateKeepsMaskedSecrets(t *testing.T) {
	svc, _, path := newSystemConfig(t)
	view, err := svc.View(context.Background())
	if err != nil {
		t.Fatalf("读取配置失败: %v", err)
	}

	if _, err := svc.Update(context.Background(), service.ConfigPayload{
		APIKey:        view.Config.APIKey,
		SessionSecret: view.Config.SessionSecret,
		PageSize:      20,
	}); err != nil {
		t.Fatalf("保存配置失败: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取配置文件失败: %v", err)
	}
	if !hasKeyValue(string(content), "API_KEY", "initial-key-1234567890") {
		t.Fatal("脱敏串不应覆盖原 API Key")
	}
	if !hasKeyValue(string(content), "SESSION_SECRET", "session-secret-1234567890") {
		t.Fatal("脱敏串不应覆盖原会话密钥")
	}
}

// TestSystemConfigRestartFlags 校验需要重启的字段被标记。
func TestSystemConfigRestartFlags(t *testing.T) {
	svc, _, _ := newSystemConfig(t)

	view, err := svc.Update(context.Background(), service.ConfigPayload{
		HTTPPort: 6000,
		Database: &service.DatabasePayload{Host: "192.168.1.10", Port: 3307},
	})
	if err != nil {
		t.Fatalf("保存配置失败: %v", err)
	}

	joined := strings.Join(view.RestartRequired, " ")
	if !strings.Contains(joined, "HTTP_PORT") {
		t.Fatalf("端口变更应提示重启: %v", view.RestartRequired)
	}
	if !strings.Contains(joined, "database") {
		t.Fatalf("数据库变更应提示重启: %v", view.RestartRequired)
	}
}

// TestSystemConfigValidation 校验非法入参被拒绝。
func TestSystemConfigValidation(t *testing.T) {
	svc, _, _ := newSystemConfig(t)
	ctx := context.Background()

	cases := []struct {
		name    string
		payload service.ConfigPayload
	}{
		{name: "分页过大", payload: service.ConfigPayload{PageSize: 500}},
		{name: "会话有效期非法", payload: service.ConfigPayload{SessionTTL: "abc"}},
		{name: "验证码有效期过长", payload: service.ConfigPayload{CaptchaTTL: "5h"}},
		{name: "慢 SQL 阈值过大", payload: service.ConfigPayload{SlowSQLMS: 90000}},
		{name: "会话密钥过短", payload: service.ConfigPayload{SessionSecret: "short"}},
		{name: "运行模式非法", payload: service.ConfigPayload{RunMode: "staging"}},
		{name: "节点为空", payload: service.ConfigPayload{APIServers: []string{"  "}}},
		{name: "数据库端口非法", payload: service.ConfigPayload{Database: &service.DatabasePayload{Port: 99999}}},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if _, err := svc.Update(ctx, item.payload); err == nil {
				t.Fatal("期望返回校验错误")
			} else if _, ok := service.AsError(err); !ok {
				t.Fatalf("应返回业务错误，实际 %T", err)
			}
		})
	}
}

// TestSystemConfigReload 校验从磁盘重新加载。
func TestSystemConfigReload(t *testing.T) {
	svc, _, path := newSystemConfig(t)

	// 手工改文件后 reload 应生效
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取配置文件失败: %v", err)
	}
	updated := strings.Replace(string(content), "PAGEMax = 10", "PAGEMax = 50", 1)
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		t.Fatalf("写入配置文件失败: %v", err)
	}

	view, err := svc.Reload(context.Background())
	if err != nil {
		t.Fatalf("重新加载失败: %v", err)
	}
	if view.Config.PageSize != 50 {
		t.Fatalf("重新加载未生效: %d", view.Config.PageSize)
	}
}

// TestSystemConfigRaw 校验原文读取。
func TestSystemConfigRaw(t *testing.T) {
	svc, _, _ := newSystemConfig(t)
	content, err := svc.Raw(context.Background())
	if err != nil {
		t.Fatalf("读取原文失败: %v", err)
	}
	if !strings.Contains(content, "[security]") {
		t.Fatalf("原文内容异常: %s", content)
	}
}

// hasKeyValue 判断 ini 文本中某个键的取值（忽略等号两侧的对齐空格）。
func hasKeyValue(text, key, want string) bool {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, key) {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) == key && strings.TrimSpace(parts[1]) == want {
			return true
		}
	}
	return false
}
