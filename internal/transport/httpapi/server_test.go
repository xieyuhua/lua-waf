package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"wafadmin/internal/config"
	"wafadmin/internal/model"
	"wafadmin/internal/nginx"
	"wafadmin/internal/repository/memory"
	"wafadmin/internal/service"
	"wafadmin/internal/transport/httpapi"
	"wafadmin/pkg/captcha"
	"wafadmin/pkg/crypto"
	"wafadmin/pkg/geo"
	"wafadmin/pkg/response"
	"wafadmin/pkg/session"
)

// 测试用的 CSRF Token。
const testCSRFToken = "0123456789abcdef0123456789abcdef"

// harness 是测试用的 HTTP 服务实例。
type harness struct {
	handler http.Handler
	session *session.Manager
	captcha *captcha.Manager
}

// envelope 是响应体的解析结构。
type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// newHarness 构造基于内存仓储与演示数据的测试服务。
func newHarness(t *testing.T) *harness {
	t.Helper()

	cfg := &config.Config{
		RunMode:          config.ModeDemo,
		AppVer:           "test",
		PageSize:         10,
		HTTPPort:         5000,
		APIServers:       []string{"127.0.0.1"},
		APIKey:           "test-api-key-1234567890",
		NginxBin:         "nginx",
		NginxVhosts:      "/tmp/vhosts",
		SSLCertDir:       "/tmp/cert",
		RulePath:         "/tmp/rules",
		WafLogDir:        "/tmp/logs",
		SessionSecret:    "test-secret-key",
		SessionTTL:       time.Hour,
		CaptchaTTL:       time.Minute,
		SlowSQLThreshold: 100 * time.Millisecond,
	}

	store := (&memory.Store{}).Init(true)
	repos := store.Repos()
	hasher := crypto.Hasher{}
	nginxManager := &nginx.Manager{DryRun: true, VhostsDir: cfg.NginxVhosts, RuleDir: cfg.RulePath}
	settings := &service.SettingService{Repo: repos.Setting}

	sites := &service.SiteService{Sites: repos.Site, Nginx: nginxManager, PageSize: cfg.PageSize}
	rules := &service.RuleService{Rules: repos.Rule, Catalog: model.Catalog, Nginx: nginxManager}
	users := &service.UserService{Users: repos.User, Hasher: hasher, PageSize: cfg.PageSize}
	logs := &service.LogService{Logs: repos.Log, PageSize: cfg.PageSize}
	slow := &service.SlowSQLService{Repo: repos.SlowSQL, Threshold: cfg.SlowSQLThreshold, PageSize: cfg.PageSize}

	captchaManager := &captcha.Manager{TTL: cfg.CaptchaTTL, Length: 4}
	configFile := filepath.Join(t.TempDir(), "app.ini")
	sysConfig := &service.SystemConfigService{
		Current: cfg,
		Loader:  &config.Loader{Path: configFile},
		Writer:  &config.Writer{Path: configFile},
	}
	sessionManager := &session.Manager{
		CookieName: "waf_session",
		Secret:     []byte(cfg.SessionSecret),
		TTL:        cfg.SessionTTL,
	}

	server := &httpapi.Server{
		Cfg:     cfg,
		Session: sessionManager,
		CSRF:    &httpapi.CSRFGuard{},
		Writer:  response.Writer{},
		Deps: httpapi.Deps{
			Auth: &service.AuthService{
				Users:   repos.User,
				Hasher:  hasher,
				Captcha: captchaManager,
			},
			Sites:   sites,
			Rules:   rules,
			Users:   users,
			Logs:    logs,
			SlowSQL: slow,
			Dashboard: &service.DashboardService{
				Sites: repos.Site, Rules: repos.Rule, Users: repos.User,
				Logs: repos.Log, Slow: repos.SlowSQL, PageSize: cfg.PageSize,
			},
			AI:        &service.AIService{Settings: settings, Logs: repos.Log},
			Map:       &service.AttackMapService{Logs: repos.Log, Sites: repos.Site, Resolver: geo.Default},
			Settings:  settings,
			SysConfig: sysConfig,
			Sync: &service.SyncService{
				Cfg:    cfg,
				Sites:  sites,
				Rules:  rules,
				Nginx:  nginxManager,
				Signer: crypto.Signer{APIKey: "test-key"},
				Client: &http.Client{Timeout: 2 * time.Second},
			},
		},
	}

	return &harness{handler: server.Handler(), session: sessionManager, captcha: captchaManager}
}

// do 发起一次请求；auth 为 true 时附带会话，csrf 为 false 时不带 CSRF 头。
func (h *harness) do(t *testing.T, method, target, body string, auth, csrf bool) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "waf_csrf", Value: testCSRFToken})
	if csrf {
		request.Header.Set("X-CSRF-Token", testCSRFToken)
	}
	if auth {
		recorder := httptest.NewRecorder()
		if err := h.session.Issue(recorder, 1, "admin"); err != nil {
			t.Fatalf("签发会话失败: %v", err)
		}
		for _, cookie := range recorder.Result().Cookies() {
			request.AddCookie(cookie)
		}
	}

	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, request)
	return recorder
}

// decode 解析响应体。
func decode(t *testing.T, recorder *httptest.ResponseRecorder) envelope {
	t.Helper()
	parsed := envelope{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("解析响应失败: %v，响应体: %s", err, recorder.Body.String())
	}
	return parsed
}

// TestGuardedRouteRejectsAnonymous 校验未登录访问受保护接口被拒绝。
func TestGuardedRouteRejectsAnonymous(t *testing.T) {
	h := newHarness(t)
	recorder := h.do(t, http.MethodGet, "/api/sites", "", false, true)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("期望 401，实际 %d", recorder.Code)
	}
	if decode(t, recorder).Code != response.CodeUnauthorized {
		t.Fatal("响应码应为未授权")
	}
}

// TestCSRFRequiredForWrite 校验写操作缺少 CSRF 头时被拒绝。
func TestCSRFRequiredForWrite(t *testing.T) {
	h := newHarness(t)
	body := `{"site_name":"csrf.example.com","port":80,"backend_addr":["10.0.0.1:80"]}`
	recorder := h.do(t, http.MethodPost, "/api/sites", body, true, false)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("期望 403，实际 %d", recorder.Code)
	}
}

// TestSiteCRUD 校验站点增查改删全链路。
func TestSiteCRUD(t *testing.T) {
	h := newHarness(t)

	create := h.do(t, http.MethodPost, "/api/sites",
		`{"site_name":"demo.example.com","port":8080,"backend_addr":["10.0.0.21:8080"],"ssl":"on","debug_level":"warn"}`,
		true, true)
	if create.Code != http.StatusOK {
		t.Fatalf("新增站点失败: %d %s", create.Code, create.Body.String())
	}
	created := struct {
		Id       int64    `json:"id"`
		SiteName string   `json:"site_name"`
		Ssl      string   `json:"ssl"`
		Backend  []string `json:"backend_addr"`
	}{}
	if err := json.Unmarshal(decode(t, create).Data, &created); err != nil {
		t.Fatalf("解析新增结果失败: %v", err)
	}
	if created.Id <= 0 || created.SiteName != "demo.example.com" || created.Ssl != "on" {
		t.Fatalf("新增结果异常: %+v", created)
	}

	list := h.do(t, http.MethodGet, "/api/sites?page=1&size=5&keyword=demo", "", true, true)
	listData := struct {
		Total int `json:"total"`
		Items []struct {
			SiteName string `json:"site_name"`
		} `json:"items"`
	}{}
	if err := json.Unmarshal(decode(t, list).Data, &listData); err != nil {
		t.Fatalf("解析列表失败: %v", err)
	}
	if listData.Total != 1 {
		t.Fatalf("关键词查询应命中 1 条，实际 %d", listData.Total)
	}

	preview := h.do(t, http.MethodGet, "/api/sites/"+strconv.FormatInt(created.Id, 10)+"/preview", "", true, true)
	if preview.Code != http.StatusOK {
		t.Fatalf("渲染配置失败: %d %s", preview.Code, preview.Body.String())
	}
	previewData := struct {
		Content string `json:"content"`
	}{}
	if err := json.Unmarshal(decode(t, preview).Data, &previewData); err != nil {
		t.Fatalf("解析渲染结果失败: %v", err)
	}
	if !strings.Contains(previewData.Content, "server_name demo.example.com") {
		t.Fatalf("渲染内容缺少 server_name: %s", previewData.Content)
	}

	remove := h.do(t, http.MethodDelete, "/api/sites/"+strconv.FormatInt(created.Id, 10), "", true, true)
	if remove.Code != http.StatusOK {
		t.Fatalf("删除站点失败: %d %s", remove.Code, remove.Body.String())
	}
}

// TestRuleOverviewAndList 校验规则子菜单与按类型查询。
func TestRuleOverviewAndList(t *testing.T) {
	h := newHarness(t)

	types := h.do(t, http.MethodGet, "/api/rules/types", "", true, true)
	if types.Code != http.StatusOK {
		t.Fatalf("查询规则类型失败: %d", types.Code)
	}
	var views []struct {
		Key   string `json:"key"`
		Name  string `json:"name"`
		Count int64  `json:"count"`
	}
	if err := json.Unmarshal(decode(t, types).Data, &views); err != nil {
		t.Fatalf("解析规则类型失败: %v", err)
	}
	if len(views) == 0 {
		t.Fatal("规则类型不应为空")
	}
	var hasArgs bool
	for _, view := range views {
		if view.Key == "args" && view.Count > 0 {
			hasArgs = true
		}
	}
	if !hasArgs {
		t.Fatal("args 规则应有内置数据")
	}

	list := h.do(t, http.MethodGet, "/api/rules?type=useragent", "", true, true)
	if list.Code != http.StatusOK {
		t.Fatalf("按类型查询规则失败: %d", list.Code)
	}
	ruleData := struct {
		Total int `json:"total"`
	}{}
	if err := json.Unmarshal(decode(t, list).Data, &ruleData); err != nil {
		t.Fatalf("解析规则列表失败: %v", err)
	}
	if ruleData.Total != 1 {
		t.Fatalf("useragent 规则应为 1 条，实际 %d", ruleData.Total)
	}
}

// TestSlowSQLTimeRangeAPI 校验慢 SQL 列表支持时间范围查询。
func TestSlowSQLTimeRangeAPI(t *testing.T) {
	h := newHarness(t)
	now := time.Now()
	from := now.Add(-48 * time.Hour).Format("2006-01-02 15:04:05")
	to := now.Add(time.Minute).Format("2006-01-02 15:04:05")

	recorder := h.do(t, http.MethodGet, "/api/slow-sql?page=1&size=10&from="+urlEscape(from)+"&to="+urlEscape(to), "", true, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("查询慢 SQL 失败: %d %s", recorder.Code, recorder.Body.String())
	}
	page := struct {
		Total int64 `json:"total"`
		Items []struct {
			DurationMs int64     `json:"duration_ms"`
			OccurredAt time.Time `json:"occurred_at"`
		} `json:"items"`
	}{}
	if err := json.Unmarshal(decode(t, recorder).Data, &page); err != nil {
		t.Fatalf("解析慢 SQL 列表失败: %v", err)
	}
	if page.Total == 0 || len(page.Items) == 0 {
		t.Fatal("时间范围内应有慢 SQL 数据")
	}

	stats := h.do(t, http.MethodGet, "/api/slow-sql/stats?from="+urlEscape(from)+"&to="+urlEscape(to), "", true, true)
	if stats.Code != http.StatusOK {
		t.Fatalf("查询慢 SQL 统计失败: %d", stats.Code)
	}
	statData := struct {
		Total  int64 `json:"total"`
		AvgMs  int64 `json:"avg_ms"`
		MaxMs  int64 `json:"max_ms"`
		P95Ms  int64 `json:"p95_ms"`
		Over1s int64 `json:"over_1s"`
	}{}
	if err := json.Unmarshal(decode(t, stats).Data, &statData); err != nil {
		t.Fatalf("解析统计失败: %v", err)
	}
	if statData.Total == 0 || statData.MaxMs < statData.AvgMs {
		t.Fatalf("统计数据异常: %+v", statData)
	}
}

// TestAttackMapSnapshot 校验攻击地图接口返回坐标点。
func TestAttackMapSnapshot(t *testing.T) {
	h := newHarness(t)
	recorder := h.do(t, http.MethodGet, "/api/attack-map?range=all", "", true, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("查询攻击地图失败: %d %s", recorder.Code, recorder.Body.String())
	}
	snapshot := struct {
		Total    int64 `json:"total"`
		Domestic int64 `json:"domestic"`
		Overseas int64 `json:"overseas"`
		Points   []struct {
			Name  string  `json:"name"`
			Lat   float64 `json:"lat"`
			Lon   float64 `json:"lon"`
			Count int64   `json:"count"`
		} `json:"points"`
	}{}
	if err := json.Unmarshal(decode(t, recorder).Data, &snapshot); err != nil {
		t.Fatalf("解析攻击地图失败: %v", err)
	}
	if snapshot.Total == 0 || len(snapshot.Points) == 0 {
		t.Fatal("攻击地图应有数据点")
	}
	if snapshot.Domestic+snapshot.Overseas != snapshot.Total {
		t.Fatalf("境内+境外应等于总数: %d + %d != %d", snapshot.Domestic, snapshot.Overseas, snapshot.Total)
	}
}

// TestCaptchaEndpoint 校验验证码接口下发图片。
func TestCaptchaEndpoint(t *testing.T) {
	h := newHarness(t)
	recorder := h.do(t, http.MethodGet, "/api/auth/captcha", "", false, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("获取验证码失败: %d", recorder.Code)
	}
	challenge := struct {
		Id    string `json:"captcha_id"`
		Image string `json:"image"`
	}{}
	if err := json.Unmarshal(decode(t, recorder).Data, &challenge); err != nil {
		t.Fatalf("解析验证码失败: %v", err)
	}
	if challenge.Id == "" || !strings.HasPrefix(challenge.Image, "data:image/png;base64,") {
		t.Fatalf("验证码数据异常: id=%q prefix=%q", challenge.Id, safePrefix(challenge.Image))
	}
}

// TestDashboardSummary 校验看板接口。
func TestDashboardSummary(t *testing.T) {
	h := newHarness(t)
	recorder := h.do(t, http.MethodGet, "/api/dashboard/summary", "", true, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("查询看板失败: %d %s", recorder.Code, recorder.Body.String())
	}
	summary := struct {
		Sites     int64 `json:"sites"`
		Rules     int64 `json:"rules"`
		Users     int64 `json:"users"`
		LogsTotal int64 `json:"logs_total"`
		Trend     []struct {
			Date string `json:"date"`
		} `json:"trend"`
	}{}
	if err := json.Unmarshal(decode(t, recorder).Data, &summary); err != nil {
		t.Fatalf("解析看板失败: %v", err)
	}
	if summary.Sites == 0 || summary.Rules == 0 || summary.LogsTotal == 0 {
		t.Fatalf("看板数据异常: %+v", summary)
	}
	if len(summary.Trend) != 15 {
		t.Fatalf("趋势应为 15 天，实际 %d", len(summary.Trend))
	}
}

// TestSyncSignatureRejected 校验同步接口拒绝无效签名。
func TestSyncSignatureRejected(t *testing.T) {
	h := newHarness(t)
	recorder := h.do(t, http.MethodGet, "/api/site/sync/?timestamp=1&hash=bad", "", false, true)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("非法签名应返回 403，实际 %d", recorder.Code)
	}
}

// urlEscape 对查询参数做最小转义（空格转义为 %20）。
func urlEscape(raw string) string {
	return strings.ReplaceAll(raw, " ", "%20")
}

// safePrefix 返回安全的前缀片段，便于断言失败时输出。
func safePrefix(value string) string {
	if len(value) > 32 {
		return value[:32]
	}
	return value
}
