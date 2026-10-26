package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"wafadmin/internal/model"
	"wafadmin/internal/nginx"
	"wafadmin/internal/repository"
)

// 站点字段校验。
var (
	siteNamePattern = regexp.MustCompile(`^(\*\.)?[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)
	backendPattern  = regexp.MustCompile(`^(?:[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?|\[[0-9a-fA-F:]+\])(?::\d{1,5})?$`)
)

// 允许的 SSL 与日志级别取值。
var (
	sslValues         = map[string]struct{}{"on": {}, "off": {}}
	debugLevelValues  = map[string]struct{}{"debug": {}, "info": {}, "notice": {}, "warn": {}, "error": {}, "crit": {}, "alert": {}, "emerg": {}}
	maxSiteNameLength = 190
)

// SitePayload 是站点新增/修改的入参。
type SitePayload struct {
	SiteName    string   `json:"site_name"`
	Port        int      `json:"port"`
	BackendAddr []string `json:"backend_addr"`
	UnrealAddr  []string `json:"unreal_addr"`
	Ssl         string   `json:"ssl"`
	DebugLevel  string   `json:"debug_level"`
}

// SiteService 负责站点配置的增删改查与配置预览。
type SiteService struct {
	Sites    repository.SiteRepository
	Nginx    *nginx.Manager
	PageSize int
}

// List 分页查询站点。
func (s *SiteService) List(ctx context.Context, q model.ListQuery) (model.PageResult, error) {
	q = q.Normalize(s.PageSize)
	items, total, err := s.Sites.List(ctx, q)
	if err != nil {
		return model.PageResult{}, ErrInternal.Wrap(err, "查询站点列表失败")
	}
	return q.Result(items, total), nil
}

// All 返回全部站点。
func (s *SiteService) All(ctx context.Context) ([]model.Site, error) {
	items, err := s.Sites.All(ctx)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "查询站点失败")
	}
	return items, nil
}

// Get 按主键查询站点。
func (s *SiteService) Get(ctx context.Context, id int64) (*model.Site, error) {
	site, err := s.Sites.Get(ctx, id)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "查询站点失败")
	}
	if site == nil {
		return nil, ErrNotFound.New("站点不存在")
	}
	return site, nil
}

// Create 新增站点。
func (s *SiteService) Create(ctx context.Context, payload SitePayload) (*model.Site, error) {
	normalized, err := s.validate(ctx, payload, 0)
	if err != nil {
		return nil, err
	}

	site := &model.Site{
		SiteName:    normalized.SiteName,
		Port:        normalized.Port,
		BackendAddr: normalized.BackendAddr,
		UnrealAddr:  normalized.UnrealAddr,
		Ssl:         normalized.Ssl,
		DebugLevel:  normalized.DebugLevel,
	}
	if err := s.Sites.Create(ctx, site); err != nil {
		return nil, ErrInternal.Wrap(err, "创建站点失败")
	}
	return site, nil
}

// Update 修改站点。
func (s *SiteService) Update(ctx context.Context, id int64, payload SitePayload) (*model.Site, error) {
	site, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	normalized, err := s.validate(ctx, payload, id)
	if err != nil {
		return nil, err
	}

	previousName := site.SiteName
	site.SiteName = normalized.SiteName
	site.Port = normalized.Port
	site.BackendAddr = normalized.BackendAddr
	site.UnrealAddr = normalized.UnrealAddr
	site.Ssl = normalized.Ssl
	site.DebugLevel = normalized.DebugLevel
	if err := s.Sites.Update(ctx, site); err != nil {
		return nil, ErrInvalid.Wrap(err, "更新站点失败，请刷新后重试")
	}

	// 域名变更时清理旧的 vhost 文件，避免残留配置生效。
	if previousName != site.SiteName && s.Nginx != nil {
		_ = s.Nginx.RemoveVhost(previousName)
	}
	return site, nil
}

// Delete 删除站点。
func (s *SiteService) Delete(ctx context.Context, id int64) error {
	site, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.Sites.Delete(ctx, id); err != nil {
		return ErrInternal.Wrap(err, "删除站点失败")
	}
	if s.Nginx != nil {
		_ = s.Nginx.RemoveVhost(site.SiteName)
	}
	return nil
}

// Preview 渲染指定站点的 nginx 配置（不落盘）。
func (s *SiteService) Preview(ctx context.Context, id int64) (string, error) {
	site, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if s.Nginx == nil {
		return "", ErrInternal.New("未初始化 nginx 配置管理器")
	}
	content, err := s.Nginx.Render(*site)
	if err != nil {
		return "", ErrInternal.Wrap(err, "渲染配置失败")
	}
	return content, nil
}

// validate 校验并归一化站点入参。
func (s *SiteService) validate(ctx context.Context, payload SitePayload, excludeId int64) (SitePayload, error) {
	out := SitePayload{
		SiteName:    strings.ToLower(strings.TrimSpace(payload.SiteName)),
		Port:        payload.Port,
		BackendAddr: normalizeList(payload.BackendAddr),
		UnrealAddr:  normalizeList(payload.UnrealAddr),
		Ssl:         strings.ToLower(strings.TrimSpace(payload.Ssl)),
		DebugLevel:  strings.ToLower(strings.TrimSpace(payload.DebugLevel)),
	}

	if out.SiteName == "" {
		return SitePayload{}, ErrInvalid.New("站点域名不能为空")
	}
	if len(out.SiteName) > maxSiteNameLength || !siteNamePattern.MatchString(out.SiteName) {
		return SitePayload{}, ErrInvalid.New("站点域名格式不正确，例如 www.example.com")
	}
	if out.Port <= 0 || out.Port > 65535 {
		return SitePayload{}, ErrInvalid.New("监听端口需在 1-65535 之间")
	}
	if len(out.BackendAddr) == 0 {
		return SitePayload{}, ErrInvalid.New("至少需要配置一个后端地址")
	}
	for _, addr := range out.BackendAddr {
		if !backendPattern.MatchString(addr) {
			return SitePayload{}, ErrInvalid.New("后端地址 %s 格式不正确，应为 ip:port 或域名:port", addr)
		}
	}
	for _, addr := range out.UnrealAddr {
		if !backendPattern.MatchString(addr) && !siteNamePattern.MatchString(addr) {
			return SitePayload{}, ErrInvalid.New("诱饵地址 %s 格式不正确", addr)
		}
	}
	if out.Ssl == "" {
		out.Ssl = "off"
	}
	if _, ok := sslValues[out.Ssl]; !ok {
		return SitePayload{}, ErrInvalid.New("SSL 状态只能是 on 或 off")
	}
	if out.DebugLevel == "" {
		out.DebugLevel = "error"
	}
	if _, ok := debugLevelValues[out.DebugLevel]; !ok {
		return SitePayload{}, ErrInvalid.New("日志级别不合法")
	}

	exists, err := s.Sites.ExistsByName(ctx, out.SiteName, excludeId)
	if err != nil {
		return SitePayload{}, ErrInternal.Wrap(err, "校验站点域名失败")
	}
	if exists {
		return SitePayload{}, ErrConflict.New("站点域名 %s 已存在", out.SiteName)
	}
	return out, nil
}

// normalizeList 去空、去重并保留顺序。
func normalizeList(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

// Describe 返回站点的一行摘要，用于日志与同步结果。
func describeSite(site model.Site) string {
	return fmt.Sprintf("%s:%d -> %s", site.SiteName, site.Port, strings.Join(site.BackendAddr, ","))
}
