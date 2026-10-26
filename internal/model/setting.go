package model

import "time"

// Setting 是键值型系统配置项（如 AI 大模型接入配置）。表名：setting
type Setting struct {
	ConfigKey   string    `xorm:"pk varchar(64) 'config_key'" json:"config_key"`
	ConfigValue string    `xorm:"text 'config_value'" json:"config_value"`
	Updated     time.Time `xorm:"updated" json:"updated"`
}

// TableName 固定表名，避免依赖 xorm 的结构体名推导。
func (Setting) TableName() string { return "setting" }
