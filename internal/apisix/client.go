// Package apisix 封装 APISIX Admin API 的最小客户端。
//
// 只涉及本后台需要的操作：读取路由列表/单条路由、整体写回路由（合并插件后 PUT）。
// 文档：PUT/GET /apisix/admin/routes/{id}，鉴权头 X-API-KEY。
package apisix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// 常见错误。
var (
	// ErrNotFound 表示路由不存在。
	ErrNotFound = errors.New("apisix: route not found")
	// ErrUnauthorized 表示 Admin Key 不正确。
	ErrUnauthorized = errors.New("apisix: unauthorized")
)

// Options 是管理接口配置。
type Options struct {
	// AdminURL 形如 http://127.0.0.1:9180/apisix/admin（不含结尾斜杠）。
	AdminURL string
	// AdminKey 为 X-API-KEY。
	AdminKey string
	// Timeout 为单次请求超时。
	Timeout time.Duration
}

// Route 是一条路由（保留原始字段，便于回写时不丢配置）。
type Route struct {
	// Id 为路由 ID。
	Id string
	// Raw 为路由原始 JSON 对象。
	Raw map[string]any
}

// Name 返回路由名称。
func (r Route) Name() string {
	if value, ok := r.Raw["name"].(string); ok {
		return value
	}
	return ""
}

// URIs 返回路由匹配的 URI 列表。
func (r Route) URIs() []string {
	uris := make([]string, 0, 2)
	if value, ok := r.Raw["uri"].(string); ok && value != "" {
		uris = append(uris, value)
	}
	if list, ok := r.Raw["uris"].([]any); ok {
		for _, item := range list {
			if text, ok := item.(string); ok && text != "" {
				uris = append(uris, text)
			}
		}
	}
	return uris
}

// Hosts 返回路由绑定的域名。
func (r Route) Hosts() []string {
	hosts := make([]string, 0, 2)
	if value, ok := r.Raw["host"].(string); ok && value != "" {
		hosts = append(hosts, value)
	}
	if list, ok := r.Raw["hosts"].([]any); ok {
		for _, item := range list {
			if text, ok := item.(string); ok && text != "" {
				hosts = append(hosts, text)
			}
		}
	}
	return hosts
}

// Plugins 返回路由上已启用的插件集合。
func (r Route) Plugins() map[string]any {
	plugins, _ := r.Raw["plugins"].(map[string]any)
	if plugins == nil {
		plugins = map[string]any{}
	}
	return plugins
}

// Client 是 Admin API 客户端。
type Client struct {
	opts Options
	http *http.Client
}

// New 构造客户端。
func New(opts Options) *Client {
	opts.AdminURL = strings.TrimRight(strings.TrimSpace(opts.AdminURL), "/")
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	return &Client{opts: opts, http: &http.Client{Timeout: opts.Timeout}}
}

// Configured 表示是否配置了管理地址。
func (c *Client) Configured() bool { return c != nil && c.opts.AdminURL != "" }

// AdminURL 返回管理地址（用于展示）。
func (c *Client) AdminURL() string {
	if c == nil {
		return ""
	}
	return c.opts.AdminURL
}

// Ping 探测管理接口是否可用，返回路由总数。
func (c *Client) Ping(ctx context.Context) (int, error) {
	routes, err := c.ListRoutes(ctx)
	if err != nil {
		return 0, err
	}
	return len(routes), nil
}

// ListRoutes 列出全部路由。
func (c *Client) ListRoutes(ctx context.Context) ([]Route, error) {
	var payload map[string]any
	if err := c.do(ctx, http.MethodGet, "/routes", nil, &payload); err != nil {
		return nil, err
	}

	items := make([]Route, 0, 16)
	// APISIX v3：{"total":N,"list":[{"key":"/apisix/routes/1","value":{...}}]}
	if list, ok := payload["list"].([]any); ok {
		for _, entry := range list {
			record, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			value, _ := record["value"].(map[string]any)
			if value == nil {
				continue
			}
			items = append(items, Route{Id: routeId(record, value), Raw: value})
		}
		return items, nil
	}

	// APISIX v2：{"node":{"nodes":[{"value":{...}}]}}
	if node, ok := payload["node"].(map[string]any); ok {
		if nodes, ok := node["nodes"].([]any); ok {
			for _, entry := range nodes {
				record, ok := entry.(map[string]any)
				if !ok {
					continue
				}
				value, _ := record["value"].(map[string]any)
				if value == nil {
					continue
				}
				items = append(items, Route{Id: routeId(record, value), Raw: value})
			}
		}
	}
	return items, nil
}

// GetRoute 读取单条路由。
func (c *Client) GetRoute(ctx context.Context, id string) (*Route, error) {
	var payload map[string]any
	if err := c.do(ctx, http.MethodGet, "/routes/"+id, nil, &payload); err != nil {
		return nil, err
	}

	value, _ := payload["value"].(map[string]any)
	if value == nil {
		// 部分版本直接返回路由对象
		value = payload
	}
	return &Route{Id: id, Raw: value}, nil
}

// PutRoute 整体写回路由（调用方负责合并后的完整对象）。
func (c *Client) PutRoute(ctx context.Context, id string, route map[string]any) error {
	return c.do(ctx, http.MethodPut, "/routes/"+id, route, nil)
}

// routeId 从条目中提取路由 ID。
func routeId(record, value map[string]any) string {
	if key, ok := record["key"].(string); ok && key != "" {
		parts := strings.Split(strings.TrimRight(key, "/"), "/")
		return parts[len(parts)-1]
	}
	if id, ok := value["id"].(string); ok {
		return id
	}
	return ""
}

// do 执行一次请求。
func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	if !c.Configured() {
		return errors.New("apisix: 未配置管理地址")
	}

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("apisix: 序列化请求失败: %w", err)
		}
		reader = bytes.NewReader(payload)
	}

	request, err := http.NewRequestWithContext(ctx, method, c.opts.AdminURL+path, reader)
	if err != nil {
		return fmt.Errorf("apisix: 构造请求失败: %w", err)
	}
	if c.opts.AdminKey != "" {
		request.Header.Set("X-API-KEY", c.opts.AdminKey)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("apisix: 请求管理接口失败（%s）: %w", c.opts.AdminURL, err)
	}
	defer response.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(response.Body, 8<<20))

	switch {
	case response.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
		return ErrUnauthorized
	case response.StatusCode >= 400:
		return fmt.Errorf("apisix: 管理接口返回 %d: %s", response.StatusCode, strings.TrimSpace(string(raw)))
	}

	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("apisix: 解析响应失败: %w", err)
	}
	return nil
}
