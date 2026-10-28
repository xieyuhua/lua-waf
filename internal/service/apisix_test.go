package service_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"wafadmin/internal/apisix"
	"wafadmin/internal/service"
)

// fakeApisix 模拟 APISIX Admin API：保存路由、记录收到的 PUT 体。
type fakeApisix struct {
	mu     sync.Mutex
	routes map[string]map[string]any
	puts   []string
}

func newFakeApisix(t *testing.T) (*fakeApisix, *service.ApisixService, func()) {
	t.Helper()

	fake := &fakeApisix{routes: map[string]map[string]any{
		"1": {
			"id":       "1",
			"name":     "订单服务",
			"uri":      "/api/orders/*",
			"hosts":    []any{"api.example.com"},
			"status":   float64(1),
			"upstream": map[string]any{"type": "roundrobin", "nodes": map[string]any{"10.0.0.1:8080": 1}},
			"plugins":  map[string]any{"cors": map[string]any{"allow_origins": "*"}},
		},
		"2": {"id": "2", "name": "静态资源", "uris": []any{"/static/*", "/assets/*"}, "plugins": map[string]any{}},
	}}

	handler := http.NewServeMux()
	handler.HandleFunc("GET /apisix/admin/routes", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fake.mu.Lock()
		list := make([]any, 0, len(fake.routes))
		for id, route := range fake.routes {
			list = append(list, map[string]any{"key": "/apisix/routes/" + id, "value": route})
		}
		fake.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"total": len(list), "list": list})
	})
	handler.HandleFunc("GET /apisix/admin/routes/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		id := r.PathValue("id")
		fake.mu.Lock()
		route, ok := fake.routes[id]
		fake.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"key": "/apisix/routes/" + id, "value": route})
	})
	handler.HandleFunc("PUT /apisix/admin/routes/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var route map[string]any
		if err := json.Unmarshal(raw, &route); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		id := r.PathValue("id")
		fake.mu.Lock()
		fake.routes[id] = route
		fake.puts = append(fake.puts, string(raw))
		fake.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	})

	server := httptest.NewServer(handler)
	svc := &service.ApisixService{Client: apisix.New(apisix.Options{
		AdminURL: server.URL + "/apisix/admin",
		AdminKey: "test-key",
		Timeout:  5 * time.Second,
	})}
	return fake, svc, server.Close
}

// TestApisixStatusAndRoutes 校验连通状态与路由列表（含已启用插件）。
func TestApisixStatusAndRoutes(t *testing.T) {
	_, svc, closeServer := newFakeApisix(t)
	defer closeServer()

	ctx := context.Background()
	status := svc.Status(ctx)
	if !status.Enabled || !status.Connected || status.RouteCount != 2 {
		t.Fatalf("状态异常: %+v", status)
	}

	routes, err := svc.Routes(ctx)
	if err != nil {
		t.Fatalf("读取路由失败: %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("应返回 2 条路由，实际 %d", len(routes))
	}

	var order *service.ApisixRouteView
	for i := range routes {
		if routes[i].Id == "1" {
			order = &routes[i]
		}
	}
	if order == nil {
		t.Fatal("未找到路由 1")
	}
	if order.Name != "订单服务" || len(order.URIs) != 1 || order.URIs[0] != "/api/orders/*" {
		t.Fatalf("路由字段解析异常: %+v", order)
	}
	if len(order.Plugins) != 1 || order.Plugins[0] != "cors" {
		t.Fatalf("插件列表异常: %+v", order.Plugins)
	}
	if order.Mirror != nil || order.Limit != nil {
		t.Fatal("未配置时镜像/限流应为空")
	}
}

// TestApisixSetMirror 校验镜像复制下发并保留其它插件与上游配置。
func TestApisixSetMirror(t *testing.T) {
	fake, svc, closeServer := newFakeApisix(t)
	defer closeServer()

	ctx := context.Background()
	if err := svc.SetMirror(ctx, "1", service.ApisixMirrorPayload{Host: "http://mirror.internal:8080"}); err != nil {
		t.Fatalf("下发镜像失败: %v", err)
	}

	fake.mu.Lock()
	route := fake.routes["1"]
	plugins, _ := route["plugins"].(map[string]any)
	mirror, _ := plugins["proxy-mirror"].(map[string]any)
	fake.mu.Unlock()

	if mirror == nil {
		t.Fatal("proxy-mirror 未写入")
	}
	if mirror["host"] != "http://mirror.internal:8080" {
		t.Fatalf("镜像地址错误: %v", mirror["host"])
	}
	if mirror["sample_ratio"] != float64(1) {
		t.Fatalf("默认采样比例应为 1: %v", mirror["sample_ratio"])
	}
	// 其它插件与上游必须保留
	if _, ok := plugins["cors"]; !ok {
		t.Fatal("原有 cors 插件被覆盖")
	}
	if _, ok := route["upstream"]; !ok {
		t.Fatal("upstream 被丢失")
	}

	// 路由视图里应能读到镜像配置
	routes, err := svc.Routes(ctx)
	if err != nil {
		t.Fatalf("读取路由失败: %v", err)
	}
	for _, item := range routes {
		if item.Id == "1" {
			if item.Mirror == nil || item.Mirror.Host != "http://mirror.internal:8080" {
				t.Fatalf("镜像配置未回读: %+v", item.Mirror)
			}
		}
	}
}

// TestApisixLimit 校验 key 限流下发、参数校验与关闭。
func TestApisixLimit(t *testing.T) {
	fake, svc, closeServer := newFakeApisix(t)
	defer closeServer()

	ctx := context.Background()
	if err := svc.SetLimit(ctx, "1", service.ApisixLimitPayload{
		Key:        "http_x_api_key",
		Count:      100,
		TimeWindow: 60,
		Policy:     "redis",
	}); err != nil {
		t.Fatalf("下发限流失败: %v", err)
	}

	fake.mu.Lock()
	plugins, _ := fake.routes["1"]["plugins"].(map[string]any)
	limit, _ := plugins["limit-count"].(map[string]any)
	fake.mu.Unlock()

	if limit == nil {
		t.Fatal("limit-count 未写入")
	}
	if limit["key"] != "http_x_api_key" || limit["key_type"] != "var" {
		t.Fatalf("限流 key 配置异常: %+v", limit)
	}
	if limit["count"] != float64(100) || limit["time_window"] != float64(60) {
		t.Fatalf("限流窗口配置异常: %+v", limit)
	}
	if limit["rejected_code"] != float64(429) {
		t.Fatalf("默认拒绝码应为 429: %v", limit["rejected_code"])
	}
	if limit["policy"] != "redis" {
		t.Fatalf("策略异常: %v", limit["policy"])
	}

	// 参数校验
	invalidCases := []service.ApisixLimitPayload{
		{Key: "", Count: 10, TimeWindow: 60},
		{Key: "remote_addr", Count: 0, TimeWindow: 60},
		{Key: "remote_addr", Count: 10, TimeWindow: 0},
		{Key: "remote_addr", Count: 10, TimeWindow: 60, Policy: "mysql"},
		{Key: "remote_addr", Count: 10, TimeWindow: 60, KeyType: "header"},
	}
	for _, payload := range invalidCases {
		if err := svc.SetLimit(ctx, "1", payload); err == nil {
			t.Fatalf("非法参数应报错: %+v", payload)
		}
	}

	// 关闭插件
	if err := svc.ClearPlugin(ctx, "1", "limit-count"); err != nil {
		t.Fatalf("关闭限流失败: %v", err)
	}
	fake.mu.Lock()
	plugins, _ = fake.routes["1"]["plugins"].(map[string]any)
	_, exists := plugins["limit-count"]
	fake.mu.Unlock()
	if exists {
		t.Fatal("limit-count 应已删除")
	}
}

// TestApisixMirrorValidation 校验镜像地址校验。
func TestApisixMirrorValidation(t *testing.T) {
	_, svc, closeServer := newFakeApisix(t)
	defer closeServer()

	ctx := context.Background()
	for _, payload := range []service.ApisixMirrorPayload{
		{Host: ""},
		{Host: "mirror.internal:8080"},
		{Host: "http://mirror.internal", SampleRatio: 1.5},
		{Host: "http://mirror.internal", Path: "mirror"},
	} {
		if err := svc.SetMirror(ctx, "1", payload); err == nil {
			t.Fatalf("非法镜像配置应报错: %+v", payload)
		}
	}

	// 不存在的路由
	if err := svc.SetMirror(ctx, "999", service.ApisixMirrorPayload{Host: "http://mirror.internal"}); err == nil {
		t.Fatal("不存在的路由应报错")
	} else if !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("错误信息应说明路由不存在: %v", err)
	}
}

// TestApisixNotConfigured 校验未配置时的提示。
func TestApisixNotConfigured(t *testing.T) {
	svc := &service.ApisixService{}
	ctx := context.Background()

	status := svc.Status(ctx)
	if status.Enabled || status.Connected {
		t.Fatalf("未配置时不应显示已连接: %+v", status)
	}
	if !strings.Contains(status.Message, "未配置") {
		t.Fatalf("应提示未配置: %s", status.Message)
	}
	if _, err := svc.Routes(ctx); err == nil {
		t.Fatal("未配置时读取路由应报错")
	}
}
