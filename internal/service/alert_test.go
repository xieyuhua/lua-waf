package service_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"wafadmin/internal/model"
	"wafadmin/internal/repository/memory"
	"wafadmin/internal/service"
)

// webhookRecorder 记录企业微信机器人收到的消息。
type webhookRecorder struct {
	mu       sync.Mutex
	messages []string
	failWith int
}

// server 起一个假的群机器人服务。
func (r *webhookRecorder) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		var payload struct {
			MsgType string `json:"msgtype"`
			Text    struct {
				Content string `json:"content"`
			} `json:"text"`
		}
		_ = json.Unmarshal(body, &payload)

		r.mu.Lock()
		r.messages = append(r.messages, payload.Text.Content)
		fail := r.failWith
		r.mu.Unlock()

		if fail != 0 {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"errcode":` + itoa(fail) + `,"errmsg":"mock error"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// count 返回收到的消息条数。
func (r *webhookRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.messages)
}

// last 返回最后一条消息内容。
func (r *webhookRecorder) last() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.messages) == 0 {
		return ""
	}
	return r.messages[len(r.messages)-1]
}

// itoa 简易整数转字符串。
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	digits := ""
	for v > 0 {
		digits = string(rune('0'+v%10)) + digits
		v /= 10
	}
	return digits
}

// TestAlertNotifyMatchesSelectedRule 校验只有命中「勾选的规则」时才推送。
func TestAlertNotifyMatchesSelectedRule(t *testing.T) {
	store := (&memory.Store{}).Init(false)
	repos := store.Repos()
	ctx := context.Background()

	// 造两条规则：只勾选第一条
	wanted := &model.Rule{RuleType: "args", RuleItem: `union\s+select`, Enabled: true}
	other := &model.Rule{RuleType: "url", RuleItem: `\.git`, Enabled: true}
	if err := repos.Rule.Create(ctx, wanted); err != nil {
		t.Fatalf("创建规则失败: %v", err)
	}
	if err := repos.Rule.Create(ctx, other); err != nil {
		t.Fatalf("创建规则失败: %v", err)
	}

	recorder := &webhookRecorder{}
	srv := recorder.server(t)

	alert := &service.AlertService{Settings: repos.Setting, Rules: repos.Rule}
	if _, err := alert.SaveConfig(ctx, service.AlertConfig{
		Enabled:     true,
		WebhookURL:  srv.URL,
		RuleIds:     []int64{wanted.Id},
		IntervalSec: 60,
	}); err != nil {
		t.Fatalf("保存告警配置失败: %v", err)
	}

	// 1) 命中勾选规则 → 推送
	alert.Notify(ctx, []model.WafLog{
		{ClientIp: "1.2.3.4", AttackType: "get参数攻击", RuleTag: `union\s+select`, ReqUrl: "/api/order?id=1", LocalTime: "2026-10-29 12:00:00"},
	})
	if recorder.count() != 1 {
		t.Fatalf("命中勾选规则应推送 1 条，实际 %d", recorder.count())
	}
	if content := recorder.last(); content == "" || !containsText(content, "/api/order") {
		t.Fatalf("消息内容应包含请求 URL，实际: %s", content)
	}

	// 2) 静默期内再次命中同一条规则 → 不重复推送
	alert.Notify(ctx, []model.WafLog{
		{ClientIp: "1.2.3.4", AttackType: "get参数攻击", RuleTag: `union\s+select`, ReqUrl: "/api/order?id=2"},
	})
	if recorder.count() != 1 {
		t.Fatalf("静默期内不应重复推送，实际 %d 条", recorder.count())
	}

	// 3) 命中未勾选的规则 → 不推送
	alert.Notify(ctx, []model.WafLog{
		{ClientIp: "1.2.3.4", AttackType: "黑名单url", RuleTag: `\.git`, ReqUrl: "/.git/config"},
	})
	if recorder.count() != 1 {
		t.Fatalf("未勾选的规则不应推送，实际 %d 条", recorder.count())
	}
}

// TestAlertDisabledDoesNotNotify 校验关闭开关后不推送。
func TestAlertDisabledDoesNotNotify(t *testing.T) {
	store := (&memory.Store{}).Init(false)
	repos := store.Repos()
	ctx := context.Background()

	rule := &model.Rule{RuleType: "args", RuleItem: `select.*from`, Enabled: true}
	if err := repos.Rule.Create(ctx, rule); err != nil {
		t.Fatalf("创建规则失败: %v", err)
	}

	recorder := &webhookRecorder{}
	srv := recorder.server(t)
	alert := &service.AlertService{Settings: repos.Setting, Rules: repos.Rule}
	if _, err := alert.SaveConfig(ctx, service.AlertConfig{
		Enabled:    false,
		WebhookURL: srv.URL,
		RuleIds:    []int64{rule.Id},
	}); err != nil {
		t.Fatalf("保存配置失败: %v", err)
	}

	alert.Notify(ctx, []model.WafLog{{ClientIp: "1.1.1.1", AttackType: "get参数攻击", RuleTag: `select.*from`}})
	if recorder.count() != 0 {
		t.Fatalf("关闭开关后不应推送，实际 %d 条", recorder.count())
	}
}

// TestAlertTestSendsMessage 校验「发送测试」能真正打到 webhook，并能识别 errcode。
func TestAlertTestSendsMessage(t *testing.T) {
	store := (&memory.Store{}).Init(false)
	repos := store.Repos()
	ctx := context.Background()

	recorder := &webhookRecorder{}
	srv := recorder.server(t)
	alert := &service.AlertService{Settings: repos.Setting, Rules: repos.Rule}

	if err := alert.Test(ctx, service.AlertConfig{WebhookURL: srv.URL}); err != nil {
		t.Fatalf("发送测试消息失败: %v", err)
	}
	if recorder.count() != 1 || !containsText(recorder.last(), "告警测试") {
		t.Fatalf("测试消息未按预期发送: %s", recorder.last())
	}

	// errcode != 0 时应返回错误
	recorder.mu.Lock()
	recorder.failWith = 93000
	recorder.mu.Unlock()
	if err := alert.Test(ctx, service.AlertConfig{WebhookURL: srv.URL}); err == nil {
		t.Fatal("企业微信返回 errcode 时应报错")
	}
}

// TestAlertConfigValidation 校验配置校验规则。
func TestAlertConfigValidation(t *testing.T) {
	store := (&memory.Store{}).Init(false)
	repos := store.Repos()
	ctx := context.Background()
	alert := &service.AlertService{Settings: repos.Setting, Rules: repos.Rule}

	cases := []struct {
		name string
		cfg  service.AlertConfig
	}{
		{"启用但没填 webhook", service.AlertConfig{Enabled: true}},
		{"webhook 协议不对", service.AlertConfig{WebhookURL: "ftp://x"}},
		{"静默期过大", service.AlertConfig{WebhookURL: "https://example.com", IntervalSec: 99999}},
	}
	for _, item := range cases {
		if _, err := alert.SaveConfig(ctx, item.cfg); err == nil {
			t.Fatalf("%s 应校验失败", item.name)
		}
	}
}

// containsText 判断子串。
func containsText(text, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(text); i++ {
		if text[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
