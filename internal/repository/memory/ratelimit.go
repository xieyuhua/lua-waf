package memory

import (
	"context"
	"sort"

	"wafadmin/internal/model"
)

// rateLimitRepo 是限频规则的内存实现（演示 / 测试用）。
type rateLimitRepo struct {
	store *Store
}

// List 按 Id 升序返回全部规则。
func (r *rateLimitRepo) List(ctx context.Context) ([]model.RateLimitRule, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	rules := make([]model.RateLimitRule, len(r.store.rateLimits))
	copy(rules, r.store.rateLimits)
	sort.Slice(rules, func(i, j int) bool { return rules[i].Id < rules[j].Id })
	return rules, nil
}

// Get 取单条规则。
func (r *rateLimitRepo) Get(ctx context.Context, id int64) (*model.RateLimitRule, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	for _, item := range r.store.rateLimits {
		if item.Id == id {
			rule := item
			return &rule, nil
		}
	}
	return nil, nil
}

// Create 新建规则。
func (r *rateLimitRepo) Create(ctx context.Context, rule *model.RateLimitRule) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	r.store.rateSeq++
	rule.Id = r.store.rateSeq
	r.store.rateLimits = append(r.store.rateLimits, *rule)
	return nil
}

// Update 更新规则。
func (r *rateLimitRepo) Update(ctx context.Context, rule *model.RateLimitRule) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	for i := range r.store.rateLimits {
		if r.store.rateLimits[i].Id == rule.Id {
			r.store.rateLimits[i] = *rule
			return nil
		}
	}
	return nil
}

// Delete 删除规则。
func (r *rateLimitRepo) Delete(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	kept := make([]model.RateLimitRule, 0, len(r.store.rateLimits))
	for _, item := range r.store.rateLimits {
		if item.Id != id {
			kept = append(kept, item)
		}
	}
	r.store.rateLimits = kept
	return nil
}
