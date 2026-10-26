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
