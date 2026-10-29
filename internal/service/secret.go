package service

// 本文件放「脱敏密钥」相关的公共小工具。
//
// 约定：接口返回密钥时一律脱敏为含 "****" 的字符串（如 `abcd****wxyz` 或 `********`）；
// 前端原样提交回表示「不修改」。判定逻辑曾经散落在 4 个文件里，这里统一收口。

import "strings"

// maskPlaceholder 为完全不显示时的占位串。
const maskPlaceholder = "********"

// containsMask 判断一个值是否为脱敏值（提交它表示保持原值不变）。
func containsMask(value string) bool {
	return strings.Contains(value, "****")
}
