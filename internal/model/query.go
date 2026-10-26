package model

import (
	"strings"
	"time"
)

// ListQuery 是分页查询的通用参数。
type ListQuery struct {
	Page    int    `json:"page"`
	Size    int    `json:"size"`
	Keyword string `json:"keyword"`
}

// Normalize 修正非法分页参数，并限制单页上限。
func (q ListQuery) Normalize(defaultSize int) ListQuery {
	if defaultSize <= 0 {
		defaultSize = 10
	}
	if q.Size <= 0 {
		q.Size = defaultSize
	}
	if q.Size > 200 {
		q.Size = 200
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	q.Keyword = strings.TrimSpace(q.Keyword)
	return q
}

// Offset 返回 SQL 分页偏移量。
func (q ListQuery) Offset() int {
	if q.Page <= 1 {
		return 0
	}
	return (q.Page - 1) * q.Size
}

// PageResult 是分页查询的统一返回体。
type PageResult struct {
	Items any   `json:"items"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Size  int   `json:"size"`
	Pages int   `json:"pages"`
}

// Result 组装分页返回体。
func (q ListQuery) Result(items any, total int64) PageResult {
	pages := 0
	if q.Size > 0 {
		pages = int((total + int64(q.Size) - 1) / int64(q.Size))
	}
	return PageResult{Items: items, Total: total, Page: q.Page, Size: q.Size, Pages: pages}
}

// LogQuery 是 WAF 攻击日志的查询条件。
// Keyword 会同时匹配 client_ip / req_url / user_agent / attack_type / rule_tag。
type LogQuery struct {
	ListQuery
	AttackType string
	ClientIp   string
	ServerName string
	// Day 精确到天（2006-01-02），From / To 支持自由时间范围。
	Day  string
	From string
	To   string
}

// SlowSQLQuery 是慢 SQL 记录的查询条件，支持时间范围过滤。
type SlowSQLQuery struct {
	ListQuery
	// From / To 为闭区间时间范围，零值表示不限。
	From   time.Time
	To     time.Time
	Source string
	// MinMs 只返回耗时大于等于该值的记录。
	MinMs  int64
	SortBy string
	Order  string
}

// HasTimeRange 表示是否设置了时间范围。
func (q SlowSQLQuery) HasTimeRange() bool { return !q.From.IsZero() || !q.To.IsZero() }

// Ascending 表示是否按耗时升序排序。
func (q SlowSQLQuery) Ascending() bool { return strings.EqualFold(q.Order, "asc") }

// TimeRange 描述一个闭区间时间范围。
type TimeRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// Valid 表示区间是否可用。
func (r TimeRange) Valid() bool { return !r.From.IsZero() && !r.To.IsZero() && !r.To.Before(r.From) }
