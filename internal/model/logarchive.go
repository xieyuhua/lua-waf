package model

// LogArchiveResult 是一次日志归档的结果。
type LogArchiveResult struct {
	// Archived 为写入归档表的条数。
	Archived int64 `json:"archived"`
	// Deleted 为从 waf_log 删除的条数（应与 Archived 相等）。
	Deleted int64 `json:"deleted"`
	// Tables 为本次涉及的归档表（如 waf_log_202610），便于日志与排查。
	Tables []string `json:"tables"`
}
