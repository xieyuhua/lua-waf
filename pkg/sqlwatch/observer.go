// Package sqlwatch 为仓储层的数据库访问提供慢 SQL 观测能力。
package sqlwatch

import (
	"context"
	"time"

	"wafadmin/internal/model"
)

// Recorder 接收慢 SQL 记录，由服务层实现。
type Recorder interface {
	Record(ctx context.Context, item model.SlowSQL)
}

// Observer 为一次仓储调用计时，超过阈值时上报给 Recorder。
// 零值（或 Recorder 为 nil）时全部跳过，便于在测试中关闭观测。
type Observer struct {
	// Threshold 为慢 SQL 阈值，默认 200ms。
	Threshold time.Duration
	// Recorder 为记录接收方。
	Recorder Recorder
	// Database 为记录中标注的库名。
	Database string
}

// Trace 返回延迟执行的计时代码块，用法：
//
//	defer obs.Trace(ctx, "log.list", "SELECT ... FROM waf_log")()
func (o *Observer) Trace(ctx context.Context, operation, statement string) func() {
	start := time.Now()
	return func() {
		if o == nil || o.Recorder == nil {
			return
		}
		elapsed := time.Since(start)
		if elapsed < o.threshold() {
			return
		}
		o.Recorder.Record(context.WithoutCancel(ctx), model.SlowSQL{
			OccurredAt: start,
			Source:     model.SlowSQLSourceAdmin,
			Database:   o.Database,
			Operation:  operation,
			Statement:  statement,
			DurationMs: elapsed.Milliseconds(),
		})
	}
}

// threshold 返回生效的慢 SQL 阈值。
func (o *Observer) threshold() time.Duration {
	if o.Threshold <= 0 {
		return 200 * time.Millisecond
	}
	return o.Threshold
}
