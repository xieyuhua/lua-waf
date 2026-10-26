package service

import (
	"context"
	"log"
	"strings"
	"time"

	"wafadmin/internal/model"
	"wafadmin/internal/repository"
)

// 慢 SQL 记录的默认截断长度与最小阈值。
const (
	defaultSlowStatementLimit = 4000
	defaultSlowThreshold      = 200 * time.Millisecond
)

// SlowSQLQueryPayload 是慢 SQL 查询的入参，时间字段支持多种写法。
type SlowSQLQueryPayload struct {
	Page    int    `json:"page"`
	Size    int    `json:"size"`
	Keyword string `json:"keyword"`
	From    string `json:"from"`
	To      string `json:"to"`
	Source  string `json:"source"`
	MinMs   int64  `json:"min_ms"`
	SortBy  string `json:"sort_by"`
	Order   string `json:"order"`
}

// SlowSQLReportPayload 是外部（openresty / 采集器）上报慢 SQL 的入参。
type SlowSQLReportPayload struct {
	OccurredAt string `json:"occurred_at"`
	Database   string `json:"database"`
	Operation  string `json:"operation"`
	Statement  string `json:"statement"`
	DurationMs int64  `json:"duration_ms"`
	Rows       int64  `json:"rows"`
	ClientIp   string `json:"client_ip"`
	TraceId    string `json:"trace_id"`
}

// SlowSQLService 负责慢 SQL 的采集、查询与统计。
type SlowSQLService struct {
	Repo repository.SlowSQLRepository
	// Threshold 为入库阈值，低于该耗时的记录直接丢弃。
	Threshold time.Duration
	// MaxStatement 为语句最大保存长度。
	MaxStatement int
	PageSize     int
}

// Record 接收仓储层上报的慢 SQL（实现 sqlwatch.Recorder）。
func (s *SlowSQLService) Record(ctx context.Context, item model.SlowSQL) {
	if item.DurationMs < s.threshold().Milliseconds() {
		return
	}
	if item.Source == "" {
		item.Source = model.SlowSQLSourceAdmin
	}
	item.Statement = s.truncate(item.Statement)
	if item.OccurredAt.IsZero() {
		item.OccurredAt = time.Now()
	}
	if err := s.Repo.Create(ctx, &item); err != nil {
		log.Printf("[warn] 记录慢 SQL 失败: %v", err)
	}
}

// Report 接收外部上报的慢 SQL。
func (s *SlowSQLService) Report(ctx context.Context, payload SlowSQLReportPayload) (*model.SlowSQL, error) {
	if strings.TrimSpace(payload.Statement) == "" {
		return nil, ErrInvalid.New("SQL 语句不能为空")
	}
	if payload.DurationMs <= 0 {
		return nil, ErrInvalid.New("执行耗时必须大于 0")
	}

	occurredAt := time.Now()
	if payload.OccurredAt != "" {
		parsed, err := parseFlexibleTime(payload.OccurredAt, false)
		if err != nil {
			return nil, ErrInvalid.New("occurred_at 时间格式不正确")
		}
		occurredAt = parsed
	}

	item := &model.SlowSQL{
		OccurredAt: occurredAt,
		Source:     model.SlowSQLSourceAgent,
		Database:   strings.TrimSpace(payload.Database),
		Operation:  strings.TrimSpace(payload.Operation),
		Statement:  s.truncate(payload.Statement),
		DurationMs: payload.DurationMs,
		Rows:       payload.Rows,
		ClientIp:   strings.TrimSpace(payload.ClientIp),
		TraceId:    strings.TrimSpace(payload.TraceId),
	}
	if err := s.Repo.Create(ctx, item); err != nil {
		return nil, ErrInternal.Wrap(err, "保存慢 SQL 记录失败")
	}
	return item, nil
}

// List 按时间范围与条件分页查询慢 SQL。
func (s *SlowSQLService) List(ctx context.Context, payload SlowSQLQueryPayload) (model.PageResult, error) {
	query, err := s.buildQuery(payload)
	if err != nil {
		return model.PageResult{}, err
	}
	items, total, err := s.Repo.List(ctx, query)
	if err != nil {
		return model.PageResult{}, ErrInternal.Wrap(err, "查询慢 SQL 失败")
	}
	return query.ListQuery.Result(items, total), nil
}

// Stats 返回慢 SQL 统计指标。
func (s *SlowSQLService) Stats(ctx context.Context, payload SlowSQLQueryPayload) (model.SlowSQLStats, error) {
	query, err := s.buildQuery(payload)
	if err != nil {
		return model.SlowSQLStats{}, err
	}
	stats, err := s.Repo.Stats(ctx, query)
	if err != nil {
		return model.SlowSQLStats{}, ErrInternal.Wrap(err, "统计慢 SQL 失败")
	}
	return stats, nil
}

// Purge 清理早于指定天数的记录。
func (s *SlowSQLService) Purge(ctx context.Context, days int) (int64, error) {
	if days <= 0 {
		return 0, ErrInvalid.New("保留天数必须大于 0")
	}
	before := time.Now().AddDate(0, 0, -days)
	removed, err := s.Repo.PurgeBefore(ctx, before)
	if err != nil {
		return 0, ErrInternal.Wrap(err, "清理慢 SQL 失败")
	}
	return removed, nil
}

// buildQuery 把入参转换为仓储查询条件。
func (s *SlowSQLService) buildQuery(payload SlowSQLQueryPayload) (model.SlowSQLQuery, error) {
	query := model.SlowSQLQuery{
		ListQuery: model.ListQuery{
			Page:    payload.Page,
			Size:    payload.Size,
			Keyword: payload.Keyword,
		}.Normalize(s.PageSize),
		Source: strings.TrimSpace(payload.Source),
		MinMs:  payload.MinMs,
		SortBy: strings.TrimSpace(payload.SortBy),
		Order:  strings.TrimSpace(payload.Order),
	}

	if strings.TrimSpace(payload.From) != "" {
		from, err := parseFlexibleTime(payload.From, false)
		if err != nil {
			return model.SlowSQLQuery{}, ErrInvalid.New("开始时间格式不正确")
		}
		query.From = from
	}
	if strings.TrimSpace(payload.To) != "" {
		to, err := parseFlexibleTime(payload.To, true)
		if err != nil {
			return model.SlowSQLQuery{}, ErrInvalid.New("结束时间格式不正确")
		}
		query.To = to
	}
	if !query.From.IsZero() && !query.To.IsZero() && query.To.Before(query.From) {
		return model.SlowSQLQuery{}, ErrInvalid.New("开始时间不能晚于结束时间")
	}
	return query, nil
}

// threshold 返回生效的入库阈值。
func (s *SlowSQLService) threshold() time.Duration {
	if s.Threshold <= 0 {
		return defaultSlowThreshold
	}
	return s.Threshold
}

// truncate 截断超长语句。
func (s *SlowSQLService) truncate(statement string) string {
	limit := s.MaxStatement
	if limit <= 0 {
		limit = defaultSlowStatementLimit
	}
	if len(statement) <= limit {
		return statement
	}
	return statement[:limit]
}

// parseFlexibleTime 支持 RFC3339、"2006-01-02 15:04:05"、"2006-01-02T15:04:05" 与 "2006-01-02"。
// endOfDay 为 true 且仅给出日期时，取当天 23:59:59。
func parseFlexibleTime(raw string, endOfDay bool) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, ErrInvalid.New("时间不能为空")
	}

	layouts := []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02 15:04", "2006-01-02"}
	for _, layout := range layouts {
		parsed, err := time.ParseInLocation(layout, value, time.Local)
		if err != nil {
			continue
		}
		if layout == "2006-01-02" && endOfDay {
			parsed = parsed.Add(24*time.Hour - time.Second)
		}
		return parsed, nil
	}
	return time.Time{}, ErrInvalid.New("无法解析时间: %s", raw)
}
