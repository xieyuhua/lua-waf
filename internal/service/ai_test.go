package service_test

import (
	"context"
	"strings"
	"testing"

	"wafadmin/internal/repository/memory"
	"wafadmin/internal/service"
)

// newAIService 构造基于内存仓储的 AI 服务。
func newAIService() *service.AIService {
	store := (&memory.Store{}).Init(false)
	return &service.AIService{
		Settings: &service.SettingService{Repo: store.Repos().Setting},
		Logs:     store.Repos().Log,
	}
}

// TestAIConfigSaveTwice 校验 AI 配置可以反复修改（首次保存后再次保存不报错）。
func TestAIConfigSaveTwice(t *testing.T) {
	ai := newAIService()
	ctx := context.Background()

	first, err := ai.SaveConfig(ctx, service.AIConfig{
		Enabled:   true,
		BaseURL:   "https://api.example.com/v1",
		APIKey:    "sk-first-key-123456",
		Model:     "gpt-4o-mini",
		MaxTokens: 512,
	})
	if err != nil {
		t.Fatalf("首次保存失败: %v", err)
	}
	if first.UpdatedAt == "" {
		t.Fatal("首次保存未写入更新时间")
	}

	// 第二次修改模型与温度，密钥传掩码应保留原值
	status, err := ai.Status(ctx)
	if err != nil {
		t.Fatalf("读取状态失败: %v", err)
	}
	masked := status.Config.APIKey
	if !strings.Contains(masked, "********") {
		t.Fatalf("回显密钥应脱敏: %q", masked)
	}

	second, err := ai.SaveConfig(ctx, service.AIConfig{
		Enabled:     true,
		BaseURL:     "https://api.example.com/v1/",
		APIKey:      masked,
		Model:       "gpt-4o",
		Temperature: 0.7,
		MaxTokens:   2048,
	})
	if err != nil {
		t.Fatalf("二次保存失败（修改应生效）: %v", err)
	}
	if second.Model != "gpt-4o" {
		t.Fatalf("模型未更新: %s", second.Model)
	}
	if second.Temperature != 0.7 {
		t.Fatalf("温度未更新: %v", second.Temperature)
	}
	if second.APIKey != "sk-first-key-123456" {
		t.Fatalf("掩码提交应保留原密钥，实际 %q", second.APIKey)
	}
	if strings.HasSuffix(second.BaseURL, "/") {
		t.Fatalf("接口地址应去掉末尾斜杠: %s", second.BaseURL)
	}

	// 第三次：关闭启用
	third, err := ai.SaveConfig(ctx, service.AIConfig{
		Enabled: false,
		BaseURL: "https://api.example.com/v1",
		APIKey:  masked,
		Model:   "gpt-4o",
	})
	if err != nil {
		t.Fatalf("三次保存失败: %v", err)
	}
	if third.Enabled {
		t.Fatal("关闭启用未生效")
	}
}

// TestAIConfigValidation 校验非法配置被拒绝。
func TestAIConfigValidation(t *testing.T) {
	ai := newAIService()
	ctx := context.Background()

	cases := []struct {
		name string
		cfg  service.AIConfig
	}{
		{name: "地址协议非法", cfg: service.AIConfig{Enabled: true, BaseURL: "api.example.com", Model: "gpt-4o"}},
		{name: "启用但缺地址", cfg: service.AIConfig{Enabled: true, Model: "gpt-4o"}},
		{name: "启用但缺模型", cfg: service.AIConfig{Enabled: true, BaseURL: "https://api.example.com/v1"}},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if _, err := ai.SaveConfig(ctx, item.cfg); err == nil {
				t.Fatal("应返回校验错误")
			}
		})
	}
}

// TestAIConfigDefaults 校验默认值与上限裁剪。
func TestAIConfigDefaults(t *testing.T) {
	ai := newAIService()
	ctx := context.Background()

	saved, err := ai.SaveConfig(ctx, service.AIConfig{
		Enabled:        true,
		BaseURL:        "https://api.example.com/v1",
		Model:          "gpt-4o-mini",
		MaxTokens:      999999,
		TimeoutSeconds: 9999,
		Temperature:    5,
	})
	if err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if saved.MaxTokens != 32768 {
		t.Fatalf("max_tokens 未裁剪到 32768: %d", saved.MaxTokens)
	}
	if saved.TimeoutSeconds != 900 {
		t.Fatalf("timeout_seconds 未裁剪到 900: %d", saved.TimeoutSeconds)
	}
	if saved.Temperature != 2 {
		t.Fatalf("temperature 未裁剪到 2: %v", saved.Temperature)
	}
	if saved.SystemPrompt == "" {
		t.Fatal("未补默认系统提示词")
	}
}
