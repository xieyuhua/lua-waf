package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// 分析结果缓存默认时长（分钟）。
const defaultAICacheMinutes = 10

// 输出 token 相关：默认 4096（避免长方案被截断），上限 32768，-1 表示不限制。
const (
	defaultAIMaxTokens = 4096
	maxAIMaxTokens     = 32768
)

// maxChatContinue 是输出被 max_tokens 截断后的自动续写轮数上限。
const maxChatContinue = 2

// 分析类请求（日志研判 / 慢 SQL 优化）的最小超时与重试次数：
// 长提示词 + 长输出很容易超过 60 秒，用户配置得更小也按该下限执行。
const (
	minAnalyzeTimeoutSeconds = 180
	maxChatRetries           = 1
)

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
	// CacheMinutes 为分析结果缓存时长（分钟），默认 10。
	CacheMinutes int    `json:"cache_minutes"`
	UpdatedAt    string `json:"updated_at"`
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
	// MaxTokens：0 表示用默认值，-1（或更小的负数）表示不限制（请求里不发送该字段）
	if out.MaxTokens == 0 {
		out.MaxTokens = defaultAIMaxTokens
	}
	if out.MaxTokens > maxAIMaxTokens {
		out.MaxTokens = maxAIMaxTokens
	}
	if out.MaxTokens < 0 {
		out.MaxTokens = -1
	}
	if out.TimeoutSeconds <= 0 {
		out.TimeoutSeconds = 60
	}
	if out.TimeoutSeconds > 900 {
		out.TimeoutSeconds = 900
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
	// 缓存时长：默认 10 分钟，上限 120 分钟
	if out.CacheMinutes <= 0 {
		out.CacheMinutes = defaultAICacheMinutes
	}
	if out.CacheMinutes > 120 {
		out.CacheMinutes = 120
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
	// Refresh 为 true 时跳过多读缓存、强制重新调用模型。
	Refresh bool `json:"refresh"`
}

// AnalyzeLogsResult 是日志 AI 分析的结果。
type AnalyzeLogsResult struct {
	Content   string  `json:"content"`
	Model     string  `json:"model"`
	ElapsedMs int64   `json:"elapsed_ms"`
	LogCount  int     `json:"log_count"`
	Usage     AIUsage `json:"usage"`
	// Cached 表示本次结果直接来自缓存（相同参数在有效期内复用）。
	Cached bool `json:"cached"`
	// CachedAt 为缓存生成时间（Cached 为 true 时有值）。
	CachedAt string `json:"cached_at,omitempty"`
	// Truncated 表示模型输出达到 max_tokens 被截断（自动续写后仍截断时为 true）。
	Truncated bool `json:"truncated,omitempty"`
	// FinishReason 为模型返回的结束原因。
	FinishReason string `json:"finish_reason,omitempty"`
}

// AIUsage 是本次调用的 token 用量。
type AIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// chatMessage 是对话消息。
//
// 推理类模型（deepseek-reasoner 等）常把思考过程放在 reasoning_content，
// 而 content 可能为空，因此需要一并接收。
type chatMessage struct {
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
	Reasoning        string `json:"reasoning,omitempty"`
}

// chatResponse 是大模型返回结构。
type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
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
	// SlowSQLs 用于慢 SQL 分析，可为空（此时慢 SQL 分析不可用）。
	SlowSQLs repository.SlowSQLRepository
	// Client 复用连接池；超时由配置中的 timeout_seconds 控制。
	Client *http.Client
	// Cache 为分析结果缓存，为 nil 时不做缓存。
	Cache *AnalyzeCache
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

	if containsMask(cfg.APIKey) || strings.TrimSpace(cfg.APIKey) == "" {
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
	// 配置变更后清空分析缓存，避免用旧提示词/模型的结果
	if s.Cache != nil {
		s.Cache.Purge()
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
	outcome, err := s.chat(ctx, cfg, []chatMessage{
		{Role: "system", Content: "你是连通性测试助手，请只回复 OK。"},
		{Role: "user", Content: "ping"},
	})
	if err != nil {
		return nil, err
	}
	reply, usage, model := outcome.Content, outcome.Usage, outcome.Model
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

	// 分析请求提示词与输出都很长，超时至少按 minAnalyzeTimeoutSeconds 执行（不改动用户配置）
	if cfg.TimeoutSeconds < minAnalyzeTimeoutSeconds {
		cfg.TimeoutSeconds = minAnalyzeTimeoutSeconds
	}

	question := strings.TrimSpace(req.Question)
	if question == "" {
		question = "请研判这些攻击日志是否为真实攻击，并给出处置建议。"
	}

	// 缓存命中直接返回（相同模型/提示词/日志/追问，且未强制刷新）
	cacheKey := ""
	if s.Cache != nil && !req.Refresh {
		cacheKey = s.Cache.Key(cfg, SceneLog, req.LogIds, question)
		if cached, storedAt, ok := s.Cache.Get(cacheKey); ok {
			cached.Cached = true
			cached.CachedAt = storedAt.Format("2006-01-02 15:04:05")
			return &cached, nil
		}
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

	start := time.Now()
	outcome, err := s.chat(ctx, cfg, []chatMessage{
		{Role: "system", Content: cfg.SystemPrompt},
		{Role: "user", Content: buildLogPrompt(logs, question)},
	})
	if err != nil {
		return nil, err
	}

	result := &AnalyzeLogsResult{
		Content:      outcome.Content,
		Model:        outcome.Model,
		ElapsedMs:    time.Since(start).Milliseconds(),
		LogCount:     len(logs),
		Usage:        outcome.Usage,
		Truncated:    outcome.Truncated,
		FinishReason: outcome.FinishReason,
	}
	// 只缓存有正文的结果，避免把空内容写进缓存
	if s.Cache != nil && strings.TrimSpace(result.Content) != "" {
		s.Cache.Set(cacheKey, SceneLog, *result, req.LogIds, time.Duration(cfg.CacheMinutes)*time.Minute)
	}
	return result, nil
}

// CachedResult 按单条日志查询服务端缓存的研判结果，用于多端查看（手机/其他浏览器）。
func (s *AIService) CachedResult(ctx context.Context, logID int64) (*AnalyzeLogsResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.Cache == nil {
		return nil, ErrNotFound.New("暂无研判结果")
	}

	result, storedAt, ok := s.Cache.FindByObject(SceneLog, logID)
	if !ok {
		return nil, ErrNotFound.New("该日志暂无研判结果（可能已超过缓存有效期，请重新分析）")
	}

	result.Cached = true
	result.CachedAt = storedAt.Format("2006-01-02 15:04:05")
	return &result, nil
}

// chat 执行一次对话补全请求。
// chatOutcome 是一次对话补全的结果（含截断信息）。
type chatOutcome struct {
	// Content 为模型正文（自动续写后拼接）。
	Content string
	// Usage 为累计 token 用量。
	Usage AIUsage
	// Model 为模型名。
	Model string
	// FinishReason 为最后一次请求的结束原因。
	FinishReason string
	// Truncated 表示仍然被截断（续写后仍 length）。
	Truncated bool
}

// chat 执行一次对话补全请求。
//
// 当 max_tokens 用尽（finish_reason=length）时会自动续写，最多续写 maxChatContinue 轮，
// 尽量给用户完整方案；仍被截断则在结果上标记 Truncated，前端给出提示。
func (s *AIService) chat(ctx context.Context, cfg AIConfig, messages []chatMessage) (*chatOutcome, error) {
	// 总长度上限，避免异常模型无限续写
	const maxTotalChars = 64 << 10

	conversation := make([]chatMessage, len(messages))
	copy(conversation, messages)

	outcome := &chatOutcome{Model: cfg.Model}
	var builder strings.Builder

	for round := 0; round <= maxChatContinue; round++ {
		reply, usage, model, finishReason, err := s.requestChat(ctx, cfg, conversation)
		if err != nil {
			return nil, err
		}

		if reply != "" {
			if builder.Len() > 0 {
				builder.WriteString("\n\n")
			}
			builder.WriteString(reply)
		}
		outcome.Usage.PromptTokens += usage.PromptTokens
		outcome.Usage.CompletionTokens += usage.CompletionTokens
		outcome.Usage.TotalTokens += usage.TotalTokens
		outcome.FinishReason = finishReason
		if strings.TrimSpace(model) != "" {
			outcome.Model = model
		}

		// 正常结束或没有正文可续写
		if !strings.EqualFold(finishReason, "length") || reply == "" {
			outcome.Truncated = strings.EqualFold(finishReason, "length")
			break
		}
		if builder.Len() >= maxTotalChars {
			outcome.Truncated = true
			break
		}

		// 续写：把已输出的内容作为助手消息附带，再要求继续
		conversation = append(conversation,
			chatMessage{Role: "assistant", Content: reply},
			chatMessage{Role: "user", Content: "请从上次中断处继续输出剩余内容，不要重复已经输出过的部分，也不要重新开始。"},
		)
		outcome.Truncated = round == maxChatContinue
	}

	outcome.Content = strings.TrimSpace(builder.String())
	return outcome, nil
}

// requestChat 发起一次请求并解析结果，返回正文、用量、模型名与 finish_reason。
func (s *AIService) requestChat(ctx context.Context, cfg AIConfig, messages []chatMessage) (string, AIUsage, string, string, error) {
	body := map[string]any{
		"model":       cfg.Model,
		"messages":    messages,
		"temperature": cfg.Temperature,
		"stream":      false,
	}
	// max_tokens 为 -1 时不发送该字段，交由模型/服务端决定（避免被硬截断）
	if cfg.MaxTokens > 0 {
		body["max_tokens"] = cfg.MaxTokens
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", AIUsage{}, "", "", ErrInternal.Wrap(err, "构造请求失败")
	}

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	endpoint := cfg.BaseURL + "/chat/completions"
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", AIUsage{}, "", "", ErrInternal.Wrap(err, "构造请求失败")
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
	// 连接被对端重置 / EOF 等瞬时错误重试一次（超时与取消不重试）
	for attempt := 0; err != nil && attempt < maxChatRetries && isRetryableTransportError(err); attempt++ {
		select {
		case <-requestCtx.Done():
			// 上下文已结束，不再重试
			attempt = maxChatRetries
			continue
		case <-time.After(300 * time.Millisecond):
		}
		newRequest, buildErr := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if buildErr != nil {
			break
		}
		newRequest.Header = request.Header.Clone()
		response, err = client.Do(newRequest)
	}
	if err != nil {
		return "", AIUsage{}, "", "", ErrInternal.Wrap(err, "调用大模型接口失败：%s", describeTransportError(err, endpoint, timeout))
	}
	defer response.Body.Close()

	raw, readErr := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if readErr != nil {
		return "", AIUsage{}, "", "", ErrInternal.Wrap(readErr, "读取模型响应失败")
	}

	parsed := new(chatResponse)
	if err := json.Unmarshal(raw, parsed); err != nil {
		return "", AIUsage{}, "", "", ErrInternal.New("模型返回内容无法解析，请检查接口是否为 OpenAI 兼容格式")
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return "", AIUsage{}, "", "", ErrInternal.New("模型返回错误: %s", parsed.Error.Message)
	}
	if response.StatusCode >= 400 {
		return "", AIUsage{}, "", "", ErrInternal.New("模型接口返回状态码 %d", response.StatusCode)
	}
	if len(parsed.Choices) == 0 {
		return "", AIUsage{}, "", "", ErrInternal.New("模型未返回分析内容")
	}

	choice := parsed.Choices[0]
	content := strings.TrimSpace(choice.Message.Content)
	if content == "" {
		// 推理类模型可能只返回 reasoning_content（content 为空），此时退化为思考过程，
		// 并在末尾标注，避免前端出现「有 token 却看不到内容」的空白结果。
		reasoning := strings.TrimSpace(choice.Message.ReasoningContent)
		if reasoning == "" {
			reasoning = strings.TrimSpace(choice.Message.Reasoning)
		}
		if reasoning != "" {
			return "（模型未返回正文，以下为思考过程）\n\n" + reasoning, parsed.Usage, parsed.Model, choice.FinishReason, nil
		}
		hint := "模型返回内容为空"
		if choice.FinishReason != "" {
			hint += "（finish_reason=" + choice.FinishReason + "）"
		}
		return "", parsed.Usage, parsed.Model, choice.FinishReason, ErrInternal.New("%s，请更换模型或调大「最大输出 tokens」后重试", hint)
	}
	return content, parsed.Usage, parsed.Model, choice.FinishReason, nil
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
		builder.WriteString(fmt.Sprintf("- 请求路径: %s\n", truncateText(item.ReqUrl, 2000)))
		builder.WriteString(fmt.Sprintf("- User-Agent: %s\n", truncateText(fallback(item.UserAgent, "无"), 600)))
		if strings.TrimSpace(item.ReqData) != "" {
			builder.WriteString(fmt.Sprintf("- 请求体: %s\n", truncateText(item.ReqData, 2000)))
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

// describeTransportError 把传输层错误翻译成可操作的提示，并附带原始错误信息。
func describeTransportError(err error, endpoint string, timeout time.Duration) string {
	raw := err.Error()
	lower := strings.ToLower(raw)

	hint := ""
	switch {
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(lower, "context deadline exceeded"):
		hint = fmt.Sprintf("请求超时（%s 内模型未返回），可在「AI 设置」调大「请求超时」，或改用更快的模型", timeout)
	case errors.Is(err, context.Canceled):
		hint = "请求已被取消（页面关闭或服务重启）"
	case strings.Contains(lower, "connection refused"):
		hint = "连接被拒绝：模型服务未启动或端口不对"
	case strings.Contains(lower, "no such host") || strings.Contains(lower, "server misbehaving"):
		hint = "域名解析失败：请检查接口地址"
	case strings.Contains(lower, "i/o timeout") || strings.Contains(lower, "dial tcp") && strings.Contains(lower, "timeout"):
		hint = "连接超时：网络不可达或被防火墙拦截"
	case strings.Contains(lower, "eof") || strings.Contains(lower, "reset by peer") || strings.Contains(lower, "broken pipe"):
		hint = "连接被对端中断：模型服务可能过载或主动断开"
	case strings.Contains(lower, "x509") || strings.Contains(lower, "certificate"):
		hint = "TLS 证书校验失败：可改用 http 内网地址，或为接口配置受信任证书"
	case strings.Contains(lower, "unsupported protocol scheme"):
		hint = "接口地址缺少 http:// 或 https:// 前缀"
	case strings.Contains(lower, "too many redirects"):
		hint = "接口发生多次重定向：请确认地址是否指向 /v1 接口"
	default:
		hint = "网络请求失败"
	}
	return fmt.Sprintf("%s（%s；原始错误：%s）", hint, endpoint, raw)
}

// isRetryableTransportError 判断是否为可重试的瞬时传输错误。
func isRetryableTransportError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}
	lower := strings.ToLower(err.Error())
	for _, keyword := range []string{"eof", "reset by peer", "broken pipe", "connection reset", "temporary failure"} {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}
