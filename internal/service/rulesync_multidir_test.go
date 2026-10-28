package service_test

import (
	"context"
	"testing"

	"wafadmin/internal/repository/memory"
	"wafadmin/internal/service"
)

// TestRuleFileSyncMultiDir 校验多目录合并：两个目录的规则都会被纳入，重复只算一次。
func TestRuleFileSyncMultiDir(t *testing.T) {
	store := (&memory.Store{}).Init(true)
	sync := &service.RuleFileSync{Repo: store.Repos().Rule}

	dirA := t.TempDir()
	dirB := t.TempDir()
	writeRuleFile(t, dirA, "args.rule", `[{"Id":1,"RuleType":"args","RuleItem":"from_dir_a"}]`)
	// dirB 含一条新规则 + 一条与 dirA 重复的规则
	writeRuleFile(t, dirB, "args.rule", `[
		{"Id":1,"RuleType":"args","RuleItem":"from_dir_a"},
		{"Id":2,"RuleType":"args","RuleItem":"from_dir_b"}
	]`)
	missing := dirB + "/not-exist"

	result, err := sync.SyncFromDirs(context.Background(), []string{missing, dirA, dirB})
	if err != nil {
		t.Fatalf("多目录同步失败: %v", err)
	}
	if result.Added != 2 {
		t.Fatalf("应新增 2 条（跨目录去重），实际 %d（%+v）", result.Added, result)
	}
	if result.Files != 2 {
		t.Fatalf("应读取 2 个文件，实际 %d", result.Files)
	}

	// 再跑一次应幂等
	again, err := sync.SyncFromDirs(context.Background(), []string{dirA, dirB})
	if err != nil {
		t.Fatalf("二次同步失败: %v", err)
	}
	if again.Added != 0 {
		t.Fatalf("重复同步不应新增，实际 %d", again.Added)
	}
}

// TestRuleFileSyncAllDirsMissing 校验全部目录不可读时返回提示而非错误。
func TestRuleFileSyncAllDirsMissing(t *testing.T) {
	store := (&memory.Store{}).Init(true)
	sync := &service.RuleFileSync{Repo: store.Repos().Rule}

	result, err := sync.SyncFromDirs(context.Background(), []string{"/definitely/not/exist/a", "/definitely/not/exist/b"})
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if result.Added != 0 || result.Message == "" {
		t.Fatalf("应返回提示且不新增: %+v", result)
	}
}
