package geo_test

import (
	"testing"

	"wafadmin/pkg/geo"
)

// TestResolveArea 校验地区文本到坐标的解析结果。
func TestResolveArea(t *testing.T) {
	cases := []struct {
		name     string
		area     string
		wantName string
		wantOK   bool
		domestic bool
	}{
		{name: "中文短横线分隔", area: "中国-广东-深圳", wantName: "深圳", wantOK: true, domestic: true},
		{name: "ip2region 竖线分隔", area: "中国|0|广东省|杭州市|电信", wantName: "杭州", wantOK: true, domestic: true},
		{name: "海外国家", area: "美国-弗吉尼亚", wantName: "美国", wantOK: true, domestic: false},
		{name: "未知地区", area: "0|0|0", wantName: "", wantOK: false},
		{name: "空字符串", area: "", wantName: "", wantOK: false},
	}

	resolver := geo.Default
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			point, ok := resolver.Resolve(item.area)
			if ok != item.wantOK {
				t.Fatalf("解析 %q 期望 ok=%v，实际 %v", item.area, item.wantOK, ok)
			}
			if !item.wantOK {
				return
			}
			if point.Name != item.wantName {
				t.Fatalf("解析 %q 期望地区 %q，实际 %q", item.area, item.wantName, point.Name)
			}
			if point.Domestic != item.domestic {
				t.Fatalf("解析 %q 境内标识期望 %v，实际 %v", item.area, item.domestic, point.Domestic)
			}
		})
	}
}

// TestResolveReturnsCoordinates 校验命中项带有合法经纬度。
func TestResolveReturnsCoordinates(t *testing.T) {
	point, ok := geo.Default.Resolve("中国-北京")
	if !ok {
		t.Fatal("应能解析北京")
	}
	if point.Lat < -90 || point.Lat > 90 {
		t.Fatalf("纬度越界: %v", point.Lat)
	}
	if point.Lon < -180 || point.Lon > 180 {
		t.Fatalf("经度越界: %v", point.Lon)
	}
}

// TestKnownEntries 校验内置词表非空。
func TestKnownEntries(t *testing.T) {
	if geo.Default.Known() < 50 {
		t.Fatalf("内置地区词表过少: %d", geo.Default.Known())
	}
}

// TestResolveIP2RegionText 校验 ip2region 拼接串（含运营商/云厂商后缀）能落到城市粒度。
func TestResolveIP2RegionText(t *testing.T) {
	cases := []struct {
		area     string
		wantName string
	}{
		{"中国重庆重庆电信", "重庆"},
		{"中国浙江杭州阿里云", "杭州"},
		{"中国重庆重庆移动", "重庆"},
		{"中国北京北京移动/基站WiFi", "北京"},
		{"中国河南郑州移动", "郑州"},
		{"中国香港", "香港"},
		{"美国加利福尼亚州山景城Google", "山景城"},
		{"美国休斯顿", "休斯顿"},
		{"美国伊利诺伊州芝加哥", "芝加哥"},
		{"日本东京都东京Google Cloud", "东京"},
		{"西班牙安达卢西亚自治区博柳略斯帕尔－德尔孔达多", "西班牙"},
		{"印尼西爪哇省万隆", "万隆"},
		{"立陶宛", "立陶宛"},
		{"孟加拉国", "孟加拉国"},
	}

	for _, item := range cases {
		point, ok := geo.Default.Resolve(item.area)
		if !ok {
			t.Fatalf("%q 应能解析", item.area)
		}
		if point.Name != item.wantName {
			t.Fatalf("%q 解析为 %q，期望 %q", item.area, point.Name, item.wantName)
		}
		if point.Lat == 0 && point.Lon == 0 {
			t.Fatalf("%q 坐标为空", item.area)
		}
	}
}

// TestResolveUnlocatable 校验无法定位的占位文本返回 false。
func TestResolveUnlocatable(t *testing.T) {
	for _, area := range []string{"保留", "", "内网IP内网IP", "亚太地区", "GoogleDNS", "未知"} {
		if _, ok := geo.Default.Resolve(area); ok {
			t.Fatalf("%q 不应被解析为具体位置", area)
		}
	}
}

// TestResolvePrefersCityOverCountry 校验城市优先级高于国家。
func TestResolvePrefersCityOverCountry(t *testing.T) {
	point, ok := geo.Default.Resolve("美国加利福尼亚州洛杉矶ColoCrossing")
	if !ok {
		t.Fatal("应能解析")
	}
	if point.Name != "洛杉矶" {
		t.Fatalf("应优先命中城市，实际 %q", point.Name)
	}
	if point.Domestic {
		t.Fatal("美国城市不应标记为境内")
	}
}
