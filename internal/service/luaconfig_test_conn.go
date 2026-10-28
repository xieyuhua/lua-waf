package service

// 本文件负责「WAF 配置」里的**连接测试**：在保存前先验证 Redis / MySQL 是否真的连得上。
//
// 为什么需要：日志入库模式（redis / mysql）如果地址、口令填错，保存后不会立刻报错，
// 而是要等到下一次 nginx 写日志才会暴露——那时线上已经在丢日志了。
// 因此提供「保存前先测一次」的能力，测试用的是**表单当前值**（可以测还没保存的配置）。

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"wafadmin/pkg/redis"
)

// LuaConnTest 是单个依赖的连接测试结果。
type LuaConnTest struct {
	// Target 为测试目标：redis / mysql。
	Target string `json:"target"`
	// Address 为实际测试的地址（脱敏后的口令不会出现在这里）。
	Address string `json:"address"`
	// Ok 表示连通且握手成功。
	Ok bool `json:"ok"`
	// LatencyMS 为往返耗时（毫秒）。
	LatencyMS int64 `json:"latency_ms"`
	// Message 为可读结果说明。
	Message string `json:"message"`
}

// LuaConnTestResult 是一次「测试连接」的完整结果。
type LuaConnTestResult struct {
	// Redis 为 Redis 队列连通性（值为配置了 host 才测试）。
	Redis *LuaConnTest `json:"redis,omitempty"`
	// MySQL 为 MySQL 落库连通性（配置了 host 才测试）。
	MySQL *LuaConnTest `json:"mysql,omitempty"`
}

// TestConnections 用给定值（通常是前端表单当前值）测试 Redis 与 MySQL 的连通性。
//
// 取值优先级：请求里的值 → config.lua 里已保存的值。
// 掩码口令（含 `****`）会被替换成已保存的真实值，这样「口令不改只改地址」也能测通。
func (s *LuaConfigService) TestConnections(ctx context.Context, values map[string]string) (*LuaConnTestResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 已保存的当前值作为兜底
	saved := map[string]string{}
	if content, err := s.Raw(ctx); err == nil {
		parsed, _ := parseLuaValues(content)
		saved = parsed
	}

	pick := func(key string) string {
		value := strings.TrimSpace(values[key])
		if value != "" && !strings.Contains(value, "****") {
			return value
		}
		return strings.TrimSpace(saved[key])
	}

	result := &LuaConnTestResult{}

	// Redis：有地址才测
	if host := pick("config_redis_host"); host != "" {
		result.Redis = s.testRedis(host, pick("config_redis_port"), pick("config_redis_password"), pick("config_redis_db"))
	}

	// MySQL：有地址才测
	if host := pick("config_mysql_host"); host != "" {
		result.MySQL = s.testMySQL(ctx, host, pick("config_mysql_port"), pick("config_mysql_user"),
			pick("config_mysql_password"), pick("config_mysql_database"))
	}

	return result, nil
}

// testRedis 连接 Redis 并执行 PING。
func (s *LuaConfigService) testRedis(host, port, password, db string) *LuaConnTest {
	if port == "" {
		port = "6379"
	}
	address := fmt.Sprintf("%s:%s", host, port)
	started := time.Now()

	test := &LuaConnTest{Target: "redis", Address: address}
	client, err := redis.Dial(redis.Options{
		Addr:     address,
		Password: password,
		DB:       atoiDefault(db, 0),
		Timeout:  3 * time.Second,
	})
	if err != nil {
		test.LatencyMS = time.Since(started).Milliseconds()
		test.Message = "连接失败：" + describeConnError(err, address)
		return test
	}
	defer client.Close()

	if err := client.Ping(); err != nil {
		test.LatencyMS = time.Since(started).Milliseconds()
		test.Message = "PING 失败（口令或权限可能不对）：" + describeConnError(err, address)
		return test
	}

	test.Ok = true
	test.LatencyMS = time.Since(started).Milliseconds()
	test.Message = fmt.Sprintf("PING 成功，耗时 %dms（库 %s）", test.LatencyMS, defaultString(db, "0"))
	return test
}

// testMySQL 连接 MySQL 并执行 Ping。
func (s *LuaConfigService) testMySQL(ctx context.Context, host, port, user, password, database string) *LuaConnTest {
	if port == "" {
		port = "3306"
	}
	address := fmt.Sprintf("%s:%s", host, port)
	test := &LuaConnTest{Target: "mysql", Address: fmt.Sprintf("%s/%s", address, defaultString(database, "?"))}

	if database == "" {
		test.Message = "缺少库名，无法测试"
		return test
	}
	if user == "" {
		user = "root"
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8&timeout=3s&readTimeout=3s&writeTimeout=3s",
		user, password, address, database)

	started := time.Now()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		test.LatencyMS = time.Since(started).Milliseconds()
		test.Message = "连接串非法：" + err.Error()
		return test
	}
	defer db.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		test.LatencyMS = time.Since(started).Milliseconds()
		test.Message = "连接失败：" + describeConnError(err, address)
		return test
	}

	test.Ok = true
	test.LatencyMS = time.Since(started).Milliseconds()
	test.Message = fmt.Sprintf("连接成功，耗时 %dms（库 %s）", test.LatencyMS, database)
	return test
}

// describeConnError 把常见网络错误翻译成可操作提示。
func describeConnError(err error, address string) string {
	message := err.Error()
	switch {
	case strings.Contains(message, "connection refused"), strings.Contains(message, "actively refused"):
		return fmt.Sprintf("%s 拒绝连接，检查服务是否启动/端口是否正确", address)
	case strings.Contains(message, "i/o timeout") || strings.Contains(message, "timeout"):
		return fmt.Sprintf("%s 连接超时，检查网络/防火墙/地址是否正确", address)
	case strings.Contains(message, "no such host"):
		return fmt.Sprintf("域名解析失败：%s", address)
	case strings.Contains(message, "Access denied"):
		return "账号或口令不正确（Access denied）"
	case strings.Contains(message, "Unknown database"):
		return "库不存在（Unknown database）"
	case strings.Contains(message, "WRONGPASS") || strings.Contains(message, "invalid password"):
		return "Redis 口令不正确"
	case strings.Contains(message, "NOAUTH"):
		return "Redis 需要口令但未配置"
	default:
		return message
	}
}

// atoiDefault 解析整数，失败返回默认值。
func atoiDefault(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return value
}

// defaultString 为空时返回兜底值。
func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// CoerceLuaValues 把前端传来的任意类型值（数字/布尔/字符串）统一转成字符串。
//
// 前端 el-input-number 提交的是数字、el-switch 提交的是字符串 "on"/"off"，
// 而 config.lua 的值最终都是字符串，这里统一收口，避免“数字导致参数格式不正确”这类问题。
func CoerceLuaValues(raw map[string]any) map[string]string {
	values := make(map[string]string, len(raw))
	for key, item := range raw {
		switch typed := item.(type) {
		case nil:
			continue
		case string:
			values[key] = typed
		case bool:
			if typed {
				values[key] = "on"
			} else {
				values[key] = "off"
			}
		case float64:
			// JSON 数字统一按十进制输出，整数去掉小数点
			if typed == float64(int64(typed)) {
				values[key] = strconv.FormatInt(int64(typed), 10)
			} else {
				values[key] = strconv.FormatFloat(typed, 'f', -1, 64)
			}
		default:
			values[key] = fmt.Sprintf("%v", typed)
		}
	}
	return values
}
