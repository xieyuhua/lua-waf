package model

// Rule 是一条 WAF 规则。表名：rules
type Rule struct {
	Id       int64  `xorm:"pk autoincr 'id'" json:"id"`
	RuleType string `xorm:"varchar(25) not null 'rule_type'" json:"rule_type"`
	RuleItem string `xorm:"varchar(255) not null 'rule_item'" json:"rule_item"`
}

// RuleFileItem 是下发给 openresty 的规则文件结构。
// 字段名必须保持首字母大写（Id/RuleType/RuleItem），
// 因为 lua 侧 util.get_rules 通过 rule["RuleItem"] 取值。
type RuleFileItem struct {
	Id       int64  `json:"Id"`
	RuleType string `json:"RuleType"`
	RuleItem string `json:"RuleItem"`
}

// ToFileItem 转换为规则文件条目。
// TableName 固定表名，避免依赖 xorm 的结构体名推导。
func (Rule) TableName() string { return "rules" }

func (r Rule) ToFileItem() RuleFileItem {
	return RuleFileItem{Id: r.Id, RuleType: r.RuleType, RuleItem: r.RuleItem}
}
