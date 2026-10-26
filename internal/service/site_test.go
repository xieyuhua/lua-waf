package service_test

import (
	"context"
	"strings"
	"testing"

	"wafadmin/internal/model"
	"wafadmin/internal/repository/memory"
	"wafadmin/internal/service"
)

// modelListQuery 构造分页查询条件。
func modelListQuery(page, size int, keyword string) model.ListQuery {
	return model.ListQuery{Page: page, Size: size, Keyword: keyword}
}

// newSiteService 构造基于内存仓储的站点服务。
func newSiteService() *service.SiteService {
	store := (&memory.Store{}).Init(true)
	return &service.SiteService{Sites: store.Site, PageSize: 10}
}

// TestSiteCreate 校验正常新增站点。
func TestSiteCreate(t *testing.T) {
	svc := newSiteService()

	site, err := svc.Create(context.Background(), service.SitePayload{
		SiteName:    "Shop.Example.com",
		Port:        80,
		BackendAddr: []string{"10.0.0.9:8080"},
		UnrealAddr:  []string{"1.1.1.1"},
		Ssl:         "off",
		DebugLevel:  "error",
	})
	if err != nil {
		t.Fatalf("新增站点失败: %v", err)
	}
	if site.SiteName != "shop.example.com" {
		t.Fatalf("域名应被归一化为小写，实际 %s", site.SiteName)
	}
	if site.Id <= 0 {
		t.Fatal("新增站点应获得主键")
	}
	if site.Upstream() == "" || strings.Contains(site.Upstream(), ".") {
		t.Fatalf("upstream 名称不应包含点号: %s", site.Upstream())
	}
}

// TestSiteValidation 校验各类非法入参。
func TestSiteValidation(t *testing.T) {
	svc := newSiteService()
	ctx := context.Background()

	cases := []struct {
		name    string
		payload service.SitePayload
		kind    service.Kind
	}{
		{
			name:    "空域名",
			payload: service.SitePayload{Port: 80, BackendAddr: []string{"10.0.0.1:80"}},
			kind:    service.KindInvalid,
		},
		{
			name:    "域名非法",
			payload: service.SitePayload{SiteName: "not a domain!", Port: 80, BackendAddr: []string{"10.0.0.1:80"}},
			kind:    service.KindInvalid,
		},
		{
			name:    "端口越界",
			payload: service.SitePayload{SiteName: "a.example.com", Port: 70000, BackendAddr: []string{"10.0.0.1:80"}},
			kind:    service.KindInvalid,
		},
		{
			name:    "缺少后端",
			payload: service.SitePayload{SiteName: "b.example.com", Port: 80},
			kind:    service.KindInvalid,
		},
		{
			name:    "后端格式非法",
			payload: service.SitePayload{SiteName: "c.example.com", Port: 80, BackendAddr: []string{"http://10.0.0.1"}},
			kind:    service.KindInvalid,
		},
		{
			name:    "SSL 取值非法",
			payload: service.SitePayload{SiteName: "d.example.com", Port: 80, BackendAddr: []string{"10.0.0.1:80"}, Ssl: "yes"},
			kind:    service.KindInvalid,
		},
		{
			name:    "域名重复",
			payload: service.SitePayload{SiteName: "www.example.com", Port: 80, BackendAddr: []string{"10.0.0.1:80"}},
			kind:    service.KindConflict,
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			_, err := svc.Create(ctx, item.payload)
			if err == nil {
				t.Fatal("期望返回业务错误，实际成功")
			}
			svcErr, ok := service.AsError(err)
			if !ok {
				t.Fatalf("期望业务错误，实际 %T", err)
			}
			if svcErr.Kind != item.kind {
				t.Fatalf("期望错误类别 %s，实际 %s", item.kind, svcErr.Kind)
			}
		})
	}
}

// TestSiteListPagination 校验分页与关键词过滤。
func TestSiteListPagination(t *testing.T) {
	svc := newSiteService()
	ctx := context.Background()

	result, err := svc.List(ctx, modelListQuery(1, 2, ""))
	if err != nil {
		t.Fatalf("查询站点失败: %v", err)
	}
	if result.Total < 3 {
		t.Fatalf("演示数据应至少包含 3 个站点，实际 %d", result.Total)
	}
	if result.Pages < 2 {
		t.Fatalf("每页 2 条时应产生多页，实际 %d", result.Pages)
	}

	filtered, err := svc.List(ctx, modelListQuery(1, 10, "api"))
	if err != nil {
		t.Fatalf("关键词查询失败: %v", err)
	}
	if filtered.Total != 1 {
		t.Fatalf("关键词 api 应命中 1 个站点，实际 %d", filtered.Total)
	}
}

// TestSiteDelete 校验删除后不可再查询。
func TestSiteDelete(t *testing.T) {
	svc := newSiteService()
	ctx := context.Background()

	site, err := svc.Create(ctx, service.SitePayload{
		SiteName:    "del.example.com",
		Port:        8080,
		BackendAddr: []string{"10.0.0.7:8080"},
	})
	if err != nil {
		t.Fatalf("新增站点失败: %v", err)
	}
	if err := svc.Delete(ctx, site.Id); err != nil {
		t.Fatalf("删除站点失败: %v", err)
	}
	if _, err := svc.Get(ctx, site.Id); err == nil {
		t.Fatal("删除后查询应返回不存在")
	}
}
