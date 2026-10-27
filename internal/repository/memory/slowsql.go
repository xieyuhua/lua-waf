package memory

import (
	"context"
	"sort"
	"strings"
	"time"

	"wafadmin/internal/model"
)

// slowSQLRepo 是 SlowSQLRepository 的内存实现。
type slowSQLRepo struct {
	store *Store
}

// List 按时间范围与条件分页查询慢 SQL，默认按发生时间倒序。
// Get 按主键查询单条慢 SQL 记录，不存在时返回 nil。
func (r *slowSQLRepo) Get(ctx context.Context, id int64) (*model.SlowSQL, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	for _, item := range r.store.slows {
		if item.Id == id {
			found := item
			return &found, nil
		}
	}
	return nil, nil
}
func (r *slowSQLRepo) List(ctx context.Context, q model.SlowSQLQuery) ([]model.SlowSQL, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	items := make([]model.SlowSQL, 0, len(r.store.slows))
	for _, item := range r.store.slows {
		if !matchSlow(item, q) {
			continue
		}
		items = append(items, item)
	}

	ascending := q.Ascending()
	byDuration := q.SortBy == "duration"
	sort.Slice(items, func(i, j int) bool {
		if byDuration && items[i].DurationMs != items[j].DurationMs {
			if ascending {
				return items[i].DurationMs < items[j].DurationMs
			}
			return items[i].DurationMs > items[j].DurationMs
		}
		if ascending {
			return items[i].OccurredAt.Before(items[j].OccurredAt)
		}
		return items[i].OccurredAt.After(items[j].OccurredAt)
	})
	return pager[model.SlowSQL]{items: items}.Slice(q.ListQuery), int64(len(items)), nil
}

// Create 写入一条慢 SQL 记录。
func (r *slowSQLRepo) Create(ctx context.Context, item *model.SlowSQL) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	r.store.slowSeq++
	item.Id = r.store.slowSeq
	if item.OccurredAt.IsZero() {
		item.OccurredAt = r.store.clock()
	}
	r.store.slows = append(r.store.slows, *item)
	return nil
}

// Stats 汇总统计慢 SQL 的关键指标。
func (r *slowSQLRepo) Stats(ctx context.Context, q model.SlowSQLQuery) (model.SlowSQLStats, error) {
	if err := ctx.Err(); err != nil {
		return model.SlowSQLStats{}, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	durations := make([]int64, 0, len(r.store.slows))
	type bucket struct {
		count int64
		total int64
		max   int64
		op    string
	}
	buckets := make(map[string]*bucket, 16)
	var total int64
	for _, item := range r.store.slows {
		if !matchSlow(item, q) {
			continue
		}
		total++
		durations = append(durations, item.DurationMs)
		key := normalizeStatement(item.Statement)
		entry, ok := buckets[key]
		if !ok {
			entry = &bucket{op: item.Operation}
			buckets[key] = entry
		}
		entry.count++
		entry.total += item.DurationMs
		if item.DurationMs > entry.max {
			entry.max = item.DurationMs
		}
	}

	stats := model.SlowSQLStats{Total: total, TopItems: make([]model.SlowSQLTop, 0, len(buckets))}
	if total > 0 {
		var sum int64
		for _, duration := range durations {
			sum += duration
			if duration > stats.MaxMs {
				stats.MaxMs = duration
			}
			if duration >= 1000 {
				stats.Over1s++
			}
		}
		stats.AvgMs = sum / total
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		index := int(float64(len(durations))*0.95) - 1
		if index < 0 {
			index = 0
		}
		if index >= len(durations) {
			index = len(durations) - 1
		}
		stats.P95Ms = durations[index]
	}

	for statement, entry := range buckets {
		stats.TopItems = append(stats.TopItems, model.SlowSQLTop{
			Statement: statement,
			Operation: entry.op,
			Count:     entry.count,
			AvgMs:     entry.total / entry.count,
			MaxMs:     entry.max,
		})
	}
	sort.Slice(stats.TopItems, func(i, j int) bool {
		if stats.TopItems[i].MaxMs == stats.TopItems[j].MaxMs {
			return stats.TopItems[i].Count > stats.TopItems[j].Count
		}
		return stats.TopItems[i].MaxMs > stats.TopItems[j].MaxMs
	})
	if len(stats.TopItems) > 10 {
		stats.TopItems = stats.TopItems[:10]
	}
	return stats, nil
}

// PurgeBefore 删除早于指定时间的记录。
func (r *slowSQLRepo) PurgeBefore(ctx context.Context, before time.Time) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	kept := make([]model.SlowSQL, 0, len(r.store.slows))
	var removed int64
	for _, item := range r.store.slows {
		if item.OccurredAt.Before(before) {
			removed++
			continue
		}
		kept = append(kept, item)
	}
	r.store.slows = kept
	return removed, nil
}

// Count 返回记录总数。
func (r *slowSQLRepo) Count(ctx context.Context) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	return int64(len(r.store.slows)), nil
}

// matchSlow 判断慢 SQL 是否命中查询条件。
func matchSlow(item model.SlowSQL, q model.SlowSQLQuery) bool {
	if !q.From.IsZero() && item.OccurredAt.Before(q.From) {
		return false
	}
	if !q.To.IsZero() && item.OccurredAt.After(q.To) {
		return false
	}
	if q.Source != "" && item.Source != q.Source {
		return false
	}
	if q.MinMs > 0 && item.DurationMs < q.MinMs {
		return false
	}
	if q.Keyword == "" {
		return true
	}
	return strings.Contains(strings.ToLower(item.Statement), strings.ToLower(q.Keyword))
}

// normalizeStatement 把 SQL 归一化为聚合键，避免变量值导致分组爆炸。
func normalizeStatement(statement string) string {
	trimmed := strings.TrimSpace(statement)
	trimmed = strings.Join(strings.Fields(trimmed), " ")
	if len(trimmed) > 120 {
		return trimmed[:120] + "..."
	}
	return trimmed
}
