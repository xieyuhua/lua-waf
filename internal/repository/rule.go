package repository

import (
	"context"

	"wafadmin/internal/model"
)

// RuleRepository 定义 WAF 规则的持久化契约。
type RuleRepository interface {
	// List 按类型查询规则，ruleType 为空时返回全部。
	List(ctx context.Context, ruleType string) ([]model.Rule, error)
	// Get 按主键查询规则。
	Get(ctx context.Context, id int64) (*model.Rule, error)
	// Create 新增规则。
	Create(ctx context.Context, rule *model.Rule) error
	// Update 更新规则内容。
	Update(ctx context.Context, rule *model.Rule) error
	// SetEnabled 启用或停用规则。
	SetEnabled(ctx context.Context, id int64, enabled bool) error
	// Delete 删除规则。
	Delete(ctx context.Context, id int64) error
	// CountByType 返回各规则类型的条数。
	CountByType(ctx context.Context) (map[string]int64, error)
	// Count 返回规则总数。
	Count(ctx context.Context) (int64, error)
	// BatchCreate 批量写入规则（用于初始化内置规则）。
	BatchCreate(ctx context.Context, rules []model.Rule) error
}
