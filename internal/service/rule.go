package service

import (
	"context"
	"regexp"
	"strings"

	"wafadmin/internal/model"
	"wafadmin/internal/nginx"
	"wafadmin/internal/repository"
)

// maxRuleItemLength 与数据库字段长度保持一致。
const maxRuleItemLength = 255

// RuleTypeView 是规则类型的子菜单视图，附带该类型的规则数量。
type RuleTypeView struct {
	model.RuleType
	Count int64 `json:"count"`
}

// RulePayload 是规则新增/修改的入参。
type RulePayload struct {
	RuleType string `json:"rule_type"`
	RuleItem string `json:"rule_item"`
}

// RuleService 负责 WAF 规则的增删改查与下发。
type RuleService struct {
	Rules   repository.RuleRepository
	Catalog model.RuleCatalog
	Nginx   *nginx.Manager
	// Logs 用于统计规则命中次数。
	Logs repository.LogRepository
}

// Overview 返回按类型分组的规则数量，用于生成规则子菜单。
func (s *RuleService) Overview(ctx context.Context) ([]RuleTypeView, error) {
	counts, err := s.Rules.CountByType(ctx)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计规则数量失败")
	}

	views := make([]RuleTypeView, 0, len(s.Catalog.All()))
	for _, ruleType := range s.Catalog.All() {
		views = append(views, RuleTypeView{RuleType: ruleType, Count: counts[ruleType.Key]})
	}
	return views, nil
}

// List 按类型与关键词查询规则。
func (s *RuleService) List(ctx context.Context, ruleType, keyword string) ([]model.Rule, error) {
	ruleType = strings.TrimSpace(ruleType)
	if ruleType != "" && !s.Catalog.Known(ruleType) {
		return nil, ErrInvalid.New("未知的规则类型: %s", ruleType)
	}

	items, err := s.Rules.List(ctx, ruleType)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "查询规则失败")
	}
	if strings.TrimSpace(keyword) == "" {
		return items, nil
	}

	needle := strings.ToLower(strings.TrimSpace(keyword))
	matched := make([]model.Rule, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.RuleItem), needle) {
			matched = append(matched, item)
		}
	}
	return matched, nil
}

// Create 新增规则。
func (s *RuleService) Create(ctx context.Context, payload RulePayload) (*model.Rule, error) {
	ruleType := strings.TrimSpace(payload.RuleType)
	if !s.Catalog.Known(ruleType) {
		return nil, ErrInvalid.New("未知的规则类型: %s", ruleType)
	}
	item, err := s.validateItem(payload.RuleItem)
	if err != nil {
		return nil, err
	}

	rule := &model.Rule{RuleType: ruleType, RuleItem: item, Enabled: true}
	if err := s.Rules.Create(ctx, rule); err != nil {
		return nil, ErrInternal.Wrap(err, "新增规则失败")
	}
	return rule, nil
}

// Update 修改规则内容。
// SetEnabled 启用或停用一条规则。
// 停用后规则保留在库中，但不会下发到规则文件，因此对 lua 侧不再生效。
func (s *RuleService) SetEnabled(ctx context.Context, id int64, enabled bool) (*model.Rule, error) {
	if id <= 0 {
		return nil, ErrInvalid.New("规则 ID 非法")
	}
	rule, err := s.Rules.Get(ctx, id)
	if err != nil {
		return nil, ErrNotFound.New("规则不存在")
	}

	if err := s.Rules.SetEnabled(ctx, id, enabled); err != nil {
		return nil, ErrInternal.Wrap(err, "更新规则状态失败")
	}
	rule.Enabled = enabled
	return rule, nil
}

// TagStats 返回带命中次数的规则列表（按命中次数倒序）。
// 命中次数来自日志的 rule_tag，用于发现高频规则与疑似误杀。
func (s *RuleService) TagStats(ctx context.Context, days int, limit int) ([]RuleHitStat, error) {
	if days < 0 {
		days = 0
	}
	if days == 0 {
		days = 7
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	stats, err := s.Logs.TagStats(ctx, days, limit)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计规则命中失败")
	}

	result := make([]RuleHitStat, 0, len(stats))
	for _, item := range stats {
		result = append(result, RuleHitStat{RuleTag: item.RuleTag, Count: item.Count})
	}
	return result, nil
}

// RuleHitStat 是单条规则的命中统计。
type RuleHitStat struct {
	// RuleTag 为日志中的命中标识。
	RuleTag string `json:"rule_tag"`
	// Count 为命中次数。
	Count int64 `json:"count"`
}

func (s *RuleService) Update(ctx context.Context, id int64, item string) (*model.Rule, error) {
	rule, err := s.Rules.Get(ctx, id)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "查询规则失败")
	}
	if rule == nil {
		return nil, ErrNotFound.New("规则不存在")
	}

	validated, err := s.validateItem(item)
	if err != nil {
		return nil, err
	}
	rule.RuleItem = validated
	if err := s.Rules.Update(ctx, rule); err != nil {
		return nil, ErrInvalid.Wrap(err, "更新规则失败")
	}
	return rule, nil
}

// Delete 删除规则。
func (s *RuleService) Delete(ctx context.Context, id int64) error {
	if err := s.Rules.Delete(ctx, id); err != nil {
		return ErrInternal.Wrap(err, "删除规则失败")
	}
	return nil
}

// Seed 在规则表为空时写入内置规则，返回写入条数。
func (s *RuleService) Seed(ctx context.Context) (int, error) {
	total, err := s.Rules.Count(ctx)
	if err != nil {
		return 0, ErrInternal.Wrap(err, "统计规则失败")
	}
	if total > 0 {
		return 0, nil
	}

	rules := make([]model.Rule, len(model.DefaultRules))
	copy(rules, model.DefaultRules)
	if err := s.Rules.BatchCreate(ctx, rules); err != nil {
		return 0, ErrInternal.Wrap(err, "初始化内置规则失败")
	}
	return len(rules), nil
}

// Grouped 返回按类型分组的全部规则，用于下发。
func (s *RuleService) Grouped(ctx context.Context) (map[string][]model.Rule, error) {
	items, err := s.Rules.List(ctx, "")
	if err != nil {
		return nil, ErrInternal.Wrap(err, "查询规则失败")
	}

	grouped := make(map[string][]model.Rule, len(s.Catalog.All()))
	for _, ruleType := range s.Catalog.All() {
		grouped[ruleType.Key] = make([]model.Rule, 0, 8)
	}
	for _, item := range items {
		grouped[item.RuleType] = append(grouped[item.RuleType], item)
	}
	return grouped, nil
}

// Validate 校验规则内容并返回格式告警（不阻断保存）。
func (s *RuleService) Validate(item string) (warning string, err error) {
	_, err = s.validateItem(item)
	if err != nil {
		return "", err
	}
	if _, compileErr := regexp.Compile(item); compileErr != nil {
		return "该表达式无法被 Go 正则引擎编译，若为 PCRE 特有语法（如断言、逆向引用）请在 openresty 侧验证", nil
	}
	return "", nil
}

// validateItem 校验规则内容。
func (s *RuleService) validateItem(raw string) (string, error) {
	item := strings.TrimSpace(raw)
	if item == "" {
		return "", ErrInvalid.New("规则内容不能为空")
	}
	if len([]rune(item)) > maxRuleItemLength {
		return "", ErrInvalid.New("规则内容长度不能超过 %d 个字符", maxRuleItemLength)
	}
	return item, nil
}
