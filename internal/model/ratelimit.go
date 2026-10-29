package model

import "time"

// RateLimitRule 是按 URL 的精细化限频规则。
//
// 一条规则 = 「URL 匹配」+「计数维度」+「阈值/窗口」：
//   - Pattern/Match 决定命中哪些请求（前缀或正则）；
//   - Key 决定按什么计数：ip（来源 IP）、header（如 X-API-Key）、cookie、arg（URL 参数）、uri（整个 URI）；
//   - Limit/Window 决定阈值，Action 决定超限后拦截还是只记录。
//
// 规则会下发为 lua 侧的 rules/ratelimit.rule（JSON 数组），由 lua-waf/ratelimit.lua 读取执行。
type RateLimitRule struct {
	// Id 为主键，同时作为 lua 侧计数键的一部分（rl:<id>:<维度值>）。
	Id int64 `xorm:"pk autoincr 'id'" json:"id"`
	// Name 为规则名（便于识别，如「登录接口」）。
	Name string `xorm:"'name' varchar(64) notnull" json:"name"`
	// Pattern 为 URL 匹配串：prefix 模式是前缀，regex 模式是 PCRE 正则。
	Pattern string `xorm:"'pattern' varchar(255) notnull" json:"pattern"`
	// Match 为匹配方式：prefix / regex。
	Match string `xorm:"'match_type' varchar(16) notnull" json:"match"`
	// Key 为计数维度：ip / header / cookie / arg / uri。
	Key string `xorm:"'key_type' varchar(16) notnull" json:"key"`
	// KeyName 为维度名（header/cookie/arg 时必填，如 X-API-Key）。
	KeyName string `xorm:"'key_name' varchar(64)" json:"key_name"`
	// Limit 为窗口内允许的最大次数（列名避开 MySQL 关键字 limit）。
	Limit int `xorm:"'limit_count' notnull" json:"limit"`
	// Window 为统计窗口（秒，列名避开关键字 window）。
	Window int `xorm:"'window_sec' notnull" json:"window"`
	// Action 为超限动作：block（拦截）/ log（只记录，用于观察）。
	Action string `xorm:"'action' varchar(16) notnull" json:"action"`
	// Enabled 为是否启用；停用的规则不会下发。
	Enabled bool `xorm:"'enabled' notnull default 1" json:"enabled"`
	// UpdatedAt 为最后修改时间。
	UpdatedAt time.Time `xorm:"'updated_at' updated" json:"updated_at"`
}
