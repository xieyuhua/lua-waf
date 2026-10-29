package mysql

import (
	"context"
	"sort"

	"wafadmin/internal/model"
)

// rateLimitRepo 是限频规则的 MySQL 实现。
type rateLimitRepo struct {
	store *Store
}

// List 按 Id 升序返回全部规则。
func (r *rateLimitRepo) List(ctx context.Context) ([]model.RateLimitRule, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	rules := make([]model.RateLimitRule, 0, 16)
	if err := r.store.Engine.OrderBy("id ASC").Find(&rules); err != nil {
		return nil, err
	}
	return rules, nil
}

// Get 取单条规则。
func (r *rateLimitRepo) Get(ctx context.Context, id int64) (*model.RateLimitRule, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var rule model.RateLimitRule
	ok, err := r.store.Engine.ID(id).Get(&rule)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return &rule, nil
}

// Create 新建规则。
func (r *rateLimitRepo) Create(ctx context.Context, rule *model.RateLimitRule) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	_, err := r.store.Engine.Insert(rule)
	return err
}

// Update 更新规则（仅业务字段，避免覆盖 Id）。
func (r *rateLimitRepo) Update(ctx context.Context, rule *model.RateLimitRule) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	_, err := r.store.Engine.ID(rule.Id).MustCols("enabled").Update(rule)
	return err
}

// Delete 删除规则。
func (r *rateLimitRepo) Delete(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	_, err := r.store.Engine.ID(id).Delete(new(model.RateLimitRule))
	return err
}

// sortRules 保证下发顺序稳定（Id 升序，越具体的规则越靠前由 Id 决定）。
func sortRules(rules []model.RateLimitRule) {
	sort.Slice(rules, func(i, j int) bool { return rules[i].Id < rules[j].Id })
}
