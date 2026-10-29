package service

// 本文件实现「按 URL 的精细化限频」规则服务。
//
// 与全局 CC/IP 频率（config_cc_check / config_ipcc_check）的差别：
//   - 全局只有一组阈值；
//   - 这里是**多条规则**：每条规则 = URL 匹配 + 计数维度（IP / Header / Cookie / 参数 / URI）+ 阈值 + 窗口 + 动作，
//     可以做到「登录接口按 IP 每分钟 10 次」「开放 API 按 X-API-Key 每分钟 1000 次」。
//
// 规则存储在各仓储里，启用后由本服务渲染成 JSON 数组写入 <RULE_PATH>/ratelimit.rule，
// 由 lua-waf/ratelimit.lua 读取执行（命中第一条匹配规则即生效）。

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"wafadmin/internal/model"
	"wafadmin/internal/repository"
)

// 限频规则的匹配方式。
const (
	// RateMatchPrefix 为 URL 前缀匹配（性能最好，适合 /api/login 这类固定路径）。
	RateMatchPrefix = "prefix"
	// RateMatchRegex 为 PCRE 正则匹配（lua 侧用 ngx.re.find，忽略大小写）。
	RateMatchRegex = "regex"
)

// 限频规则的计数维度。
const (
	// RateKeyIP 按来源 IP 计数。
	RateKeyIP = "ip"
	// RateKeyHeader 按请求头计数（如 X-API-Key）。
	RateKeyHeader = "header"
	// RateKeyCookie 按 Cookie 计数。
	RateKeyCookie = "cookie"
	// RateKeyArg 按 URL 参数计数。
	RateKeyArg = "arg"
	// RateKeyURI 按完整 URI 计数（等价「每个 URL 各自计数」）。
	RateKeyURI = "uri"
)

// 超限后的动作。
const (
	// RateActionBlock 直接拦截。
	RateActionBlock = "block"
	// RateActionLog 只记录不拦截（用于先观察是否误伤）。
	RateActionLog = "log"
)

// RateLimitRuleView 是返回给前端的规则（带可读说明）。
type RateLimitRuleView struct {
	model.RateLimitRule
	// KeyLabel 为维度可读名（如「Header：X-API-Key」）。
	KeyLabel string `json:"key_label"`
	// RateText 为可读阈值（如「60 次 / 60 秒」）。
	RateText string `json:"rate_text"`
}

// RateLimitService 提供限频规则的增删改查与下发。
type RateLimitService struct {
	// Rules 为规则存储。
	Rules repository.RateLimitRepository
	// RuleDir 为 lua 侧规则目录（下发 ratelimit.rule 到这里）。
	RuleDir string
	// Reloader 可选：下发后让 nginx 重新加载规则文件。
	Reloader LuaReloader
}

// 规则文件在 RULE_PATH 下的名字。
const rateLimitFileName = "ratelimit.rule"

// List 返回全部规则（含停用的），并补充可读字段。
func (s *RateLimitService) List(ctx context.Context) ([]RateLimitRuleView, error) {
	rules, err := s.Rules.List(ctx)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "读取限频规则失败")
	}

	views := make([]RateLimitRuleView, 0, len(rules))
	for _, rule := range rules {
		views = append(views, RateLimitRuleView{
			RateLimitRule: rule,
			KeyLabel:      describeRateKey(rule),
			RateText:      fmt.Sprintf("%d 次 / %d 秒", rule.Limit, rule.Window),
		})
	}
	return views, nil
}

// Create 新建规则并下发。
func (s *RateLimitService) Create(ctx context.Context, input model.RateLimitRule) (*RateLimitRuleView, error) {
	rule, err := normalizeRateRule(input)
	if err != nil {
		return nil, err
	}
	rule.Id = 0

	if err := s.Rules.Create(ctx, &rule); err != nil {
		return nil, ErrInternal.Wrap(err, "保存限频规则失败")
	}
	if err := s.sync(ctx); err != nil {
		return nil, err
	}

	return &RateLimitRuleView{
		RateLimitRule: rule,
		KeyLabel:      describeRateKey(rule),
		RateText:      fmt.Sprintf("%d 次 / %d 秒", rule.Limit, rule.Window),
	}, nil
}

// Update 更新规则并下发。
func (s *RateLimitService) Update(ctx context.Context, id int64, input model.RateLimitRule) (*RateLimitRuleView, error) {
	if id <= 0 {
		return nil, ErrInvalid.New("规则 ID 不合法")
	}

	exists, err := s.Rules.Get(ctx, id)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "读取限频规则失败")
	}
	if exists == nil {
		return nil, ErrNotFound.New("限频规则不存在")
	}

	rule, err := normalizeRateRule(input)
	if err != nil {
		return nil, err
	}
	rule.Id = id

	if err := s.Rules.Update(ctx, &rule); err != nil {
		return nil, ErrInternal.Wrap(err, "更新限频规则失败")
	}
	if err := s.sync(ctx); err != nil {
		return nil, err
	}

	return &RateLimitRuleView{
		RateLimitRule: rule,
		KeyLabel:      describeRateKey(rule),
		RateText:      fmt.Sprintf("%d 次 / %d 秒", rule.Limit, rule.Window),
	}, nil
}

// SetEnabled 启停规则并下发。
func (s *RateLimitService) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	rule, err := s.Rules.Get(ctx, id)
	if err != nil {
		return ErrInternal.Wrap(err, "读取限频规则失败")
	}
	if rule == nil {
		return ErrNotFound.New("限频规则不存在")
	}

	rule.Enabled = enabled
	if err := s.Rules.Update(ctx, rule); err != nil {
		return ErrInternal.Wrap(err, "更新限频规则失败")
	}
	return s.sync(ctx)
}

// Delete 删除规则并下发。
func (s *RateLimitService) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrInvalid.New("规则 ID 不合法")
	}
	if err := s.Rules.Delete(ctx, id); err != nil {
		return ErrInternal.Wrap(err, "删除限频规则失败")
	}
	return s.sync(ctx)
}

// Sync 手动下发（把启用规则写入 ratelimit.rule，必要时 reload）。
func (s *RateLimitService) Sync(ctx context.Context) (map[string]any, error) {
	if err := s.sync(ctx); err != nil {
		return nil, err
	}

	rules, err := s.Rules.List(ctx)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "读取限频规则失败")
	}

	enabled := 0
	for _, rule := range rules {
		if rule.Enabled {
			enabled++
		}
	}
	return map[string]any{
		"path":    filepath.Join(s.RuleDir, rateLimitFileName),
		"total":   len(rules),
		"enabled": enabled,
		"message": fmt.Sprintf("已下发 %d 条启用规则（共 %d 条）", enabled, len(rules)),
	}, nil
}

// sync 把启用规则渲染到规则文件。
func (s *RateLimitService) sync(ctx context.Context) error {
	rules, err := s.Rules.List(ctx)
	if err != nil {
		return ErrInternal.Wrap(err, "读取限频规则失败")
	}

	payload := make([]map[string]any, 0, len(rules))
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		payload = append(payload, map[string]any{
			"id":       rule.Id,
			"name":     rule.Name,
			"pattern":  rule.Pattern,
			"match":    rule.Match,
			"key":      rule.Key,
			"key_name": rule.KeyName,
			"limit":    rule.Limit,
			"window":   rule.Window,
			"action":   rule.Action,
			"enabled":  true,
		})
	}

	content, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return ErrInternal.Wrap(err, "序列化限频规则失败")
	}

	path := filepath.Join(s.RuleDir, rateLimitFileName)
	if err := os.MkdirAll(s.RuleDir, 0o755); err != nil {
		return ErrInternal.Wrap(err, "创建规则目录失败")
	}
	if err := os.WriteFile(path, append(content, '\n'), 0o644); err != nil {
		return ErrInternal.Wrap(err, "写入限频规则文件失败")
	}

	// 规则文件由 lua 侧按 mtime 热加载，这里不强制 reload；调用方仍可手动 reload。
	if s.Reloader != nil {
		_, _ = s.Reloader.Reload()
	}
	return nil
}

// normalizeRateRule 校验并规整一条规则。
func normalizeRateRule(input model.RateLimitRule) (model.RateLimitRule, error) {
	rule := input

	rule.Name = strings.TrimSpace(rule.Name)
	if rule.Name == "" {
		return rule, ErrInvalid.New("规则名称不能为空")
	}
	if len([]rune(rule.Name)) > 64 {
		return rule, ErrInvalid.New("规则名称过长（最多 64 个字符）")
	}

	rule.Pattern = strings.TrimSpace(rule.Pattern)
	if rule.Pattern == "" {
		return rule, ErrInvalid.New("URL 匹配内容不能为空")
	}
	if len(rule.Pattern) > 255 {
		return rule, ErrInvalid.New("URL 匹配内容过长（最多 255 个字符）")
	}

	rule.Match = strings.ToLower(strings.TrimSpace(rule.Match))
	switch rule.Match {
	case "":
		rule.Match = RateMatchPrefix
	case RateMatchPrefix, RateMatchRegex:
	default:
		return rule, ErrInvalid.New("匹配方式只能是 prefix 或 regex")
	}
	if rule.Match == RateMatchRegex {
		if _, err := regexp.Compile(rule.Pattern); err != nil {
			return rule, ErrInvalid.Wrap(err, "URL 正则不合法")
		}
	}

	rule.Key = strings.ToLower(strings.TrimSpace(rule.Key))
	switch rule.Key {
	case "":
		rule.Key = RateKeyIP
	case RateKeyIP, RateKeyHeader, RateKeyCookie, RateKeyArg, RateKeyURI:
	default:
		return rule, ErrInvalid.New("计数维度只能是 ip / header / cookie / arg / uri")
	}

	rule.KeyName = strings.TrimSpace(rule.KeyName)
	if rule.Key == RateKeyHeader || rule.Key == RateKeyCookie || rule.Key == RateKeyArg {
		if rule.KeyName == "" {
			return rule, ErrInvalid.New("选择了 header / cookie / 参数维度时必须填写维度名（如 X-API-Key）")
		}
		if len(rule.KeyName) > 64 {
			return rule, ErrInvalid.New("维度名过长（最多 64 个字符）")
		}
	} else {
		// ip / uri 维度不需要维度名
		rule.KeyName = ""
	}

	if rule.Limit < 1 || rule.Limit > 1000000 {
		return rule, ErrInvalid.New("阈值必须在 1 ~ 1000000 之间")
	}
	if rule.Window < 1 || rule.Window > 86400 {
		return rule, ErrInvalid.New("统计窗口必须在 1 ~ 86400 秒之间")
	}

	rule.Action = strings.ToLower(strings.TrimSpace(rule.Action))
	switch rule.Action {
	case "":
		rule.Action = RateActionBlock
	case RateActionBlock, RateActionLog:
	default:
		return rule, ErrInvalid.New("超限动作只能是 block（拦截）或 log（只记录）")
	}

	return rule, nil
}

// describeRateKey 生成维度的可读说明。
func describeRateKey(rule model.RateLimitRule) string {
	switch rule.Key {
	case RateKeyIP:
		return "来源 IP"
	case RateKeyURI:
		return "完整 URI"
	case RateKeyHeader:
		return "请求头 " + rule.KeyName
	case RateKeyCookie:
		return "Cookie " + rule.KeyName
	case RateKeyArg:
		return "URL 参数 " + rule.KeyName
	default:
		return rule.Key
	}
}
