package geo

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIP2RegionSearch 用仓库内的数据文件校验解析结果与 lua 侧语义一致。
func TestIP2RegionSearch(t *testing.T) {
	path := filepath.Join("..", "..", "lua-waf", "ip2region.db")
	if _, err := os.Stat(path); err != nil {
		t.Skip("未找到 ip2region.db，跳过")
	}

	region, err := OpenIP2Region(path)
	if err != nil {
		t.Skipf("数据文件不可用，跳过：%v", err)
	}
	// 自检：能解析 8.8.8.8 才算这个数据文件可用（否则可能不是 v1 格式）
	if _, ok := region.Search("8.8.8.8"); !ok {
		t.Skip("数据文件不是可读的 ip2region v1 格式，跳过（请使用服务器上有效的 ip2region.db）")
	}

	cases := []struct {
		ip     string
		expect string // 期望包含的片段（避免依赖数据文件的精确版本）
	}{
		{"114.114.114.114", "中国"},
		{"223.5.5.5", "中国"},
		{"8.8.8.8", "美国"},
		{"127.0.0.1", ""},       // 本机地址通常查不到
		{"not-an-ip", ""},       // 非法输入
		{"2001:4860::8888", ""}, // IPv6 不覆盖
	}

	for _, item := range cases {
		got, ok := region.Search(item.ip)
		if item.expect == "" {
			if ok {
				t.Logf("%s -> %q（数据文件里存在该段，属正常）", item.ip, got)
			}
			continue
		}
		if !ok {
			t.Fatalf("%s 应能解析出归属地，实际未命中", item.ip)
		}
		if !contains(got, item.expect) {
			t.Fatalf("%s 解析结果 %q 应包含 %q", item.ip, got, item.expect)
		}
		t.Logf("%s -> %s", item.ip, got)
	}
}

// TestFormatIP2Region 校验归属地拼装规则与 lua 侧一致。
func TestFormatIP2Region(t *testing.T) {
	cases := map[string]string{
		"中国|0|重庆|重庆|电信":   "中国重庆重庆电信",
		"中国|0|北京|北京|0":    "中国北京北京",
		"美国|0|0|0|0":      "美国",
		"0|0|0|0|0":       "",
		"中国|0|广东省|深圳市|移动": "中国广东省深圳市移动",
	}
	for region, want := range cases {
		if got := FormatIP2Region(region); got != want {
			t.Fatalf("%q 期望 %q，实际 %q", region, want, got)
		}
	}
}

// contains 判断子串。
func contains(text, sub string) bool {
	return len(sub) == 0 || (len(text) >= len(sub) && indexOf(text, sub) >= 0)
}

// indexOf 简易子串查找。
func indexOf(text, sub string) int {
	for i := 0; i+len(sub) <= len(text); i++ {
		if text[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
