package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"wafadmin/internal/model"
	"wafadmin/internal/repository/memory"
	"wafadmin/internal/service"
)

// writeRuleFile 写一个规则文件（JSON 数组，字段与 lua 侧一致）。
func writeRuleFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("写入规则文件失败: %v", err)
	}
}

// TestRuleFileSyncUnion 校验「取并集」：文件有库里没有的新增，库里已有的保留不动。
func TestRuleFileSyncUnion(t *testing.T) {
	store := (&memory.Store{}).Init(true)
	repo := store.Repos().Rule

	// 预置：库里已存在一条 args 规则（模拟人工维护），另外有一条文件里没有的 cookie 规则
	existingArgs := model.Rule{RuleType: "args", RuleItem: `\.\./`}
	if err := repo.Create(context.Background(), &existingArgs); err != nil {
		t.Fatalf("预置规则失败: %v", err)
	}
	keepOnly := model.Rule{RuleType: "cookie", RuleItem: `only_in_db`}
	if err := repo.Create(context.Background(), &keepOnly); err != nil {
		t.Fatalf("预置规则失败: %v", err)
	}

	dir := t.TempDir()
	// 文件里包含：已存在的规则（应保留、不重复插入）、两条新规则
	writeRuleFile(t, dir, "args.rule", `[
		{"Id":1,"RuleType":"args","RuleItem":"\\.\\./"},
		{"Id":2,"RuleType":"args","RuleItem":"\\$\\{jndi:"}
	]`)
	writeRuleFile(t, dir, "url.rule", `[
		{"Id":3,"RuleType":"url","RuleItem":"/actuator/heapdump"}
	]`)
	// 非 .rule 文件应被忽略
	writeRuleFile(t, dir, "readme.txt", `ignore me`)

	before, err := repo.Count(context.Background())
	if err != nil {
		t.Fatalf("统计失败: %v", err)
	}

	sync := &service.RuleFileSync{Repo: repo}
	result, err := sync.SyncFromDir(context.Background(), dir)
	if err != nil {
		t.Fatalf("同步失败: %v", err)
	}

	if result.Files != 2 {
		t.Fatalf("应读取 2 个规则文件，实际 %d", result.Files)
	}
	if result.Added != 2 {
		t.Fatalf("应新增 2 条，实际 %d（%+v）", result.Added, result)
	}
	if result.Kept < 1 {
		t.Fatalf("已存在的规则应计为保留，实际 %+v", result)
	}

	after, err := repo.Count(context.Background())
	if err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if after != before+2 {
		t.Fatalf("总条数应增加 2（%d -> %d）", before, after)
	}

	// 库里原有且文件里没有的规则必须保留
	rules, err := repo.List(context.Background(), "cookie")
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	found := false
	for _, rule := range rules {
		if rule.RuleItem == "only_in_db" {
			found = true
		}
	}
	if !found {
		t.Fatal("库里独有的规则不应被删除")
	}

	// 再同步一次应无新增（幂等）
	again, err := sync.SyncFromDir(context.Background(), dir)
	if err != nil {
		t.Fatalf("二次同步失败: %v", err)
	}
	if again.Added != 0 {
		t.Fatalf("重复同步不应新增，实际 %d", again.Added)
	}
}

// TestRuleFileSyncMissingDir 校验目录不存在时不算错误。
func TestRuleFileSyncMissingDir(t *testing.T) {
	store := (&memory.Store{}).Init(true)
	sync := &service.RuleFileSync{Repo: store.Repos().Rule}

	result, err := sync.SyncFromDir(context.Background(), filepath.Join(t.TempDir(), "not-exist"))
	if err != nil {
		t.Fatalf("目录不存在不应报错: %v", err)
	}
	if result.Added != 0 {
		t.Fatalf("不应新增规则: %+v", result)
	}
	if result.Message == "" {
		t.Fatal("应给出提示信息")
	}
}

// TestRuleFileSyncInvalidFile 校验坏文件被跳过且不影响其它文件。
func TestRuleFileSyncInvalidFile(t *testing.T) {
	store := (&memory.Store{}).Init(true)
	dir := t.TempDir()
	writeRuleFile(t, dir, "broken.rule", `{not json`)
	writeRuleFile(t, dir, "ok.rule", `[{"Id":1,"RuleType":"args","RuleItem":"good_rule"}]`)

	sync := &service.RuleFileSync{Repo: store.Repos().Rule}
	result, err := sync.SyncFromDir(context.Background(), dir)
	if err != nil {
		t.Fatalf("同步失败: %v", err)
	}
	if result.Added != 1 {
		t.Fatalf("应只新增 1 条，实际 %d（%+v）", result.Added, result)
	}
	if result.Message == "" {
		t.Fatal("应提示存在解析失败的文件")
	}
}
