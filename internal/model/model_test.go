package model_test

import (
	"reflect"
	"strings"
	"testing"

	"wafadmin/internal/model"
)

// TestPrimaryKeyTags 校验主键标签使用 xorm 认可的关键字。
// xorm 只认 autoincr，写成 autoincrement 会被当成列名（曾导致主键列变成 autoincrement）。
func TestPrimaryKeyTags(t *testing.T) {
	beams := []struct {
		name string
		bean any
	}{
		{name: "Site", bean: model.Site{}},
		{name: "Rule", bean: model.Rule{}},
		{name: "User", bean: model.User{}},
		{name: "SlowSQL", bean: model.SlowSQL{}},
		{name: "WafLog", bean: model.WafLog{}},
	}

	for _, item := range beams {
		t.Run(item.name, func(t *testing.T) {
			field, ok := reflect.TypeOf(item.bean).FieldByName("Id")
			if !ok {
				t.Fatalf("%s 缺少 Id 字段", item.name)
			}

			tag := field.Tag.Get("xorm")
			if strings.Contains(tag, "autoincrement") {
				t.Fatalf("%s 的主键标签含非法关键字 autoincrement（应为 autoincr）：%s", item.name, tag)
			}
			if !strings.Contains(tag, "pk") {
				t.Fatalf("%s 的 Id 未声明主键：%s", item.name, tag)
			}
			if !strings.Contains(tag, "autoincr") {
				t.Fatalf("%s 的 Id 未声明自增：%s", item.name, tag)
			}
			if !strings.Contains(tag, "'id'") {
				t.Fatalf("%s 的 Id 未固定列名为 id：%s", item.name, tag)
			}
		})
	}
}

// TestTableNames 校验表名固定，不依赖结构体名推导。
func TestTableNames(t *testing.T) {
	cases := []struct {
		bean any
		want string
	}{
		{bean: model.Site{}, want: "site"},
		{bean: model.Rule{}, want: "rules"},
		{bean: model.User{}, want: "user"},
		{bean: model.SlowSQL{}, want: "slow_sql"},
		{bean: model.Setting{}, want: "setting"},
		{bean: model.WafLog{}, want: "waf_log"},
	}

	for _, item := range cases {
		namer, ok := item.bean.(interface{ TableName() string })
		if !ok {
			t.Fatalf("%T 未实现 TableName()", item.bean)
		}
		if got := namer.TableName(); got != item.want {
			t.Fatalf("%T 表名 = %s，期望 %s", item.bean, got, item.want)
		}
	}
}
