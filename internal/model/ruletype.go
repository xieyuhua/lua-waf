package model

// RuleType 描述一类 WAF 规则，同时用于生成后台的规则子菜单。
type RuleType struct {
	// Key 为数据库中 rule_type 的取值。
	Key string `json:"key"`
	// Name 为中文展示名（子菜单标题）。
	Name string `json:"name"`
	// Group 用于把子菜单归类：拦截策略 / 访问控制。
	Group string `json:"group"`
	// Desc 为子菜单页面的说明文案。
	Desc string `json:"desc"`
	// File 为下发到 openresty 的规则文件名。
	File string `json:"file"`
}

// RuleCatalog 维护受支持的规则类型清单及其顺序。
type RuleCatalog struct {
	types []RuleType
}

// Catalog 是全局规则类型目录，顺序即后台子菜单顺序。
var Catalog = RuleCatalog{types: []RuleType{
	{Key: "args", Name: "URL 参数规则", Group: "拦截策略", Desc: "针对 GET 参数的注入特征匹配", File: "args.rule"},
	{Key: "post", Name: "POST 参数规则", Group: "拦截策略", Desc: "针对请求体的注入特征匹配", File: "post.rule"},
	{Key: "cookie", Name: "Cookie 规则", Group: "拦截策略", Desc: "针对 Cookie 头的注入特征匹配", File: "cookie.rule"},
	{Key: "url", Name: "URL 规则", Group: "拦截策略", Desc: "针对请求路径的特征匹配", File: "url.rule"},
	{Key: "useragent", Name: "User-Agent 规则", Group: "拦截策略", Desc: "拦截扫描器、爬虫类 UA", File: "useragent.rule"},
	{Key: "headers", Name: "Header 规则", Group: "拦截策略", Desc: "针对自定义请求头的特征匹配", File: "headers.rule"},
	{Key: "blackip", Name: "IP 黑名单", Group: "访问控制", Desc: "命中即拦截的来源 IP", File: "blackip.rule"},
	{Key: "whiteip", Name: "IP 白名单", Group: "访问控制", Desc: "不受 WAF 检测的来源 IP", File: "whiteip.rule"},
	{Key: "whiteUrl", Name: "URL 白名单", Group: "访问控制", Desc: "免检测的请求路径", File: "whiteUrl.rule"},
}}

// All 返回全部规则类型。
func (c RuleCatalog) All() []RuleType {
	out := make([]RuleType, len(c.types))
	copy(out, c.types)
	return out
}

// Get 按 key 查找规则类型。
func (c RuleCatalog) Get(key string) (RuleType, bool) {
	for _, item := range c.types {
		if item.Key == key {
			return item, true
		}
	}
	return RuleType{}, false
}

// Known 判断 key 是否为受支持的规则类型。
func (c RuleCatalog) Known(key string) bool {
	_, ok := c.Get(key)
	return ok
}

// Name 返回规则类型的中文名，未知类型回退为原始 key。
func (c RuleCatalog) Name(key string) string {
	if item, ok := c.Get(key); ok {
		return item.Name
	}
	return key
}

// KeyOfFile 由规则文件名反查规则类型 key。
func (c RuleCatalog) KeyOfFile(file string) (string, bool) {
	for _, item := range c.types {
		if item.File == file {
			return item.Key, true
		}
	}
	return "", false
}
