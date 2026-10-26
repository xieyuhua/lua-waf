package mysql

import (
	"context"
	"fmt"

	"github.com/go-xorm/xorm"

	"wafadmin/internal/model"
	"wafadmin/pkg/sqlwatch"
)

// ruleTypeCount 用于承载分组统计结果。
type ruleTypeCount struct {
	RuleType string `xorm:"'rule_type'"`
	Num      int64  `xorm:"'num'"`
}

// ruleRepo 是 RuleRepository 的 MySQL 实现。
type ruleRepo struct {
	engine   *xorm.Engine
	observer *sqlwatch.Observer
}

// List 按类型查询规则。
func (r *ruleRepo) List(ctx context.Context, ruleType string) ([]model.Rule, error) {
	defer r.observer.Trace(ctx, "rule.list", "SELECT * FROM rules WHERE rule_type = ? ORDER BY id")()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	items := make([]model.Rule, 0, 32)
	session := r.engine.NewSession()
	defer session.Close()
	if ruleType != "" {
		session = session.Where("rule_type = ?", ruleType)
	}
	if err := session.OrderBy("id").Find(&items); err != nil {
		return nil, err
	}
	return items, nil
}

// Get 按主键查询规则。
func (r *ruleRepo) Get(ctx context.Context, id int64) (*model.Rule, error) {
	defer r.observer.Trace(ctx, "rule.get", "SELECT * FROM rules WHERE id = ?")()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rule := new(model.Rule)
	has, err := r.engine.Id(id).Get(rule)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, nil
	}
	return rule, nil
}

// Create 新增规则。
func (r *ruleRepo) Create(ctx context.Context, rule *model.Rule) error {
	defer r.observer.Trace(ctx, "rule.create", "INSERT INTO rules (rule_type, rule_item) VALUES (?, ?)")()
	if err := ctx.Err(); err != nil {
		return err
	}
	affected, err := r.engine.Insert(rule)
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("写入规则失败")
	}
	return nil
}

// Update 更新规则内容。
func (r *ruleRepo) Update(ctx context.Context, rule *model.Rule) error {
	defer r.observer.Trace(ctx, "rule.update", "UPDATE rules SET rule_item = ? WHERE id = ?")()
	if err := ctx.Err(); err != nil {
		return err
	}
	affected, err := r.engine.Id(rule.Id).Cols("rule_item").Update(rule)
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("规则不存在或内容未变化")
	}
	return nil
}

// Delete 删除规则。
func (r *ruleRepo) Delete(ctx context.Context, id int64) error {
	defer r.observer.Trace(ctx, "rule.delete", "DELETE FROM rules WHERE id = ?")()
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := r.engine.Id(id).Delete(new(model.Rule))
	return err
}

// CountByType 返回各规则类型的条数。
func (r *ruleRepo) CountByType(ctx context.Context) (map[string]int64, error) {
	defer r.observer.Trace(ctx, "rule.count_by_type", "SELECT rule_type, COUNT(*) AS num FROM rules GROUP BY rule_type")()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows := make([]ruleTypeCount, 0, 8)
	if err := r.engine.SQL("SELECT rule_type, COUNT(*) AS num FROM rules GROUP BY rule_type").Find(&rows); err != nil {
		return nil, err
	}
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.RuleType] = row.Num
	}
	return counts, nil
}

// Count 返回规则总数。
func (r *ruleRepo) Count(ctx context.Context) (int64, error) {
	defer r.observer.Trace(ctx, "rule.count", "SELECT COUNT(*) FROM rules")()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.engine.Count(new(model.Rule))
}

// BatchCreate 批量写入规则。
func (r *ruleRepo) BatchCreate(ctx context.Context, rules []model.Rule) error {
	defer r.observer.Trace(ctx, "rule.batch_create", "INSERT INTO rules (...) VALUES (...)")()
	if err := ctx.Err(); err != nil {
		return err
	}

	session := r.engine.NewSession()
	defer session.Close()
	if err := session.Begin(); err != nil {
		return err
	}
	for i := range rules {
		if _, err := session.Insert(&rules[i]); err != nil {
			_ = session.Rollback()
			return err
		}
	}
	return session.Commit()
}
