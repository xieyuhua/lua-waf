package model

import "time"

// Site 表示一个受 WAF 保护的后端站点。
// 表名：site
type Site struct {
	Id       int64  `xorm:"pk autoincrement" json:"id"`
	SiteName string `xorm:"varchar(190) unique not null" json:"site_name"`
	Port     int    `json:"port"`
	// BackendAddr 为后端应用服务器地址列表，例如 10.0.0.1:80。
	BackendAddr []string `json:"backend_addr"`
	// UnrealAddr 为“诱饵地址”：来自这些地址的请求直接返回 404，不做反向代理。
	UnrealAddr []string  `xorm:"unreal_addr" json:"unreal_addr"`
	Ssl        string    `xorm:"varchar(10) not null default 'off'" json:"ssl"`
	DebugLevel string    `xorm:"varchar(10) not null default 'error'" json:"debug_level"`
	LastChange time.Time `xorm:"updated" json:"last_change"`
	Version    int       `xorm:"version" json:"version"`
}

// SSL 生效时返回 true。
func (s Site) SSLEnabled() bool { return s.Ssl == "on" }

// Upstream 返回 nginx upstream 名称。
func (s Site) Upstream() string { return upstreamName(s.SiteName) }

// upstreamName 把域名转换为合法的 nginx upstream / 变量名。
func upstreamName(siteName string) string {
	out := make([]rune, 0, len(siteName)+8)
	for _, r := range siteName {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			out = append(out, r)
		case r == '-' || r == '_':
			out = append(out, '_')
		case r == '.':
			out = append(out, '_')
		default:
			out = append(out, '_')
		}
	}
	return "upstream_" + string(out)
}
