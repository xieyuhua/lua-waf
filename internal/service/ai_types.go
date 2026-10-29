package service

// 本文件是 AI 模块的「配置与数据结构」：配置模型、请求/响应 DTO、OpenAI 兼容协议结构。
// 分析编排见 ai.go，提示词与错误翻译见 ai_prompt.go。

import (
	"strings"
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
