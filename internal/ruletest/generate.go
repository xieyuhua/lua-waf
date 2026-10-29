package ruletest

// 本文件实现「从正则反推一条能命中它的样例」。
//
// 为什么需要：光靠内置攻击样例集判断「规则能不能命中」会误伤——很多规则本身是有效的，
// 只是它针对的载荷形态没写进样例集（比如 `into\s+outfile`、`base64_decode\(`）。
// 真正能证明一条规则「永远不会命中」的方法，是按规则自己生成一条它应当匹配的字符串，
// 如果连这条都匹配不上，那这条规则就是死的（典型原因：转义多写一层、括号不配对、
// 大小写标志写进了内容里）。
//
// 生成器只覆盖 PCRE 的常用构造（字面量、转义类、分组、交替、量词、字符类、锚点、内联标志），
// 遇到无法推断的构造返回 ok=false，此时跳过判定（不误报）。

import (
	"strings"
)

// SampleFromRegex 按正则生成一条「应当能命中」的样例字符串。
//
// 生成策略：取每个原子的最小可能形式（量词按最小次数展开、交替取第一个分支），
// 因此生成结果是一个合法实例而不是最长匹配。
func SampleFromRegex(pattern string) (string, bool) {
	gen := &generator{input: pattern}
	return gen.parse(false)
}

// generator 是正则到样例的生成器。
type generator struct {
	input string
	pos   int
}

// parse 解析到字符串结尾或遇到 delim 指定的结束符。
func (g *generator) parse(insideGroup bool) (string, bool) {
	var builder strings.Builder

	for g.pos < len(g.input) {
		char := g.input[g.pos]

		switch char {
		case ')':
			if insideGroup {
				g.pos++
				return builder.String(), true
			}
			// 多余的右括号：跳过，交给编译阶段报错
			g.pos++
		case '|':
			// 交替：第一个分支已生成，跳过其余分支到组结束
			g.pos++
			g.skipAlternatives(insideGroup)
			return builder.String(), true
		case '^', '$':
			// 锚点不产生字符
			g.pos++
		case '(':
			text, ok := g.parseGroup()
			if !ok {
				return "", false
			}
			builder.WriteString(text)
		case '[':
			text, ok := g.parseClass()
			if !ok {
				return "", false
			}
			text, ok = g.applyQuantifier(text)
			if !ok {
				return "", false
			}
			builder.WriteString(text)
		case '\\':
			text, ok := g.parseEscape()
			if !ok {
				return "", false
			}
			text, ok = g.applyQuantifier(text)
			if !ok {
				return "", false
			}
			builder.WriteString(text)
		case '.', '?', '*', '+', '{':
			if char == '.' {
				g.pos++
				text, ok := g.applyQuantifier("a")
				if !ok {
					return "", false
				}
				builder.WriteString(text)
				continue
			}
			// 裸量词（前面没有原子）：跳过，交给编译阶段处理
			g.pos++
		default:
			g.pos++
			text, ok := g.applyQuantifier(string(char))
			if !ok {
				return "", false
			}
			builder.WriteString(text)
		}
	}

	if insideGroup {
		// 组没有闭合
		return "", false
	}
	return builder.String(), true
}

// parseGroup 解析 (...) 分组，返回组内生成的字符串。
func (g *generator) parseGroup() (string, bool) {
	g.pos++ // 跳过 '('

	// 处理内联标志与特殊分组前缀，如 (?: (?i) (?= (?! (?<= (?<name>
	if g.pos < len(g.input) && g.input[g.pos] == '?' {
		// 跳过 '?' 与紧随的修饰符
		g.pos++
		for g.pos < len(g.input) {
			char := g.input[g.pos]
			if char == ':' || char == '=' || char == '!' || char == '<' || char == 'P' || char == '\'' {
				g.pos++
				// (?<name> 里的 name 需要跳过
				if char == '<' || char == '\'' {
					for g.pos < len(g.input) && g.input[g.pos] != '>' && g.input[g.pos] != '\'' {
						g.pos++
					}
					if g.pos < len(g.input) {
						g.pos++
					}
				}
				break
			}
			if strings.ContainsRune("imsxU-", rune(char)) {
				g.pos++
				continue
			}
			break
		}
	}

	text, ok := g.parse(true)
	if !ok {
		return "", false
	}

	// 组后面可能跟量词
	return g.applyQuantifier(text)
}

// parseClass 解析 [...] 字符类，返回一个属于该类的字符。
func (g *generator) parseClass() (string, bool) {
	start := g.pos
	g.pos++ // 跳过 '['

	negated := false
	if g.pos < len(g.input) && g.input[g.pos] == '^' {
		negated = true
		g.pos++
	}

	var first string
	for g.pos < len(g.input) && g.input[g.pos] != ']' {
		char := g.input[g.pos]
		if char == '\\' && g.pos+1 < len(g.input) {
			next := g.input[g.pos+1]
			g.pos += 2
			if first == "" {
				first = singleCharForEscape(next)
				if first == "" {
					// \d \w \s 这类
					first = classCharForEscape(next)
				}
			}
			// 跳过区间后半段
			if g.pos < len(g.input) && g.input[g.pos] == '-' && g.pos+1 < len(g.input) && g.input[g.pos+1] != ']' {
				g.pos += 2
			}
			continue
		}

		// 处理 a-z 区间
		if g.pos+2 < len(g.input) && g.input[g.pos+1] == '-' && g.input[g.pos+2] != ']' {
			if first == "" {
				first = string(char)
			}
			g.pos += 3
			continue
		}

		if first == "" {
			first = string(char)
		}
		g.pos++
	}

	if g.pos >= len(g.input) {
		// 没有闭合的字符类：无法生成
		g.pos = start
		return "", false
	}
	g.pos++ // 跳过 ']'

	if first == "" {
		return "", false
	}
	if negated {
		// 取反的字符类：返回一个大概率不在类里的字符
		if first != "a" {
			return "a", true
		}
		return "b", true
	}
	return first, true
}

// parseEscape 解析反斜杠转义，返回它代表的字符。
func (g *generator) parseEscape() (string, bool) {
	if g.pos+1 >= len(g.input) {
		return "", false
	}

	next := g.input[g.pos+1]
	g.pos += 2

	if text := singleCharForEscape(next); text != "" {
		return text, true
	}
	if text := classCharForEscape(next); text != "" {
		return text, true
	}

	// \b \B \A \z 等零宽断言
	if strings.ContainsRune("bBAzZ", rune(next)) {
		return "", true
	}

	// \x20 \u4e2d \p{L} \Q..\E 这类无法推断具体字符的转义：整体放弃，
	// 宁可跳过判定也不能误报「死规则」
	return "", false
}

// singleCharForEscape 返回转义后的字面字符（\\. -> .），不是则返回空串。
func singleCharForEscape(char byte) string {
	switch char {
	case 'n':
		return "\n"
	case 'r':
		return "\r"
	case 't':
		return "\t"
	case 'f':
		return "\f"
	case 'v':
		return "\v"
	case '0':
		return "\x00"
	case 'd', 'D', 'w', 'W', 's', 'S', 'b', 'B', 'A', 'z', 'Z', 'x', 'u', 'p', 'P', 'Q', 'E', 'K', 'G', 'R', 'X':
		return ""
	default:
		return string(char)
	}
}

// classCharForEscape 返回字符类转义的一个代表字符（\d -> 1）。
func classCharForEscape(char byte) string {
	switch char {
	case 'd':
		return "1"
	case 'D':
		return "a"
	case 'w':
		return "a"
	case 'W':
		return "-"
	case 's':
		return " "
	case 'S':
		return "a"
	}
	return ""
}

// applyQuantifier 读取紧跟其后的量词，并按其最小次数重复 text。
func (g *generator) applyQuantifier(text string) (string, bool) {
	if g.pos >= len(g.input) {
		return text, true
	}

	switch g.input[g.pos] {
	case '?':
		g.pos++
		// 可选：出现 0 次即可满足，但为了让字符串更“像攻击载荷”，这里保留一次
		return text, true
	case '*':
		g.pos++
		return text, true
	case '+':
		g.pos++
		return text, true
	case '{':
		end := strings.IndexByte(g.input[g.pos:], '}')
		if end < 0 {
			return "", false
		}
		spec := g.input[g.pos+1 : g.pos+end]
		g.pos += end + 1

		minText := spec
		if comma := strings.IndexByte(spec, ','); comma >= 0 {
			minText = spec[:comma]
		}
		count := 1
		if minText != "" {
			count = 0
			for i := 0; i < len(minText); i++ {
				if minText[i] < '0' || minText[i] > '9' {
					return "", false
				}
				count = count*10 + int(minText[i]-'0')
			}
			if count > 32 {
				count = 32
			}
		}
		return strings.Repeat(text, count), true
	}

	return text, true
}

// skipAlternatives 跳过交替分支，直到组结束（或输入结束）。
func (g *generator) skipAlternatives(insideGroup bool) {
	depth := 0
	for g.pos < len(g.input) {
		switch g.input[g.pos] {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				if insideGroup {
					g.pos++
				}
				return
			}
			depth--
		}
		g.pos++
	}
}
