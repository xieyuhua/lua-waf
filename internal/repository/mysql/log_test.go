package mysql

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"wafadmin/internal/model"
	"wafadmin/pkg/sqlwatch"
)

// 测试数据标记，便于清理
const testLogIpPrefix = "10.99.99."

// TestLogRepoListFilter 校验按条件查询日志时，列表与总数都应用过滤条件。
// 回归点：xorm 的 Count 执行后会重置 session 的 WHERE，若统计与列表复用同一 session，
// 列表会退化为全量返回（表现为「筛选没生效」）。
func TestLogRepoListFilter(t *testing.T) {
	engine := newTestEngine(t)
	defer engine.Close()

	// 只检查表是否存在：绝不对真实表执行 Sync2（会 ALTER 线上表结构）
	if exists, err := engine.IsTableExist(new(model.WafLog)); err != nil {
		t.Fatalf("检查 waf_log 表失败: %v", err)
	} else if !exists {
		if err := engine.Sync2(new(model.WafLog)); err != nil {
			t.Fatalf("创建 waf_log 表失败: %v", err)
		}
	}

	db, err := sql.Open("mysql", dsnFromEnv())
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer db.Close()

	// 清理历史测试数据
	if _, err := db.Exec("DELETE FROM `waf_log` WHERE client_ip LIKE ?", testLogIpPrefix+"%"); err != nil {
		t.Fatalf("清理测试数据失败: %v", err)
	}
	defer func() {
		_, _ = db.Exec("DELETE FROM `waf_log` WHERE client_ip LIKE ?", testLogIpPrefix+"%")
	}()

	now := time.Now().Format("2006-01-02 15:04:05")
	rows := []struct {
		ip    string
		atype string
		url   string
	}{
		{testLogIpPrefix + "1", "SQL注入", "/a?id=1"},
		{testLogIpPrefix + "1", "SQL注入", "/a?id=2"},
		{testLogIpPrefix + "2", "XSS跨站", "/b?x=1"},
		{testLogIpPrefix + "2", "CC攻击", "/c"},
		{testLogIpPrefix + "3", "SQL注入", "/d"},
	}
	for _, row := range rows {
		if _, err := db.Exec(
			"INSERT INTO `waf_log` (client_ip, local_time, server_name, user_agent, attack_type, req_url, area, req_data, rule_tag) VALUES (?,?,?,?,?,?,?,?,?)",
			row.ip, now, "test.example.com", "go-test", row.atype, row.url, "测试", "", "test",
		); err != nil {
			t.Fatalf("写入测试数据失败: %v", err)
		}
	}

	repo := &logRepo{engine: engine, observer: &sqlwatch.Observer{Database: "test", Threshold: time.Minute}}
	ctx := context.Background()

	// 1) 按攻击类型过滤：3 条 SQL注入
	items, total, err := repo.List(ctx, model.LogQuery{
		ListQuery:  model.ListQuery{Page: 1, Size: 50},
		AttackType: "SQL注入",
	})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 3 {
		t.Fatalf("SQL注入 总数应为 3，实际 %d", total)
	}
	if len(items) != 3 {
		t.Fatalf("SQL注入 列表条数应为 3，实际 %d（筛选未生效会返回全量）", len(items))
	}
	for _, item := range items {
		if item.AttackType != "SQL注入" {
			t.Fatalf("列表包含非目标类型: %s", item.AttackType)
		}
	}

	// 2) 按来源 IP 过滤：2 条
	items, total, err = repo.List(ctx, model.LogQuery{
		ListQuery: model.ListQuery{Page: 1, Size: 50},
		ClientIp:  testLogIpPrefix + "1",
	})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("IP 过滤应为 2 条，实际 total=%d items=%d", total, len(items))
	}

	// 3) 关键词过滤：命中 URL 中的 /b
	items, total, err = repo.List(ctx, model.LogQuery{
		ListQuery: model.ListQuery{Page: 1, Size: 50, Keyword: "/b?"},
	})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("关键词过滤应为 1 条，实际 total=%d items=%d", total, len(items))
	}

	// 4) 分页：size=2 时首页 2 条、总数仍为 3
	items, total, err = repo.List(ctx, model.LogQuery{
		ListQuery:  model.ListQuery{Page: 1, Size: 2},
		AttackType: "SQL注入",
	})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 3 || len(items) != 2 {
		t.Fatalf("分页结果异常：total=%d items=%d", total, len(items))
	}
}

// TestSiteUserRepoListFilter 校验站点/账号列表的关键词过滤同样生效。
func TestSiteUserRepoListFilter(t *testing.T) {
	engine := newTestEngine(t)
	defer engine.Close()

	if exists, err := engine.IsTableExist(new(model.Site)); err != nil {
		t.Fatalf("检查 site 表失败: %v", err)
	} else if !exists {
		if err := engine.Sync2(new(model.Site)); err != nil {
			t.Fatalf("创建 site 表失败: %v", err)
		}
	}

	db, err := sql.Open("mysql", dsnFromEnv())
	if err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	defer db.Close()

	// 使用唯一前缀，避免污染真实数据
	suffix := time.Now().Format("150405")
	names := []string{"zz-test-a-" + suffix, "zz-test-b-" + suffix, "zz-test-b-" + suffix + "-2"}
	for _, name := range names {
		if _, err := db.Exec(
			"INSERT INTO `site` (`site_name`, `port`, `backend_addr`, `ssl`, `debug_level`, `last_change`, `version`, `unreal_addr`) VALUES (?,?,?,?,?,?,?,?)",
			name, 80, "[]", "off", "error", time.Now().Format("2006-01-02 15:04:05"), 1, "[]",
		); err != nil {
			t.Fatalf("写入站点失败: %v", err)
		}
	}
	defer func() {
		_, _ = db.Exec("DELETE FROM `site` WHERE site_name LIKE ?", "zz-test-%")
	}()

	repo := &siteRepo{engine: engine, observer: &sqlwatch.Observer{Database: "test", Threshold: time.Minute}}
	items, total, err := repo.List(context.Background(), model.ListQuery{Page: 1, Size: 20, Keyword: "zz-test-b-" + suffix})
	if err != nil {
		t.Fatalf("查询站点失败: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("站点关键词过滤应为 2 条，实际 total=%d items=%d", total, len(items))
	}
	for _, item := range items {
		if item.SiteName == names[0] {
			t.Fatal("列表包含不匹配的站点")
		}
	}
}
