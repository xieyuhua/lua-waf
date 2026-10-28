package service

import "context"

// RulePipelineStep 是 WAF 检测链路上的一个检查步骤。
type RulePipelineStep struct {
	// Order 为执行顺序（从 1 开始）。
	Order int `json:"order"`
	// Name 为函数名（对应 waf.lua）。
	Name string `json:"name"`
	// Title 为中文名称。
	Title string `json:"title"`
	// Action 为命中后的动作：allow（直接放行）/ deny（拦截）/ inspect（检测，命中则拦截）。
	Action string `json:"action"`
	// RuleFile 为该步骤使用的规则文件（为空表示由代码或配置实现）。
	RuleFile string `json:"rule_file"`
	// Switch 为控制该步骤的配置项（config.lua 中的键）。
	Switch string `json:"switch"`
	// Desc 为说明。
	Desc string `json:"desc"`
}

// RulePipeline 描述 waf.lua 的检测顺序。
type RulePipeline struct {
	// Steps 为有序步骤。
	Steps []RulePipelineStep `json:"steps"`
	// Notes 为整体说明。
	Notes []string `json:"notes"`
}

// RulePipelineService 提供检测顺序的只读说明。
//
// 顺序与 lua-waf/waf.lua 的 _M.check() 保持一致，改动 lua 时需同步更新这里。
type RulePipelineService struct{}

// Pipeline 返回检测链路说明。
func (s *RulePipelineService) Pipeline(ctx context.Context) (*RulePipeline, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	steps := []RulePipelineStep{
		{
			Order:    1,
			Name:     "white_ip_check",
			Title:    "IP 白名单",
			Action:   "allow",
			RuleFile: "whiteip.rule",
			Switch:   "config_white_ip_check",
			Desc:     "来源 IP 命中白名单直接放行，跳过后续全部检查（含黑名单）。用于内部系统、监控拨测。",
		},
		{
			Order:  2,
			Name:   "ban_check",
			Title:  "封禁名单 / 自动封禁",
			Action: "deny",
			Switch: "config_auto_ban_check",
			Desc:   "检查共享内存封禁表（后台「IP 黑名单」下发 + 命中攻击后自动封禁写入）。命中直接返回封禁页，不写新日志，避免重复刷屏。",
		},
		{
			Order:  3,
			Name:   "malformed_request_check",
			Title:  "畸形请求检测",
			Action: "deny",
			Switch: "config_malformed_check",
			Desc:   "请求行/头字段非法（超长、控制字符、重复 Host、异常编码）直接在解析前拦掉，避免绕过后续正则检测。",
		},
		{
			Order:    4,
			Name:     "black_ip_check",
			Title:    "IP 黑名单",
			Action:   "deny",
			RuleFile: "blackip.rule",
			Switch:   "config_black_ip_check",
			Desc:     "来源 IP 命中静态黑名单规则（可写单个 IP、网段或正则）时拦截。",
		},
		{
			Order:    5,
			Name:     "user_agent_attack_check",
			Title:    "User-Agent 检测",
			Action:   "inspect",
			RuleFile: "useragent.rule",
			Switch:   "config_user_agent_check",
			Desc:     "扫描器、漏洞利用工具、脚本化客户端 UA 命中即拦截；放在 URL 检测之前，可以先挡掉无意义的扫描流量。",
		},
		{
			Order:    6,
			Name:     "white_url_check",
			Title:    "URL 白名单",
			Action:   "allow",
			RuleFile: "whiteUrl.rule",
			Switch:   "config_white_url_check",
			Desc:     "URL 命中白名单直接放行，跳过后续 URL/参数/请求体检测（IP 白名单与封禁检查仍然优先）。",
		},
		{
			Order:    7,
			Name:     "url_attack_check",
			Title:    "URL 检测",
			Action:   "inspect",
			RuleFile: "url.rule",
			Switch:   "config_url_check",
			Desc:     "对请求 URI 做正则匹配：目录穿越、备份文件、CVE 探测路径（actuator/nacos/moveit 等）、命令注入分隔符。",
		},
		{
			Order:  8,
			Name:   "cc_attack_check",
			Title:  "CC 频率检测",
			Action: "inspect",
			Switch: "config_cc_check / config_cc_rate",
			Desc:   "按「IP + URI」在时间窗口内计数，超过 config_cc_rate（如 300/60）即判定 CC 攻击，可联动自动封禁。",
		},
		{
			Order:    9,
			Name:     "cookie_attack_check",
			Title:    "Cookie 检测",
			Action:   "inspect",
			RuleFile: "cookie.rule",
			Switch:   "config_cookie_check",
			Desc:     "Cookie 中的 SQL 注入、XSS、Shiro rememberMe、Java 序列化（rO0AB）等特征。",
		},
		{
			Order:    10,
			Name:     "url_args_attack_check",
			Title:    "GET 参数检测",
			Action:   "inspect",
			RuleFile: "args.rule",
			Switch:   "config_url_args_check",
			Desc:     "对解码后的查询串做正则匹配（SQL 注入、命令执行、SSRF、SSTI、Log4Shell、原型链等）。",
		},
		{
			Order:    11,
			Name:     "post_attack_check",
			Title:    "请求体检测",
			Action:   "inspect",
			RuleFile: "post.rule",
			Switch:   "config_post_check / config_post_multipart_check / config_post_max_size",
			Desc:     "读取请求体（超过 config_post_max_size 只按流式/分段检测），对表单、JSON、multipart 分段内容做同样的正则匹配。",
		},
	}

	notes := []string{
		"检测顺序是串行的：任一检查命中即结束本次请求的后续检查（白名单命中则直接放行）。",
		"白名单（IP / URL）优先于所有检测；封禁名单优先于规则检测，可第一时间压掉已知攻击源。",
		"白名单 URL 会跳过 URL / 参数 / 请求体检测，因此只应放行确有需要的静态或回调地址。",
		"命中检测后统一记录日志（类型、规则 ID、命中内容），再按 config_waf_model 输出拦截页（html / redirect / 幻影降级）。",
		"自动封禁（config_auto_ban_check=on）会按 IP 在窗口内累计命中次数，达到阈值后写入封禁表，后续请求在第 2 步就被拦掉。",
	}

	return &RulePipeline{Steps: steps, Notes: notes}, nil
}
