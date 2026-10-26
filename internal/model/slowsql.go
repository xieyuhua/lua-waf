package model

import (
	"fmt"
	"time"
)

// 慢 SQL 记录的来源。
const (
	// SlowSQLSourceAdmin 表示由管理后台自身的数据库访问采集。
	SlowSQLSourceAdmin = "admin"
	// SlowSQLSourceAgent 表示由 openresty / 外部采集器上报。
	SlowSQLSourceAgent = "agent"
)

// SlowSQL 是一条慢 SQL 记录。表名：slow_sql
type SlowSQL struct {
	Id int64 `xorm:"pk autoincrement" json:"id"`
	// OccurredAt 为 SQL 执行时间，列表按时间范围查询时使用该字段。
	OccurredAt time.Time `xorm:"index 'occurred_at'" json:"occurred_at"`
	// Source 为采集来源：admin（后台自身）/ agent（外部上报）。
	Source string `xorm:"varchar(16) not null default 'admin'" json:"source"`
	// Database 为库名。
	Database string `xorm:"varchar(64)" json:"database"`
	// Operation 为操作类型，例如 select / update / sync。
	Operation string `xorm:"varchar(32)" json:"operation"`
	// Statement 为 SQL 语句（已截断）。
	Statement string `xorm:"text" json:"statement"`
	// DurationMs 为执行耗时（毫秒）。
	DurationMs int64 `xorm:"'duration_ms'" json:"duration_ms"`
	// Rows 为影响/扫描行数，未知时为 0。
	Rows int64 `json:"rows"`
	// ClientIP 为触发该 SQL 的来源 IP（外部上报时有效）。
	ClientIp string `xorm:"varchar(64) 'client_ip'" json:"client_ip"`
	// TraceId 用于串联一次请求内的多条慢 SQL。
	TraceId string `xorm:"varchar(64) 'trace_id'" json:"trace_id"`
}

// SlowSQLTop 是慢 SQL 聚合排行的条目。
type SlowSQLTop struct {
	Statement string `json:"statement"`
	Operation string `json:"operation"`
	Count     int64  `json:"count"`
	AvgMs     int64  `json:"avg_ms"`
	MaxMs     int64  `json:"max_ms"`
}

// SlowSQLStats 是慢 SQL 概览统计。
type SlowSQLStats struct {
	Total    int64        `json:"total"`
	AvgMs    int64        `json:"avg_ms"`
	MaxMs    int64        `json:"max_ms"`
	P95Ms    int64        `json:"p95_ms"`
	Over1s   int64        `json:"over_1s"`
	TopItems []SlowSQLTop `json:"top_items"`
}

// TruncatedStatement 返回截断后的语句，避免超长文本入库。
func (s SlowSQL) TruncatedStatement(limit int) string {
	if limit <= 0 || len(s.Statement) <= limit {
		return s.Statement
	}
	return s.Statement[:limit] + "..."
}

// Describe 返回可读的摘要，用于日志输出。
func (s SlowSQL) Describe() string {
	return fmt.Sprintf("%s %s %dms", s.Source, s.Operation, s.DurationMs)
}
