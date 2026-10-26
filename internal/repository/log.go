package repository

import (
	"context"
	"time"

	"wafadmin/internal/model"
)

// StatsFilter 描述日志统计的时间与条数过滤条件。
type StatsFilter struct {
	// Day 非空时只统计该天（格式 2006-01-02）。
	Day string
	// Limit 为返回条数上限，<=0 表示不限。
	Limit int
}

// LogRepository 定义 WAF 攻击日志的只读访问契约。
type LogRepository interface {
	// Get 按主键查询单条日志。
	Get(ctx context.Context, id int64) (*model.WafLog, error)
	// List 按筛选条件分页查询日志。
	List(ctx context.Context, q model.LogQuery) ([]model.WafLog, int64, error)
	// Count 返回日志总数。
	Count(ctx context.Context) (int64, error)
	// CountOnDay 返回指定日期的日志条数。
	CountOnDay(ctx context.Context, day string) (int64, error)
	// IPStats 按来源 IP 聚合，filter.Day 为空表示全部数据。
	IPStats(ctx context.Context, filter StatsFilter) ([]model.IPStat, error)
	// TypeStats 按攻击类型聚合，filter.Day 为空表示全部数据。
	TypeStats(ctx context.Context, filter StatsFilter) ([]model.TypeStat, error)
	// DateTrend 返回最近 days 天的每日攻击趋势（按日期升序）。
	DateTrend(ctx context.Context, days int) ([]model.DateStat, error)
}

// SlowSQLRepository 定义慢 SQL 记录的持久化与统计契约。
type SlowSQLRepository interface {
	// List 按时间范围与条件分页查询慢 SQL。
	List(ctx context.Context, q model.SlowSQLQuery) ([]model.SlowSQL, int64, error)
	// Create 写入一条慢 SQL 记录。
	Create(ctx context.Context, item *model.SlowSQL) error
	// Stats 汇总统计（总数 / 平均 / 最大 / P95 / 排行）。
	Stats(ctx context.Context, q model.SlowSQLQuery) (model.SlowSQLStats, error)
	// PurgeBefore 删除早于指定时间的记录，返回删除条数。
	PurgeBefore(ctx context.Context, before time.Time) (int64, error)
	// Count 返回记录总数。
	Count(ctx context.Context) (int64, error)
}
