package memory

import (
	"context"
	"sort"
	"strings"
	"time"

	"wafadmin/internal/model"
	"wafadmin/internal/repository"
)

// 日志时间统一按 openresty 写入的本地时间格式处理。
const logTimeLayout = "2006-01-02 15:04:05"

// logRepo 是 LogRepository 的内存实现。
type logRepo struct {
	store *Store
}

// Get 按主键查询单条日志。
func (r *logRepo) Get(ctx context.Context, id int64) (*model.WafLog, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	for _, item := range r.store.logs {
		if item.Id == id {
			found := item
			return &found, nil
		}
	}
	return nil, nil
}

// List 按筛选条件分页查询日志，返回结果按 ID 倒序（最新在前）。
func (r *logRepo) List(ctx context.Context, q model.LogQuery) ([]model.WafLog, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	items := make([]model.WafLog, 0, len(r.store.logs))
	for _, item := range r.store.logs {
		if !matchLog(item, q) {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Id > items[j].Id })
	return pager[model.WafLog]{items: items}.Slice(q.ListQuery), int64(len(items)), nil
}

// Count 返回日志总数。
func (r *logRepo) Count(ctx context.Context) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	return int64(len(r.store.logs)), nil
}

// CountOnDay 返回指定日期的日志条数。
func (r *logRepo) CountOnDay(ctx context.Context, day string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	var total int64
	for _, item := range r.store.logs {
		if strings.HasPrefix(item.LocalTime, day) {
			total++
		}
	}
	return total, nil
}

// IPStats 按来源 IP 聚合攻击次数。
func (r *logRepo) IPStats(ctx context.Context, filter repository.StatsFilter) ([]model.IPStat, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	type bucket struct {
		area  string
		num   int64
		types map[string]struct{}
	}
	buckets := make(map[string]*bucket, 16)
	for _, item := range r.store.logs {
		if filter.Day != "" && !strings.HasPrefix(item.LocalTime, filter.Day) {
			continue
		}
		entry, ok := buckets[item.ClientIp]
		if !ok {
			entry = &bucket{area: item.Area, types: make(map[string]struct{}, 2)}
			buckets[item.ClientIp] = entry
		}
		entry.num++
		if item.AttackType != "" {
			entry.types[item.AttackType] = struct{}{}
		}
	}

	stats := make([]model.IPStat, 0, len(buckets))
	for ip, entry := range buckets {
		stats = append(stats, model.IPStat{
			ClientIp:   ip,
			Area:       entry.area,
			Num:        entry.num,
			AttackType: strings.Join(sortedKeys(entry.types), ","),
		})
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Num == stats[j].Num {
			return stats[i].ClientIp < stats[j].ClientIp
		}
		return stats[i].Num > stats[j].Num
	})
	if filter.Limit > 0 && len(stats) > filter.Limit {
		stats = stats[:filter.Limit]
	}
	return stats, nil
}

// TypeStats 按攻击类型聚合。
func (r *logRepo) TypeStats(ctx context.Context, filter repository.StatsFilter) ([]model.TypeStat, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	type bucket struct {
		num   int64
		areas map[string]struct{}
	}
	buckets := make(map[string]*bucket, 8)
	for _, item := range r.store.logs {
		if filter.Day != "" && !strings.HasPrefix(item.LocalTime, filter.Day) {
			continue
		}
		key := item.AttackType
		if key == "" {
			key = "未知"
		}
		entry, ok := buckets[key]
		if !ok {
			entry = &bucket{areas: make(map[string]struct{}, 2)}
			buckets[key] = entry
		}
		entry.num++
		if item.Area != "" {
			entry.areas[item.Area] = struct{}{}
		}
	}

	stats := make([]model.TypeStat, 0, len(buckets))
	for name, entry := range buckets {
		stats = append(stats, model.TypeStat{
			AttackType: name,
			Num:        entry.num,
			Area:       strings.Join(sortedKeys(entry.areas), ","),
		})
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Num == stats[j].Num {
			return stats[i].AttackType < stats[j].AttackType
		}
		return stats[i].Num > stats[j].Num
	})
	if filter.Limit > 0 && len(stats) > filter.Limit {
		stats = stats[:filter.Limit]
	}
	return stats, nil
}

// DateTrend 返回最近 days 天的每日攻击量，按日期升序并补齐空缺日期。
func (r *logRepo) DateTrend(ctx context.Context, days int) ([]model.DateStat, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if days <= 0 {
		days = 15
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	counts := make(map[string]int64, days)
	for _, item := range r.store.logs {
		day := localDay(item.LocalTime)
		if day == "" {
			continue
		}
		counts[day]++
	}

	today := time.Now()
	stats := make([]model.DateStat, 0, days)
	for offset := days - 1; offset >= 0; offset-- {
		day := today.AddDate(0, 0, -offset).Format("2006-01-02")
		stats = append(stats, model.DateStat{Date: day, Num: counts[day]})
	}
	return stats, nil
}

// matchLog 判断日志是否命中查询条件。
func matchLog(item model.WafLog, q model.LogQuery) bool {
	if q.AttackType != "" && item.AttackType != q.AttackType {
		return false
	}
	if q.ClientIp != "" && !strings.Contains(item.ClientIp, q.ClientIp) {
		return false
	}
	if q.ServerName != "" && item.ServerName != q.ServerName {
		return false
	}
	if q.Day != "" && !strings.HasPrefix(item.LocalTime, q.Day) {
		return false
	}
	if q.From != "" && item.LocalTime < q.From {
		return false
	}
	if q.To != "" && item.LocalTime > q.To {
		return false
	}
	if q.Keyword == "" {
		return true
	}
	keyword := strings.ToLower(q.Keyword)
	fields := []string{item.ClientIp, item.ReqUrl, item.UserAgent, item.AttackType, item.RuleTag, item.ServerName, item.ReqData}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), keyword) {
			return true
		}
	}
	return false
}

// localDay 从 "2006-01-02 15:04:05" 中取出日期部分。
func localDay(localTime string) string {
	if len(localTime) >= 10 {
		return localTime[:10]
	}
	return ""
}

// sortedKeys 返回排序后的集合内容，保证输出稳定。
func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
