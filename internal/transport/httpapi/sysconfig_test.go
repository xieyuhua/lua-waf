package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// configView 是运行配置接口的解析结构。
type configView struct {
	Source   string `json:"source"`
	File     string `json:"file"`
	Writable bool   `json:"writable"`
	Config   struct {
		RunMode       string `json:"run_mode"`
		HTTPPort      int    `json:"http_port"`
		PageSize      int    `json:"page_size"`
		APIKey        string `json:"api_key"`
		SessionSecret string `json:"session_secret"`
		SlowSQLMS     int64  `json:"slow_sql_ms"`
	} `json:"config"`
	RestartRequired []string `json:"restart_required"`
}

// TestSystemConfigRead 校验运行配置可读取且密钥脱敏。
func TestSystemConfigRead(t *testing.T) {
	h := newHarness(t)
	recorder := h.do(t, http.MethodGet, "/api/system/config", "", true, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("读取配置失败: %d %s", recorder.Code, recorder.Body.String())
	}

	view := configView{}
	if err := json.Unmarshal(decode(t, recorder).Data, &view); err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	if view.Config.RunMode != "demo" || view.Config.PageSize != 10 {
		t.Fatalf("配置内容异常: %+v", view.Config)
	}
	if !strings.Contains(view.Config.APIKey, "****") {
		t.Fatalf("API Key 应脱敏: %q", view.Config.APIKey)
	}
	if !strings.Contains(view.Config.SessionSecret, "****") {
		t.Fatalf("会话密钥应脱敏: %q", view.Config.SessionSecret)
	}
	if !strings.HasSuffix(view.File, "app.ini") {
		t.Fatalf("配置文件路径异常: %s", view.File)
	}
}

// TestSystemConfigSave 校验保存后落盘并可读取原文。
func TestSystemConfigSave(t *testing.T) {
	h := newHarness(t)

	saved := h.do(t, http.MethodPut, "/api/system/config", `{"page_size":25,"slow_sql_ms":350}`, true, true)
	if saved.Code != http.StatusOK {
		t.Fatalf("保存配置失败: %d %s", saved.Code, saved.Body.String())
	}

	view := configView{}
	if err := json.Unmarshal(decode(t, saved).Data, &view); err != nil {
		t.Fatalf("解析保存结果失败: %v", err)
	}
	if view.Config.PageSize != 25 || view.Config.SlowSQLMS != 350 {
		t.Fatalf("保存结果未更新: %+v", view.Config)
	}
	if len(view.RestartRequired) != 0 {
		t.Fatalf("仅改热更新项不应提示重启: %v", view.RestartRequired)
	}

	raw := h.do(t, http.MethodGet, "/api/system/config/raw", "", true, true)
	if raw.Code != http.StatusOK {
		t.Fatalf("读取原文失败: %d", raw.Code)
	}
	content := struct {
		Content string `json:"content"`
	}{}
	if err := json.Unmarshal(decode(t, raw).Data, &content); err != nil {
		t.Fatalf("解析原文失败: %v", err)
	}
	if !strings.Contains(content.Content, "PAGEMax") || !strings.Contains(content.Content, "SLOW_SQL_MS") {
		t.Fatalf("原文缺少关键配置项:\n%s", content.Content)
	}
}

// TestSystemConfigRestartHint 校验需要重启的字段被提示。
func TestSystemConfigRestartHint(t *testing.T) {
	h := newHarness(t)
	saved := h.do(t, http.MethodPut, "/api/system/config", `{"http_port":6000}`, true, true)
	if saved.Code != http.StatusOK {
		t.Fatalf("保存配置失败: %d %s", saved.Code, saved.Body.String())
	}

	view := configView{}
	if err := json.Unmarshal(decode(t, saved).Data, &view); err != nil {
		t.Fatalf("解析保存结果失败: %v", err)
	}
	if len(view.RestartRequired) == 0 {
		t.Fatal("修改端口应提示需要重启")
	}
	if !strings.Contains(strings.Join(view.RestartRequired, " "), "HTTP_PORT") {
		t.Fatalf("重启提示未包含端口: %v", view.RestartRequired)
	}
}

// TestSystemConfigValidation 校验非法入参返回 400。
func TestSystemConfigValidation(t *testing.T) {
	h := newHarness(t)
	recorder := h.do(t, http.MethodPut, "/api/system/config", `{"page_size":999}`, true, true)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("非法分页大小应返回 400，实际 %d", recorder.Code)
	}
}

// TestSystemConfigRequiresAuth 校验未登录无法读取配置。
func TestSystemConfigRequiresAuth(t *testing.T) {
	h := newHarness(t)
	recorder := h.do(t, http.MethodGet, "/api/system/config", "", false, true)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("未登录应返回 401，实际 %d", recorder.Code)
	}
}
