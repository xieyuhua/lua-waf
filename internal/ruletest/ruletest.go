// Package ruletest 提供 WAF 规则的「可命中性自检」。
//
// 解决的问题：规则库里经常混着几条永远匹配不上的规则（正则写错、大小写写反、转义多了一层），
// 平时没人发现，直到真的被绕过才暴露。本包把每条规则拿一组**贴近真实攻击的样例载荷**跑一遍，
// 输出三种异常：
//   - bad-regex：正则本身不合法（Go 与 PCRE 都不认）；
//   - no-match：正则合法，但样例载荷一条都没命中（大概率是写错了，需要人工确认）；
//   - go-incompat：Go 的 RE2 不支持该语法（如反向引用/前瞻），但 lua 侧 PCRE 可能是有效的。
//
// 另外支持「给定一条请求载荷，看看会命中哪些规则」，用于上线前验证拦截链路。
package ruletest

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"wafadmin/internal/model"
)

// 规则自检的状态。
const (
	// StatusHit 表示样例载荷能命中该规则。
	StatusHit = "hit"
	// StatusNoMatch 表示正则合法但样例载荷都没命中。
	StatusNoMatch = "no-match"
	// StatusBadRegex 表示正则明显不合法。
	StatusBadRegex = "bad-regex"
	// StatusGoIncompat 表示 RE2 不支持，但 PCRE 可能有效。
	StatusGoIncompat = "go-incompat"
	// StatusSkipped 表示该类型无法用样例载荷判断（如 IP 黑名单）。
	StatusSkipped = "skipped"
	// StatusDead 表示按规则自己生成的样例都命中不了 —— 这条规则永远不会生效。
	StatusDead = "dead"
	// StatusNarrow 表示规则本身可用，只是内置攻击样例没覆盖它的形态（不算问题）。
	StatusNarrow = "narrow"
)

// 规则的检查顺序，与 lua-waf/waf.lua 的 check() 链路保持一致，
// 用于「给定载荷命中哪些规则」时按真实拦截顺序展示。
var CheckOrder = []string{
	"whiteip",
	"blackip",
	"header",
	"useragent",
	"whiteUrl",
	"url",
	"cookie",
	"args",
	"post",
}

// Result 是单条规则的自检结果。
type Result struct {
	// RuleID 为规则 ID。
	RuleID int64 `json:"rule_id"`
	// RuleType 为规则类型。
	RuleType string `json:"rule_type"`
	// RuleItem 为规则内容（正则）。
	RuleItem string `json:"rule_item"`
	// Enabled 表示规则是否启用（停用的规则只做语法检查）。
	Enabled bool `json:"enabled"`
	// Status 为自检结论，见 Status* 常量。
	Status string `json:"status"`
	// Hits 为命中的样例数量。
	Hits int `json:"hits"`
	// Sample 为第一个命中的样例（便于人工判断是否误杀/过宽）。
	Sample string `json:"sample,omitempty"`
	// Error 为错误说明（语法错误等）。
	Error string `json:"error,omitempty"`
}

// TypeSummary 是某类规则的汇总。
type TypeSummary struct {
	// Type 为规则类型。
	Type string `json:"type"`
	// Total 为规则总数。
	Total int `json:"total"`
	// Hit 为能命中的条数。
	Hit int `json:"hit"`
	// NoMatch 为样例未命中的条数（含死规则，兼容旧字段）。
	NoMatch int `json:"no_match"`
	// Dead 为规则自身无法命中的条数。
	Dead int `json:"dead"`
	// Narrow 为有效但样例未覆盖的条数。
	Narrow int `json:"narrow"`
	// BadRegex 为正则不合法的条数。
	BadRegex int `json:"bad_regex"`
	// GoIncompat 为 RE2 不支持的条数。
	GoIncompat int `json:"go_incompat"`
	// Skipped 为无法用样例判断的条数。
	Skipped int `json:"skipped"`
}

// Report 是一次自检的完整结果。
type Report struct {
	// Source 为规则来源说明（数据库 / 规则目录）。
	Source string `json:"source"`
	// GeneratedAt 为生成时间。
	GeneratedAt string `json:"generated_at"`
	// Total 为规则总数。
	Total int `json:"total"`
	// Hit / NoMatch / BadRegex / GoIncompat / Skipped 为各类结论条数。
	Hit     int `json:"hit"`
	NoMatch int `json:"no_match"`
	// Dead 为「连按规则自己生成的样例都命中不了」的死规则条数。
	Dead int `json:"dead"`
	// Narrow 为规则有效、但内置攻击样例未覆盖的条数（正常情况）。
	Narrow     int `json:"narrow"`
	BadRegex   int `json:"bad_regex"`
	GoIncompat int `json:"go_incompat"`
	Skipped    int `json:"skipped"`
	// Types 为按类型汇总（按检查顺序排列）。
	Types []TypeSummary `json:"types"`
	// Results 为逐条结果。
	Results []Result `json:"results"`
	// Payload 非空时表示这是「载荷匹配」模式：列出命中该载荷的规则。
	Payload string `json:"payload,omitempty"`
}

// Options 控制自检行为。
type Options struct {
	// Payload 非空时只判断这条载荷会命中哪些规则（不跑样例集）。
	Payload string
	// Type 非空时只检查该类型。
	Type string
	// IncludeDisabled 为 true 时也检查停用的规则（默认跳过停用规则）。
	IncludeDisabled bool
}

// Run 对规则集合执行自检。
//
// rules 来自数据库或规则文件；source 用于在报告里标明来源。
func Run(rules []model.Rule, source string, opts Options) Report {
	report := Report{
		Source:      source,
		GeneratedAt: time.Now().Format("2006-01-02 15:04:05"),
		Payload:     opts.Payload,
		Results:     make([]Result, 0, len(rules)),
	}

	if strings.TrimSpace(opts.Payload) != "" {
		return runPayload(rules, report, opts)
	}

	summaries := make(map[string]*TypeSummary, len(CheckOrder))
	for _, rule := range rules {
		if opts.Type != "" && rule.RuleType != opts.Type {
			continue
		}
		if !rule.Enabled && !opts.IncludeDisabled {
			continue
		}

		result := checkRule(rule)
		report.Results = append(report.Results, result)
		report.Total++

		switch result.Status {
		case StatusHit:
			report.Hit++
		case StatusNoMatch:
			report.NoMatch++
		case StatusDead:
			report.Dead++
			report.NoMatch++
		case StatusNarrow:
			report.Narrow++
		case StatusBadRegex:
			report.BadRegex++
		case StatusGoIncompat:
			report.GoIncompat++
		case StatusSkipped:
			report.Skipped++
		}

		summary, ok := summaries[rule.RuleType]
		if !ok {
			summary = &TypeSummary{Type: rule.RuleType}
			summaries[rule.RuleType] = summary
		}
		summary.Total++
		switch result.Status {
		case StatusHit:
			summary.Hit++
		case StatusNoMatch:
			summary.NoMatch++
		case StatusDead:
			summary.Dead++
			summary.NoMatch++
		case StatusNarrow:
			summary.Narrow++
		case StatusBadRegex:
			summary.BadRegex++
		case StatusGoIncompat:
			summary.GoIncompat++
		case StatusSkipped:
			summary.Skipped++
		}
	}

	report.Types = sortedSummaries(summaries)
	return report
}

// runPayload 判断一条载荷会命中哪些规则（按 lua 的检查顺序输出）。
func runPayload(rules []model.Rule, report Report, opts Options) Report {
	payload := opts.Payload
	for _, ruleType := range CheckOrder {
		if opts.Type != "" && ruleType != opts.Type {
			continue
		}

		subjects := subjectsFor(ruleType, payload)
		for _, rule := range rules {
			if rule.RuleType != ruleType || !rule.Enabled {
				continue
			}

			result := Result{RuleID: rule.Id, RuleType: rule.RuleType, RuleItem: rule.RuleItem, Enabled: true, Status: StatusNoMatch}
			report.Total++

			re, err := compile(rule.RuleItem)
			if err != nil {
				result.Status = classifyCompileError(err)
				result.Error = err.Error()
				report.Results = append(report.Results, result)
				continue
			}

			for _, subject := range subjects {
				if index := re.FindStringIndex(subject); index != nil {
					result.Status = StatusHit
					result.Hits = 1
					result.Sample = subject[index[0]:index[1]]
					break
				}
			}

			if result.Status == StatusHit {
				report.Hit++
			} else {
				report.NoMatch++
			}
			report.Results = append(report.Results, result)
		}
	}
	return report
}

// checkRule 用样例集检查单条规则。
func checkRule(rule model.Rule) Result {
	result := Result{RuleID: rule.Id, RuleType: rule.RuleType, RuleItem: rule.RuleItem, Enabled: rule.Enabled}

	re, err := compile(rule.RuleItem)
	if err != nil {
		result.Status = classifyCompileError(err)
		result.Error = err.Error()
		return result
	}

	// 第一步：用「按规则自己生成的样例」判断这条规则是否永远命中不了。
	// 连自己生成的样例都匹配不上，说明规则本身写坏了（转义多一层、括号不配对等）。
	if generated, ok := SampleFromRegex(rule.RuleItem); ok {
		if generated == "" {
			// 纯锚点/断言类规则，没有可生成的字符，退回到样例集判断
		} else if !re.MatchString(generated) {
			result.Status = StatusDead
			result.Error = fmt.Sprintf("按规则生成的样例 %q 也无法命中，规则可能写错", generated)
			return result
		}
	}

	// 第二步：跑内置攻击样例集，命中说明该规则覆盖了常见的真实攻击形态。
	// IP 名单 / URL 白名单这类规则没有固定样例，语法与自生成检查通过即算正常。
	samples := samplesFor(rule.RuleType)
	if len(samples) == 0 {
		result.Status = StatusSkipped
		return result
	}

	for _, sample := range samples {
		if index := re.FindStringIndex(sample); index != nil {
			result.Status = StatusHit
			result.Hits++
			if result.Sample == "" {
				result.Sample = sample[index[0]:index[1]]
			}
		}
	}

	if result.Hits == 0 {
		// 规则自身能匹配（说明有效），只是样例集没覆盖 → 不算问题
		result.Status = StatusNarrow
	}
	return result
}

// compile 按 lua 侧一致的大小写不敏感语义编译规则。
func compile(item string) (*regexp.Regexp, error) {
	pattern := item
	// lua 侧统一用 "joi"（大小写不敏感），这里对齐
	if !strings.HasPrefix(pattern, "(?i)") {
		pattern = "(?i)" + pattern
	}
	return regexp.Compile(pattern)
}

// classifyCompileError 区分「语法错误」与「RE2 不支持的 PCRE 特性」。
func classifyCompileError(err error) string {
	text := err.Error()
	// RE2 对反向引用/前瞻后顾/占有量词等会报 "invalid or unsupported Perl syntax"
	if strings.Contains(text, "invalid or unsupported Perl syntax") ||
		strings.Contains(text, "invalid escape sequence") ||
		strings.Contains(text, "invalid character class range") {
		return StatusGoIncompat
	}
	return StatusBadRegex
}

// sortedSummaries 按检查顺序输出类型汇总，未知类型排在最后。
func sortedSummaries(summaries map[string]*TypeSummary) []TypeSummary {
	result := make([]TypeSummary, 0, len(summaries))
	seen := make(map[string]bool, len(summaries))
	for _, ruleType := range CheckOrder {
		if summary, ok := summaries[ruleType]; ok {
			result = append(result, *summary)
			seen[ruleType] = true
		}
	}

	rest := make([]string, 0, len(summaries))
	for ruleType := range summaries {
		if !seen[ruleType] {
			rest = append(rest, ruleType)
		}
	}
	sort.Strings(rest)
	for _, ruleType := range rest {
		result = append(result, *summaries[ruleType])
	}
	return result
}

// subjectsFor 返回某类规则在该载荷下实际被匹配的文本（与 lua 侧取值方式一致）。
func subjectsFor(ruleType, payload string) []string {
	switch ruleType {
	case "args", "url":
		// lua 侧同时用原始串和 normalize() 后的串各匹配一次
		return []string{payload, normalize(payload)}
	default:
		return []string{payload}
	}
}

// normalize 复刻 lua-waf/util.lua 的 normalize()：URL 解码 + 小写 + 去多余空白。
func normalize(input string) string {
	decoded := urlDecode(input)
	decoded = strings.ToLower(decoded)
	return strings.Join(strings.Fields(decoded), " ")
}

// urlDecode 做最小限度的 %XX 解码（足够覆盖攻击载荷里的编码绕过）。
func urlDecode(input string) string {
	if !strings.Contains(input, "%") {
		return input
	}

	var builder strings.Builder
	builder.Grow(len(input))
	for i := 0; i < len(input); i++ {
		if input[i] == '%' && i+2 < len(input) {
			high, okHigh := hexValue(input[i+1])
			low, okLow := hexValue(input[i+2])
			if okHigh && okLow {
				builder.WriteByte(byte(high<<4 | low))
				i += 2
				continue
			}
		}
		builder.WriteByte(input[i])
	}
	return builder.String()
}

// hexValue 返回十六进制字符的数值。
func hexValue(char byte) (int, bool) {
	switch {
	case char >= '0' && char <= '9':
		return int(char - '0'), true
	case char >= 'a' && char <= 'f':
		return int(char-'a') + 10, true
	case char >= 'A' && char <= 'F':
		return int(char-'A') + 10, true
	}
	return 0, false
}

// FormatText 把报告渲染成适合在终端阅读的文本。
func (r Report) FormatText() string {
	var builder strings.Builder

	if r.Payload != "" {
		builder.WriteString(fmt.Sprintf("载荷匹配模式：%s\n", r.Payload))
		builder.WriteString(fmt.Sprintf("规则来源：%s\n\n", r.Source))
		if r.Hit == 0 {
			builder.WriteString("没有任何规则命中该载荷（可能放行）\n")
			return builder.String()
		}
		builder.WriteString(fmt.Sprintf("命中 %d 条规则（按 lua 检查顺序）：\n", r.Hit))
		for _, item := range r.Results {
			if item.Status != StatusHit {
				continue
			}
			builder.WriteString(fmt.Sprintf("  [%s] #%d  %s\n", item.RuleType, item.RuleID, item.RuleItem))
			if item.Sample != "" {
				builder.WriteString(fmt.Sprintf("        命中片段：%s\n", item.Sample))
			}
		}
		return builder.String()
	}

	builder.WriteString(fmt.Sprintf("规则来源：%s\n", r.Source))
	builder.WriteString(fmt.Sprintf("生成时间：%s\n", r.GeneratedAt))
	builder.WriteString(fmt.Sprintf("共 %d 条：命中攻击样例 %d / 死规则 %d / 未覆盖 %d / 正则错误 %d / RE2 不支持 %d / 跳过 %d\n\n",
		r.Total, r.Hit, r.Dead, r.Narrow, r.BadRegex, r.GoIncompat, r.Skipped))
	builder.WriteString("说明：死规则 = 连按规则自己生成的样例都命中不了（永远不会生效）；\n      未覆盖 = 规则有效，只是内置攻击样例没覆盖它的形态（正常）。\n\n")

	builder.WriteString("按类型：\n")
	for _, summary := range r.Types {
		builder.WriteString(fmt.Sprintf("  %-12s 共 %3d  命中 %3d  死规则 %2d  未覆盖 %2d  错误 %2d  RE2 %2d  跳过 %2d\n",
			summary.Type, summary.Total, summary.Hit, summary.Dead, summary.Narrow, summary.BadRegex, summary.GoIncompat, summary.Skipped))
	}

	problems := make([]Result, 0, r.Dead+r.BadRegex+r.GoIncompat)
	for _, item := range r.Results {
		switch item.Status {
		case StatusDead, StatusNoMatch, StatusBadRegex, StatusGoIncompat:
			problems = append(problems, item)
		}
	}
	if len(problems) == 0 {
		builder.WriteString("\n没有发现可疑规则。\n")
		return builder.String()
	}

	builder.WriteString(fmt.Sprintf("\n需要人工确认的规则（%d 条）：\n", len(problems)))
	for _, item := range problems {
		builder.WriteString(fmt.Sprintf("  [%s] #%d  %s\n", item.Status, item.RuleID, item.RuleItem))
		if item.RuleType != "" {
			builder.WriteString(fmt.Sprintf("        类型：%s", item.RuleType))
			if item.Enabled {
				builder.WriteString("（启用中）\n")
			} else {
				builder.WriteString("（已停用）\n")
			}
		}
		if item.Error != "" {
			builder.WriteString(fmt.Sprintf("        错误：%s\n", item.Error))
		}
	}
	return builder.String()
}
