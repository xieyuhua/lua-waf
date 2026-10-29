package service_test

import (
	"context"
	"encoding/json"
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

// writeRuleItemsFile 写一个规则文件：items 为 "type|item" 形式（本文件专用，避免与上面重名）。
func writeRuleItemsFile(t *testing.T, path string, items []string) {
	t.Helper()

	type fileItem struct {
		Id       int64  `json:"Id"`
		RuleType string `json:"RuleType"`
		RuleItem string `json:"RuleItem"`
	}

	list := make([]fileItem, 0, len(items))
	for i, raw := range items {
		var ruleType, ruleItem string
		for j := 0; j < len(raw); j++ {
			if raw[j] == '|' {
				ruleType, ruleItem = raw[:j], raw[j+1:]
				break
			}
		}
		list = append(list, fileItem{Id: int64(i + 1), RuleType: ruleType, RuleItem: ruleItem})
	}

	payload, err := json.Marshal(list)
	if err != nil {
		t.Fatalf("序列化规则失败: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("写入 %s 失败: %v", path, err)
	}
}

// TestSyncFromDirsCounts 校验「从规则文件同步」的条数口径：
// 重复条目不能计入「已存在」，新增/已存在要能对上库内总数。
func TestSyncFromDirsCounts(t *testing.T) {
	store := (&memory.Store{}).Init(false)
	repo := store.Repos().Rule
	ctx := context.Background()

	// 库里先有一条已存在规则
	if err := repo.BatchCreate(ctx, []model.Rule{
		{RuleType: "args", RuleItem: "old-rule", Enabled: true},
	}); err != nil {
		t.Fatalf("预置规则失败: %v", err)
	}

	dirA := t.TempDir()
	dirB := t.TempDir()
	// 目录 A：args 3 条（其中 old-rule 已在库）+ 与 B 重复的 1 条
	writeRuleItemsFile(t, filepath.Join(dirA, "args.rule"), []string{
		"args|old-rule",
		"args|a1",
		"args|a2",
	})
	// 目录 B：同样内容一份（模拟 RULE_PATH 与仓库副本同时存在）
	writeRuleItemsFile(t, filepath.Join(dirB, "args.rule"), []string{
		"args|a1",
		"args|a2",
		"args|b1",
	})

	sync := &service.RuleFileSync{Repo: repo}
	result, err := sync.SyncFromDirs(ctx, []string{dirA, dirB})
	if err != nil {
		t.Fatalf("同步失败: %v", err)
	}

	// 去重后文件里共 4 条：old-rule（库中已有）、a1、a2、b1（新增 3 条）
	if result.Files != 2 {
		t.Fatalf("文件数期望 2，实际 %d", result.Files)
	}
	if result.InFiles != 4 {
		t.Fatalf("去重后文件条数期望 4，实际 %d（重复条目被算进来了）", result.InFiles)
	}
	if result.Kept != 1 {
		t.Fatalf("已存在期望 1，实际 %d（重复条目不应计入已存在）", result.Kept)
	}
	if result.Added != 3 {
		t.Fatalf("新增期望 3，实际 %d", result.Added)
	}
	if result.Duplicates != 2 {
		t.Fatalf("重复跳过期望 2（a1、a2 各重复一次），实际 %d", result.Duplicates)
	}
	if result.Total != 4 {
		t.Fatalf("库内总数期望 4（1 原有 + 3 新增），实际 %d", result.Total)
	}

	// 口径自查：去重后条数 = 新增 + 已存在
	if result.InFiles != result.Added+result.Kept {
		t.Fatalf("口径不一致：InFiles=%d 应等于 Added+Kept=%d", result.InFiles, result.Added+result.Kept)
	}

	// 再同步一次：应全部为「已存在」，新增 0
	again, err := sync.SyncFromDirs(ctx, []string{dirA, dirB})
	if err != nil {
		t.Fatalf("二次同步失败: %v", err)
	}
	if again.Added != 0 || again.Kept != 4 || again.Total != 4 {
		t.Fatalf("二次同步期望 新增0/已存在4/总数4，实际 %+v", again)
	}
	if again.Duplicates != 2 {
		t.Fatalf("二次同步重复跳过期望 2，实际 %d", again.Duplicates)
	}
}

// TestSyncSkipsRateLimitFile 校验同步会忽略 ratelimit.rule（限频规则的下发文件，schema 不同）。
func TestSyncSkipsRateLimitFile(t *testing.T) {
	store := (&memory.Store{}).Init(false)
	repo := store.Repos().Rule
	ctx := context.Background()

	dir := t.TempDir()
	writeRuleItemsFile(t, filepath.Join(dir, "args.rule"), []string{"args|x1"})
	if err := os.WriteFile(filepath.Join(dir, "ratelimit.rule"), []byte(`[{"id":1,"pattern":"/api","key":"ip","limit":10,"window":60,"action":"block","enabled":true}]`), 0o644); err != nil {
		t.Fatalf("写入 ratelimit.rule 失败: %v", err)
	}

	sync := &service.RuleFileSync{Repo: repo}
	result, err := sync.SyncFromDirs(ctx, []string{dir})
	if err != nil {
		t.Fatalf("同步失败: %v", err)
	}

	if result.Files != 1 {
		t.Fatalf("文件数期望 1（应忽略 ratelimit.rule），实际 %d", result.Files)
	}
	if result.Skipped != 0 {
		t.Fatalf("skipped 期望 0，实际 %d", result.Skipped)
	}
	if result.Added != 1 || result.Total != 1 {
		t.Fatalf("期望新增 1 / 总数 1，实际新增 %d / 总数 %d", result.Added, result.Total)
	}
}
