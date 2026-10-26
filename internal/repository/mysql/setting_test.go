package mysql

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/go-xorm/xorm"

	"wafadmin/internal/model"
	"wafadmin/pkg/sqlwatch"
)

// newTestEngine 连接测试库；未设置 WAF_TEST_MYSQL_DSN 时跳过。
// 例：WAF_TEST_MYSQL_DSN='waf:waf@tcp(127.0.0.1:3306)/waf?charset=utf8&parseTime=true&loc=Local'
func newTestEngine(t *testing.T) *xorm.Engine {
	t.Helper()

	dsn := os.Getenv("WAF_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("未设置 WAF_TEST_MYSQL_DSN，跳过 MySQL 集成测试")
	}

	engine, err := xorm.NewEngine("mysql", dsn)
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	engine.SetMaxOpenConns(4)
	if err := engine.Ping(); err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	return engine
}

// TestSettingRepoOverwrite 校验同一键可以反复写入（先更新后插入），
// 这正是「AI 配置保存一次后无法再修改」的回归点。
func TestSettingRepoOverwrite(t *testing.T) {
	engine := newTestEngine(t)
	defer engine.Close()

	if err := engine.Sync2(new(model.Setting)); err != nil {
		t.Fatalf("准备 setting 表失败: %v", err)
	}

	repo := &settingRepo{
		engine:   engine,
		observer: &sqlwatch.Observer{Database: "test", Threshold: time.Minute},
	}
	ctx := context.Background()
	key := "test.setting.upsert"
	defer func() { _ = repo.Delete(ctx, key) }()

	for _, value := range []string{"v1", "v2", "v3"} {
		if err := repo.Set(ctx, key, value); err != nil {
			t.Fatalf("写入 %s 失败: %v", value, err)
		}
		got, ok, err := repo.Get(ctx, key)
		if err != nil {
			t.Fatalf("读取失败: %v", err)
		}
		if !ok || got != value {
			t.Fatalf("写入 %s 后读取到 %q（ok=%v）", value, got, ok)
		}
	}

	// 覆盖后 All 中也是最新值
	all, err := repo.All(ctx)
	if err != nil {
		t.Fatalf("读取全部配置失败: %v", err)
	}
	if all[key] != "v3" {
		t.Fatalf("全部配置中 %s = %q", key, all[key])
	}
}
