package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"wafadmin/internal/repository/memory"
	"wafadmin/internal/service"
)

// newRawAI 构造指向自定义 handler 的 AI 服务。
func newRawAI(t *testing.T, handler http.HandlerFunc, cfg service.AIConfig) (*service.AIService, func()) {
	t.Helper()

	server := httptest.NewServer(handler)
	store := (&memory.Store{}).Init(true)
	ai := &service.AIService{
		Settings: &service.SettingService{Repo: store.Repos().Setting},
		Logs:     store.Repos().Log,
		SlowSQLs: store.Repos().SlowSQL,
		Client:   server.Client(),
		Cache:    &service.AnalyzeCache{},
	}
	cfg.Enabled = true
	cfg.BaseURL = server.URL
	if _, err := ai.SaveConfig(context.Background(), cfg); err != nil {
		server.Close()
		t.Fatalf("保存 AI 配置失败: %v", err)
	}
	return ai, server.Close
}

// TestAnalyzeTransportErrorDetail 校验网络错误时错误信息包含原因与地址，而不是笼统提示。
func TestAnalyzeTransportErrorDetail(t *testing.T) {
	// 服务端直接断开连接，模拟对端不可用
	ai, closeServer := newRawAI(t, func(w http.ResponseWriter, r *http.Request) {
		if hijacker, ok := w.(http.Hijacker); ok {
			conn, _, err := hijacker.Hijack()
			if err == nil {
				_ = conn.Close()
				return
			}
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}, service.AIConfig{Model: "fake-model", TimeoutSeconds: 5, MaxTokens: 512})
	defer closeServer()

	_, err := ai.AnalyzeLogs(context.Background(), service.AnalyzeLogsRequest{LogIds: []int64{1}})
	if err == nil {
		t.Fatal("应返回错误")
	}
	message := err.Error()
	for _, want := range []string{"调用大模型接口失败", "原始错误", "http://"} {
		if !strings.Contains(message, want) {
			t.Fatalf("错误信息应包含 %q，实际：%q", want, message)
		}
	}
}

// TestAnalyzeRetryOnTransientError 校验瞬时中断后自动重试一次即可成功。
func TestAnalyzeRetryOnTransientError(t *testing.T) {
	var calls int64
	ai, closeServer := newRawAI(t, func(w http.ResponseWriter, r *http.Request) {
		// 第一次直接断开，第二次正常返回
		if atomic.AddInt64(&calls, 1) == 1 {
			if hijacker, ok := w.(http.Hijacker); ok {
				conn, _, err := hijacker.Hijack()
				if err == nil {
					_ = conn.Close()
					return
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "fake-model",
			"choices": []any{map[string]any{
				"message":       map[string]any{"role": "assistant", "content": "重试后成功"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}, service.AIConfig{Model: "fake-model", TimeoutSeconds: 10, MaxTokens: 512})
	defer closeServer()

	result, err := ai.AnalyzeLogs(context.Background(), service.AnalyzeLogsRequest{LogIds: []int64{1}})
	if err != nil {
		t.Fatalf("重试后应成功: %v", err)
	}
	if !strings.Contains(result.Content, "重试后成功") {
		t.Fatalf("内容异常: %q", result.Content)
	}
	if atomic.LoadInt64(&calls) < 2 {
		t.Fatalf("应至少请求 2 次，实际 %d", calls)
	}
}

// TestAnalyzeTimeoutFloor 校验分析请求至少按 180 秒执行：即使配置里只给 1 秒也能跑完。
func TestAnalyzeTimeoutFloor(t *testing.T) {
	var calls int64
	ai, closeServer := newRawAI(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		time.Sleep(2 * time.Second) // 超过配置的 1 秒，但在最小超时 180 秒之内
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "fake-model",
			"choices": []any{map[string]any{
				"message":       map[string]any{"role": "assistant", "content": "慢模型也能返回"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}, service.AIConfig{Model: "fake-model", TimeoutSeconds: 1, MaxTokens: 512})
	defer closeServer()

	start := time.Now()
	result, err := ai.AnalyzeLogs(context.Background(), service.AnalyzeLogsRequest{LogIds: []int64{1}})
	if err != nil {
		t.Fatalf("分析请求应按最小超时兜底并成功，实际报错: %v", err)
	}
	if !strings.Contains(result.Content, "慢模型也能返回") {
		t.Fatalf("内容异常: %q", result.Content)
	}
	if elapsed := time.Since(start); elapsed < 2*time.Second {
		t.Fatalf("耗时异常（应 >= 2s）: %s", elapsed)
	}
	if atomic.LoadInt64(&calls) != 1 {
		t.Fatalf("不应重试，实际请求 %d 次", calls)
	}
}
