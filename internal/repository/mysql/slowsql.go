package mysql

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-xorm/xorm"

	"wafadmin/internal/model"
	"wafadmin/pkg/sqlwatch"
)

// slowSQLRepo 是 SlowSQLRepository 的 MySQL 实现。
type slowSQLRepo struct {
	engine   *xorm.Engine
	observer *sqlwatch.Observer
}

// List 按时间范围与条件分页查询慢 SQL。
func (r *slowSQLRepo) List(ctx context.Context, q model.SlowSQLQuery) ([]model.SlowSQL, int64, error) {
	defer r.observer.Trace(ctx, "slow_sql.list", "SELECT * FROM slow_sql WHERE occurred_at BETWEEN ? AND ? ORDER BY occurred_at DESC LIMIT ? OFFSET ?")()
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}

	where, args := r.buildFilter(q)

	// Count 会重置 session 的 WHERE，统计与列表各用一个 session
	countSession := r.engine.NewSession()
	defer countSession.Close()
	if where != "" {
		countSession = countSession.Where(where, args...)
	}
	total, err := countSession.Count(new(model.SlowSQL))
	if err != nil {
		return nil, 0, err
	}

	session := r.engine.NewSession()
	defer session.Close()
	if where != "" {
		session = session.Where(where, args...)
	}

	items := make([]model.SlowSQL, 0, q.Size)
	if q.SortBy == "duration" {
		if q.Ascending() {
			session = session.Asc("duration_ms")
		} else {
			session = session.Desc("duration_ms")
		}
	} else if q.Ascending() {
		session = session.Asc("occurred_at")
	} else {
		session = session.Desc("occurred_at")
	}
	if err := session.Limit(q.Size, q.Offset()).Find(&items); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Create 写入一条慢 SQL 记录。
func (r *slowSQLRepo) Create(ctx context.Context, item *model.SlowSQL) error {
	defer r.observer.Trace(ctx, "slow_sql.create", "INSERT INTO slow_sql (...) VALUES (...)")()
	if err := ctx.Err(); err != nil {
		return err
	}
	if item.OccurredAt.IsZero() {
		item.OccurredAt = time.Now()
	}
	if _, err := r.engine.Insert(item); err != nil {
		return err
	}
	return nil
}

// Stats 汇总统计慢 SQL 的关键指标。
func (r *slowSQLRepo) Stats(ctx context.Context, q model.SlowSQLQuery) (model.SlowSQLStats, error) {
	defer r.observer.Trace(ctx, "slow_sql.stats", "SELECT COUNT(*), AVG(duration_ms), MAX(duration_ms) FROM slow_sql WHERE ...")()
	if err := ctx.Err(); err != nil {
		return model.SlowSQLStats{}, err
	}

	where, args := r.buildFilter(q)
	suffix := ""
	if where != "" {
		suffix = " WHERE " + where
	}

	summary := new(struct {
		Total  int64 `xorm:"'total'"`
		AvgMs  int64 `xorm:"'avg_ms'"`
		MaxMs  int64 `xorm:"'max_ms'"`
		Over1s int64 `xorm:"'over_1s'"`
	})
	summarySQL := "SELECT COUNT(*) AS total, CAST(COALESCE(AVG(duration_ms), 0) AS SIGNED) AS avg_ms, " +
		"COALESCE(MAX(duration_ms), 0) AS max_ms, " +
		"COALESCE(SUM(CASE WHEN duration_ms >= 1000 THEN 1 ELSE 0 END), 0) AS over_1s FROM slow_sql" + suffix
	if _, err := r.engine.SQL(summarySQL, args...).Get(summary); err != nil {
		return model.SlowSQLStats{}, err
	}

	stats := model.SlowSQLStats{
		Total:    summary.Total,
		AvgMs:    summary.AvgMs,
		MaxMs:    summary.MaxMs,
		Over1s:   summary.Over1s,
		TopItems: make([]model.SlowSQLTop, 0, 10),
	}
	if stats.Total == 0 {
		return stats, nil
	}

	offset := int(float64(stats.Total)*0.95) - 1
	if offset < 0 {
		offset = 0
	}
	p95SQL := "SELECT duration_ms FROM slow_sql" + suffix + " ORDER BY duration_ms ASC LIMIT 1 OFFSET ?"
	p95Args := append(append([]any{}, args...), offset)
	p95 := new(struct {
		DurationMs int64 `xorm:"'duration_ms'"`
	})
	if _, err := r.engine.SQL(p95SQL, p95Args...).Get(p95); err == nil {
		stats.P95Ms = p95.DurationMs
	}

	topSQL := "SELECT statement, MAX(operation) AS operation, COUNT(*) AS count, " +
		"CAST(COALESCE(AVG(duration_ms), 0) AS SIGNED) AS avg_ms, MAX(duration_ms) AS max_ms " +
		"FROM slow_sql" + suffix + " GROUP BY statement ORDER BY max_ms DESC LIMIT 10"
	if err := r.engine.SQL(topSQL, args...).Find(&stats.TopItems); err != nil {
		return stats, fmt.Errorf("统计慢 SQL 排行失败: %w", err)
	}
	return stats, nil
}

// PurgeBefore 删除早于指定时间的记录。
func (r *slowSQLRepo) PurgeBefore(ctx context.Context, before time.Time) (int64, error) {
	defer r.observer.Trace(ctx, "slow_sql.purge", "DELETE FROM slow_sql WHERE occurred_at < ?")()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	result, err := r.engine.Exec("DELETE FROM slow_sql WHERE occurred_at < ?", before)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Count 返回记录总数。
func (r *slowSQLRepo) Count(ctx context.Context) (int64, error) {
	defer r.observer.Trace(ctx, "slow_sql.count", "SELECT COUNT(*) FROM slow_sql")()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.engine.Count(new(model.SlowSQL))
}

// buildFilter 把查询条件转换为 SQL 片段与参数。
func (r *slowSQLRepo) buildFilter(q model.SlowSQLQuery) (string, []any) {
	conditions := make([]string, 0, 5)
	args := make([]any, 0, 6)

	if !q.From.IsZero() {
		conditions = append(conditions, "occurred_at >= ?")
		args = append(args, q.From)
	}
	if !q.To.IsZero() {
		conditions = append(conditions, "occurred_at <= ?")
		args = append(args, q.To)
	}
	if q.Source != "" {
		conditions = append(conditions, "source = ?")
		args = append(args, q.Source)
	}
	if q.MinMs > 0 {
		conditions = append(conditions, "duration_ms >= ?")
		args = append(args, q.MinMs)
	}
	if q.Keyword != "" {
		conditions = append(conditions, "statement LIKE ?")
		args = append(args, "%"+q.Keyword+"%")
	}
	return strings.Join(conditions, " AND "), args
}
