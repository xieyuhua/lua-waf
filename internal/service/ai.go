package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"wafadmin/internal/model"
	"wafadmin/internal/repository"
)

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
