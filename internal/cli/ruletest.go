package cli

// 本文件实现 `wafadmin ruletest` 子命令：检查规则库里的规则「是否真的能命中」。
//
// 两种用法：
//   1. 全量自检：把每条启用规则跑一遍内置攻击样例，列出「正则合法但一条样例都命中不了」的规则
//      以及正则写错的规则——这些通常就是永远不会生效的死规则。
//   2. 载荷验证：给定一条请求参数/URL，看它会命中哪些规则（按 lua 侧检查顺序输出），
//      用于上线前确认「这条攻击会被拦」，或排查「为什么这条没拦住」。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"wafadmin/internal/config"
	"wafadmin/internal/repository"
	"wafadmin/internal/ruletest"

	"wafadmin/internal/app"
)

// RuleTest 执行规则自检并输出结果。
type RuleTest struct {
	// Rules 为规则仓储。
	Rules repository.RuleRepository
	// Out 为输出目标。
	Out io.Writer
	// Options 为自检参数。
	Options ruletest.Options
	// AsJSON 为 true 时输出 JSON。
	AsJSON bool
	// Source 为规则来源说明（显示在报告头部）。
	Source string
}

// Run 读取规则并执行自检，返回「是否发现可疑规则」。
func (t *RuleTest) Run(ctx context.Context) (suspicious bool, err error) {
	rules, err := t.Rules.List(ctx, "")
	if err != nil {
		return false, fmt.Errorf("读取规则失败: %w", err)
	}

	source := t.Source
	if source == "" {
		source = "数据库 rules 表"
	}
	report := ruletest.Run(rules, source, t.Options)

	if t.AsJSON {
		payload, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return false, fmt.Errorf("序列化报告失败: %w", err)
		}
		fmt.Fprintln(t.Out, string(payload))
	} else {
		fmt.Fprint(t.Out, report.FormatText())
	}

	return report.Dead+report.BadRegex+report.GoIncompat > 0, nil
}

// ruleTest 解析命令行参数并执行自检。
func (r *Runner) ruleTest(args []string) int {
	flags := r.newFlagSet("ruletest")
	configPath := flags.String("config", "", "配置文件路径，默认取 WAF_CONFIG 或 conf/app.ini")
	ruleType := flags.String("type", "", "只检查某一类规则（args/post/url/cookie/user-agent/header）")
	payload := flags.String("payload", "", "载荷模式：给定参数或 URL，输出会命中的规则")
	all := flags.Bool("all", false, "包含已停用的规则（默认只看启用中的）")
	asJSON := flags.Bool("json", false, "输出 JSON，便于接入 CI")
	strict := flags.Bool("strict", false, "发现可疑规则时以退出码 1 结束，便于 CI 卡门禁")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	exitCode := 0
	code := r.withStore(*configPath, func(store *app.Store, _ *config.Config) error {
		task := &RuleTest{
			Rules:  store.Repos.Rule,
			Out:    r.Stdout,
			AsJSON: *asJSON,
			Options: ruletest.Options{
				Type:            strings.TrimSpace(*ruleType),
				Payload:         *payload,
				IncludeDisabled: *all,
			},
		}

		suspicious, err := task.Run(context.Background())
		if err != nil {
			return err
		}
		if suspicious && *strict {
			exitCode = 1
		}
		return nil
	})
	if code != 0 {
		return code
	}
	return exitCode
}
