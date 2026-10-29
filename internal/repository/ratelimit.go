package repository

import (
	"context"

	"wafadmin/internal/model"
)

// RateLimitRepository 是限频规则的存储接口。
type RateLimitRepository interface {
	// List 按 Id 升序返回全部规则（含停用的）。
	List(ctx context.Context) ([]model.RateLimitRule, error)
	// Get 按 Id 取单条规则，不存在返回 nil, nil。
	Get(ctx context.Context, id int64) (*model.RateLimitRule, error)
	// Create 新建规则并回填 Id。
	Create(ctx context.Context, rule *model.RateLimitRule) error
	// Update 更新规则（按 Id）。
	Update(ctx context.Context, rule *model.RateLimitRule) error
	// Delete 删除规则。
	Delete(ctx context.Context, id int64) error
}
