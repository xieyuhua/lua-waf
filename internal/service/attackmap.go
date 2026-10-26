package service

import (
	"context"
	"sort"
	"strings"
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
}

// AttackMapService 负责攻击来源的地理解析与聚合。
type AttackMapService struct {
	Logs     repository.LogRepository
	Sites    repository.SiteRepository
	Resolver geo.Resolver
}

// Snapshot 生成大屏快照，day 为 today 时只统计当日数据。
func (s *AttackMapService) Snapshot(ctx context.Context, day string) (*AttackMapSnapshot, error) {
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
			point = geo.Point{Name: "未知"}
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
	return snapshot, nil
}
