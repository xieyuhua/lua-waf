package memory

import (
	"context"
	"sort"

	"wafadmin/internal/model"
)

// ruleRepo 是 RuleRepository 的内存实现。
type ruleRepo struct {
	store *Store
}

// List 按类型查询规则，ruleType 为空时返回全部。
func (r *ruleRepo) List(ctx context.Context, ruleType string) ([]model.Rule, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	items := make([]model.Rule, 0, len(r.store.rules))
	for _, item := range r.store.rules {
		if ruleType != "" && item.RuleType != ruleType {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Id < items[j].Id })
	return items, nil
}

// Get 按主键查询规则。
func (r *ruleRepo) Get(ctx context.Context, id int64) (*model.Rule, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	for _, item := range r.store.rules {
		if item.Id == id {
			found := item
			return &found, nil
		}
	}
	return nil, nil
}

// Create 新增规则。
func (r *ruleRepo) Create(ctx context.Context, rule *model.Rule) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	r.store.ruleSeq++
	rule.Id = r.store.ruleSeq
	r.store.rules = append(r.store.rules, *rule)
	return nil
}

// Update 更新规则内容。
func (r *ruleRepo) Update(ctx context.Context, rule *model.Rule) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	for i := range r.store.rules {
		if r.store.rules[i].Id == rule.Id {
			r.store.rules[i].RuleType = rule.RuleType
			r.store.rules[i].RuleItem = rule.RuleItem
			return nil
		}
	}
	return nil
}

// Delete 删除规则。
func (r *ruleRepo) Delete(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	kept := make([]model.Rule, 0, len(r.store.rules))
	for _, item := range r.store.rules {
		if item.Id != id {
			kept = append(kept, item)
		}
	}
	r.store.rules = kept
	return nil
}

// CountByType 返回各规则类型的条数。
func (r *ruleRepo) CountByType(ctx context.Context) (map[string]int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	counts := make(map[string]int64, 8)
	for _, item := range r.store.rules {
		counts[item.RuleType]++
	}
	return counts, nil
}

// Count 返回规则总数。
func (r *ruleRepo) Count(ctx context.Context) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	return int64(len(r.store.rules)), nil
}

// BatchCreate 批量写入规则。
func (r *ruleRepo) BatchCreate(ctx context.Context, rules []model.Rule) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	for _, item := range rules {
		if item.Id > 0 {
			r.store.rules = append(r.store.rules, item)
			if item.Id > r.store.ruleSeq {
				r.store.ruleSeq = item.Id
			}
			continue
		}
		r.store.ruleSeq++
		item.Id = r.store.ruleSeq
		r.store.rules = append(r.store.rules, item)
	}
	return nil
}
