package service_test

import (
	"context"
	"testing"
	"time"

	"wafadmin/internal/model"
	"wafadmin/internal/repository/memory"
	"wafadmin/internal/service"
)

// newSlowSQLService 构造基于内存仓储的慢 SQL 服务。
func newSlowSQLService() *service.SlowSQLService {
	store := (&memory.Store{}).Init(true)
	return &service.SlowSQLService{
		Repo:         store.SlowSQL,
		Threshold:    200 * time.Millisecond,
		MaxStatement: 4000,
		PageSize:     10,
	}
}

// TestSlowSQLTimeRangeQuery 校验时间范围过滤。
func TestSlowSQLTimeRangeQuery(t *testing.T) {
	svc := newSlowSQLService()
	ctx := context.Background()

	now := time.Now()
	payload := service.SlowSQLQueryPayload{
		Page: 1,
		Size: 200,
		From: now.Add(-24 * time.Hour).Format("2006-01-02 15:04:05"),
		To:   now.Add(time.Minute).Format("2006-01-02 15:04:05"),
	}

	result, err := svc.List(ctx, payload)
	if err != nil {
		t.Fatalf("查询慢 SQL 失败: %v", err)
	}
	items, ok := result.Items.([]model.SlowSQL)
	if !ok {
		t.Fatalf("返回类型异常: %T", result.Items)
	}
	if len(items) == 0 {
		t.Fatal("最近 24 小时应有慢 SQL 演示数据")
	}

	from := now.Add(-24 * time.Hour).Add(-time.Second)
	for _, item := range items {
		if item.OccurredAt.Before(from) {
			t.Fatalf("记录时间 %s 超出查询范围", item.OccurredAt.Format(time.RFC3339))
		}
		if item.OccurredAt.After(now.Add(2 * time.Minute)) {
			t.Fatalf("记录时间 %s 超出查询范围", item.OccurredAt.Format(time.RFC3339))
		}
	}
}

// TestSlowSQLDateOnlyRange 校验仅传日期时按整天（含 23:59:59）处理。
func TestSlowSQLDateOnlyRange(t *testing.T) {
	svc := newSlowSQLService()
	ctx := context.Background()

	today := time.Now().Format("2006-01-02")
	result, err := svc.List(ctx, service.SlowSQLQueryPayload{Page: 1, Size: 100, From: today, To: today})
	if err != nil {
		t.Fatalf("按日期查询失败: %v", err)
	}
	items := result.Items.([]model.SlowSQL)
	for _, item := range items {
		if item.OccurredAt.Format("2006-01-02") != today {
			t.Fatalf("按日期查询命中了非当日记录: %s", item.OccurredAt)
		}
	}
}

// TestSlowSQLInvalidTime 校验非法时间格式返回业务错误。
func TestSlowSQLInvalidTime(t *testing.T) {
	svc := newSlowSQLService()
	_, err := svc.List(context.Background(), service.SlowSQLQueryPayload{From: "昨天"})
	if err == nil {
		t.Fatal("非法时间格式应返回错误")
	}
	if _, ok := service.AsError(err); !ok {
		t.Fatalf("应返回业务错误，实际 %T", err)
	}
}

// TestSlowSQLStats 校验统计口径与列表总数一致。
func TestSlowSQLStats(t *testing.T) {
	svc := newSlowSQLService()
	ctx := context.Background()

	now := time.Now()
	payload := service.SlowSQLQueryPayload{
		Page: 1,
		Size: 200,
		From: now.AddDate(0, 0, -7).Format("2006-01-02 15:04:05"),
		To:   now.Add(time.Minute).Format("2006-01-02 15:04:05"),
	}

	list, err := svc.List(ctx, payload)
	if err != nil {
		t.Fatalf("查询列表失败: %v", err)
	}
	stats, err := svc.Stats(ctx, payload)
	if err != nil {
		t.Fatalf("查询统计失败: %v", err)
	}
	if stats.Total != list.Total {
		t.Fatalf("统计总数 %d 与列表总数 %d 不一致", stats.Total, list.Total)
	}
	if stats.Total > 0 && (stats.AvgMs <= 0 || stats.MaxMs < stats.AvgMs) {
		t.Fatalf("统计指标异常: avg=%d max=%d", stats.AvgMs, stats.MaxMs)
	}
	if len(stats.TopItems) == 0 {
		t.Fatal("应有慢 SQL 排行数据")
	}
}

// TestSlowSQLRecordAndPurge 校验采集与清理。
func TestSlowSQLRecordAndPurge(t *testing.T) {
	store := (&memory.Store{}).Init(false)
	svc := &service.SlowSQLService{Repo: store.SlowSQL, Threshold: 100 * time.Millisecond, PageSize: 10}
	ctx := context.Background()

	// 低于阈值不应入库
	svc.Record(ctx, model.SlowSQL{Statement: "SELECT 1", DurationMs: 10})
	count, err := store.SlowSQL.Count(ctx)
	if err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("低于阈值的记录不应入库，实际 %d 条", count)
	}

	svc.Record(ctx, model.SlowSQL{Statement: "SELECT * FROM waf_log", DurationMs: 1500})
	count, err = store.SlowSQL.Count(ctx)
	if err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("超过阈值的记录应入库，实际 %d 条", count)
	}

	removed, err := svc.Purge(ctx, 1)
	if err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	if removed != 0 {
		t.Fatalf("刚写入的记录不应被清理，实际清理 %d 条", removed)
	}
}
