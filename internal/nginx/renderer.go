// Package nginx 负责把后台的站点与规则配置渲染成 openresty 可用的文件，
// 并在需要时触发 nginx 配置检查与重载。
package nginx

import (
	"bytes"
	"fmt"
	"path"
	"strings"
	"sync"
	"text/template"
	"time"

	"wafadmin/internal/model"
)

// SiteView 是模板渲染用的站点视图。
type SiteView struct {
	SiteName      string
	Port          int
	SSLPort       int
	SSLEnabled    bool
	CertFile      string
	KeyFile       string
	BackendAddr   []string
	Upstream      string
	HasUnreal     bool
	UnrealPattern string
	AccessLog     string
	ErrorLog      string
	DebugLevel    string
	GeneratedAt   string
}

// Manager 汇总 nginx 相关配置，并提供渲染、写入、重载能力。
// DryRun 为 true 时只渲染不落盘（演示模式使用）。
type Manager struct {
	// Bin 为 nginx / openresty 可执行文件路径。
	Bin string
	// VhostsDir 为站点配置目录。
	VhostsDir string
	// CertDir 为证书目录，按站点名建立子目录。
	CertDir string
	// RuleDir 为 WAF 规则文件目录。
	RuleDir string
	// LogDir 为站点访问日志目录。
	LogDir string
	// DryRun 为 true 时不写入文件、不重载 nginx。
	DryRun bool

	once sync.Once
	tmpl *template.Template
	err  error
}

// Render 渲染站点 vhost 配置。
func (m *Manager) Render(site model.Site) (string, error) {
	tmpl, err := m.template()
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, m.view(site)); err != nil {
		return "", fmt.Errorf("渲染站点 %s 配置失败: %w", site.SiteName, err)
	}
	return buf.String(), nil
}

// template 懒加载并缓存解析后的模板。
func (m *Manager) template() (*template.Template, error) {
	m.once.Do(func() {
		m.tmpl, m.err = template.New("proxy").Parse(proxyTemplate)
	})
	return m.tmpl, m.err
}

// view 把领域对象转换为模板数据。
func (m *Manager) view(site model.Site) SiteView {
	unreal := make([]string, 0, len(site.UnrealAddr))
	for _, item := range site.UnrealAddr {
		if item = strings.TrimSpace(item); item != "" {
			unreal = append(unreal, regexpQuote(item))
		}
	}

	port := site.Port
	if port <= 0 {
		port = 80
	}
	sslPort := 443
	if site.SSLEnabled() && port != 443 {
		sslPort = 443
	} else if site.SSLEnabled() {
		sslPort = port
	}

	return SiteView{
		SiteName:      site.SiteName,
		Port:          port,
		SSLPort:       sslPort,
		SSLEnabled:    site.SSLEnabled(),
		CertFile:      path.Join(m.certDir(), site.SiteName, "fullchain.pem"),
		KeyFile:       path.Join(m.certDir(), site.SiteName, "privkey.pem"),
		BackendAddr:   site.BackendAddr,
		Upstream:      site.Upstream(),
		HasUnreal:     len(unreal) > 0,
		UnrealPattern: strings.Join(unreal, "|"),
		AccessLog:     path.Join(m.logDir(), site.SiteName+".access.log"),
		ErrorLog:      path.Join(m.logDir(), site.SiteName+".error.log"),
		DebugLevel:    normalizeLevel(site.DebugLevel),
		GeneratedAt:   time.Now().Format("2006-01-02 15:04:05"),
	}
}

// certDir 返回生效的证书目录。
func (m *Manager) certDir() string {
	if m.CertDir == "" {
		return "/www/server/panel/vhost/cert"
	}
	return m.CertDir
}

// logDir 返回生效的日志目录。
func (m *Manager) logDir() string {
	if m.LogDir == "" {
		return "/www/wwwlogs/waf"
	}
	return m.LogDir
}

// normalizeLevel 校正 nginx 日志级别。
func normalizeLevel(level string) string {
	allowed := map[string]struct{}{
		"debug": {}, "info": {}, "notice": {}, "warn": {},
		"error": {}, "crit": {}, "alert": {}, "emerg": {},
	}
	lower := strings.ToLower(strings.TrimSpace(level))
	if _, ok := allowed[lower]; ok {
		return lower
	}
	return "error"
}

// regexpQuote 转义正则元字符，用于把 IP 原样嵌入匹配表达式。
func regexpQuote(raw string) string {
	var out strings.Builder
	for _, char := range raw {
		switch char {
		case '.', '*', '+', '?', '(', ')', '[', ']', '{', '}', '^', '$', '|', '\\':
			out.WriteByte('\\')
			out.WriteRune(char)
		default:
			out.WriteRune(char)
		}
	}
	return out.String()
}
