package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wafadmin/internal/repository/memory"
	"wafadmin/internal/service"
)

// newReplyAI 构造一个返回自定义响应的 AI 服务（用于验证不同模型返回形态）。
func newReplyAI(t *testing.T, reply map[string]any) (*service.AIService, func()) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(reply)
	}))

	store := (&memory.Store{}).Init(true)
	ai := &service.AIService{
		Settings: &service.SettingService{Repo: store.Repos().Setting},
		Logs:     store.Repos().Log,
		Client:   server.Client(),
		Cache:    &service.AnalyzeCache{},
	}
	if _, err := ai.SaveConfig(context.Background(), service.AIConfig{
		Enabled: true,
		BaseURL: server.URL,
		Model:   "fake-model",
	}); err != nil {
		server.Close()
		t.Fatalf("保存 AI 配置失败: %v", err)
	}
	return ai, server.Close
}

// TestAnalyzeLogsFallbackToReasoning 校验推理模型只返回 reasoning_content 时不会出现空结果。
func TestAnalyzeLogsFallbackToReasoning(t *testing.T) {
	ai, closeServer := newReplyAI(t, map[string]any{
		"model": "reasoner",
		"choices": []any{map[string]any{
			"message":       map[string]any{"role": "assistant", "content": "", "reasoning_content": "先看请求路径，存在目录穿越特征。"},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30},
	})
	defer closeServer()

	result, err := ai.AnalyzeLogs(context.Background(), service.AnalyzeLogsRequest{LogIds: []int64{1}})
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if strings.TrimSpace(result.Content) == "" {
		t.Fatal("应回退到 reasoning_content，不能返回空正文")
	}
	if !strings.Contains(result.Content, "思考过程") || !strings.Contains(result.Content, "目录穿越") {
		t.Fatalf("回退内容不符合预期: %q", result.Content)
	}
}

// TestAnalyzeLogsEmptyContentError 校验模型返回空正文时给出明确错误，且不写入缓存。
func TestAnalyzeLogsEmptyContentError(t *testing.T) {
	ai, closeServer := newReplyAI(t, map[string]any{
		"model": "empty-model",
		"choices": []any{map[string]any{
			"message":       map[string]any{"role": "assistant", "content": ""},
			"finish_reason": "length",
		}},
		"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 1024, "total_tokens": 1034},
	})
	defer closeServer()

	_, err := ai.AnalyzeLogs(context.Background(), service.AnalyzeLogsRequest{LogIds: []int64{1}})
	if err == nil {
		t.Fatal("空正文应返回错误")
	}
	message := err.Error()
	for _, want := range []string{"内容为空", "finish_reason=length", "最大输出 tokens"} {
		if !strings.Contains(message, want) {
			t.Fatalf("错误信息应包含 %q，实际 %q", want, message)
		}
	}

	// 空结果不应进缓存
	if _, cacheErr := ai.CachedResult(context.Background(), 1); cacheErr == nil {
		t.Fatal("空正文不应写入缓存")
	}
}
