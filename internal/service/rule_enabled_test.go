package service_test

import (
	"context"
	"testing"

	"wafadmin/internal/model"
	"wafadmin/internal/repository/memory"
	"wafadmin/internal/service"
)

// TestRuleSetEnabled 校验规则启停：停用后仍能查到，但状态为关闭。
func TestRuleSetEnabled(t *testing.T) {
	store := (&memory.Store{}).Init(true)
	repos := store.Repos()

	rule := &service.RuleService{Rules: repos.Rule, Catalog: model.Catalog, Logs: repos.Log}
	created, err := rule.Create(context.Background(), service.RulePayload{
		RuleType: "args",
		RuleItem: `(?i)enabled_test_pattern`,
	})
	if err != nil {
		t.Fatalf("新增规则失败: %v", err)
	}
	if !created.Enabled {
		t.Fatal("新增规则应默认启用")
	}

	disabled, err := rule.SetEnabled(context.Background(), created.Id, false)
	if err != nil {
		t.Fatalf("停用规则失败: %v", err)
	}
	if disabled.Enabled {
		t.Fatal("停用后状态应为关闭")
	}

	// 停用的规则仍然存在（不删除），保证可随时重新启用
	rules, err := repos.Rule.List(context.Background(), "args")
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	found := false
	for _, item := range rules {
		if item.Id == created.Id {
			found = true
			if item.Enabled {
				t.Fatal("库中状态应为停用")
			}
		}
	}
	if !found {
		t.Fatal("停用的规则不应被删除")
	}

	if _, err := rule.SetEnabled(context.Background(), 999999, false); err == nil {
		t.Fatal("不存在的规则应报错")
	}
}

// TestRuleServiceTagStats 校验命中统计按 rule_tag 聚合且按次数倒序。
func TestRuleServiceTagStats(t *testing.T) {
	store := (&memory.Store{}).Init(true)
	repos := store.Repos()
	ctx := context.Background()

	logs := []model.WafLog{
		{ClientIp: "10.0.0.1", RuleTag: "ruleA", AttackType: "SQL注入", LocalTime: "2026-10-28 10:00:00"},
		{ClientIp: "10.0.0.2", RuleTag: "ruleA", AttackType: "SQL注入", LocalTime: "2026-10-28 10:01:00"},
		{ClientIp: "10.0.0.3", RuleTag: "ruleB", AttackType: "XSS", LocalTime: "2026-10-28 10:02:00"},
		{ClientIp: "10.0.0.4", RuleTag: "", AttackType: "XSS", LocalTime: "2026-10-28 10:03:00"},
	}
	if _, err := repos.Log.BatchCreate(ctx, logs); err != nil {
		t.Fatalf("写入日志失败: %v", err)
	}

	rule := &service.RuleService{Rules: repos.Rule, Catalog: model.Catalog, Logs: repos.Log}
	stats, err := rule.TagStats(ctx, 7, 10)
	if err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	// 演示数据里也有日志，因此只断言本次写入的两条聚合结果与排序
	counts := map[string]int64{}
	for _, item := range stats {
		if item.RuleTag == "" {
			t.Fatal("空 rule_tag 不应出现在统计里")
		}
		counts[item.RuleTag] = item.Count
	}
	if counts["ruleA"] != 2 || counts["ruleB"] != 1 {
		t.Fatalf("聚合结果错误: %+v", counts)
	}
	for i := 1; i < len(stats); i++ {
		if stats[i-1].Count < stats[i].Count {
			t.Fatalf("结果应按命中次数倒序: %+v", stats)
		}
	}
}
