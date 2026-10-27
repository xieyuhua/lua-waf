// Package geo 把 ip2region 的地区文本解析为经纬度，供攻击地图大屏使用。
package geo

import (
	"strings"
)

// Point 是地图上的一个地理坐标。
type Point struct {
	// Name 为展示名，例如「深圳」「美国」。
	Name string
	// Lat / Lon 为纬度与经度。
	Lat float64
	Lon float64
	// Domestic 表示是否为中国境内。
	Domestic bool
}

// entry 是内置的地区关键字与坐标映射。
type entry struct {
	// keyword 为地区文本中包含的关键字。
	keyword string
	point   Point
}

// Resolver 按内置词表解析地区文本。
//
// 词表分三层，依次匹配：具体城市 → 省/州 → 国家，
// 这样「美国加利福尼亚州山景城Google」会命中「山景城」而不是「美国」。
type Resolver struct {
	cities    []entry
	regions   []entry
	countries []entry
}

// Default 是内置解析器（城市优先、省/州次之、国家兜底）。
var Default = Resolver{
	cities:    append(append([]entry{}, cities...), entries...),
	regions:   regions,
	countries: countries,
}

// Resolve 解析地区文本，未命中时返回 false。
//
// area 常见形式为 ip2region 的拼接串（如「中国重庆重庆电信」），
// 解析前会去掉运营商与云厂商后缀；「保留 / 内网IP」等占位值视为无法定位。
func (r Resolver) Resolve(area string) (Point, bool) {
	normalized := normalize(area)
	if normalized == "" || isUnknownArea(normalized) {
		return Point{}, false
	}
	cleaned := stripProvider(normalized)

	for _, list := range [][]entry{r.cities, r.regions, r.countries} {
		for _, item := range list {
			if item.keyword != "" && strings.Contains(cleaned, item.keyword) {
				return item.point, true
			}
		}
	}
	return Point{}, false
}

// isUnknownArea 判断是否为无法定位的占位文本。
func isUnknownArea(area string) bool {
	// 内网地址可能是重复拼接（如「内网IP内网IP」），按前缀判断
	if strings.HasPrefix(area, "内网") || strings.HasPrefix(area, "局域网") {
		return true
	}
	for _, item := range unknownAreas {
		if area == item {
			return true
		}
	}
	return false
}

// stripProvider 去掉运营商 / 云厂商后缀（长词优先匹配）。
func stripProvider(area string) string {
	out := area
	for _, suffix := range providerSuffixes {
		out = strings.ReplaceAll(out, suffix, " ")
	}
	return strings.Join(strings.Fields(out), " ")
}

// Known 表示当前词表条目数，便于测试断言。
func (r Resolver) Known() int { return len(r.cities) + len(r.regions) + len(r.countries) }

// normalize 统一分隔符，便于关键字匹配。
func normalize(area string) string {
	replacer := strings.NewReplacer("|", " ", "-", " ", "/", " ", "\t", " ", "·", " ")
	return strings.TrimSpace(replacer.Replace(area))
}

// entries 是内置地区坐标词表（先城市、后国家）。
var entries = []entry{
	// ---------- 中国大陆主要城市 ----------
	{"北京", Point{Name: "北京", Lat: 39.90, Lon: 116.41, Domestic: true}},
	{"上海", Point{Name: "上海", Lat: 31.23, Lon: 121.47, Domestic: true}},
	{"广州", Point{Name: "广州", Lat: 23.13, Lon: 113.26, Domestic: true}},
	{"深圳", Point{Name: "深圳", Lat: 22.54, Lon: 114.06, Domestic: true}},
	{"杭州", Point{Name: "杭州", Lat: 30.27, Lon: 120.15, Domestic: true}},
	{"宁波", Point{Name: "宁波", Lat: 29.87, Lon: 121.55, Domestic: true}},
	{"南京", Point{Name: "南京", Lat: 32.06, Lon: 118.80, Domestic: true}},
	{"苏州", Point{Name: "苏州", Lat: 31.30, Lon: 120.58, Domestic: true}},
	{"成都", Point{Name: "成都", Lat: 30.57, Lon: 104.07, Domestic: true}},
	{"重庆", Point{Name: "重庆", Lat: 29.56, Lon: 106.55, Domestic: true}},
	{"武汉", Point{Name: "武汉", Lat: 30.59, Lon: 114.31, Domestic: true}},
	{"西安", Point{Name: "西安", Lat: 34.34, Lon: 108.94, Domestic: true}},
	{"天津", Point{Name: "天津", Lat: 39.08, Lon: 117.20, Domestic: true}},
	{"长沙", Point{Name: "长沙", Lat: 28.23, Lon: 112.94, Domestic: true}},
	{"郑州", Point{Name: "郑州", Lat: 34.75, Lon: 113.63, Domestic: true}},
	{"济南", Point{Name: "济南", Lat: 36.65, Lon: 117.12, Domestic: true}},
	{"青岛", Point{Name: "青岛", Lat: 36.07, Lon: 120.38, Domestic: true}},
	{"沈阳", Point{Name: "沈阳", Lat: 41.81, Lon: 123.43, Domestic: true}},
	{"大连", Point{Name: "大连", Lat: 38.91, Lon: 121.61, Domestic: true}},
	{"哈尔滨", Point{Name: "哈尔滨", Lat: 45.80, Lon: 126.53, Domestic: true}},
	{"长春", Point{Name: "长春", Lat: 43.82, Lon: 125.32, Domestic: true}},
	{"石家庄", Point{Name: "石家庄", Lat: 38.04, Lon: 114.51, Domestic: true}},
	{"太原", Point{Name: "太原", Lat: 37.87, Lon: 112.55, Domestic: true}},
	{"合肥", Point{Name: "合肥", Lat: 31.82, Lon: 117.23, Domestic: true}},
	{"福州", Point{Name: "福州", Lat: 26.07, Lon: 119.30, Domestic: true}},
	{"厦门", Point{Name: "厦门", Lat: 24.48, Lon: 118.09, Domestic: true}},
	{"南昌", Point{Name: "南昌", Lat: 28.68, Lon: 115.86, Domestic: true}},
	{"南宁", Point{Name: "南宁", Lat: 22.82, Lon: 108.32, Domestic: true}},
	{"贵阳", Point{Name: "贵阳", Lat: 26.65, Lon: 106.63, Domestic: true}},
	{"昆明", Point{Name: "昆明", Lat: 25.04, Lon: 102.71, Domestic: true}},
	{"海口", Point{Name: "海口", Lat: 20.04, Lon: 110.32, Domestic: true}},
	{"兰州", Point{Name: "兰州", Lat: 36.06, Lon: 103.83, Domestic: true}},
	{"西宁", Point{Name: "西宁", Lat: 36.62, Lon: 101.78, Domestic: true}},
	{"银川", Point{Name: "银川", Lat: 38.49, Lon: 106.23, Domestic: true}},
	{"乌鲁木齐", Point{Name: "乌鲁木齐", Lat: 43.83, Lon: 87.62, Domestic: true}},
	{"呼和浩特", Point{Name: "呼和浩特", Lat: 40.84, Lon: 111.75, Domestic: true}},
	{"拉萨", Point{Name: "拉萨", Lat: 29.65, Lon: 91.14, Domestic: true}},
	{"香港", Point{Name: "香港", Lat: 22.32, Lon: 114.17, Domestic: true}},
	{"澳门", Point{Name: "澳门", Lat: 22.20, Lon: 113.54, Domestic: true}},
	{"台北", Point{Name: "台北", Lat: 25.03, Lon: 121.57, Domestic: true}},
	{"台湾", Point{Name: "台湾", Lat: 23.70, Lon: 120.96, Domestic: true}},

	// ---------- 海外主要国家 / 地区 ----------
	{"美国", Point{Name: "美国", Lat: 37.09, Lon: -95.71}},
	{"俄罗斯", Point{Name: "俄罗斯", Lat: 61.52, Lon: 105.32}},
	{"日本", Point{Name: "日本", Lat: 36.20, Lon: 138.25}},
	{"韩国", Point{Name: "韩国", Lat: 35.91, Lon: 127.77}},
	{"印度", Point{Name: "印度", Lat: 20.59, Lon: 78.96}},
	{"巴西", Point{Name: "巴西", Lat: -14.24, Lon: -51.93}},
	{"英国", Point{Name: "英国", Lat: 55.38, Lon: -3.44}},
	{"德国", Point{Name: "德国", Lat: 51.17, Lon: 10.45}},
	{"法国", Point{Name: "法国", Lat: 46.23, Lon: 2.21}},
	{"荷兰", Point{Name: "荷兰", Lat: 52.13, Lon: 5.29}},
	{"加拿大", Point{Name: "加拿大", Lat: 56.13, Lon: -106.35}},
	{"澳大利亚", Point{Name: "澳大利亚", Lat: -25.27, Lon: 133.78}},
	{"新加坡", Point{Name: "新加坡", Lat: 1.35, Lon: 103.82}},
	{"越南", Point{Name: "越南", Lat: 14.06, Lon: 108.28}},
	{"泰国", Point{Name: "泰国", Lat: 15.87, Lon: 100.99}},
	{"马来西亚", Point{Name: "马来西亚", Lat: 4.21, Lon: 101.98}},
	{"印度尼西亚", Point{Name: "印度尼西亚", Lat: -0.79, Lon: 113.92}},
	{"菲律宾", Point{Name: "菲律宾", Lat: 12.88, Lon: 121.77}},
	{"乌克兰", Point{Name: "乌克兰", Lat: 48.38, Lon: 31.17}},
	{"波兰", Point{Name: "波兰", Lat: 51.92, Lon: 19.15}},
	{"瑞典", Point{Name: "瑞典", Lat: 60.13, Lon: 18.64}},
	{"瑞士", Point{Name: "瑞士", Lat: 46.82, Lon: 8.23}},
	{"西班牙", Point{Name: "西班牙", Lat: 40.46, Lon: -3.75}},
	{"意大利", Point{Name: "意大利", Lat: 41.87, Lon: 12.57}},
	{"土耳其", Point{Name: "土耳其", Lat: 38.96, Lon: 35.24}},
	{"伊朗", Point{Name: "伊朗", Lat: 32.43, Lon: 53.69}},
	{"以色列", Point{Name: "以色列", Lat: 31.05, Lon: 34.85}},
	{"阿联酋", Point{Name: "阿联酋", Lat: 23.42, Lon: 53.85}},
	{"沙特阿拉伯", Point{Name: "沙特阿拉伯", Lat: 23.89, Lon: 45.08}},
	{"南非", Point{Name: "南非", Lat: -30.56, Lon: 22.94}},
	{"埃及", Point{Name: "埃及", Lat: 26.82, Lon: 30.80}},
	{"尼日利亚", Point{Name: "尼日利亚", Lat: 9.08, Lon: 8.68}},
	{"阿根廷", Point{Name: "阿根廷", Lat: -38.42, Lon: -63.62}},
	{"智利", Point{Name: "智利", Lat: -35.68, Lon: -71.54}},
	{"墨西哥", Point{Name: "墨西哥", Lat: 23.63, Lon: -102.55}},
	{"新西兰", Point{Name: "新西兰", Lat: -40.90, Lon: 174.89}},
	{"罗马尼亚", Point{Name: "罗马尼亚", Lat: 45.94, Lon: 24.97}},
	{"保加利亚", Point{Name: "保加利亚", Lat: 42.73, Lon: 25.49}},
	{"捷克", Point{Name: "捷克", Lat: 49.82, Lon: 15.47}},
	{"奥地利", Point{Name: "奥地利", Lat: 47.52, Lon: 14.55}},
	{"比利时", Point{Name: "比利时", Lat: 50.50, Lon: 4.47}},
	{"丹麦", Point{Name: "丹麦", Lat: 56.26, Lon: 9.50}},
	{"挪威", Point{Name: "挪威", Lat: 60.47, Lon: 8.47}},
	{"芬兰", Point{Name: "芬兰", Lat: 61.92, Lon: 25.75}},
	{"爱尔兰", Point{Name: "爱尔兰", Lat: 53.14, Lon: -7.69}},
	{"葡萄牙", Point{Name: "葡萄牙", Lat: 39.40, Lon: -8.22}},
	{"希腊", Point{Name: "希腊", Lat: 39.07, Lon: 21.82}},
	{"匈牙利", Point{Name: "匈牙利", Lat: 47.16, Lon: 19.50}},
	{"巴基斯坦", Point{Name: "巴基斯坦", Lat: 30.38, Lon: 69.35}},
	{"孟加拉国", Point{Name: "孟加拉国", Lat: 23.68, Lon: 90.36}},
	{"斯里兰卡", Point{Name: "斯里兰卡", Lat: 7.87, Lon: 80.77}},
	{"缅甸", Point{Name: "缅甸", Lat: 21.91, Lon: 95.96}},
	{"柬埔寨", Point{Name: "柬埔寨", Lat: 12.57, Lon: 104.99}},
	{"老挝", Point{Name: "老挝", Lat: 19.86, Lon: 102.50}},
	{"蒙古", Point{Name: "蒙古", Lat: 46.86, Lon: 103.85}},
	{"哈萨克斯坦", Point{Name: "哈萨克斯坦", Lat: 48.02, Lon: 66.92}},
	{"乌兹别克斯坦", Point{Name: "乌兹别克斯坦", Lat: 41.38, Lon: 64.59}},
	{"白俄罗斯", Point{Name: "白俄罗斯", Lat: 53.71, Lon: 27.95}},
	{"立陶宛", Point{Name: "立陶宛", Lat: 55.17, Lon: 23.88}},
	{"拉脱维亚", Point{Name: "拉脱维亚", Lat: 56.88, Lon: 24.60}},
	{"爱沙尼亚", Point{Name: "爱沙尼亚", Lat: 58.60, Lon: 25.01}},
	{"哥伦比亚", Point{Name: "哥伦比亚", Lat: 4.57, Lon: -74.30}},
	{"秘鲁", Point{Name: "秘鲁", Lat: -9.19, Lon: -75.02}},
	{"委内瑞拉", Point{Name: "委内瑞拉", Lat: 6.42, Lon: -66.59}},
	{"厄瓜多尔", Point{Name: "厄瓜多尔", Lat: -1.83, Lon: -78.18}},
	{"摩洛哥", Point{Name: "摩洛哥", Lat: 31.79, Lon: -7.09}},
	{"肯尼亚", Point{Name: "肯尼亚", Lat: -0.02, Lon: 37.91}},
	{"埃塞俄比亚", Point{Name: "埃塞俄比亚", Lat: 9.15, Lon: 40.49}},
	{"加纳", Point{Name: "加纳", Lat: 7.95, Lon: -1.02}},
	{"坦桑尼亚", Point{Name: "坦桑尼亚", Lat: -6.37, Lon: 34.89}},
	{"突尼斯", Point{Name: "突尼斯", Lat: 33.89, Lon: 9.54}},
	{"阿尔及利亚", Point{Name: "阿尔及利亚", Lat: 28.03, Lon: 1.66}},
	{"伊拉克", Point{Name: "伊拉克", Lat: 33.22, Lon: 43.68}},
	{"卡塔尔", Point{Name: "卡塔尔", Lat: 25.35, Lon: 51.18}},
	{"科威特", Point{Name: "科威特", Lat: 29.31, Lon: 47.48}},
	{"阿曼", Point{Name: "阿曼", Lat: 21.51, Lon: 55.92}},
	{"约旦", Point{Name: "约旦", Lat: 30.59, Lon: 36.24}},
	{"黎巴嫩", Point{Name: "黎巴嫩", Lat: 33.85, Lon: 35.86}},
	{"叙利亚", Point{Name: "叙利亚", Lat: 34.80, Lon: 38.99}},
	{"阿富汗", Point{Name: "阿富汗", Lat: 33.94, Lon: 67.71}},
	{"尼泊尔", Point{Name: "尼泊尔", Lat: 28.39, Lon: 84.12}},
}
