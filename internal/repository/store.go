package repository

import (
	"context"
)

// SettingRepository 定义键值型配置的持久化契约，
// 用于保存 AI 大模型接入等可在后台动态修改的配置。
type SettingRepository interface {
	// All 返回全部配置项。
	All(ctx context.Context) (map[string]string, error)
	// Get 读取单个配置项。
	Get(ctx context.Context, key string) (string, bool, error)
	// Set 写入或覆盖单个配置项。
	Set(ctx context.Context, key, value string) error
	// Delete 删除配置项。
	Delete(ctx context.Context, key string) error
}

// Repositories 聚合各业务域的仓储实现，由 app 层统一装配。
type Repositories struct {
	Site    SiteRepository
	Rule    RuleRepository
	User    UserRepository
	Log     LogRepository
	SlowSQL SlowSQLRepository
	Setting SettingRepository
	// RateLimit 为按 URL 的精细化限频规则。
	RateLimit RateLimitRepository
}

// Ready 判断仓储是否已完成装配，避免空指针导致的运行时 panic。
func (r Repositories) Ready() bool {
	return r.Site != nil && r.Rule != nil && r.User != nil && r.Log != nil && r.SlowSQL != nil && r.Setting != nil && r.RateLimit != nil
}

// Ensure 返回缺失的仓储名称，便于启动时给出明确错误。
func (r Repositories) Ensure() error {
	missing := make([]string, 0, 6)
	if r.Site == nil {
		missing = append(missing, "Site")
	}
	if r.Rule == nil {
		missing = append(missing, "Rule")
	}
	if r.User == nil {
		missing = append(missing, "User")
	}
	if r.Log == nil {
		missing = append(missing, "Log")
	}
	if r.SlowSQL == nil {
		missing = append(missing, "SlowSQL")
	}
	if r.Setting == nil {
		missing = append(missing, "Setting")
	}
	if len(missing) == 0 {
		return nil
	}
	return &MissingRepositoryError{Names: missing}
}

// MissingRepositoryError 表示仓储装配不完整。
type MissingRepositoryError struct {
	Names []string
}

// Error 实现 error 接口。
func (e *MissingRepositoryError) Error() string {
	out := "仓储未装配: "
	for index, name := range e.Names {
		if index > 0 {
			out += ", "
		}
		out += name
	}
	return out
}
