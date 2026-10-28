package service

// 本文件是「WAF 配置」板块的**文本解析与校验层**（纯函数，不碰文件系统）。
//
// 为什么要自己按行解析，而不解析整个 Lua 文件：
//   - config.lua 里有大量注释、空行、以及 `[[ ... ]]` 长字符串，用现成 Lua 解析器代价大且容易误伤；
//   - 后台只负责「把某个 config_xxx 的值换掉」，其余内容必须原样保留（含注释与顺序），
//     这样运维直接在服务器上维护的文件也不会被后台改乱；
//   - 写回后仍需 lua 自己加载，所以这里只做「按行替换 + 值校验」，不做语义求值。
//
// 支持两种形态：
//  1. 单行：`config_xxx = "value", -- 注释`
//  2. 长字符串：`config_xxx = [[` … 多行内容 … `]],`
//     （目前用于 config_output_html 拦截页模板）

import (
	"regexp"
	"strconv"
	"strings"
)

// luaLinePattern 匹配单行赋值：缩进 + 键 + `=` + 值 + 可选逗号 + 可选行尾注释。
var luaLinePattern = regexp.MustCompile(`^(\s*)(config_[A-Za-z0-9_]+)(\s*=\s*)(.+?)(,?)(\s*(?:--.*)?)$`)

// luaBlockStartPattern 匹配长字符串的起始行：`config_x = [[` 或 `config_x = [[内容`。
var luaBlockStartPattern = regexp.MustCompile(`^(\s*)(config_[A-Za-z0-9_]+)(\s*=\s*)\[\[(.*)$`)

// luaBlockEndPattern 匹配长字符串的结束行：`]],` 或 `]] -- 注释`。
var luaBlockEndPattern = regexp.MustCompile(`^(\s*)\]\](,?)(\s*(?:--.*)?)$`)

// ratePattern 校验「次数/秒」形式的限速配置，例如 300/60。
var ratePattern = regexp.MustCompile(`^\d+/\d+$`)

// parseLuaValues 解析文件内容，返回「键 → 值（已去引号）」与「出现的键集合」。
// 长字符串会去掉首尾的 `[[` / `]]` 并保留内部换行。
func parseLuaValues(content string) (map[string]string, map[string]struct{}) {
	values := make(map[string]string, 64)
	keys := make(map[string]struct{}, 64)

	lines := strings.Split(content, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// 1) 长字符串块
		if matched := luaBlockStartPattern.FindStringSubmatch(line); matched != nil {
			key := matched[2]
			keys[key] = struct{}{}
			parts := make([]string, 0, 32)
			if first := matched[4]; strings.TrimSpace(first) != "" {
				parts = append(parts, first)
			}
			closed := false
			for j := i + 1; j < len(lines); j++ {
				if luaBlockEndPattern.MatchString(lines[j]) {
					i = j
					closed = true
					break
				}
				parts = append(parts, lines[j])
			}
			// 未闭合时也把已读到的内容当作值，避免整份配置解析失败
			values[key] = strings.Join(parts, "\n")
			if !closed {
				return values, keys
			}
			continue
		}

		// 2) 单行赋值
		matched := luaLinePattern.FindStringSubmatch(line)
		if matched == nil {
			continue
		}
		key := matched[2]
		keys[key] = struct{}{}
		values[key] = unquoteLua(strings.TrimSpace(matched[4]))
	}

	return values, keys
}

// rewriteLuaValues 把 values 里的键值替换进原文，返回新内容与替换条数。
//
// 替换规则：
//   - 单行项：保留原缩进、键名、等号、行尾逗号与注释，只替换值；原有引号风格（" 或 '）保持不变，
//     文本/开关/枚举类若原本没有引号则补双引号（lua 里字符串必须带引号）。
//   - 长字符串项（code 类型）：保留起始行的 `config_x = [[` 与结束行的 `]],`，
//     只把中间内容换成新值（按 \n 拆成多行，缩进跟随原文件首行）。
//   - 多行模板类字段（非 code 类型）跳过，避免误伤。
func rewriteLuaValues(content string, values map[string]string, fields map[string]luaField) (string, int) {
	if len(values) == 0 {
		return content, 0
	}

	lines := strings.Split(content, "\n")
	applied := 0

	for i := 0; i < len(lines); i++ {
		// 1) 长字符串块：只有 code 类型允许改写
		if matched := luaBlockStartPattern.FindStringSubmatch(lines[i]); matched != nil {
			key := matched[2]
			value, ok := values[key]
			field, known := fields[key]
			if !ok || !known || field.Type != luaTypeCode {
				// 跳过整个块，避免把值写到错误的位置
				for j := i + 1; j < len(lines); j++ {
					if luaBlockEndPattern.MatchString(lines[j]) {
						i = j
						break
					}
				}
				continue
			}

			end := -1
			for j := i + 1; j < len(lines); j++ {
				if luaBlockEndPattern.MatchString(lines[j]) {
					end = j
					break
				}
			}
			if end < 0 {
				continue
			}

			// 用结束行的缩进作为内容缩进基准，保持文件风格一致
			indent := luaBlockEndPattern.FindStringSubmatch(lines[end])[1]
			body := make([]string, 0, 32)
			for _, piece := range strings.Split(value, "\n") {
				if strings.TrimSpace(piece) == "" {
					body = append(body, "")
					continue
				}
				body = append(body, indent+piece)
			}

			replacement := append([]string{matched[1] + matched[2] + matched[3] + "[["}, body...)
			replacement = append(replacement, lines[end])

			newLines := make([]string, 0, len(lines))
			newLines = append(newLines, lines[:i]...)
			newLines = append(newLines, replacement...)
			newLines = append(newLines, lines[end+1:]...)

			lines = newLines
			i += len(replacement) - 1
			applied++
			continue
		}

		// 2) 单行赋值
		matched := luaLinePattern.FindStringSubmatch(lines[i])
		if matched == nil {
			continue
		}
		key := matched[2]
		value, ok := values[key]
		if !ok {
			continue
		}
		field, known := fields[key]
		if known && field.Type == luaTypeCode {
			continue
		}

		oldValue := strings.TrimSpace(matched[4])
		if strings.HasPrefix(oldValue, "[[") {
			// 单行写法的长字符串交给上面的分支处理
			continue
		}

		quote := ""
		if len(oldValue) > 0 && (oldValue[0] == '"' || oldValue[0] == '\'') {
			quote = string(oldValue[0])
		}
		if quote == "" && known {
			switch field.Type {
			case luaTypeText, luaTypeSecret, luaTypeSwitch, luaTypeSelect:
				quote = `"`
			}
		}

		rendered := value
		if quote != "" {
			rendered = quote + escapeLua(value, quote) + quote
		}
		lines[i] = matched[1] + matched[2] + matched[3] + rendered + matched[5] + matched[6]
		applied++
	}

	return strings.Join(lines, "\n"), applied
}

// unquoteLua 去掉 Lua 字符串两侧的引号并还原转义。
func unquoteLua(raw string) string {
	if len(raw) >= 2 {
		if (raw[0] == '"' && raw[len(raw)-1] == '"') || (raw[0] == '\'' && raw[len(raw)-1] == '\'') {
			inner := raw[1 : len(raw)-1]
			inner = strings.ReplaceAll(inner, `\"`, `"`)
			inner = strings.ReplaceAll(inner, `\'`, `'`)
			inner = strings.ReplaceAll(inner, `\\`, `\`)
			return inner
		}
	}
	return raw
}

// escapeLua 按 Lua 字符串规则转义内容（反斜杠与引号）。
func escapeLua(value, quote string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, quote, `\`+quote)
	return value
}

// normalizeLuaValue 按字段类型校验并归一化取值，返回可直接写入文件的值。
func normalizeLuaValue(field luaField, raw string) (string, error) {
	// 代码/模板类保留原始换行，仅去掉首尾空白
	if field.Type == luaTypeCode {
		value := strings.Trim(raw, "\r\n")
		if strings.Contains(value, "]]") {
			return "", ErrInvalid.New("%s 不能包含 ]]（会破坏 Lua 长字符串）", field.Label)
		}
		if strings.TrimSpace(value) == "" {
			return "", ErrInvalid.New("%s 不能为空", field.Label)
		}
		return value, nil
	}

	value := strings.TrimSpace(raw)

	switch field.Type {
	case luaTypeSwitch:
		if value != "on" && value != "off" {
			return "", ErrInvalid.New("%s 只能是 on 或 off", field.Label)
		}
		return value, nil

	case luaTypeNumber:
		if value == "" {
			return "", ErrInvalid.New("%s 不能为空", field.Label)
		}
		number, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return "", ErrInvalid.New("%s 必须是数字", field.Label)
		}
		if field.Max > 0 && number > field.Max {
			return "", ErrInvalid.New("%s 不能大于 %v", field.Label, field.Max)
		}
		if number < field.Min {
			return "", ErrInvalid.New("%s 不能小于 %v", field.Label, field.Min)
		}
		// 整数去掉小数点，保持配置文件整洁
		if number == float64(int64(number)) {
			return strconv.FormatInt(int64(number), 10), nil
		}
		return strconv.FormatFloat(number, 'f', -1, 64), nil

	case luaTypeSelect:
		for _, option := range field.Options {
			if option == value {
				return value, nil
			}
		}
		return "", ErrInvalid.New("%s 取值必须是 %s 之一", field.Label, strings.Join(field.Options, " / "))

	case luaTypeSecret, luaTypeText:
		if strings.Contains(value, "[[") {
			return "", ErrInvalid.New("%s 不支持多行内容", field.Label)
		}
		if field.Key == "config_cc_rate" || field.Key == "config_ipcc_rate" {
			if !ratePattern.MatchString(value) {
				return "", ErrInvalid.New("%s 格式应为 次数/秒，例如 300/60", field.Label)
			}
		}
		return value, nil

	default:
		return value, nil
	}
}

// sortStrings 简单插入排序，避免为几处排序引入额外依赖。
func sortStrings(items []string) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j] < items[j-1]; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
