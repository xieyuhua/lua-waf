package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"wafadmin/internal/model"
	"wafadmin/internal/repository"
)

// AIConfigKey 是 AI 配置在配置表中的键名。
const AIConfigKey = "ai.config"

// 单次分析允许的最大日志条数。
const maxAnalyzeLogs = 20

// AIConfig 是大模型接入配置（OpenAI 兼容接口）。
type AIConfig struct {
	Enabled        bool    `json:"enabled"`
	BaseURL        string  `json:"base_url"`
	APIKey         string  `json:"api_key"`
	Model          string  `json:"model"`
	Temperature    float64 `json:"temperature"`
	MaxTokens      int     `json:"max_tokens"`
	TimeoutSeconds int     `json:"timeout_seconds"`
	SystemPrompt   string  `json:"system_prompt"`
	UpdatedAt      string  `json:"updated_at"`
}

// Configured 表示是否已具备调用条件。
func (c AIConfig) Configured() bool {
	return c.Enabled && strings.TrimSpace(c.BaseURL) != "" && strings.TrimSpace(c.Model) != ""
}

// WithDefaults 补全默认值。
func (c AIConfig) WithDefaults() AIConfig {
	out := c
	out.BaseURL = strings.TrimRight(strings.TrimSpace(out.BaseURL), "/")
	if out.BaseURL == "" {
		out.BaseURL = "https://api.openai.com/v1"
	}
	if strings.TrimSpace(out.Model) == "" {
		out.Model = "gpt-4o-mini"
	}
	if out.MaxTokens <= 0 {
		out.MaxTokens = 1024
	}
	if out.MaxTokens > 8192 {
		out.MaxTokens = 8192
	}
	if out.TimeoutSeconds <= 0 {
		out.TimeoutSeconds = 60
	}
	if out.TimeoutSeconds > 300 {
		out.TimeoutSeconds = 300
	}
	if out.Temperature < 0 {
		out.Temperature = 0
	}
	if out.Temperature > 2 {
		out.Temperature = 2
	}
	if strings.TrimSpace(out.SystemPrompt) == "" {
		out.SystemPrompt = defaultAISystemPrompt
	}
	return out
}

// Masked 返回脱敏配置，供前端回显。
func (c AIConfig) Masked() AIConfig {
	out := c
	if key := strings.TrimSpace(out.APIKey); key != "" {
		if len(key) <= 8 {
			out.APIKey = "********"
		} else {
			out.APIKey = key[:4] + "********" + key[len(key)-4:]
		}
	}
	return out
}

// 默认系统提示词。
const defaultAISystemPrompt = `你是一名资深的 Web 应用防火墙（WAF）安全分析师。
请基于给定的 WAF 攻击日志进行研判，输出内容包含：
1. 结论：是否为真实攻击、是否疑似误报；
2. 依据：结合请求路径、参数、User-Agent、命中规则说明判断理由；
3. 风险等级：高/中/低；
4. 处置建议：是否需要封禁 IP、调整或新增规则、需要关注的关联日志。
输出使用简体中文、结构化的 Markdown，不要编造日志中不存在的信息。`

// AIStatus 是 AI 功能的可用状态。
type AIStatus struct {
	Enabled    bool     `json:"enabled"`
	Configured bool     `json:"configured"`
	Config     AIConfig `json:"config"`
}

// AnalyzeLogsRequest 是日志 AI 分析的入参。
type AnalyzeLogsRequest struct {
	LogIds   []int64 `json:"log_ids"`
	Question string  `json:"question"`
}

// AnalyzeLogsResult 是日志 AI 分析的结果。
type AnalyzeLogsResult struct {
	Content   string  `json:"content"`
	Model     string  `json:"model"`
	ElapsedMs int64   `json:"elapsed_ms"`
	LogCount  int     `json:"log_count"`
	Usage     AIUsage `json:"usage"`
}

// AIUsage 是本次调用的 token 用量。
type AIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// chatMessage 是对话消息。
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse 是大模型返回结构。
type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage AIUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// AIService 负责 AI 配置管理与大模型调用。
type AIService struct {
	Settings *SettingService
	Logs     repository.LogRepository
	// Client 复用连接池；超时由配置中的 timeout_seconds 控制。
	Client *http.Client
}

// LoadConfig 读取并补全 AI 配置。
func (s *AIService) LoadConfig(ctx context.Context) (AIConfig, error) {
	raw, err := s.Settings.Get(ctx, AIConfigKey)
	if err != nil {
		return AIConfig{}, err
	}
	cfg := AIConfig{}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return AIConfig{}, ErrInternal.Wrap(err, "AI 配置格式损坏，请重新保存")
		}
	}
	return cfg.WithDefaults(), nil
}

// SaveConfig 校验并保存 AI 配置，APIKey 为掩码时保留原值。
func (s *AIService) SaveConfig(ctx context.Context, cfg AIConfig) (AIConfig, error) {
	current, err := s.LoadConfig(ctx)
	if err != nil {
		return AIConfig{}, err
	}

	if strings.Contains(cfg.APIKey, "********") || strings.TrimSpace(cfg.APIKey) == "" {
		cfg.APIKey = current.APIKey
	}
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	if cfg.BaseURL != "" && !strings.HasPrefix(cfg.BaseURL, "http://") && !strings.HasPrefix(cfg.BaseURL, "https://") {
		return AIConfig{}, ErrInvalid.New("接口地址需以 http:// 或 https:// 开头")
	}
	if cfg.Enabled && cfg.BaseURL == "" {
		return AIConfig{}, ErrInvalid.New("启用 AI 时必须填写接口地址")
	}
	if cfg.Enabled && strings.TrimSpace(cfg.Model) == "" {
		return AIConfig{}, ErrInvalid.New("启用 AI 时必须填写模型名称")
	}

	cfg = cfg.WithDefaults()
	cfg.UpdatedAt = time.Now().Format("2006-01-02 15:04:05")

	payload, err := json.Marshal(cfg)
	if err != nil {
		return AIConfig{}, ErrInternal.Wrap(err, "序列化 AI 配置失败")
	}
	if err := s.Settings.Set(ctx, AIConfigKey, string(payload)); err != nil {
		return AIConfig{}, err
	}
	return cfg, nil
}

// Status 返回 AI 功能状态。
func (s *AIService) Status(ctx context.Context) (*AIStatus, error) {
	cfg, err := s.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &AIStatus{Enabled: cfg.Enabled, Configured: cfg.Configured(), Config: cfg.Masked()}, nil
}

// Test 用一次最小请求验证接口连通性。
func (s *AIService) Test(ctx context.Context) (*AnalyzeLogsResult, error) {
	cfg, err := s.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Configured() {
		return nil, ErrInvalid.New("AI 未启用或配置不完整，请先完成配置")
	}

	start := time.Now()
	reply, usage, model, err := s.chat(ctx, cfg, []chatMessage{
		{Role: "system", Content: "你是连通性测试助手，请只回复 OK。"},
		{Role: "user", Content: "ping"},
	})
	if err != nil {
		return nil, err
	}
	return &AnalyzeLogsResult{
		Content:   reply,
		Model:     model,
		ElapsedMs: time.Since(start).Milliseconds(),
		Usage:     usage,
	}, nil
}

// AnalyzeLogs 调用大模型分析指定日志。
func (s *AIService) AnalyzeLogs(ctx context.Context, req AnalyzeLogsRequest) (*AnalyzeLogsResult, error) {
	cfg, err := s.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Configured() {
		return nil, ErrInvalid.New("AI 未启用或配置不完整，请先在「AI 设置」中完成配置")
	}
	if len(req.LogIds) == 0 {
		return nil, ErrInvalid.New("请至少选择一条日志")
	}
	if len(req.LogIds) > maxAnalyzeLogs {
		return nil, ErrInvalid.New("单次最多分析 %d 条日志", maxAnalyzeLogs)
	}

	logs := make([]model.WafLog, 0, len(req.LogIds))
	for _, id := range req.LogIds {
		item, findErr := s.Logs.Get(ctx, id)
		if findErr != nil {
			return nil, ErrInternal.Wrap(findErr, "查询日志失败")
		}
		if item != nil {
			logs = append(logs, *item)
		}
	}
	if len(logs) == 0 {
		return nil, ErrNotFound.New("未找到可分析的日志")
	}

	question := strings.TrimSpace(req.Question)
	if question == "" {
		question = "请研判这些攻击日志是否为真实攻击，并给出处置建议。"
	}

	start := time.Now()
	reply, usage, replyModel, err := s.chat(ctx, cfg, []chatMessage{
		{Role: "system", Content: cfg.SystemPrompt},
		{Role: "user", Content: buildLogPrompt(logs, question)},
	})
	if err != nil {
		return nil, err
	}
	return &AnalyzeLogsResult{
		Content:   reply,
		Model:     replyModel,
		ElapsedMs: time.Since(start).Milliseconds(),
		LogCount:  len(logs),
		Usage:     usage,
	}, nil
}

// chat 执行一次对话补全请求。
func (s *AIService) chat(ctx context.Context, cfg AIConfig, messages []chatMessage) (string, AIUsage, string, error) {
	body := map[string]any{
		"model":       cfg.Model,
		"messages":    messages,
		"temperature": cfg.Temperature,
		"max_tokens":  cfg.MaxTokens,
		"stream":      false,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", AIUsage{}, "", ErrInternal.Wrap(err, "构造请求失败")
	}

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	endpoint := cfg.BaseURL + "/chat/completions"
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", AIUsage{}, "", ErrInternal.Wrap(err, "构造请求失败")
	}
	request.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(cfg.APIKey); key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}

	client := s.Client
	if client == nil {
		client = &http.Client{}
	}
	response, err := client.Do(request)
	if err != nil {
		return "", AIUsage{}, "", ErrInternal.Wrap(err, "调用大模型接口失败，请检查网络与接口地址")
	}
	defer response.Body.Close()

	raw, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if readErr != nil {
		return "", AIUsage{}, "", ErrInternal.Wrap(readErr, "读取模型响应失败")
	}

	parsed := new(chatResponse)
	if err := json.Unmarshal(raw, parsed); err != nil {
		return "", AIUsage{}, "", ErrInternal.New("模型返回内容无法解析，请检查接口是否为 OpenAI 兼容格式")
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return "", AIUsage{}, "", ErrInternal.New("模型返回错误: %s", parsed.Error.Message)
	}
	if response.StatusCode >= 400 {
		return "", AIUsage{}, "", ErrInternal.New("模型接口返回状态码 %d", response.StatusCode)
	}
	if len(parsed.Choices) == 0 {
		return "", AIUsage{}, "", ErrInternal.New("模型未返回分析内容")
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), parsed.Usage, parsed.Model, nil
}

// buildLogPrompt 把日志拼装为分析提示词。
func buildLogPrompt(logs []model.WafLog, question string) string {
	var builder strings.Builder
	builder.WriteString("以下是 WAF 命中的攻击日志：\n\n")
	for index, item := range logs {
		builder.WriteString(fmt.Sprintf("### 日志 %d\n", index+1))
		builder.WriteString(fmt.Sprintf("- 时间: %s\n", item.LocalTime))
		builder.WriteString(fmt.Sprintf("- 来源 IP: %s（%s）\n", item.ClientIp, fallback(item.Area, "未知")))
		builder.WriteString(fmt.Sprintf("- 站点: %s\n", fallback(item.ServerName, "未知")))
		builder.WriteString(fmt.Sprintf("- 攻击类型: %s\n", fallback(item.AttackType, "未知")))
		builder.WriteString(fmt.Sprintf("- 命中规则: %s\n", fallback(item.RuleTag, "未知")))
		builder.WriteString(fmt.Sprintf("- 请求路径: %s\n", truncateText(item.ReqUrl, 300)))
		builder.WriteString(fmt.Sprintf("- User-Agent: %s\n", truncateText(fallback(item.UserAgent, "无"), 200)))
		if strings.TrimSpace(item.ReqData) != "" {
			builder.WriteString(fmt.Sprintf("- 请求体: %s\n", truncateText(item.ReqData, 300)))
		}
		builder.WriteString("\n")
	}
	builder.WriteString("### 请回答\n")
	builder.WriteString(question)
	return builder.String()
}

// fallback 返回非空值。
func fallback(value, def string) string {
	if strings.TrimSpace(value) == "" {
		return def
	}
	return value
}

// truncateText 截断过长文本。
func truncateText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
