package service

// 本文件实现 waf_log 的「数据保留策略」：定时删除超过保留天数的日志。
//
// 为什么需要：waf_log 是只增不减的表，长期跑下来会到千万级，查询、备份、DDL 都会变慢。
// 策略由 conf/app.ini 的 [log] RETENTION_DAYS 控制：
//   - 0（默认）= 不清理，保持现有行为；
//   - N > 0   = 每天检查一次，删除 local_time 早于「今天 - N 天」的日志。
//
// 安全约定：
//   1. 分批删除（每批上限由仓储控制），避免一次大事务锁表；
//   2. 只按 local_time 字符串比较，不做任何结构变更；
//   3. 删除前建议先 mysqldump 归档（见 docs/OPS.md）。

import (
	"context"
	"log"
	"time"

	"wafadmin/internal/repository"
)

// RetentionService 负责按保留天数清理历史日志。
type RetentionService struct {
	// Logs 为日志仓储。
	Logs repository.LogRepository
	// Days 为保留天数，<=0 表示不清理。
	Days int
}

// Enabled 判断保留策略是否生效。
func (s *RetentionService) Enabled() bool {
	return s != nil && s.Days > 0 && s.Logs != nil
}

// Cutoff 返回保留边界（形如 2026-10-29 00:00:00 的字符串）。
func (s *RetentionService) Cutoff(now time.Time) string {
	return now.AddDate(0, 0, -s.Days).Format("2006-01-02") + " 00:00:00"
}

// RunOnce 执行一次清理，返回删除条数。
func (s *RetentionService) RunOnce(ctx context.Context) (int64, error) {
	if !s.Enabled() {
		return 0, nil
	}

	cutoff := s.Cutoff(time.Now())
	var total int64
	// 循环删到没有可删的为止：每次最多删一批，避免长事务
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}

		archived, err := s.Logs.ArchiveBefore(ctx, cutoff)
		if err != nil {
			return total, err
		}
		total += archived.Archived
		if archived.Archived == 0 {
			break
		}
	}
	return total, nil
}

// Run 启动每日清理循环，ctx 结束时退出。
func (s *RetentionService) Run(ctx context.Context) {
	if !s.Enabled() {
		log.Printf("[info] 日志保留策略未启用（[log] RETENTION_DAYS = 0），历史日志不会自动清理")
		return
	}

	log.Printf("[info] 日志保留策略已启用：保留 %d 天，超期日志每日自动清理（删除前建议备份）", s.Days)

	// 启动后先跑一次（清理上次停机期间累积的超期数据），随后每 24 小时一次
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		if removed, err := s.RunOnce(ctx); err != nil {
			log.Printf("[warn] 归档超期日志失败: %v", err)
		} else if removed > 0 {
			log.Printf("[info] 已归档超期日志 %d 条（早于 %s）", removed, s.Cutoff(time.Now()))
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
