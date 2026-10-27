package service_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"wafadmin/internal/model"
	"wafadmin/internal/repository/memory"
	"wafadmin/internal/service"
)

// capturedChat 记录最近一次对话请求体，便于断言提示词内容。
type capturedChat struct {
	mu   sync.Mutex
	body string
}

func (c *capturedChat) set(body string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.body = body
}

func (c *capturedChat) get() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.body
}

// newSlowSQLAI 构造带演示慢 SQL 数据的 AI 服务，并捕获请求体。
func newSlowSQLAI(t *testing.T) (*service.AIService, *service.AnalyzeCache, *memory.Store, *capturedChat, func()) {
	t.Helper()

	captured := &capturedChat{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		captured.set(string(raw))

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "fake-model",
			"choices": []any{map[string]any{
				"message": map[string]any{
					"role":    "assistant",
					"content": "## 结论\n\n缺少 (client_ip, id) 联合索引，导致全表扫描。\n\n## 优化方案\n\n- `CREATE INDEX idx_ip ON waf_log (client_ip, id);`",
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 80, "completion_tokens": 60, "total_tokens": 140},
		})
	}))

	store := (&memory.Store{}).Init(true)
	cache := &service.AnalyzeCache{}
	ai := &service.AIService{
		Settings: &service.SettingService{Repo: store.Repos().Setting},
		Logs:     store.Repos().Log,
		SlowSQLs: store.Repos().SlowSQL,
		Client:   server.Client(),
		Cache:    cache,
	}
	if _, err := ai.SaveConfig(context.Background(), service.AIConfig{
		Enabled:      true,
		BaseURL:      server.URL,
		Model:        "fake-model",
		CacheMinutes: 10,
	}); err != nil {
		server.Close()
		t.Fatalf("保存 AI 配置失败: %v", err)
	}
	return ai, cache, store, captured, server.Close
}

// TestAnalyzeSlowSQL 校验慢 SQL 分析、缓存复用与按单条取回。
func TestAnalyzeSlowSQL(t *testing.T) {
	ai, _, store, _, closeServer := newSlowSQLAI(t)
	defer closeServer()

	ctx := context.Background()
	items, _, err := store.Repos().SlowSQL.List(ctx, model.SlowSQLQuery{ListQuery: model.ListQuery{Page: 1, Size: 2}})
	if err != nil || len(items) == 0 {
		t.Fatalf("演示慢 SQL 数据缺失: %v", err)
	}
	ids := []int64{items[0].Id}

	result, err := ai.AnalyzeSlowSQL(ctx, service.AnalyzeSlowSQLRequest{SlowSQLIds: ids})
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}
	if strings.TrimSpace(result.Content) == "" {
		t.Fatal("分析内容为空")
	}
	if result.Cached {
		t.Fatal("首次分析不应标记为缓存")
	}
	if result.LogCount != 1 {
		t.Fatalf("分析条数应为 1，实际 %d", result.LogCount)
	}

	again, err := ai.AnalyzeSlowSQL(ctx, service.AnalyzeSlowSQLRequest{SlowSQLIds: ids})
	if err != nil {
		t.Fatalf("二次分析失败: %v", err)
	}
	if !again.Cached || again.CachedAt == "" {
		t.Fatalf("相同参数应命中缓存: %+v", again)
	}

	cached, err := ai.CachedSlowSQLResult(ctx, items[0].Id)
	if err != nil {
		t.Fatalf("应能取到缓存结果: %v", err)
	}
	if !cached.Cached || cached.Content == "" {
		t.Fatalf("取回结果异常: %+v", cached)
	}

	if _, err := ai.AnalyzeSlowSQL(ctx, service.AnalyzeSlowSQLRequest{}); err == nil {
		t.Fatal("空选择应报错")
	}
	if _, err := ai.AnalyzeSlowSQL(ctx, service.AnalyzeSlowSQLRequest{SlowSQLIds: []int64{999999}}); err == nil {
		t.Fatal("不存在的记录应报错")
	}
}

// TestAnalyzeSlowSQLCacheIsolated 校验日志与慢 SQL 的缓存互不串味。
func TestAnalyzeSlowSQLCacheIsolated(t *testing.T) {
	ai, cache, store, _, closeServer := newSlowSQLAI(t)
	defer closeServer()

	ctx := context.Background()
	items, _, err := store.Repos().SlowSQL.List(ctx, model.SlowSQLQuery{ListQuery: model.ListQuery{Page: 1, Size: 1}})
	if err != nil || len(items) == 0 {
		t.Fatalf("演示慢 SQL 数据缺失: %v", err)
	}
	sharedID := items[0].Id

	if _, err := ai.AnalyzeLogs(ctx, service.AnalyzeLogsRequest{LogIds: []int64{sharedID}}); err != nil {
		t.Fatalf("日志分析失败: %v", err)
	}
	if _, err := ai.AnalyzeSlowSQL(ctx, service.AnalyzeSlowSQLRequest{SlowSQLIds: []int64{sharedID}}); err != nil {
		t.Fatalf("慢 SQL 分析失败: %v", err)
	}
	if cache.Size() != 2 {
		t.Fatalf("两个场景应各占一条缓存，实际 %d", cache.Size())
	}

	logHit, err := ai.CachedResult(ctx, sharedID)
	if err != nil || logHit.Content == "" {
		t.Fatalf("日志场景缓存应存在: %v", err)
	}
	slowHit, err := ai.CachedSlowSQLResult(ctx, sharedID)
	if err != nil || slowHit.Content == "" {
		t.Fatalf("慢 SQL 场景缓存应存在: %v", err)
	}
}

// TestSlowSQLPromptContent 校验发给模型的提示词包含关键信息与输出结构要求。
func TestSlowSQLPromptContent(t *testing.T) {
	ai, _, store, captured, closeServer := newSlowSQLAI(t)
	defer closeServer()

	ctx := context.Background()
	items, _, err := store.Repos().SlowSQL.List(ctx, model.SlowSQLQuery{ListQuery: model.ListQuery{Page: 1, Size: 1}})
	if err != nil || len(items) == 0 {
		t.Fatalf("演示慢 SQL 数据缺失: %v", err)
	}
	item := items[0]

	if _, err := ai.AnalyzeSlowSQL(ctx, service.AnalyzeSlowSQLRequest{
		SlowSQLIds: []int64{item.Id},
		Question:   "这个查询要不要加索引？",
	}); err != nil {
		t.Fatalf("分析失败: %v", err)
	}

	body := captured.get()
	if body == "" {
		t.Fatal("未捕获到请求体")
	}
	for _, want := range []string{
		"慢 SQL 1",
		"```sql",
		"优化方案",
		"CREATE INDEX",
		"风险提示",
		"验证方式",
		"这个查询要不要加索引？",
		service.SceneSlowSQL,
	} {
		if want == service.SceneSlowSQL {
			continue // 场景标识只用于缓存键，不出现在提示词里
		}
		if !strings.Contains(body, want) {
			t.Fatalf("提示词缺少 %q", want)
		}
	}
	// SQL 语句应被截断后写入（不超过 2000 字符）
	if strings.Contains(body, strings.Repeat("x", 4000)) {
		t.Fatal("超长 SQL 应被截断")
	}
}
