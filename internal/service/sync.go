package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"wafadmin/internal/config"
	"wafadmin/internal/nginx"
	"wafadmin/pkg/crypto"
)

// SyncResult 是单台 WAF 节点的同步结果。
type SyncResult struct {
	Server  string `json:"server"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// SyncService 负责把站点与规则配置应用到本机，并推送到集群内其他节点。
type SyncService struct {
	Cfg    *config.Config
	Sites  *SiteService
	Rules  *RuleService
	Nginx  *nginx.Manager
	Signer crypto.Signer
	Client *http.Client
}

// ApplySites 在本机生成 vhost 文件并重载 nginx，siteID 为 0 时应用全部站点。
func (s *SyncService) ApplySites(ctx context.Context, siteID int64) (int, error) {
	if s.Nginx == nil {
		return 0, ErrInternal.New("未初始化 nginx 管理组件")
	}

	sites := make([]string, 0, 8)
	if siteID > 0 {
		site, err := s.Sites.Get(ctx, siteID)
		if err != nil {
			return 0, err
		}
		sites = append(sites, site.SiteName)
		if _, writeErr := s.Nginx.WriteVhost(*site); writeErr != nil {
			return 0, ErrInternal.Wrap(writeErr, "写入站点 %s 配置失败", site.SiteName)
		}
	} else {
		all, err := s.Sites.All(ctx)
		if err != nil {
			return 0, err
		}
		for _, site := range all {
			if _, writeErr := s.Nginx.WriteVhost(site); writeErr != nil {
				return 0, ErrInternal.Wrap(writeErr, "写入站点 %s 配置失败", site.SiteName)
			}
			sites = append(sites, site.SiteName)
		}
	}

	if _, err := s.Nginx.ReloadIfNeeded(); err != nil {
		return len(sites), ErrInternal.Wrap(err, "重载 nginx 失败")
	}
	return len(sites), nil
}

// ApplyRules 在本机把规则写入 .rule 文件并重载 nginx。
func (s *SyncService) ApplyRules(ctx context.Context) (int, error) {
	if s.Nginx == nil {
		return 0, ErrInternal.New("未初始化 nginx 管理组件")
	}
	grouped, err := s.Rules.Grouped(ctx)
	if err != nil {
		return 0, err
	}
	written, err := s.Nginx.WriteRules(grouped)
	if err != nil {
		return len(written), ErrInternal.Wrap(err, "写入规则文件失败")
	}
	if _, err := s.Nginx.ReloadIfNeeded(); err != nil {
		return len(written), ErrInternal.Wrap(err, "重载 nginx 失败")
	}
	return len(written), nil
}

// PushSites 通知集群内所有节点同步站点配置。
func (s *SyncService) PushSites(ctx context.Context, siteID int64) ([]SyncResult, error) {
	path := "/api/site/sync/"
	if siteID > 0 {
		return s.broadcast(ctx, path, fmt.Sprintf("id=%d", siteID))
	}
	return s.broadcast(ctx, path, "")
}

// PushRules 通知集群内所有节点同步规则。
func (s *SyncService) PushRules(ctx context.Context) ([]SyncResult, error) {
	return s.broadcast(ctx, "/api/rule/sync/", "")
}

// Verify 校验来自其他节点的同步请求签名。
func (s *SyncService) Verify(timestamp, hash string) bool { return s.Signer.Verify(timestamp, hash) }

// Sign 生成同步请求签名，便于测试与自检。
func (s *SyncService) Sign(timestamp int64) string { return s.Signer.Sign(timestamp) }

// broadcast 向所有配置的节点发起同步请求。
func (s *SyncService) broadcast(ctx context.Context, path, extra string) ([]SyncResult, error) {
	results := make([]SyncResult, 0, len(s.Cfg.APIServers))
	timestamp := time.Now().Unix()
	signature := s.Signer.Sign(timestamp)

	for _, server := range s.Cfg.APIServers {
		server = strings.TrimSpace(server)
		if server == "" {
			continue
		}
		url := fmt.Sprintf("http://%s:%d%s?hash=%s&timestamp=%d", server, s.Cfg.HTTPPort, path, signature, timestamp)
		if extra != "" {
			url += "&" + extra
		}

		result := SyncResult{Server: server}
		body, err := s.fetch(ctx, url)
		if err != nil {
			result.Status = "failed"
			result.Message = err.Error()
			results = append(results, result)
			continue
		}
		result.Status = "success"
		result.Message = body
		results = append(results, result)
	}
	return results, nil
}

// fetch 发起一次同步请求并读取响应体。
func (s *SyncService) fetch(ctx context.Context, url string) (string, error) {
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode >= 400 {
		return "", fmt.Errorf("节点返回状态码 %d", response.StatusCode)
	}
	return strings.TrimSpace(string(raw)), nil
}
