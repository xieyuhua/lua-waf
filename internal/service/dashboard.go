package service

import (
	"context"
	"sync"
	"time"

	"wafadmin/internal/model"
	"wafadmin/internal/repository"
)

// DashboardSummary 是后台首页看板的数据。
type DashboardSummary struct {
	Sites         int64            `json:"sites"`
	Rules         int64            `json:"rules"`
	Users         int64            `json:"users"`
	LogsTotal     int64            `json:"logs_total"`
	LogsToday     int64            `json:"logs_today"`
	SlowSQLTotal  int64            `json:"slow_sql_total"`
	SlowSQLToday  int64            `json:"slow_sql_today"`
	Trend         []model.DateStat `json:"trend"`
	TodayTypes    []model.TypeStat `json:"today_types"`
	TodayIPs      []model.IPStat   `json:"today_ips"`
	RecentLogs    []model.WafLog   `json:"recent_logs"`
	RecentSlowSQL []model.SlowSQL  `json:"recent_slow_sql"`
	GeneratedAt   string           `json:"generated_at"`
}

// DashboardService 汇总各业务域的指标，供首页看板与攻击大屏使用。
type DashboardService struct {
	Sites    repository.SiteRepository
	Rules    repository.RuleRepository
	Users    repository.UserRepository
	Logs     repository.LogRepository
	Slow     repository.SlowSQLRepository
	PageSize int

	// summaryCache 缓存看板汇总（默认 45 秒）。
	summaryCache *statsCache[*DashboardSummary]
	cacheOnce    sync.Once
}

// initCache 懒初始化缓存。
func (s *DashboardService) initCache() {
	s.cacheOnce.Do(func() {
		s.summaryCache = newStatsCache[*DashboardSummary](defaultStatsCacheTTL)
	})
}

// Summary 返回看板数据。
func (s *DashboardService) Summary(ctx context.Context) (*DashboardSummary, error) {
	s.initCache()
	if cached, ok := s.summaryCache.Get("summary"); ok {
		return cached, nil
	}

	now := time.Now()
	today := now.Format("2006-01-02")
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	sites, err := s.Sites.Count(ctx)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计站点失败")
	}
	rules, err := s.Rules.Count(ctx)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计规则失败")
	}
	users, err := s.Users.Count(ctx)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计账号失败")
	}
	logsTotal, err := s.Logs.Count(ctx)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计日志失败")
	}
	logsToday, err := s.Logs.CountOnDay(ctx, today)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计今日日志失败")
	}
	trend, err := s.Logs.DateTrend(ctx, 15)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计攻击趋势失败")
	}
	todayTypes, err := s.Logs.TypeStats(ctx, repository.StatsFilter{Day: today, Limit: 10})
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计今日攻击类型失败")
	}
	todayIPs, err := s.Logs.IPStats(ctx, repository.StatsFilter{Day: today, Limit: 10})
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计今日攻击来源失败")
	}

	slowTotal, err := s.Slow.Count(ctx)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计慢 SQL 失败")
	}
	// 今日慢 SQL 与今日耗时排行统计复用同一份聚合结果。
	todaySlowQuery := model.SlowSQLQuery{ListQuery: model.ListQuery{Page: 1, Size: 6}, From: dayStart, SortBy: "duration"}
	todayStats, err := s.Slow.Stats(ctx, todaySlowQuery)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计今日慢 SQL 失败")
	}
	recentSlow, _, err := s.Slow.List(ctx, model.SlowSQLQuery{ListQuery: model.ListQuery{Page: 1, Size: 6}, SortBy: "duration"})
	if err != nil {
		return nil, ErrInternal.Wrap(err, "查询最近慢 SQL 失败")
	}
	recentLogs, _, err := s.Logs.List(ctx, model.LogQuery{ListQuery: model.ListQuery{Page: 1, Size: 6}})
	if err != nil {
		return nil, ErrInternal.Wrap(err, "查询最新攻击失败")
	}

	summary := &DashboardSummary{
		Sites:         sites,
		Rules:         rules,
		Users:         users,
		LogsTotal:     logsTotal,
		LogsToday:     logsToday,
		SlowSQLTotal:  slowTotal,
		SlowSQLToday:  todayStats.Total,
		Trend:         trend,
		TodayTypes:    todayTypes,
		TodayIPs:      todayIPs,
		RecentLogs:    recentLogs,
		RecentSlowSQL: recentSlow,
		GeneratedAt:   now.Format("2006-01-02 15:04:05"),
	}
	s.summaryCache.Set("summary", summary)
	return summary, nil
}
