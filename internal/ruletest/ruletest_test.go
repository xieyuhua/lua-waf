package ruletest

import (
	"strings"
	"testing"

	"wafadmin/internal/model"
)

// TestCheckRuleStatuses 校验四种结论的判定是否符合预期。
func TestCheckRuleStatuses(t *testing.T) {
	rules := []model.Rule{
		// 正常规则：样例里有 union select
		{Id: 1, RuleType: "args", RuleItem: `union\s+select`, Enabled: true},
		// 有效但内置样例没覆盖：narrow（正常情况，不算问题）
		{Id: 2, RuleType: "args", RuleItem: `this_will_never_match_anything`, Enabled: true},
		// 死规则：零宽断言自相矛盾，永远匹配不上
		{Id: 7, RuleType: "args", RuleItem: `\b\Bfoo`, Enabled: true},
		// 正则语法错误
		{Id: 3, RuleType: "args", RuleItem: `(unclosed`, Enabled: true},
		// RE2 不支持（前瞻）：PCRE 有效
		{Id: 4, RuleType: "args", RuleItem: `select(?=\s+from)`, Enabled: true},
		// IP 名单类：跳过
		{Id: 5, RuleType: "blackip", RuleItem: `^1\.1\.1\.1$`, Enabled: true},
		// 停用规则默认不检查
		{Id: 6, RuleType: "args", RuleItem: `union\s+select`, Enabled: false},
	}

	report := Run(rules, "test", Options{})
	if report.Total != 6 {
		t.Fatalf("停用规则应被跳过，期望检查 5 条，实际 %d", report.Total)
	}

	statusOf := make(map[int64]string, len(report.Results))
	for _, item := range report.Results {
		statusOf[item.RuleID] = item.Status
	}

	cases := map[int64]string{
		1: StatusHit,
		2: StatusNarrow,
		3: StatusBadRegex,
		4: StatusGoIncompat,
		5: StatusSkipped,
		7: StatusDead,
	}
	for id, want := range cases {
		if got := statusOf[id]; got != want {
			t.Fatalf("规则 #%d 期望 %s，实际 %s", id, want, got)
		}
	}

	if report.Hit != 1 || report.Dead != 1 || report.Narrow != 1 || report.BadRegex != 1 || report.GoIncompat != 1 || report.Skipped != 1 {
		t.Fatalf("汇总数字不正确: %+v", report)
	}
}

// TestPayloadModeMatchesInCheckOrder 校验载荷模式下按 lua 检查顺序输出命中规则。
func TestPayloadModeMatchesInCheckOrder(t *testing.T) {
	rules := []model.Rule{
		{Id: 1, RuleType: "args", RuleItem: `union\s+select`, Enabled: true},
		{Id: 2, RuleType: "url", RuleItem: `\.git`, Enabled: true},
		{Id: 3, RuleType: "args", RuleItem: `never_match_me`, Enabled: true},
	}

	report := Run(rules, "test", Options{Payload: "/.git/config?id=1 union select 1"})
	if report.Hit != 2 {
		t.Fatalf("期望命中 2 条，实际 %d", report.Hit)
	}

	// url 类在 lua 链路里先于 args
	if report.Results[0].RuleType != "url" {
		t.Fatalf("应按检查顺序输出，首条应为 url，实际 %s", report.Results[0].RuleType)
	}

	noHit := Run(rules, "test", Options{Payload: "/index.html"})
	if noHit.Hit != 0 {
		t.Fatalf("正常请求不应命中，实际 %d 条", noHit.Hit)
	}
}

// TestPayloadModeURIDecode 校验编码绕过（%20 / %3C）也能命中。
func TestPayloadModeURIDecode(t *testing.T) {
	rules := []model.Rule{
		{Id: 1, RuleType: "args", RuleItem: `union\s+select`, Enabled: true},
		{Id: 2, RuleType: "args", RuleItem: `<script`, Enabled: true},
	}

	for _, payload := range []string{
		"id=1%20union%20select%201",
		"q=%3Cscript%3Ealert(1)%3C/script%3E",
	} {
		report := Run(rules, "test", Options{Payload: payload})
		if report.Hit == 0 {
			t.Fatalf("编码载荷 %s 应能命中", payload)
		}
	}
}

// TestTypeFilterAndFormat 校验类型过滤与文本报告渲染。
func TestTypeFilterAndFormat(t *testing.T) {
	rules := []model.Rule{
		{Id: 1, RuleType: "url", RuleItem: `\.git`, Enabled: true},
		{Id: 2, RuleType: "user-agent", RuleItem: `sqlmap`, Enabled: true},
	}

	report := Run(rules, "test", Options{Type: "user-agent"})
	if report.Total != 1 || report.Results[0].RuleID != 2 {
		t.Fatalf("类型过滤不正确: %+v", report.Results)
	}

	text := report.FormatText()
	if !strings.Contains(text, "user-agent") || !strings.Contains(text, "共 1 条") {
		t.Fatalf("文本报告内容不完整:\n%s", text)
	}
}

// TestOnDefaultCorpusEveryTypeHitsSomething 用默认规则库验证样例集本身足够覆盖。
func TestOnDefaultCorpusEveryTypeHitsSomething(t *testing.T) {
	// 每类各挑一条典型规则，确认样例集能命中
	rules := []model.Rule{
		{Id: 1, RuleType: "args", RuleItem: `union.*select`, Enabled: true},
		{Id: 2, RuleType: "post", RuleItem: `<script`, Enabled: true},
		{Id: 3, RuleType: "url", RuleItem: `\.git|\.env`, Enabled: true},
		{Id: 4, RuleType: "cookie", RuleItem: `\.\./`, Enabled: true},
		{Id: 5, RuleType: "user-agent", RuleItem: `sqlmap|nmap`, Enabled: true},
		{Id: 6, RuleType: "header", RuleItem: `(?i)host:\s*(127\.0\.0\.1|localhost)`, Enabled: true},
	}

	report := Run(rules, "test", Options{})
	if report.NoMatch != 0 || report.BadRegex != 0 {
		for _, item := range report.Results {
			if item.Status != StatusHit {
				t.Fatalf("规则 #%d(%s) 未命中样例：%s", item.RuleID, item.RuleType, item.RuleItem)
			}
		}
	}
	if report.Hit != len(rules) {
		t.Fatalf("期望全部命中，实际 %d/%d", report.Hit, len(rules))
	}
}
