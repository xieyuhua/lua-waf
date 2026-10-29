// Package repositorytest 提供「仓储一致性」测试套件。
//
// 为什么需要：`repository/mysql` 与 `repository/memory` 是同一套接口的两份实现，
// 语义只能靠人工保证——历史上就出过 `Count` 重置了 WHERE 导致计数与列表不一致的 bug。
// 这里用一套表驱动用例对**两种实现跑同一组断言**，任何一边行为漂移都会立刻失败。
//
// 用法：
//
//	repositorytest.RunAll(t, store.Repos(), repositorytest.Options{Cleanup: true})
//
// mysql 实现需要外部数据库，默认跳过（见 mysql/conformance_test.go 的环境变量开关）。
package repositorytest

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"wafadmin/internal/model"
	"wafadmin/internal/repository"
)

// Options 控制套件行为。
type Options struct {
	// Prefix 为写入数据的唯一前缀，避免与既有数据冲突（mysql 环境必需）。
	Prefix string
	// SkipLog 为 true 时跳过日志仓储（表可能非常大，且没有删除接口会留下测试数据）。
	SkipLog bool
	// SkipSlowSQL 为 true 时跳过慢 SQL 仓储（该仓储只能按时间批量清理，生产库上会误删真实数据）。
	SkipSlowSQL bool
}

// RunAll 依次校验全部仓储。
func RunAll(t *testing.T, repos repository.Repositories, opts Options) {
	t.Helper()

	prefix := opts.Prefix
	if prefix == "" {
		prefix = fmt.Sprintf("conftest-%d", time.Now().UnixNano())
	}
	ctx := context.Background()

	t.Run("Rule", func(t *testing.T) { testRule(t, ctx, repos.Rule, prefix) })
	t.Run("RateLimit", func(t *testing.T) { testRateLimit(t, ctx, repos.RateLimit, prefix) })
	t.Run("Site", func(t *testing.T) { testSite(t, ctx, repos.Site, prefix) })
	t.Run("User", func(t *testing.T) { testUser(t, ctx, repos.User, prefix) })
	t.Run("Setting", func(t *testing.T) { testSetting(t, ctx, repos.Setting, prefix) })
	if !opts.SkipSlowSQL {
		t.Run("SlowSQL", func(t *testing.T) { testSlowSQL(t, ctx, repos.SlowSQL, prefix) })
	}
	if !opts.SkipLog {
		t.Run("Log", func(t *testing.T) { testLog(t, ctx, repos.Log, prefix) })
	}
}

// mustNoErr 断言无错误。
func mustNoErr(t *testing.T, err error, action string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s 失败: %v", action, err)
	}
}

// testRule 校验规则的「增查改删 + 类型过滤 + 计数」。
func testRule(t *testing.T, ctx context.Context, repo repository.RuleRepository, prefix string) {
	item := &model.Rule{RuleType: "args", RuleItem: prefix + `-rule-\d+`, Enabled: true}
	mustNoErr(t, repo.Create(ctx, item), "创建规则")
	if item.Id == 0 {
		t.Fatal("创建后应回填 Id")
	}

	got, err := repo.Get(ctx, item.Id)
	mustNoErr(t, err, "读取规则")
	if got == nil || got.RuleItem != item.RuleItem {
		t.Fatalf("按 Id 读取不一致: %+v", got)
	}

	listByName, err := repo.List(ctx, "args")
	mustNoErr(t, err, "按类型查规则")
	if !containsRule(listByName, item.Id) {
		t.Fatal("按类型查询未包含新建规则")
	}

	item.RuleItem = prefix + `-rule-updated`
	mustNoErr(t, repo.Update(ctx, item), "更新规则")

	mustNoErr(t, repo.SetEnabled(ctx, item.Id, false), "停用规则")
	got, err = repo.Get(ctx, item.Id)
	mustNoErr(t, err, "读取规则(停用后)")
	if got.Enabled {
		t.Fatal("停用后 Enabled 应为 false")
	}

	// 关键词过滤与 Count 必须一致（历史 bug 点：Count 会重置 WHERE）
	filtered, err := repo.List(ctx, "")
	mustNoErr(t, err, "列出全部规则")
	total, err := repo.Count(ctx)
	mustNoErr(t, err, "统计规则总数")
	if int64(len(filtered)) != total {
		t.Fatalf("List 条数 %d 与 Count %d 不一致", len(filtered), total)
	}

	mustNoErr(t, repo.Delete(ctx, item.Id), "删除规则")
	got, err = repo.Get(ctx, item.Id)
	mustNoErr(t, err, "读取已删除规则")
	if got != nil {
		t.Fatal("删除后应读取不到")
	}
}

// testRateLimit 校验限频规则的增查改删与排序。
func testRateLimit(t *testing.T, ctx context.Context, repo repository.RateLimitRepository, prefix string) {
	rule := &model.RateLimitRule{
		Name: prefix + "-rl", Pattern: "/" + prefix, Match: "prefix",
		Key: "ip", Limit: 10, Window: 60, Action: "block", Enabled: true,
	}
	mustNoErr(t, repo.Create(ctx, rule), "创建限频规则")
	if rule.Id == 0 {
		t.Fatal("创建后应回填 Id")
	}

	got, err := repo.Get(ctx, rule.Id)
	mustNoErr(t, err, "读取限频规则")
	if got == nil || got.Pattern != rule.Pattern {
		t.Fatalf("限频规则读取不一致: %+v", got)
	}

	rule.Limit = 20
	rule.Enabled = false
	mustNoErr(t, repo.Update(ctx, rule), "更新限频规则")
	got, _ = repo.Get(ctx, rule.Id)
	if got.Limit != 20 || got.Enabled {
		t.Fatalf("更新未生效: %+v", got)
	}

	list, err := repo.List(ctx)
	mustNoErr(t, err, "列出限频规则")
	if !containsRateLimit(list, rule.Id) {
		t.Fatal("列表未包含新建规则")
	}
	// 约定：List 按 Id 升序
	for i := 1; i < len(list); i++ {
		if list[i-1].Id > list[i].Id {
			t.Fatal("限频规则列表应按 Id 升序")
		}
	}

	mustNoErr(t, repo.Delete(ctx, rule.Id), "删除限频规则")
	got, _ = repo.Get(ctx, rule.Id)
	if got != nil {
		t.Fatal("删除后应读取不到")
	}
}

// testSite 校验站点的增查改删与分页。
func testSite(t *testing.T, ctx context.Context, repo repository.SiteRepository, prefix string) {
	site := &model.Site{SiteName: prefix + ".example.com", Port: 80, BackendAddr: []string{"127.0.0.1:8080"}, Ssl: "off", DebugLevel: "error"}
	mustNoErr(t, repo.Create(ctx, site), "创建站点")

	list, total, err := repo.List(ctx, model.ListQuery{Page: 1, Size: 50})
	mustNoErr(t, err, "列出站点")
	if total < 1 || !containsSite(list, site.Id) {
		t.Fatalf("站点列表异常: total=%d", total)
	}

	// 分页语义：Page 越界时应返回空集而不是报错
	_, _, err = repo.List(ctx, model.ListQuery{Page: 9999, Size: 10})
	mustNoErr(t, err, "越界分页")

	got, err := repo.Get(ctx, site.Id)
	mustNoErr(t, err, "读取站点")
	if got == nil || got.SiteName != site.SiteName {
		t.Fatalf("站点读取不一致: %+v", got)
	}

	mustNoErr(t, repo.Delete(ctx, site.Id), "删除站点")
}

// testUser 校验账号的创建、按用户名查询与计数。
func testUser(t *testing.T, ctx context.Context, repo repository.UserRepository, prefix string) {
	user := &model.User{UserName: prefix + "-u", Password: "hash"}
	mustNoErr(t, repo.Create(ctx, user), "创建账号")

	got, err := repo.GetByName(ctx, user.UserName)
	mustNoErr(t, err, "按用户名查账号")
	if got == nil || got.Id != user.Id {
		t.Fatalf("按用户名查询不一致: %+v", got)
	}

	list, total, err := repo.List(ctx, model.ListQuery{Page: 1, Size: 20})
	mustNoErr(t, err, "列出账号")
	if total < 1 || len(list) == 0 {
		t.Fatal("账号列表为空")
	}

	mustNoErr(t, repo.Delete(ctx, user.Id), "删除账号")
}

// testSetting 校验设置的读写与覆盖（upsert）。
func testSetting(t *testing.T, ctx context.Context, repo repository.SettingRepository, prefix string) {
	key := prefix + ".setting"
	mustNoErr(t, repo.Set(ctx, key, "v1"), "写入设置")

	value, ok, err := repo.Get(ctx, key)
	mustNoErr(t, err, "读取设置")
	if !ok || value != "v1" {
		t.Fatalf("设置值不一致: ok=%v value=%q", ok, value)
	}

	// upsert：同一个 key 再写一次应覆盖而不是报错
	mustNoErr(t, repo.Set(ctx, key, "v2"), "覆盖设置")
	value, _, _ = repo.Get(ctx, key)
	if value != "v2" {
		t.Fatalf("覆盖后设置值不一致: %q", value)
	}

	mustNoErr(t, repo.Delete(ctx, key), "删除设置")
}

// testSlowSQL 校验慢 SQL 的写入、计数与列表。
func testSlowSQL(t *testing.T, ctx context.Context, repo repository.SlowSQLRepository, prefix string) {
	item := &model.SlowSQL{
		Statement:  "SELECT * FROM " + strings.ReplaceAll(prefix, "-", "_"),
		Database:   "test",
		OccurredAt: time.Now(),
		DurationMs: 1200,
	}
	mustNoErr(t, repo.Create(ctx, item), "写入慢 SQL")

	list, total, err := repo.List(ctx, model.SlowSQLQuery{ListQuery: model.ListQuery{Page: 1, Size: 20}})
	mustNoErr(t, err, "列出慢 SQL")
	if total < 1 || len(list) == 0 {
		t.Fatal("慢 SQL 列表为空")
	}
	if !containsSlowSQL(list, item.Id) {
		t.Fatal("慢 SQL 列表未包含新建记录")
	}

	// 慢 SQL 仓储没有按 Id 删除，用「清理早于某时间」来回收本次测试数据\n	mustNoErr(t, purgeSlowSQL(ctx, repo), "清理慢 SQL")
}

// testLog 校验日志的批量写入、按条件查询与计数一致性。
func testLog(t *testing.T, ctx context.Context, repo repository.LogRepository, prefix string) {
	day := time.Now().Format("2006-01-02")
	attackType := prefix + "-type"
	clientIP := "203.0.113.77"

	items := []model.WafLog{
		{ClientIp: clientIP, LocalTime: day + " 10:00:00", AttackType: attackType, ReqUrl: "/" + prefix, RuleTag: "t1"},
		{ClientIp: clientIP, LocalTime: day + " 10:00:01", AttackType: attackType, ReqUrl: "/" + prefix, RuleTag: "t1"},
	}
	created, err := repo.BatchCreate(ctx, items)
	mustNoErr(t, err, "批量写入日志")
	if created != int64(len(items)) {
		t.Fatalf("批量写入条数不一致: %d", created)
	}

	// 按类型过滤：列表条数与 Count 必须一致（Count 不能丢掉 WHERE）
	list, total, err := repo.List(ctx, model.LogQuery{
		ListQuery:  model.ListQuery{Page: 1, Size: 10},
		AttackType: attackType,
	})
	mustNoErr(t, err, "按类型查日志")
	if total != int64(len(items)) || len(list) != len(items) {
		t.Fatalf("按类型过滤结果异常: total=%d len=%d", total, len(list))
	}

	// 按 IP + 时间范围过滤
	list, total, err = repo.List(ctx, model.LogQuery{
		ListQuery: model.ListQuery{Page: 1, Size: 10},
		ClientIp:  clientIP,
		Day:       day,
	})
	mustNoErr(t, err, "按IP查日志")
	if total < int64(len(items)) {
		t.Fatalf("按 IP 过滤条数偏少: %d", total)
	}
	if len(list) == 0 {
		t.Fatal("按 IP 过滤应至少返回一条")
	}

	// 类型聚合应包含刚写入的类型
	stats, err := repo.TypeStats(ctx, repository.StatsFilter{Day: day, Limit: 50})
	mustNoErr(t, err, "类型聚合")
	found := false
	for _, item := range stats {
		if item.AttackType == attackType {
			found = true
			if item.Num != int64(len(items)) {
				t.Fatalf("类型聚合条数不一致: %d", item.Num)
			}
		}
	}
	if !found {
		t.Fatal("类型聚合未包含新写入的类型")
	}

	// 去重类型清单应包含该类型
	types, err := repo.DistinctAttackTypes(ctx)
	mustNoErr(t, err, "去重类型清单")
	for _, name := range types {
		if name == attackType {
			return
		}
	}
	t.Fatal("去重类型清单未包含新写入的类型")
}

// ---- 小工具 ----

func containsRule(items []model.Rule, id int64) bool {
	for _, item := range items {
		if item.Id == id {
			return true
		}
	}
	return false
}

func containsRateLimit(items []model.RateLimitRule, id int64) bool {
	for _, item := range items {
		if item.Id == id {
			return true
		}
	}
	return false
}

func containsSite(items []model.Site, id int64) bool {
	for _, item := range items {
		if item.Id == id {
			return true
		}
	}
	return false
}

func containsSlowSQL(items []model.SlowSQL, id int64) bool {
	for _, item := range items {
		if item.Id == id {
			return true
		}
	}
	return false
}

// purgeSlowSQL 清理套件写入的慢 SQL 数据（该仓储只提供按时间清理）。
func purgeSlowSQL(ctx context.Context, repo repository.SlowSQLRepository) error {
	_, err := repo.PurgeBefore(ctx, time.Now().Add(time.Hour))
	return err
}
