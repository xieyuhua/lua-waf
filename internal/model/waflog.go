package model

// WafLog 是一条 WAF 攻击日志。表名：waf_log
// 由 openresty 侧的 lua 规则命中后写入，后台只读。
type WafLog struct {
	Id         int64  `xorm:"pk autoincr 'id'" json:"id"`
	ClientIp   string `xorm:"varchar(64) 'client_ip'" json:"client_ip"`
	LocalTime  string `xorm:"varchar(32) 'local_time'" json:"local_time"`
	ServerName string `xorm:"varchar(190) 'server_name'" json:"server_name"`
	UserAgent  string `xorm:"varchar(512) 'user_agent'" json:"user_agent"`
	AttackType string `xorm:"varchar(64) 'attack_type'" json:"attack_type"`
	ReqUrl     string `xorm:"varchar(1024) 'req_url'" json:"req_url"`
	Area       string `xorm:"varchar(64)" json:"area"`
	ReqData    string `xorm:"text 'req_data'" json:"req_data"`
	RuleTag    string `xorm:"varchar(190) 'rule_tag'" json:"rule_tag"`
}

// IPStat 是按来源 IP 聚合的攻击统计。
type IPStat struct {
	ClientIp   string `xorm:"'client_ip'" json:"client_ip"`
	Area       string `json:"area"`
	Num        int64  `json:"num"`
	AttackType string `xorm:"'attack_type'" json:"attack_type"`
}

// TypeStat 是按攻击类型聚合的统计。
type TypeStat struct {
	AttackType string `xorm:"'attack_type'" json:"attack_type"`
	Num        int64  `json:"num"`
	Area       string `json:"area"`
}

// DateStat 是按天聚合的攻击趋势。
type DateStat struct {
	Date string `xorm:"'date'" json:"date"`
	Num  int64  `json:"num"`
}

// TableName 固定表名（该表由 openresty 侧写入，后台只读）。
func (WafLog) TableName() string { return "waf_log" }

// TagStat 是按命中规则（rule_tag）聚合的统计条目。
type TagStat struct {
	// RuleTag 为命中规则标识（来自日志的 rule_tag 字段）。
	RuleTag string `json:"rule_tag"`
	// Count 为命中次数。
	Count int64 `json:"count"`
}
