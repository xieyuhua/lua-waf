package service_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wafadmin/internal/service"
)

// TestCoerceLuaValues 校验前端混类型值被统一转成字符串。
// 背景：el-input-number 提交 number，后端若按 map[string]string 绑定会报「请求参数格式不正确」。
func TestCoerceLuaValues(t *testing.T) {
	raw := map[string]any{
		"config_expire_time":   float64(600),
		"config_post_max_size": float64(8388608),
		"config_auto_ban_off":  false,
		"config_auto_ban_on":   true,
		"config_rule_dir":      "/www/server/nginx/x-waf/rules",
		"config_ratio":         1.5,
		"config_skip":          nil,
	}

	values := service.CoerceLuaValues(raw)

	cases := map[string]string{
		"config_expire_time":   "600",
		"config_post_max_size": "8388608",
		"config_auto_ban_off":  "off",
		"config_auto_ban_on":   "on",
		"config_rule_dir":      "/www/server/nginx/x-waf/rules",
		"config_ratio":         "1.5",
	}
	for key, want := range cases {
		if values[key] != want {
			t.Fatalf("%s 期望 %q，实际 %q", key, want, values[key])
		}
	}
	if _, ok := values["config_skip"]; ok {
		t.Fatal("nil 值应被忽略")
	}
}

// TestTestConnectionsRedis 用本地监听端口验证 Redis 连通测试（PING 失败会返回不可用）。
func TestTestConnectionsRedis(t *testing.T) {
	// 起一个只接受连接、不回包的服务，模拟「端口通但不是 Redis」
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	_, port, _ := net.SplitHostPort(listener.Addr().String())

	dir := t.TempDir()
	path := filepath.Join(dir, "config.lua")
	if err := os.WriteFile(path, []byte(`
local _M = {
    config_redis_host = "127.0.0.1",
    config_redis_port = 6379,
    config_redis_password = "",
    config_redis_db = 0,
    config_mysql_host = "",
}
return _M
`), 0o644); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}

	svc := &service.LuaConfigService{Path: path}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 用表单值覆盖端口为测试端口（模拟「保存前先测」）
	result, err := svc.TestConnections(ctx, map[string]string{
		"config_redis_host": "127.0.0.1",
		"config_redis_port": port,
	})
	if err != nil {
		t.Fatalf("测试连接失败: %v", err)
	}
	if result.Redis == nil {
		t.Fatal("应返回 Redis 测试结果")
	}
	if result.Redis.Ok {
		t.Fatal("非 Redis 端口不应判定为成功")
	}
	if result.Redis.Message == "" {
		t.Fatal("应给出失败原因")
	}
	if result.MySQL != nil {
		t.Fatal("未配置 MySQL 地址时不应测试 MySQL")
	}
}

// TestTestConnectionsDisabled 校验未配置地址时两个目标都不测。
func TestTestConnectionsDisabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.lua")
	if err := os.WriteFile(path, []byte(`
local _M = {
    config_redis_host = "",
    config_mysql_host = "",
}
return _M
`), 0o644); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}

	svc := &service.LuaConfigService{Path: path}
	result, err := svc.TestConnections(context.Background(), map[string]string{})
	if err != nil {
		t.Fatalf("测试连接失败: %v", err)
	}
	if result.Redis != nil || result.MySQL != nil {
		t.Fatalf("未配置地址时不应测试: %+v", result)
	}
}
