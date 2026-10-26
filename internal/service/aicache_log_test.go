package service_test

import (
	"context"
	"testing"
	"time"

	"wafadmin/internal/service"
)

// TestCachedResultByLog 校验可按单条日志取到服务端缓存的研判结果（多端查看用）。
func TestCachedResultByLog(t *testing.T) {
	ai, cache, _, closeServer := newCachedAI(t)
	defer closeServer()

	ctx := context.Background()
	if _, err := ai.AnalyzeLogs(ctx, service.AnalyzeLogsRequest{LogIds: []int64{1, 2}}); err != nil {
		t.Fatalf("分析失败: %v", err)
	}

	for _, id := range []int64{1, 2} {
		result, err := ai.CachedResult(ctx, id)
		if err != nil {
			t.Fatalf("日志 %d 应能取到缓存结果: %v", id, err)
		}
		if !result.Cached || result.CachedAt == "" {
			t.Fatalf("返回结果应带缓存标记: %+v", result)
		}
		if result.Content == "" {
			t.Fatal("缓存内容为空")
		}
	}

	if _, err := ai.CachedResult(ctx, 9999); err == nil {
		t.Fatal("未分析过的日志应返回错误")
	}
	if _, err := ai.CachedResult(ctx, 0); err == nil {
		t.Fatal("非法日志 ID 应返回错误")
	}

	// 清空后不可再取
	cache.Purge()
	if _, err := ai.CachedResult(ctx, 1); err == nil {
		t.Fatal("缓存清空后不应再返回结果")
	}
}

// TestCachedResultExpired 校验超过有效期后按日志也取不到结果。
func TestCachedResultExpired(t *testing.T) {
	ai, cache, _, closeServer := newCachedAI(t)
	defer closeServer()

	current := time.Date(2026, 10, 26, 16, 0, 0, 0, time.Local)
	cache.Now = func() time.Time { return current }

	ctx := context.Background()
	if _, err := ai.AnalyzeLogs(ctx, service.AnalyzeLogsRequest{LogIds: []int64{5}}); err != nil {
		t.Fatalf("分析失败: %v", err)
	}
	if _, err := ai.CachedResult(ctx, 5); err != nil {
		t.Fatalf("缓存期内应可取到结果: %v", err)
	}

	current = current.Add(11 * time.Minute)
	if _, err := ai.CachedResult(ctx, 5); err == nil {
		t.Fatal("超过 10 分钟后不应再取到结果")
	}
}
