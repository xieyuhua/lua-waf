package service

import (
	"context"
	"strconv"
	"sync"
	"time"

	"wafadmin/internal/model"
	"wafadmin/internal/repository"
)

// LogOverview 是攻击日志页面的统计摘要。
type LogOverview struct {
	Total int64            `json:"total"`
	Today int64            `json:"today"`
	Types []model.TypeStat `json:"types"`
	IPs   []model.IPStat   `json:"ips"`
	Trend []model.DateStat `json:"trend"`
	Top   []model.WafLog   `json:"top"`
}

// LogService 负责攻击日志的查询与统计。
type LogService struct {
	Logs     repository.LogRepository
	PageSize int

	// 攻击类型清单缓存：筛选项变动不频繁，缓存 60 秒，
	// 避免每次打开日志页都在百万行日志上做聚合查询。
	cacheMu    sync.Mutex
	cacheTypes []string
	cacheAt    time.Time
	cacheTTL   time.Duration

	// overviewCache 缓存概览结果（默认 45 秒），避免反复做多次聚合查询。
	overviewCache *statsCache[*LogOverview]
	// cacheOnce 保证懒初始化只执行一次。
	cacheOnce sync.Once
}

// initCache 懒初始化缓存（零值 LogService 也能安全使用）。
func (s *LogService) initCache() {
	s.cacheOnce.Do(func() {
		s.overviewCache = newStatsCache[*LogOverview](defaultStatsCacheTTL)
	})
}

// cachedAttackTypes 命中缓存时返回类型清单。
func (s *LogService) cachedAttackTypes() ([]string, bool) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	ttl := s.cacheTTL
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	if len(s.cacheTypes) > 0 && time.Since(s.cacheAt) < ttl {
		return s.cacheTypes, true
	}
	return nil, false
}

// storeAttackTypes 写入类型清单缓存。
func (s *LogService) storeAttackTypes(types []string) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	s.cacheTypes = types
	s.cacheAt = time.Now()
}

// List 按筛选条件分页查询日志。
func (s *LogService) List(ctx context.Context, q model.LogQuery) (model.PageResult, error) {
	q.ListQuery = q.ListQuery.Normalize(s.PageSize)
	if q.From != "" && q.To != "" && q.From > q.To {
		return model.PageResult{}, ErrInvalid.New("开始时间不能晚于结束时间")
	}

	items, total, err := s.Logs.List(ctx, q)
	if err != nil {
		return model.PageResult{}, ErrInternal.Wrap(err, "查询日志失败")
	}
	return q.ListQuery.Result(items, total), nil
}

// Detail 返回单条日志详情。
func (s *LogService) Detail(ctx context.Context, id int64) (*model.WafLog, error) {
	item, err := s.Logs.Get(ctx, id)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "查询日志失败")
	}
	if item == nil {
		return nil, ErrNotFound.New("日志不存在")
	}
	return item, nil
}

// IPSources 返回按来源 IP 聚合的统计，day 为空表示全部时间。
func (s *LogService) IPSources(ctx context.Context, day string, limit int) ([]model.IPStat, error) {
	stats, err := s.Logs.IPStats(ctx, repository.StatsFilter{Day: day, Limit: limit})
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计攻击来源失败")
	}
	return stats, nil
}

// TypeStats 返回按攻击类型聚合的统计，day 为空表示全部时间。
func (s *LogService) TypeStats(ctx context.Context, day string) ([]model.TypeStat, error) {
	stats, err := s.Logs.TypeStats(ctx, repository.StatsFilter{Day: day})
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计攻击类型失败")
	}
	return stats, nil
}

// Trend 返回最近 days 天的攻击趋势。
func (s *LogService) Trend(ctx context.Context, days int) ([]model.DateStat, error) {
	trend, err := s.Logs.DateTrend(ctx, days)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计攻击趋势失败")
	}
	return trend, nil
}

// Overview 返回日志统计摘要，days 为趋势天数。
func (s *LogService) Overview(ctx context.Context, days int) (*LogOverview, error) {
	s.initCache()
	cacheKey := "overview:" + strconv.Itoa(days)
	if cached, ok := s.overviewCache.Get(cacheKey); ok {
		return cached, nil
	}

	total, err := s.Logs.Count(ctx)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计日志失败")
	}
	today, err := s.Logs.CountOnDay(ctx, time.Now().Format("2006-01-02"))
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计今日日志失败")
	}
	types, err := s.Logs.TypeStats(ctx, repository.StatsFilter{Limit: 20})
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计攻击类型失败")
	}
	ips, err := s.Logs.IPStats(ctx, repository.StatsFilter{Limit: 20})
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计攻击来源失败")
	}
	trend, err := s.Logs.DateTrend(ctx, days)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计攻击趋势失败")
	}

	recent, _, err := s.Logs.List(ctx, model.LogQuery{ListQuery: model.ListQuery{Page: 1, Size: 8}})
	if err != nil {
		return nil, ErrInternal.Wrap(err, "查询最新日志失败")
	}

	overview := &LogOverview{Total: total, Today: today, Types: types, IPs: ips, Trend: trend, Top: recent}
	s.overviewCache.Set(cacheKey, overview)
	return overview, nil
}

// AttackTypes 返回出现过的攻击类型，用于筛选项。
func (s *LogService) AttackTypes(ctx context.Context) ([]string, error) {
	// 筛选项只需“有哪些类型”，用 DISTINCT 查询（走索引），并缓存 60 秒，
	// 避免每次打开日志页都在百万行日志上做一次 GROUP BY。
	if types, ok := s.cachedAttackTypes(); ok {
		return types, nil
	}

	types, err := s.Logs.DistinctAttackTypes(ctx)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计攻击类型失败")
	}
	s.storeAttackTypes(types)
	return types, nil
}
