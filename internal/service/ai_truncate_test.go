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

	"wafadmin/internal/repository/memory"
	"wafadmin/internal/service"
)

// truncServer 模拟「输出被 max_tokens 截断」的服务端：前 N 次返回 length，之后返回 stop。
type truncServer struct {
	mu       sync.Mutex
	rounds   int
	lengths  int
	requests []string
}

func (s *truncServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.requests = append(s.requests, string(raw))
		s.rounds++
		round := s.rounds
		s.mu.Unlock()

		finish := "length"
		content := "第一段：结论是真实攻击。"
		if round > s.lengths {
			finish = "stop"
			content = "第二段：建议封禁该 IP。"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "trunc-model",
			"choices": []any{map[string]any{
				"message":       map[string]any{"role": "assistant", "content": content},
				"finish_reason": finish,
			}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30},
		})
	})
}

// newTruncAI 构造使用 truncServer 的 AI 服务。
func newTruncAI(t *testing.T, lengths int, maxTokens int) (*service.AIService, *truncServer, func()) {
	t.Helper()

	mock := &truncServer{lengths: lengths}
	server := httptest.NewServer(mock.handler())
	store := (&memory.Store{}).Init(true)
	ai := &service.AIService{
		Settings: &service.SettingService{Repo: store.Repos().Setting},
		Logs:     store.Repos().Log,
		SlowSQLs: store.Repos().SlowSQL,
		Client:   server.Client(),
		Cache:    &service.AnalyzeCache{},
	}
	if _, err := ai.SaveConfig(context.Background(), service.AIConfig{
		Enabled:   true,
		BaseURL:   server.URL,
		Model:     "trunc-model",
		MaxTokens: maxTokens,
	}); err != nil {
		server.Close()
		t.Fatalf("保存 AI 配置失败: %v", err)
	}
	return ai, mock, server.Close
}

// TestAnalyzeAutoContinue 校验输出被截断时自动续写并拼接完整内容。
func TestAnalyzeAutoContinue(t *testing.T) {
	ai, mock, closeServer := newTruncAI(t, 1, 4096)
	defer closeServer()

	result, err := ai.AnalyzeLogs(context.Background(), service.AnalyzeLogsRequest{LogIds: []int64{1}})
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}
	if !strings.Contains(result.Content, "第一段") || !strings.Contains(result.Content, "第二段") {
		t.Fatalf("应拼接两段内容: %q", result.Content)
	}
	if result.Truncated {
		t.Fatal("续写成功后不应标记截断")
	}
	if result.FinishReason != "stop" {
		t.Fatalf("finish_reason 应为 stop，实际 %q", result.FinishReason)
	}
	if mock.rounds != 2 {
		t.Fatalf("应请求 2 次（1 次截断 + 1 次续写），实际 %d", mock.rounds)
	}
	// 续写请求里应带上「继续」指令
	if !strings.Contains(mock.requests[1], "继续输出剩余内容") {
		t.Fatal("续写请求应包含继续指令")
	}
	// 用量应累计
	if result.Usage.TotalTokens != 60 {
		t.Fatalf("用量应累计为 60，实际 %d", result.Usage.TotalTokens)
	}
}

// TestAnalyzeStillTruncated 校验续写后仍截断时标记 Truncated。
func TestAnalyzeStillTruncated(t *testing.T) {
	// 每次都返回 length（lengths 设得很大）
	ai, mock, closeServer := newTruncAI(t, 99, 4096)
	defer closeServer()

	result, err := ai.AnalyzeLogs(context.Background(), service.AnalyzeLogsRequest{LogIds: []int64{1}})
	if err != nil {
		t.Fatalf("分析失败: %v", err)
	}
	if !result.Truncated {
		t.Fatal("仍被截断时应标记 Truncated")
	}
	if result.FinishReason != "length" {
		t.Fatalf("finish_reason 应为 length，实际 %q", result.FinishReason)
	}
	// 1 次原始 + 2 次续写
	if mock.rounds != 3 {
		t.Fatalf("应请求 3 次，实际 %d", mock.rounds)
	}
}

// TestAnalyzeMaxTokensUnlimited 校验 max_tokens=-1 时请求不携带该字段。
func TestAnalyzeMaxTokensUnlimited(t *testing.T) {
	ai, mock, closeServer := newTruncAI(t, 0, -1)
	defer closeServer()

	if _, err := ai.AnalyzeLogs(context.Background(), service.AnalyzeLogsRequest{LogIds: []int64{1}}); err != nil {
		t.Fatalf("分析失败: %v", err)
	}
	if len(mock.requests) == 0 {
		t.Fatal("未捕获请求")
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(mock.requests[0]), &body); err != nil {
		t.Fatalf("请求体解析失败: %v", err)
	}
	if _, exists := body["max_tokens"]; exists {
		t.Fatalf("不限制模式不应发送 max_tokens: %v", body["max_tokens"])
	}
}

// TestDefaultMaxTokens 校验默认输出 token 为 4096（避免长方案被截断）。
func TestDefaultMaxTokens(t *testing.T) {
	ai := newAIService()
	saved, err := ai.SaveConfig(context.Background(), service.AIConfig{
		Enabled: true,
		BaseURL: "https://api.example.com/v1",
		Model:   "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if saved.MaxTokens != 4096 {
		t.Fatalf("默认 max_tokens 应为 4096，实际 %d", saved.MaxTokens)
	}
}
