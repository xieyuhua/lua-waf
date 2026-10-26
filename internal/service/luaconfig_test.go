package service_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wafadmin/internal/service"
)

// luaFixture 生成一份临时的 config.lua。
func luaFixture(t *testing.T) (*service.LuaConfigService, string) {
	t.Helper()

	content := `-- 测试用配置
config_waf_enable = "on",
config_waf_model = "html",
config_cc_rate = "300/60",
config_ipcc_check = 'on',
config_post_max_size = 8388608,
config_auto_ban_check = "off",
config_mysql_password = "secret-pass",
config_output_html = [[
<html>%s</html>
]],
config_custom_unknown = "keep-me",
`
	path := filepath.Join(t.TempDir(), "config.lua")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}
	return &service.LuaConfigService{Path: path}, path
}

// TestLuaConfigView 校验解析结果、分组与脱敏。
func TestLuaConfigView(t *testing.T) {
	svc, _ := luaFixture(t)
	view, err := svc.View(context.Background())
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if !view.Exists || !view.Writable {
		t.Fatalf("文件应存在且可写: exists=%v writable=%v", view.Exists, view.Writable)
	}
	if len(view.Groups) == 0 {
		t.Fatal("配置分组为空")
	}

	values := map[string]service.LuaConfigItem{}
	for _, group := range view.Groups {
		for _, item := range group.Items {
			values[item.Key] = item
		}
	}

	if got := values["config_waf_enable"].Value; got != "on" {
		t.Fatalf("config_waf_enable = %q", got)
	}
	if got := values["config_cc_rate"].Value; got != "300/60" {
		t.Fatalf("config_cc_rate = %q", got)
	}
	// 单引号也要正确解析
	if got := values["config_ipcc_check"].Value; got != "on" {
		t.Fatalf("config_ipcc_check = %q", got)
	}
	// 密钥类需要脱敏
	secret := values["config_mysql_password"]
	if !secret.Masked || strings.Contains(secret.Value, "secret-pass") {
		t.Fatalf("口令应脱敏: %+v", secret)
	}
	// 未纳入表单的键进入 unknown
	found := false
	for _, key := range view.Unknown {
		if key == "config_custom_unknown" {
			found = true
		}
	}
	if !found {
		t.Fatalf("未识别键应进入 unknown: %v", view.Unknown)
	}
}

// TestLuaConfigUpdate 校验写回时保留注释、格式与未涉及的行。
func TestLuaConfigUpdate(t *testing.T) {
	svc, path := luaFixture(t)

	view, err := svc.Update(context.Background(), map[string]string{
		"config_waf_enable":    "off",
		"config_cc_rate":       "60/30",
		"config_post_max_size": "1048576",
		"config_ipcc_check":    "off",
	})
	if err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	if view.Groups == nil {
		t.Fatal("保存后应返回最新视图")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取文件失败: %v", err)
	}
	text := string(content)

	for _, want := range []string{
		`config_waf_enable = "off",`,
		`config_cc_rate = "60/30",`,
		`config_post_max_size = 1048576,`, // 数字保持裸值
		`config_ipcc_check = 'off',`,      // 保留单引号
		`-- 测试用配置`,                        // 注释保留
		`config_output_html = [[`,         // 多行字符串不受影响
		`config_custom_unknown = "keep-me"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("文件内容缺少 %q:\n%s", want, text)
		}
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatalf("应生成 .bak 备份: %v", err)
	}
}

// TestLuaConfigValidation 校验非法取值被拒绝且不写文件。
func TestLuaConfigValidation(t *testing.T) {
	svc, path := luaFixture(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}

	cases := []struct {
		name   string
		values map[string]string
	}{
		{name: "开关取值非法", values: map[string]string{"config_waf_enable": "yes"}},
		{name: "数字非法", values: map[string]string{"config_post_max_size": "abc"}},
		{name: "数字超范围", values: map[string]string{"config_mysql_port": "70000"}},
		{name: "下拉取值非法", values: map[string]string{"config_waf_model": "block"}},
		{name: "限速格式非法", values: map[string]string{"config_cc_rate": "300-60"}},
		{name: "未知配置项", values: map[string]string{"config_not_exist": "1"}},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if _, err := svc.Update(context.Background(), item.values); err == nil {
				t.Fatal("应返回校验错误")
			}
		})
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("校验失败时不应修改文件")
	}
}

// TestLuaConfigMissingFile 校验文件缺失时的行为。
func TestLuaConfigMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-exist.lua")
	svc := &service.LuaConfigService{Path: path}

	view, err := svc.View(context.Background())
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if view.Exists {
		t.Fatal("文件不存在时 Exists 应为 false")
	}
	if _, err := svc.Update(context.Background(), map[string]string{"config_waf_enable": "off"}); err == nil {
		t.Fatal("文件不存在时保存应报错")
	}
}
