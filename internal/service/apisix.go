package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"wafadmin/internal/apisix"
	"wafadmin/internal/config"
)

// APISIX 插件名。
const (
	// pluginProxyMirror 对应 APISIX 的 proxy-mirror（流量镜像复制）。
	pluginProxyMirror = "proxy-mirror"
	// pluginLimitCount 对应 APISIX 的 limit-count（按 key 限流）。
	pluginLimitCount = "limit-count"
)

// ApisixMirrorPayload 是镜像复制的配置。
type ApisixMirrorPayload struct {
	// Host 为镜像目标地址，例如 http://mirror.internal:8080。
	Host string `json:"host"`
	// Path 为可选路径前缀（为空表示沿用原路径）。
	Path string `json:"path"`
	// SampleRatio 为采样比例（0~1，默认 1 表示全量镜像）。
	SampleRatio float64 `json:"sample_ratio"`
}

// ApisixLimitPayload 是 key 限流的配置。
type ApisixLimitPayload struct {
	// Key 为限流维度：remote_addr / http_x_api_key / consumer_name 等。
	Key string `json:"key"`
	// KeyType 为 var / var_combination / constant。
	KeyType string `json:"key_type"`
	// Count 为时间窗口内允许的请求数。
	Count int `json:"count"`
	// TimeWindow 为时间窗口（秒）。
	TimeWindow int `json:"time_window"`
	// RejectedCode 为被限流时返回的状态码（默认 429）。
	RejectedCode int `json:"rejected_code"`
	// Policy 为 local / redis / redis-cluster。
	Policy string `json:"policy"`
	// ShowLimitQuotaHeader 表示是否返回限流相关响应头。
	ShowLimitQuotaHeader bool `json:"show_limit_quota_header"`
}

// ApisixRouteView 是路由列表项。
type ApisixRouteView struct {
	Id      string   `json:"id"`
	Name    string   `json:"name"`
	URIs    []string `json:"uris"`
	Hosts   []string `json:"hosts"`
	Status  int      `json:"status"`
	Plugins []string `json:"plugins"`
	// Mirror 为当前的镜像复制配置（未启用为 nil）。
	Mirror *ApisixMirrorPayload `json:"mirror,omitempty"`
	// Limit 为当前的 key 限流配置（未启用为 nil）。
	Limit *ApisixLimitPayload `json:"limit,omitempty"`
	// PluginCount 为路由上启用的插件总数。
	PluginCount int `json:"plugin_count"`
}

// ApisixStatus 是管理接口连通状态。
type ApisixStatus struct {
	// Enabled 表示是否配置了 APISIX 管理接口。
	Enabled bool `json:"enabled"`
	// AdminURL 为管理地址。
	AdminURL string `json:"admin_url"`
	// Connected 表示能否正常访问。
	Connected bool `json:"connected"`
	// RouteCount 为路由数量。
	RouteCount int `json:"route_count"`
	// Message 为提示信息。
	Message string `json:"message"`
}

// ApisixService 负责 APISIX 路由插件的查询与配置（镜像复制、key 限流）。
//
// 设计原则：只读写 plugins 中的 proxy-mirror / limit-count 两个键，
// 其余插件与路由字段原样保留（GET 路由 → 修改 plugins → PUT 回写）。
type ApisixService struct {
	// Client 为 Admin API 客户端，可为空（此时功能不可用）。
	Client *apisix.Client
	// Cfg 为当前配置（保存后热更新）。
	Cfg *config.Config
	// Writer 负责把配置写回 conf/app.ini。
	Writer *config.Writer
}

// ApisixConfigPayload 是集成开关与连接参数的更新请求。
type ApisixConfigPayload struct {
	// Enabled 为是否启用集成。
	Enabled bool `json:"enabled"`
	// AdminURL 为管理接口地址。
	AdminURL string `json:"admin_url"`
	// AdminKey 为 Admin Key（留空表示不修改已保存的值）。
	AdminKey string `json:"admin_key"`
	// TimeoutMS 为超时毫秒。
	TimeoutMS int `json:"timeout_ms"`
}

// ApisixConfigView 是集成的当前配置（密钥脱敏）。
type ApisixConfigView struct {
	Enabled   bool   `json:"enabled"`
	AdminURL  string `json:"admin_url"`
	AdminKey  string `json:"admin_key"`
	TimeoutMS int    `json:"timeout_ms"`
	Writable  bool   `json:"writable"`
	File      string `json:"file"`
}

// Config 返回当前集成配置（密钥脱敏）。
func (s *ApisixService) Config() ApisixConfigView {
	view := ApisixConfigView{}
	if s.Cfg != nil {
		view.Enabled = s.Cfg.Apisix.Enabled
		view.AdminURL = s.Cfg.Apisix.AdminURL
		view.AdminKey = mask(s.Cfg.Apisix.AdminKey)
		view.TimeoutMS = s.Cfg.Apisix.TimeoutMS
	}
	if s.Writer != nil {
		view.File = s.Writer.Target()
		view.Writable = writable(view.File)
	}
	return view
}

// SaveConfig 更新集成开关与连接参数：写回 ini 并热更新客户端。
func (s *ApisixService) SaveConfig(ctx context.Context, payload ApisixConfigPayload) (ApisixConfigView, error) {
	if err := ctx.Err(); err != nil {
		return ApisixConfigView{}, err
	}
	if s.Cfg == nil || s.Writer == nil {
		return ApisixConfigView{}, ErrInvalid.New("当前模式不支持修改配置")
	}

	adminURL := strings.TrimRight(strings.TrimSpace(payload.AdminURL), "/")
	if payload.Enabled && adminURL == "" {
		return ApisixConfigView{}, ErrInvalid.New("启用前请先填写管理接口地址（如 http://127.0.0.1:9180/apisix/admin）")
	}
	if adminURL != "" && !strings.HasPrefix(adminURL, "http://") && !strings.HasPrefix(adminURL, "https://") {
		return ApisixConfigView{}, ErrInvalid.New("管理接口地址必须以 http:// 或 https:// 开头")
	}

	candidate := *s.Cfg
	candidate.Apisix = s.Cfg.Apisix
	candidate.Apisix.Enabled = payload.Enabled
	candidate.Apisix.AdminURL = adminURL
	if key := strings.TrimSpace(payload.AdminKey); key != "" && !isMasked(key) {
		candidate.Apisix.AdminKey = key
	}
	if payload.TimeoutMS > 0 {
		candidate.Apisix.TimeoutMS = payload.TimeoutMS
	}
	if candidate.Apisix.TimeoutMS <= 0 {
		candidate.Apisix.TimeoutMS = 10000
	}
	// 没有地址时强制关闭，避免出现"启用但不可用"的状态
	if candidate.Apisix.AdminURL == "" {
		candidate.Apisix.Enabled = false
	}

	if err := s.Writer.Save(&candidate); err != nil {
		return ApisixConfigView{}, ErrInternal.Wrap(err, "保存配置失败")
	}

	// 热更新：替换内存配置与客户端
	*s.Cfg = candidate
	if candidate.Apisix.Enabled {
		s.Client = apisix.New(apisix.Options{
			AdminURL: candidate.Apisix.AdminURL,
			AdminKey: candidate.Apisix.AdminKey,
			Timeout:  time.Duration(candidate.Apisix.TimeoutMS) * time.Millisecond,
		})
	} else {
		s.Client = nil
	}
	return s.Config(), nil
}

// Status 返回管理接口连通状态。
func (s *ApisixService) Status(ctx context.Context) ApisixStatus {
	status := ApisixStatus{}
	if s.Client == nil || !s.Client.Configured() {
		status.Message = "未配置 APISIX 管理接口（[apisix] ADMIN_URL / ADMIN_KEY）"
		return status
	}
	status.Enabled = true
	status.AdminURL = s.Client.AdminURL()

	count, err := s.Client.Ping(ctx)
	if err != nil {
		status.Message = describeApisixError(err)
		return status
	}
	status.Connected = true
	status.RouteCount = count
	status.Message = "连接正常"
	return status
}

// Routes 列出路由及镜像/限流配置。
func (s *ApisixService) Routes(ctx context.Context) ([]ApisixRouteView, error) {
	if err := s.ensure(ctx); err != nil {
		return nil, err
	}

	list, err := s.Client.ListRoutes(ctx)
	if err != nil {
		return nil, ErrInternal.New("读取 APISIX 路由失败：%s", describeApisixError(err))
	}

	views := make([]ApisixRouteView, 0, len(list))
	for _, route := range list {
		plugins := route.Plugins()
		names := make([]string, 0, len(plugins))
		for name := range plugins {
			names = append(names, name)
		}
		sortStrings(names)

		view := ApisixRouteView{
			Id:          route.Id,
			Name:        route.Name(),
			URIs:        route.URIs(),
			Hosts:       route.Hosts(),
			Plugins:     names,
			PluginCount: len(plugins),
		}
		if value, ok := route.Raw["status"].(float64); ok {
			view.Status = int(value)
		}
		view.Mirror = parseMirror(plugins[pluginProxyMirror])
		view.Limit = parseLimit(plugins[pluginLimitCount])
		views = append(views, view)
	}
	return views, nil
}

// SetMirror 开启/更新某条路由的流量镜像复制。
func (s *ApisixService) SetMirror(ctx context.Context, routeID string, payload ApisixMirrorPayload) error {
	if err := s.ensure(ctx); err != nil {
		return err
	}

	host := strings.TrimSpace(payload.Host)
	if host == "" {
		return ErrInvalid.New("请填写镜像目标地址")
	}
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		return ErrInvalid.New("镜像目标地址必须以 http:// 或 https:// 开头")
	}
	parsed, err := url.Parse(host)
	if err != nil || parsed.Host == "" {
		return ErrInvalid.New("镜像目标地址不合法")
	}

	ratio := payload.SampleRatio
	if ratio == 0 {
		ratio = 1
	}
	if ratio < 0 || ratio > 1 {
		return ErrInvalid.New("采样比例必须在 0~1 之间")
	}

	config := map[string]any{"host": host, "sample_ratio": ratio}
	if path := strings.TrimSpace(payload.Path); path != "" {
		if !strings.HasPrefix(path, "/") {
			return ErrInvalid.New("镜像路径需以 / 开头")
		}
		config["path"] = path
	}

	return s.updatePlugin(ctx, routeID, pluginProxyMirror, config)
}

// SetLimit 开启/更新某条路由的 key 限流。
func (s *ApisixService) SetLimit(ctx context.Context, routeID string, payload ApisixLimitPayload) error {
	if err := s.ensure(ctx); err != nil {
		return err
	}

	key := strings.TrimSpace(payload.Key)
	if key == "" {
		return ErrInvalid.New("请填写限流 key（例如 remote_addr 或 http_x_api_key）")
	}
	keyType := strings.TrimSpace(payload.KeyType)
	if keyType == "" {
		keyType = "var"
	}
	switch keyType {
	case "var", "var_combination", "constant":
	default:
		return ErrInvalid.New("key_type 只能是 var / var_combination / constant")
	}
	if payload.Count <= 0 {
		return ErrInvalid.New("限流次数必须大于 0")
	}
	if payload.TimeWindow <= 0 {
		return ErrInvalid.New("时间窗口必须大于 0 秒")
	}
	code := payload.RejectedCode
	if code == 0 {
		code = 429
	}
	if code < 200 || code > 599 {
		return ErrInvalid.New("拒绝状态码需在 200~599 之间")
	}
	policy := strings.TrimSpace(payload.Policy)
	if policy == "" {
		policy = "local"
	}
	switch policy {
	case "local", "redis", "redis-cluster":
	default:
		return ErrInvalid.New("限流策略只能是 local / redis / redis-cluster")
	}

	config := map[string]any{
		"key":                     key,
		"key_type":                keyType,
		"count":                   payload.Count,
		"time_window":             payload.TimeWindow,
		"rejected_code":           code,
		"policy":                  policy,
		"show_limit_quota_header": payload.ShowLimitQuotaHeader,
	}
	return s.updatePlugin(ctx, routeID, pluginLimitCount, config)
}

// ClearPlugin 关闭某条路由上的镜像或限流插件。
func (s *ApisixService) ClearPlugin(ctx context.Context, routeID, plugin string) error {
	if err := s.ensure(ctx); err != nil {
		return err
	}
	switch plugin {
	case pluginProxyMirror, pluginLimitCount:
	default:
		return ErrInvalid.New("不支持的插件：%s", plugin)
	}
	return s.updatePlugin(ctx, routeID, plugin, nil)
}

// ensure 校验客户端可用。
func (s *ApisixService) ensure(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.Client == nil || !s.Client.Configured() {
		return ErrInvalid.New("未配置 APISIX 管理接口，请先在 conf/app.ini 的 [apisix] 段填写 ADMIN_URL 与 ADMIN_KEY")
	}
	return nil
}

// updatePlugin 读出路由、合并插件后写回；config 为 nil 时删除该插件。
func (s *ApisixService) updatePlugin(ctx context.Context, routeID, plugin string, config map[string]any) error {
	routeID = strings.TrimSpace(routeID)
	if routeID == "" {
		return ErrInvalid.New("请指定路由 ID")
	}

	route, err := s.Client.GetRoute(ctx, routeID)
	if err != nil {
		if errors.Is(err, apisix.ErrNotFound) {
			return ErrNotFound.New("APISIX 路由 %s 不存在", routeID)
		}
		return ErrInternal.New("读取路由失败：%s", describeApisixError(err))
	}

	plugins := route.Plugins()
	if config == nil {
		delete(plugins, plugin)
	} else {
		plugins[plugin] = config
	}
	route.Raw["plugins"] = plugins

	if err := s.Client.PutRoute(ctx, routeID, route.Raw); err != nil {
		return ErrInternal.New("写回路由失败：%s", describeApisixError(err))
	}
	return nil
}

// parseMirror 解析已有镜像配置。
func parseMirror(value any) *ApisixMirrorPayload {
	config, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	payload := &ApisixMirrorPayload{}
	if host, ok := config["host"].(string); ok {
		payload.Host = host
	}
	if path, ok := config["path"].(string); ok {
		payload.Path = path
	}
	if ratio, ok := config["sample_ratio"].(float64); ok {
		payload.SampleRatio = ratio
	} else {
		payload.SampleRatio = 1
	}
	return payload
}

// parseLimit 解析已有 key 限流配置。
func parseLimit(value any) *ApisixLimitPayload {
	config, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	payload := &ApisixLimitPayload{}
	if key, ok := config["key"].(string); ok {
		payload.Key = key
	}
	if keyType, ok := config["key_type"].(string); ok {
		payload.KeyType = keyType
	}
	if count, ok := config["count"].(float64); ok {
		payload.Count = int(count)
	}
	if window, ok := config["time_window"].(float64); ok {
		payload.TimeWindow = int(window)
	}
	if code, ok := config["rejected_code"].(float64); ok {
		payload.RejectedCode = int(code)
	}
	if policy, ok := config["policy"].(string); ok {
		payload.Policy = policy
	}
	if header, ok := config["show_limit_quota_header"].(bool); ok {
		payload.ShowLimitQuotaHeader = header
	}
	return payload
}

// describeApisixError 把客户端错误翻译为可操作提示。
func describeApisixError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, apisix.ErrUnauthorized):
		return "Admin Key 不正确（X-API-KEY）"
	case errors.Is(err, apisix.ErrNotFound):
		return "路由不存在"
	default:
		return err.Error()
	}
}

// String 便于日志输出。
func (v ApisixRouteView) String() string {
	return fmt.Sprintf("route(%s)", v.Id)
}
