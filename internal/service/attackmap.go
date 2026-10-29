package service

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"wafadmin/internal/model"
	"wafadmin/internal/repository"
	"wafadmin/pkg/geo"
)

// 大屏单次聚合的最大来源 IP 数量。
const attackMapIPLimit = 400

// AttackPoint 是攻击地图上的一个来源点。
type AttackPoint struct {
	Name        string   `json:"name"`
	Lat         float64  `json:"lat"`
	Lon         float64  `json:"lon"`
	Count       int64    `json:"count"`
	Domestic    bool     `json:"domestic"`
	Area        string   `json:"area"`
	Ips         []string `json:"ips"`
	AttackTypes []string `json:"attack_types"`
}

// CountryStat 是国家/地区维度的排行条目。
type CountryStat struct {
	Name     string `json:"name"`
	Count    int64  `json:"count"`
	Domestic bool   `json:"domestic"`
}

// AttackTarget 是大屏的防护目标点（攻击箭头的终点）。
type AttackTarget struct {
	Name string  `json:"name"`
	Area string  `json:"area"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

// AttackStatus 是大屏顶部的攻击状态。
type AttackStatus struct {
	// Level 为状态等级：normal（窗口内无攻击）/ elevated（有攻击）/ high（持续高频）。
	Level string `json:"level"`
	// Message 为状态描述。
	Message string `json:"message"`
	// LastAttack 为最近一次攻击时间。
	LastAttack string `json:"last_attack"`
	// TodayCount 为今日攻击总数。
	TodayCount int64 `json:"today_count"`
	// WindowCount 为最近 5 分钟内（相对最近一条日志）的攻击数。
	WindowCount int64 `json:"window_count"`
	// RatePerMin 为窗口内的每分钟攻击数。
	RatePerMin float64 `json:"rate_per_min"`
}

// AttackMapSnapshot 是攻击大屏的数据快照。
type AttackMapSnapshot struct {
	Points       []AttackPoint    `json:"points"`
	Total        int64            `json:"total"`
	Domestic     int64            `json:"domestic"`
	Overseas     int64            `json:"overseas"`
	ForeignRate  float64          `json:"foreign_rate"`
	TopCountries []CountryStat    `json:"top_countries"`
	TopTypes     []model.TypeStat `json:"top_types"`
	Trend        []model.DateStat `json:"trend"`
	Recent       []model.WafLog   `json:"recent"`
	SiteCount    int64            `json:"site_count"`
	Range        string           `json:"range"`
	UpdatedAt    string           `json:"updated_at"`
	// Target 为攻击箭头的终点（防护目标位置）。
	Target AttackTarget `json:"target"`
	// Status 为顶部的攻击状态。
	Status AttackStatus `json:"status"`
	// Unresolved 为归属地无法定位的来源 IP 数量（例如 ip2region 返回「保留」）。
	Unresolved int64 `json:"unresolved"`
}

// AttackMapService 负责攻击来源的地理解析与聚合。
type AttackMapService struct {
	Logs     repository.LogRepository
	Sites    repository.SiteRepository
	Resolver geo.Resolver
	// TargetArea 为防护目标地区文本（如 中国-北京），用于绘制攻击箭头。
	TargetArea string

	// snapshotCache 缓存大屏快照（默认 45 秒），按统计范围区分。
	snapshotCache *statsCache[*AttackMapSnapshot]
	cacheOnce     sync.Once
}

// initCache 懒初始化缓存。
func (s *AttackMapService) initCache() {
	s.cacheOnce.Do(func() {
		s.snapshotCache = newStatsCache[*AttackMapSnapshot](defaultStatsCacheTTL)
	})
}

// Snapshot 生成大屏快照，day 为 today 时只统计当日数据。
func (s *AttackMapService) Snapshot(ctx context.Context, day string) (*AttackMapSnapshot, error) {
	s.initCache()
	if cached, ok := s.snapshotCache.Get("snapshot:" + day); ok {
		return cached, nil
	}

	filter := repository.StatsFilter{Limit: attackMapIPLimit}
	rangeLabel := "全部时间"
	if day == "today" {
		filter.Day = time.Now().Format("2006-01-02")
		rangeLabel = "今日"
	} else if day != "" && day != "all" {
		filter.Day = strings.TrimSpace(day)
		rangeLabel = filter.Day
	}

	ipStats, err := s.Logs.IPStats(ctx, filter)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计攻击来源失败")
	}

	type bucket struct {
		point geo.Point
		area  string
		count int64
		ips   []string
		types map[string]struct{}
	}
	buckets := make(map[string]*bucket, 32)

	snapshot := &AttackMapSnapshot{
		Points:    make([]AttackPoint, 0, 32),
		Range:     rangeLabel,
		UpdatedAt: time.Now().Format("2006-01-02 15:04:05"),
	}

	for _, stat := range ipStats {
		point, ok := s.Resolver.Resolve(stat.Area)
		if !ok {
			// 归属地无法解析（如「保留」「内网IP」）：不计入地图，只做计数提示
			snapshot.Unresolved++
			continue
		}

		entry, exists := buckets[point.Name]
		if !exists {
			entry = &bucket{point: point, area: stat.Area, types: make(map[string]struct{}, 2)}
			buckets[point.Name] = entry
		}
		entry.count += stat.Num
		if len(entry.ips) < 5 && stat.ClientIp != "" {
			entry.ips = append(entry.ips, stat.ClientIp)
		}
		for _, attackType := range strings.Split(stat.AttackType, ",") {
			if attackType = strings.TrimSpace(attackType); attackType != "" {
				entry.types[attackType] = struct{}{}
			}
		}
	}

	countries := make([]CountryStat, 0, len(buckets))
	for name, entry := range buckets {
		types := make([]string, 0, len(entry.types))
		for attackType := range entry.types {
			types = append(types, attackType)
		}
		sort.Strings(types)

		snapshot.Points = append(snapshot.Points, AttackPoint{
			Name:        name,
			Lat:         entry.point.Lat,
			Lon:         entry.point.Lon,
			Count:       entry.count,
			Domestic:    entry.point.Domestic,
			Area:        entry.area,
			Ips:         entry.ips,
			AttackTypes: types,
		})
		countries = append(countries, CountryStat{Name: name, Count: entry.count, Domestic: entry.point.Domestic})

		snapshot.Total += entry.count
		if entry.point.Domestic {
			snapshot.Domestic += entry.count
		} else {
			snapshot.Overseas += entry.count
		}
	}

	sort.Slice(snapshot.Points, func(i, j int) bool { return snapshot.Points[i].Count > snapshot.Points[j].Count })
	sort.Slice(countries, func(i, j int) bool { return countries[i].Count > countries[j].Count })
	if len(countries) > 10 {
		countries = countries[:10]
	}
	snapshot.TopCountries = countries
	if snapshot.Total > 0 {
		snapshot.ForeignRate = float64(snapshot.Overseas) * 100 / float64(snapshot.Total)
	}

	topTypes, err := s.Logs.TypeStats(ctx, repository.StatsFilter{Day: filter.Day, Limit: 8})
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计攻击类型失败")
	}
	snapshot.TopTypes = topTypes

	trend, err := s.Logs.DateTrend(ctx, 15)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计攻击趋势失败")
	}
	snapshot.Trend = trend

	recent, _, err := s.Logs.List(ctx, model.LogQuery{ListQuery: model.ListQuery{Page: 1, Size: 12}})
	if err != nil {
		return nil, ErrInternal.Wrap(err, "查询最新攻击失败")
	}
	snapshot.Recent = recent

	siteCount, err := s.Sites.Count(ctx)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "统计站点失败")
	}
	snapshot.SiteCount = siteCount

	snapshot.Target = s.target()
	snapshot.Status = s.status(ctx, recent)
	s.snapshotCache.Set("snapshot:"+day, snapshot)
	return snapshot, nil
}

// target 解析防护目标坐标（攻击箭头终点）。
func (s *AttackMapService) target() AttackTarget {
	area := strings.TrimSpace(s.TargetArea)
	if area == "" {
		area = "中国-北京"
	}

	point, ok := s.Resolver.Resolve(area)
	if !ok {
		// 词表里没有时退回北京坐标，至少保证箭头可绘制
		return AttackTarget{Name: "防护目标", Area: area, Lat: 39.9042, Lon: 116.4074}
	}
	return AttackTarget{Name: point.Name, Area: area, Lat: point.Lat, Lon: point.Lon}
}

// status 计算大屏顶部的攻击状态：以「最近一条日志时间」为基准回看 5 分钟。
func (s *AttackMapService) status(ctx context.Context, recent []model.WafLog) AttackStatus {
	const window = 5 * time.Minute

	todayCount, err := s.Logs.CountOnDay(ctx, time.Now().Format("2006-01-02"))
	if err != nil {
		todayCount = 0
	}

	status := AttackStatus{Level: "normal", Message: "近期无攻击", TodayCount: todayCount}
	if len(recent) == 0 {
		return status
	}

	latest, ok := parseLogTime(recent[0].LocalTime)
	if !ok {
		return status
	}
	status.LastAttack = recent[0].LocalTime

	for _, item := range recent {
		at, parsed := parseLogTime(item.LocalTime)
		if !parsed {
			continue
		}
		if latest.Sub(at) <= window {
			status.WindowCount++
		}
	}

	status.RatePerMin = float64(status.WindowCount) / window.Minutes()
	switch {
	case status.WindowCount >= 30:
		status.Level = "high"
		status.Message = "正在遭受高频攻击"
	case status.WindowCount > 0:
		status.Level = "elevated"
		status.Message = "检测到攻击流量"
	default:
		status.Message = "窗口内无攻击（最近一次 " + status.LastAttack + "）"
	}
	return status
}

// parseLogTime 解析 waf_log.local_time（YYYY-MM-DD HH:MM:SS）。
func parseLogTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if at, err := time.ParseInLocation("2006-01-02 15:04:05", value, time.Local); err == nil {
		return at, true
	}
	if at, err := time.Parse(time.RFC3339, value); err == nil {
		return at, true
	}
	return time.Time{}, false
}
