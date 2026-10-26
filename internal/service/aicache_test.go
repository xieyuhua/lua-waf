package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"wafadmin/internal/repository/memory"
	"wafadmin/internal/service"
)

// newChatServer 构造一个 OpenAI 兼容的假模型服务，并统计被调用次数。
func newChatServer(t *testing.T, calls *int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "fake-model",
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "结论：真实攻击"}},
			},
			"usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		})
	}))
}

// newCachedAI 构造带缓存与假模型的 AI 服务。
func newCachedAI(t *testing.T) (*service.AIService, *service.AnalyzeCache, *int64, func()) {
	t.Helper()

	var calls int64
	server := newChatServer(t, &calls)
	store := (&memory.Store{}).Init(true) // 载入演示日志
	settings := &service.SettingService{Repo: store.Repos().Setting}

	cache := &service.AnalyzeCache{}
	ai := &service.AIService{
		Settings: settings,
		Logs:     store.Repos().Log,
		Client:   server.Client(),
		Cache:    cache,
	}

	ctx := context.Background()
	if _, err := ai.SaveConfig(ctx, service.AIConfig{
		Enabled:      true,
		BaseURL:      server.URL,
		Model:        "fake-model",
		CacheMinutes: 10,
	}); err != nil {
		t.Fatalf("保存 AI 配置失败: %v", err)
	}

	return ai, cache, &calls, server.Close
}

// TestAnalyzeCacheHit 校验相同参数第二次直接命中缓存、不再请求模型。
func TestAnalyzeCacheHit(t *testing.T) {
	ai, _, calls, closeServer := newCachedAI(t)
	defer closeServer()

	ctx := context.Background()
	req := service.AnalyzeLogsRequest{LogIds: []int64{1, 2}, Question: "是否同一来源？"}

	first, err := ai.AnalyzeLogs(ctx, req)
	if err != nil {
		t.Fatalf("首次分析失败: %v", err)
	}
	if first.Cached {
		t.Fatal("首次分析不应来自缓存")
	}
	if first.Content == "" || first.Usage.TotalTokens != 15 {
		t.Fatalf("首次结果异常: %+v", first)
	}
	if got := atomic.LoadInt64(calls); got != 1 {
		t.Fatalf("首次分析应调用模型 1 次，实际 %d", got)
	}

	second, err := ai.AnalyzeLogs(ctx, req)
	if err != nil {
		t.Fatalf("二次分析失败: %v", err)
	}
	if !second.Cached {
		t.Fatal("相同参数第二次应命中缓存")
	}
	if second.CachedAt == "" {
		t.Fatal("命中缓存时应返回缓存生成时间")
	}
	if second.Content != first.Content {
		t.Fatalf("缓存内容不一致: %q vs %q", second.Content, first.Content)
	}
	if got := atomic.LoadInt64(calls); got != 1 {
		t.Fatalf("命中缓存不应再调用模型，实际调用 %d 次", got)
	}
}

// TestAnalyzeCacheRefreshAndVariants 校验强制刷新与参数变化不命中缓存。
func TestAnalyzeCacheRefreshAndVariants(t *testing.T) {
	ai, _, calls, closeServer := newCachedAI(t)
	defer closeServer()

	ctx := context.Background()
	base := service.AnalyzeLogsRequest{LogIds: []int64{1}, Question: ""}

	if _, err := ai.AnalyzeLogs(ctx, base); err != nil {
		t.Fatalf("首次分析失败: %v", err)
	}

	// 强制刷新
	refresh := base
	refresh.Refresh = true
	if _, err := ai.AnalyzeLogs(ctx, refresh); err != nil {
		t.Fatalf("强制刷新失败: %v", err)
	}
	if got := atomic.LoadInt64(calls); got != 2 {
		t.Fatalf("强制刷新应重新调用模型，实际 %d", got)
	}

	// 不同日志集合
	if _, err := ai.AnalyzeLogs(ctx, service.AnalyzeLogsRequest{LogIds: []int64{2}}); err != nil {
		t.Fatalf("不同日志分析失败: %v", err)
	}
	// 不同追问
	if _, err := ai.AnalyzeLogs(ctx, service.AnalyzeLogsRequest{LogIds: []int64{1}, Question: "要不要封禁"}); err != nil {
		t.Fatalf("不同追问分析失败: %v", err)
	}
	if got := atomic.LoadInt64(calls); got != 4 {
		t.Fatalf("参数变化应各调用一次模型，实际 %d", got)
	}

	// 日志顺序不同视为同一请求
	orderA := service.AnalyzeLogsRequest{LogIds: []int64{3, 4}}
	orderB := service.AnalyzeLogsRequest{LogIds: []int64{4, 3}}
	if _, err := ai.AnalyzeLogs(ctx, orderA); err != nil {
		t.Fatalf("分析失败: %v", err)
	}
	before := atomic.LoadInt64(calls)
	cached, err := ai.AnalyzeLogs(ctx, orderB)
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}
	if !cached.Cached || atomic.LoadInt64(calls) != before {
		t.Fatal("日志顺序不同应视为同一请求并命中缓存")
	}
}

// TestAnalyzeCacheExpiry 校验缓存到期后重新调用模型。
func TestAnalyzeCacheExpiry(t *testing.T) {
	ai, cache, calls, closeServer := newCachedAI(t)
	defer closeServer()

	// 冻结时间，便于断言过期
	current := time.Date(2026, 10, 26, 12, 0, 0, 0, time.Local)
	cache.Now = func() time.Time { return current }

	ctx := context.Background()
	req := service.AnalyzeLogsRequest{LogIds: []int64{1}}

	if _, err := ai.AnalyzeLogs(ctx, req); err != nil {
		t.Fatalf("首次分析失败: %v", err)
	}
	if _, err := ai.AnalyzeLogs(ctx, req); err != nil {
		t.Fatalf("二次分析失败: %v", err)
	}
	if got := atomic.LoadInt64(calls); got != 1 {
		t.Fatalf("缓存期内应只调用一次，实际 %d", got)
	}

	// 9 分钟后仍命中，11 分钟后过期
	current = current.Add(9 * time.Minute)
	if hit, err := ai.AnalyzeLogs(ctx, req); err != nil || !hit.Cached {
		t.Fatalf("9 分钟后应仍命中缓存: cached=%v err=%v", hit != nil && hit.Cached, err)
	}
	current = current.Add(2 * time.Minute)
	if fresh, err := ai.AnalyzeLogs(ctx, req); err != nil {
		t.Fatalf("分析失败: %v", err)
	} else if fresh.Cached {
		t.Fatal("超过 10 分钟后不应再命中缓存")
	}
	if got := atomic.LoadInt64(calls); got != 2 {
		t.Fatalf("过期后应重新调用模型，实际 %d", got)
	}
}

// TestAnalyzeCacheConfigDefaults 校验缓存时长默认值与上限。
func TestAnalyzeCacheConfigDefaults(t *testing.T) {
	cfg := service.AIConfig{}.WithDefaults()
	if cfg.CacheMinutes != 10 {
		t.Fatalf("默认缓存时长应为 10 分钟，实际 %d", cfg.CacheMinutes)
	}
	cfg = service.AIConfig{CacheMinutes: 999}.WithDefaults()
	if cfg.CacheMinutes != 120 {
		t.Fatalf("缓存时长应裁剪到 120 分钟，实际 %d", cfg.CacheMinutes)
	}
}

// TestAnalyzeCacheKeyStable 校验缓存键的稳定性与敏感性。
func TestAnalyzeCacheKeyStable(t *testing.T) {
	cache := &service.AnalyzeCache{}
	cfg := service.AIConfig{BaseURL: "https://api.example.com/v1", Model: "gpt-4o", SystemPrompt: "prompt"}

	a := cache.Key(cfg, []int64{1, 2, 3}, "问题")
	b := cache.Key(cfg, []int64{3, 2, 1}, " 问题 ")
	if a != b {
		t.Fatal("日志顺序与追问首尾空格不应影响缓存键")
	}

	other := cache.Key(cfg, []int64{1, 2}, "问题")
	if a == other {
		t.Fatal("日志集合不同应产生不同缓存键")
	}

	cfg.Model = "gpt-4o-mini"
	if a == cache.Key(cfg, []int64{1, 2, 3}, "问题") {
		t.Fatal("模型不同应产生不同缓存键")
	}
}
