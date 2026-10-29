package mysql_test

import (
	"os"
	"testing"

	"wafadmin/internal/config"
	"wafadmin/internal/repository/mysql"
	"wafadmin/internal/repository/repositorytest"
)

// TestConformance 用统一套件校验 MySQL 仓储，确保与内存实现语义一致。
//
// 需要外部数据库，默认跳过；启用方式：
//
//	$env:WAF_TEST_MYSQL_ADDR="192.168.2.6:3306"
//	$env:WAF_TEST_MYSQL_USER="waf"; $env:WAF_TEST_MYSQL_PASS="waf"; $env:WAF_TEST_MYSQL_DB="waf"
//	go test ./internal/repository/mysql/ -run TestConformance -v
func TestConformance(t *testing.T) {
	addr := os.Getenv("WAF_TEST_MYSQL_ADDR")
	if addr == "" {
		t.Skip("未设置 WAF_TEST_MYSQL_ADDR，跳过 MySQL 一致性校验")
	}

	host, port := splitAddr(addr)
	store := &mysql.Store{}
	cfg := config.Database{
		Host:              host,
		Port:              port,
		User:              envOr("WAF_TEST_MYSQL_USER", "waf"),
		Password:          envOr("WAF_TEST_MYSQL_PASS", "waf"),
		Name:              envOr("WAF_TEST_MYSQL_DB", "waf"),
		AutoMigrate:       true,
		AddMissingColumns: true,
	}
	if err := store.Open(cfg); err != nil {
		t.Fatalf("连接数据库失败: %v", err)
	}
	defer store.Close()

	repositorytest.RunAll(t, store.Repos(), repositorytest.Options{
		Prefix:      "mysql-conf",
		SkipLog:     true, // 日志仓储没有删除接口，避免往大表写测试数据
		SkipSlowSQL: true, // 慢 SQL 仓储只能按时间批量清理，生产库上会误删真实数据
	})
}

// envOr 读取环境变量，为空时返回默认值。
func envOr(key, def string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return def
}

// splitAddr 拆分 host:port。
func splitAddr(addr string) (string, int) {
	host := addr
	port := 3306
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			host = addr[:i]
			if p := atoi(addr[i+1:]); p > 0 {
				port = p
			}
			break
		}
	}
	return host, port
}

// atoi 简易整数解析（避免为一行引入 strconv 之外的依赖）。
func atoi(raw string) int {
	value := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return 0
		}
		value = value*10 + int(raw[i]-'0')
	}
	return value
}
