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
	// BatchCreate 批量写入攻击日志（供 Redis 消费者落库），返回写入条数。
	BatchCreate(ctx context.Context, items []model.WafLog) (int64, error)
	// TagStats 按命中规则聚合最近 days 天的日志（days<=0 表示全部时间）。
	TagStats(ctx context.Context, days int, limit int) ([]model.TagStat, error)
	// TagSamples 返回某条命中规则（rule_tag）最近的日志样例，用于判断是否误杀。
	TagSamples(ctx context.Context, tag string, days int, limit int) ([]model.WafLog, error)
	// Count 返回日志总数。
	Count(ctx context.Context) (int64, error)
	// CountOnDay 返回指定日期的日志条数。
	CountOnDay(ctx context.Context, day string) (int64, error)
	// IPStats 按来源 IP 聚合，filter.Day 为空表示全部数据。
	IPStats(ctx context.Context, filter StatsFilter) ([]model.IPStat, error)
	// TypeStats 按攻击类型聚合，filter.Day 为空表示全部数据。
	TypeStats(ctx context.Context, filter StatsFilter) ([]model.TypeStat, error)
	// DistinctAttackTypes 返回出现过的攻击类型（去重，供筛选项使用）。
	DistinctAttackTypes(ctx context.Context) ([]string, error)
	// ArchiveBefore 把本地时间早于 before（形如 2026-10-29 00:00:00）的日志**归档**到按月表
	// （如 waf_log_202610）后从 waf_log 删除，返回归档条数与删除条数。
	// 用于「数据保留策略」：waf_log 只保留最近 N 天，更早的数据按月归档、不丢弃。
	ArchiveBefore(ctx context.Context, before string) (model.LogArchiveResult, error)
	// DateTrend 返回最近 days 天的每日攻击趋势（按日期升序）。
	DateTrend(ctx context.Context, days int) ([]model.DateStat, error)
}

// SlowSQLRepository 定义慢 SQL 记录的持久化与统计契约。
type SlowSQLRepository interface {
	// Get 按主键查询单条慢 SQL 记录，不存在时返回 nil。
	Get(ctx context.Context, id int64) (*model.SlowSQL, error)
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
