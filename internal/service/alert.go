package service

// 本文件实现「命中规则时推送企业微信机器人提醒」。
//
// 使用场景：只关心少数几条关键规则（例如订单接口的参数规则），命中时第一时间推到企业微信群，
// 而不是让人盯着后台看。因此告警是**按规则条目勾选**的：只有当命中的规则在勾选列表里才发消息。
//
// 设计要点：
//   - 配置存在 setting 表（key = alert.config），无需新表；
//   - 触发点在日志入库消费者：一批日志成功入库后，把命中勾选规则的挑出来推送；
//   - 同一个规则有**静默期**（默认 60 秒），避免刷屏；
//   - 推送失败只记日志，绝不影响入库。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"wafadmin/internal/model"
	"wafadmin/internal/repository"
)

// AlertConfigKey 为告警配置在 setting 表中的键名。
const AlertConfigKey = "alert.config"

// alertMaxPerMessage 为单条消息最多列出的命中条数（其余折叠为「等 N 条」）。
const alertMaxPerMessage = 8

// AlertConfig 是企业微信机器人告警配置。
type AlertConfig struct {
	// Enabled 为总开关。
	Enabled bool `json:"enabled"`
	// WebhookURL 为企业微信群机器人地址（https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxx）。
	WebhookURL string `json:"webhook_url"`
	// RuleIds 为需要提醒的规则 ID 列表（后台勾选）。
	RuleIds []int64 `json:"rule_ids"`
	// RuleNames 为规则的展示名（仅用于界面回显，不参与匹配）。
	RuleNames map[int64]string `json:"rule_names,omitempty"`
	// IntervalSec 为同一规则的静默期（秒），默认 60。
	IntervalSec int `json:"interval_sec"`
	// MentionAll 为 true 时消息里 @所有人。
	MentionAll bool `json:"mention_all"`
}

// AlertStatus 为告警配置的展示状态。
type AlertStatus struct {
	AlertConfig
	// RuleCount 为当前勾选的规则数。
	RuleCount int `json:"rule_count"`
	// LastSentAt 为最近一次成功推送时间（进程内）。
	LastSentAt string `json:"last_sent_at"`
	// LastError 为最近一次失败原因（进程内）。
	LastError string `json:"last_error"`
}

// AlertService 负责告警配置管理与消息推送。
type AlertService struct {
	// Settings 为配置存储（setting 表）。
	Settings repository.SettingRepository
	// Rules 为规则仓储，用于把勾选的 ID 解析成规则内容。
	Rules repository.RuleRepository
	// Client 为推送用的 HTTP 客户端（为空时使用默认 5 秒超时客户端）。
	Client *http.Client

	// 进程内状态：静默期记录 + 最近的推送结果
	mu        sync.Mutex
	lastSent  map[string]time.Time
	lastAt    time.Time
	lastError string
	// 规则内容缓存（Id -> RuleItem）
	ruleCache   map[int64]string
	ruleCacheAt time.Time
}

// httpClient 返回推送用客户端。
func (s *AlertService) httpClient() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: 5 * time.Second}
}

// LoadConfig 读取告警配置（未配置时返回安全默认值）。
func (s *AlertService) LoadConfig(ctx context.Context) (AlertConfig, error) {
	cfg := AlertConfig{IntervalSec: 60, RuleIds: []int64{}}

	if s.Settings == nil {
		return cfg, nil
	}

	raw, ok, err := s.Settings.Get(ctx, AlertConfigKey)
	if err != nil {
		return cfg, ErrInternal.Wrap(err, "读取告警配置失败")
	}
	if !ok || strings.TrimSpace(raw) == "" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return AlertConfig{IntervalSec: 60, RuleIds: []int64{}}, ErrInternal.Wrap(err, "告警配置格式不正确")
	}
	if cfg.IntervalSec <= 0 {
		cfg.IntervalSec = 60
	}
	if cfg.RuleIds == nil {
		cfg.RuleIds = []int64{}
	}
	return cfg, nil
}

// SaveConfig 校验并保存告警配置。
func (s *AlertService) SaveConfig(ctx context.Context, cfg AlertConfig) (AlertStatus, error) {
	cfg.WebhookURL = strings.TrimSpace(cfg.WebhookURL)
	if cfg.Enabled && cfg.WebhookURL == "" {
		return AlertStatus{}, ErrInvalid.New("启用告警时必须填写企业微信机器人 Webhook 地址")
	}
	if cfg.WebhookURL != "" && !strings.HasPrefix(cfg.WebhookURL, "https://") && !strings.HasPrefix(cfg.WebhookURL, "http://") {
		return AlertStatus{}, ErrInvalid.New("Webhook 地址必须以 http:// 或 https:// 开头")
	}
	if cfg.IntervalSec <= 0 {
		cfg.IntervalSec = 60
	}
	if cfg.IntervalSec > 3600 {
		return AlertStatus{}, ErrInvalid.New("静默期不能超过 3600 秒")
	}
	if len(cfg.RuleIds) > 500 {
		return AlertStatus{}, ErrInvalid.New("勾选的规则数不能超过 500 条")
	}

	// 去重并排序，保持稳定
	seen := make(map[int64]struct{}, len(cfg.RuleIds))
	ids := make([]int64, 0, len(cfg.RuleIds))
	for _, id := range cfg.RuleIds {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	cfg.RuleIds = ids

	payload, err := json.Marshal(cfg)
	if err != nil {
		return AlertStatus{}, ErrInternal.Wrap(err, "序列化告警配置失败")
	}
	if s.Settings == nil {
		return AlertStatus{}, ErrInternal.New("配置存储不可用")
	}
	if err := s.Settings.Set(ctx, AlertConfigKey, string(payload)); err != nil {
		return AlertStatus{}, ErrInternal.Wrap(err, "保存告警配置失败")
	}

	// 规则可能变了，清掉缓存
	s.mu.Lock()
	s.ruleCache = nil
	s.ruleCacheAt = time.Time{}
	s.mu.Unlock()

	return s.Status(ctx, cfg)
}

// Status 返回告警配置与进程内运行状态。
func (s *AlertService) Status(ctx context.Context, cfg AlertConfig) (AlertStatus, error) {
	status := AlertStatus{AlertConfig: cfg, RuleCount: len(cfg.RuleIds)}

	s.mu.Lock()
	if !s.lastAt.IsZero() {
		status.LastSentAt = s.lastAt.Format("2006-01-02 15:04:05")
	}
	status.LastError = s.lastError
	s.mu.Unlock()

	return status, nil
}

// View 读取配置并返回状态（供 GET 接口使用）。
func (s *AlertService) View(ctx context.Context) (AlertStatus, error) {
	cfg, err := s.LoadConfig(ctx)
	if err != nil {
		return AlertStatus{}, err
	}
	return s.Status(ctx, cfg)
}

// Test 用当前（或传入）配置发一条测试消息。
func (s *AlertService) Test(ctx context.Context, cfg AlertConfig) error {
	if strings.TrimSpace(cfg.WebhookURL) == "" {
		loaded, err := s.LoadConfig(ctx)
		if err != nil {
			return err
		}
		cfg = loaded
	}
	if strings.TrimSpace(cfg.WebhookURL) == "" {
		return ErrInvalid.New("请先填写企业微信机器人 Webhook 地址")
	}

	content := fmt.Sprintf("【X-WAF 告警测试】\n收到这条消息说明机器人配置正常。\n时间：%s",
		time.Now().Format("2006-01-02 15:04:05"))
	return s.send(ctx, cfg, content)
}

// Notify 把一批日志里「命中勾选规则」的挑出来推送。
//
// 由日志入库消费者在成功入库后调用；内部做了静默期与异常兜底，不会影响入库流程。
func (s *AlertService) Notify(ctx context.Context, logs []model.WafLog) {
	if s == nil || len(logs) == 0 {
		return
	}

	cfg, err := s.LoadConfig(ctx)
	if err != nil || !cfg.Enabled || cfg.WebhookURL == "" || len(cfg.RuleIds) == 0 {
		return
	}

	rules, err := s.selectedRuleItems(ctx, cfg.RuleIds)
	if err != nil || len(rules) == 0 {
		return
	}

	now := time.Now()
	interval := time.Duration(cfg.IntervalSec) * time.Second

	// 按规则静默期过滤；同一规则在静默期内只提醒一次
	matched := make([]model.WafLog, 0, alertMaxPerMessage)
	s.mu.Lock()
	if s.lastSent == nil {
		s.lastSent = make(map[string]time.Time, 8)
	}
	for _, item := range logs {
		tag := strings.TrimSpace(item.RuleTag)
		if tag == "" {
			continue
		}
		if _, ok := rules[tag]; !ok {
			continue
		}
		if last, ok := s.lastSent[tag]; ok && now.Sub(last) < interval {
			continue
		}
		s.lastSent[tag] = now
		matched = append(matched, item)
	}
	s.mu.Unlock()

	if len(matched) == 0 {
		return
	}

	content := buildAlertContent(matched, len(logs), cfg.MentionAll)
	if err := s.send(ctx, cfg, content); err != nil {
		s.mu.Lock()
		s.lastError = err.Error()
		s.mu.Unlock()
		return
	}

	s.mu.Lock()
	s.lastAt = now
	s.lastError = ""
	s.mu.Unlock()
}

// selectedRuleItems 把勾选的规则 ID 解析为「规则内容集合」（用于和日志的 rule_tag 匹配）。
func (s *AlertService) selectedRuleItems(ctx context.Context, ids []int64) (map[string]struct{}, error) {
	s.mu.Lock()
	cached := s.ruleCache
	fresh := time.Since(s.ruleCacheAt) < time.Minute
	s.mu.Unlock()

	if cached == nil || !fresh {
		all, err := s.Rules.List(ctx, "")
		if err != nil {
			return nil, err
		}
		mapping := make(map[int64]string, len(all))
		for _, rule := range all {
			mapping[rule.Id] = strings.TrimSpace(rule.RuleItem)
		}
		s.mu.Lock()
		s.ruleCache = mapping
		s.ruleCacheAt = time.Now()
		s.mu.Unlock()
		cached = mapping
	}

	result := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if item, ok := cached[id]; ok && item != "" {
			result[item] = struct{}{}
		}
	}
	return result, nil
}

// send 发送企业微信机器人文本消息。
func (s *AlertService) send(ctx context.Context, cfg AlertConfig, content string) error {
	body := map[string]any{
		"msgtype": "text",
		"text":    map[string]any{"content": content},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return ErrInternal.Wrap(err, "构造企业微信消息失败")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.WebhookURL, bytes.NewReader(payload))
	if err != nil {
		return ErrInternal.Wrap(err, "构造企业微信请求失败")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient().Do(req)
	if err != nil {
		return ErrInternal.Wrap(err, "推送企业微信消息失败")
	}
	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return ErrInternal.Wrap(fmt.Errorf("企业微信返回 HTTP %d", resp.StatusCode), "推送企业微信消息失败")
	}

	// 企业微信即使 HTTP 200 也可能返回 errcode != 0
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(raw, &result); err == nil && result.ErrCode != 0 {
		return ErrInternal.Wrap(fmt.Errorf("企业微信返回错误 %d：%s", result.ErrCode, result.ErrMsg), "推送企业微信消息失败")
	}
	return nil
}

// buildAlertContent 组装提醒文本。
func buildAlertContent(matched []model.WafLog, batchSize int, mentionAll bool) string {
	var builder strings.Builder
	builder.WriteString("【X-WAF 告警】命中关注的规则\n")

	limit := len(matched)
	if limit > alertMaxPerMessage {
		limit = alertMaxPerMessage
	}
	for i := 0; i < limit; i++ {
		item := matched[i]
		builder.WriteString(fmt.Sprintf("\n%d. %s｜%s\n   站点：%s\n   URL：%s\n   来源：%s %s\n   规则：%s",
			i+1, item.LocalTime, item.AttackType, fallback(item.ServerName, "-"),
			truncateText(item.ReqUrl, 120), item.ClientIp, fallback(item.Area, ""),
			truncateText(item.RuleTag, 80)))
	}
	if len(matched) > limit {
		builder.WriteString(fmt.Sprintf("\n\n…另有 %d 条同类命中（本批共 %d 条日志）", len(matched)-limit, batchSize))
	}
	if mentionAll {
		builder.WriteString("\n@all")
	}
	return builder.String()
}
