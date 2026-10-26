# 架构与开发指引

本文档说明 wafadmin v2 的分层设计、各层职责、扩展方式，以及前端工程结构。

## 1. 分层与职责

| 层 | 目录 | 职责 | 禁止事项 |
| --- | --- | --- | --- |
| 接入层 | `internal/transport/httpapi` | 路由注册、参数绑定、身份与 CSRF 校验、错误码映射、SPA 静态托管 | 不写业务规则、不直接访问数据库 |
| 业务层 | `internal/service` | 校验、编排、事务边界、跨域协作（如同步下发） | 不感知 `http.Request`、不拼 SQL |
| 仓储层 | `internal/repository` | 持久化契约与实现（MySQL / 内存） | 不含业务判断（除过滤条件外） |
| 领域模型 | `internal/model` | 实体、查询条件、分页结构、内置规则数据 | 不依赖任何其它层 |
| 基础设施 | `pkg/*`、`internal/nginx` | 无业务语义的通用能力 | 不反向依赖上层 |

依赖方向严格单向：`transport → service → repository → model`。

## 2. 一个业务域的完整形态

以「站点管理」为例，同一业务域分散在固定位置，便于定位：

```
internal/model/site.go                    # 实体 + 与实体强相关的派生方法
internal/repository/site.go               # SiteRepository 接口
internal/repository/mysql/site.go         # MySQL 实现（带慢 SQL 观测）
internal/repository/memory/site.go        # 内存实现（演示/测试）
internal/service/site.go                  # SiteService：校验 + 编排 + 配置预览
internal/transport/httpapi/handler_site.go# SiteHandler：路由 + 参数 + 响应
web/src/api/site.js                       # 前端接口封装
web/src/views/SitesView.vue               # 前端页面
```

## 3. 新增一个业务域的步骤

1. **模型**：在 `internal/model/` 新建 `xxx.go`，定义实体与查询条件（如需要，追加到 `query.go`）。
2. **仓储契约**：在 `internal/repository/xxx.go` 定义 `XxxRepository` 接口，
   并把接口字段加入 `internal/repository/store.go` 的 `Repositories` 与 `Ensure()`。
3. **两套实现**：
   - `internal/repository/mysql/xxx.go`：xorm 实现，每个方法首行
     `defer r.observer.Trace(ctx, "xxx.op", "SQL ...")()` 以便计入慢 SQL。
   - `internal/repository/memory/xxx.go`：内存实现，注意加读写锁、保持与 SQL 一致的分页/排序语义。
4. **服务**：`internal/service/xxx.go` 定义 `XxxService`，字段即依赖（导出以便组合根注入）；
   校验失败时返回 `ErrInvalid.New(...)` 这类业务错误。
5. **接入**：
   - `internal/transport/httpapi/handler_xxx.go` 定义 `XxxHandler` + `Register(mux, routes)`；
   - 在 `server.go` 的 `register()` 中注册，并在 `Deps` 增加对应服务字段。
6. **装配**：在 `internal/app/app.go` 的 `services()` 中构造并注入。
7. **前端**：`web/src/api/xxx.js` + `web/src/views/XxxView.vue` + 路由与侧边栏菜单。
8. **测试**：`internal/service/xxx_test.go`（业务规则）与 `httpapi` 中的接口用例（鉴权 + 正常/异常路径）。

## 4. 错误与响应约定

业务错误统一使用 `service.Error`（由 `ErrInvalid`、`ErrNotFound`、`ErrConflict`、
`ErrForbidden`、`ErrUnauthorized`、`ErrInternal` 这些 `Fault` 构造）：

```go
if target.Port <= 0 {
    return nil, service.ErrInvalid.New("监听端口需在 1-65535 之间")
}
```

接入层 `Responder` 负责把 `service.Error` 映射为 HTTP 状态码与业务码：

| Kind | HTTP | code |
| --- | --- | --- |
| invalid | 400 | 1001 |
| unauthorized | 401 | 1002 |
| forbidden | 403 | 1003 |
| not_found | 404 | 1004 |
| conflict | 409 | 1005 |
| internal | 500 | 1500 |

前端 `web/src/api/client.js` 统一解析 `{code,message,data}`，非 0 时抛出 `ApiError`，
401 时触发跳转登录页。

## 5. 安全设计

- **会话**：HMAC-SHA256 签名的无状态 Cookie（`pkg/session`），HttpOnly + SameSite=Lax，默认 12 小时。
- **CSRF**：双提交 Cookie，`waf_csrf`（非 HttpOnly，便于前端读取）+ 请求头 `X-CSRF-Token`；
  写操作缺少或不等即 403。
- **验证码**：进程内 TTL 存储，校验一次即销毁，图片由标准库绘制（无第三方图形依赖）。
- **登录限流**：同一来源 IP 5 分钟内最多 10 次尝试。
- **口令**：保持与 1.x 完全一致的摘要算法（`md5(md5(raw)[5:10])`），升级后老账号可继续登录；
  新增/修改口令时要求至少 8 位。
- **节点同步**：`md5(API_KEY + md5(timestamp + API_KEY))` 签名 + ±10 分钟时间窗，
  不影响后台会话鉴权。
- **响应头**：`X-Content-Type-Options`、`X-Frame-Options`、`Referrer-Policy`；
  API 响应 `Cache-Control: no-store`。

## 6. nginx 配置下发

`internal/nginx` 负责：

- `Render(site)`：用 `template.go` 中的 vhost 模板渲染 upstream + server 段（含 SSL、诱饵地址、日志级别、
  请求体上限，配合 WAF 的 POST 分段检测）。
- `WriteVhost` / `RemoveVhost`：写入 `NGINX_VHOSTS/<域名>.conf`，统一 LF 换行。
- `WriteRules`：按类型写出 `<type>.rule`，**字段名保持 `Id/RuleType/RuleItem`**，
  因为 lua 侧 `util.get_rules` 以 `rule["RuleItem"]` 取值。
- `Test` / `Reload`：`nginx -t` 通过后才 `-s reload`；演示模式（`DryRun`）只渲染不落盘。

## 7. 前端工程结构

```
web/src/
├── main.js               # 应用装配：路由、Element Plus（中文包）、图标、全局样式
├── App.vue               # 根组件 + 会话失效跳转
├── router/index.js       # 路由表 + 守卫（未登录跳登录页）
├── api/                  # 按业务域拆分的接口封装 + 统一 client
├── stores/               # 轻量响应式状态（登录态、主题、规则类型缓存）
├── layouts/AdminLayout   # 侧边栏 + 顶栏 + 内容区（含移动端抽屉）
├── components/           # 指标卡、图表容器、AI 分析弹窗、侧边栏、顶栏
├── views/                # 每个菜单一个页面
├── styles/main.css       # 设计系统：色板、卡片、导航、表格、动效、暗色主题
├── assets/world.geo.json # 攻击地图数据（scripts/build-world-geo.mjs 生成）
└── utils/format.js       # 时间/数字/语句格式化
```

工程细节（命令、接口约定、设计 token、新增页面步骤）见 [web/README.md](../web/README.md)。

设计规范要点：

- 品牌色渐变 `--waf-gradient: #6366f1 → #8b5cf6`，卡片圆角 14px，阴影轻量不抢内容；
- 暗色主题通过 `html.dark` + Element Plus 官方暗色变量，状态持久化到 `localStorage`；
- 侧边栏深色渐变、选中态使用左侧渐变条 + 半透明底色；规则子菜单按「拦截策略 / 访问控制」分组；
- 攻击地图大屏使用独立的深色皮肤（不随主题变化），ECharts `geo + effectScatter` 呈现涟漪动效。

## 8. 测试策略

- 单元测试：`pkg/crypto`（口令兼容、签名）、`pkg/geo`（地区解析）、
  `internal/service`（站点校验、慢 SQL 时间范围与统计口径）。
- 接口测试：`internal/transport/httpapi` 用内存仓储 + `httptest` 覆盖
  未登录 401、CSRF 403、登录全流程（含验证码单次有效）、站点 CRUD、规则子菜单、
  慢 SQL 时间查询、攻击地图、看板、同步签名拒绝。
- 演示模式可直接 `go run .` 手工验收，或按 README 用无头浏览器截图核对界面。

## 9. 规则与配置的生效链路

配置写盘并不等于生效，链路如下：

```
后台操作                      磁盘                       openresty
──────────────────────────────────────────────────────────────────────────
「下发全部规则」  →  RULE_PATH/<type>.rule  →  reload → 新 worker 执行
「站点同步」      →  NGINX_VHOSTS/<域名>.conf          init_by_lua →
                                                        waf.load_rules()
「节点同步」(其他机器) → 各节点自身重复上述两步
```

要点：

- 规则表在 **worker 内存**（`waf.lua` 的 `_M.RULES`），由 `init_by_lua_file` 在配置加载时填充，
  因此必须让 worker 重建（`reload`）才会看到新规则；`restart` 不是必需的，且会打断连接。
- `SyncService.ApplyRules/ApplySites` 的步骤固定为：**写文件 → `nginx -t` → `nginx -s reload`**，
  `-t` 失败即中止并把错误返回前端，坏配置不会生效。
- 演示模式（`RUN_MODE=demo`）下 `nginx.Manager.DryRun = true`：只渲染、不写盘、不 reload，
  便于本机体验。
- `RULE_PATH`（后台）必须与 lua `config_rule_dir` 完全一致，含尾斜杠，否则规则写了却读不到。
- 只有修改 `lua_shared_dict` 名称/大小、升级 nginx 二进制等场景才需要完全重启。

## 10. 文档地图

| 文档 | 面向 |
| --- | --- |
| [../README.md](../README.md) | 使用与部署（功能、快速开始、配置项） |
| [API.md](API.md) | 前后端联调（接口结构、错误码） |
| [../lua-waf/README.md](../lua-waf/README.md) | openresty 侧 WAF（检测顺序、规则格式、开关） |
| [../web/README.md](../web/README.md) | 前端开发（工程结构、设计系统） |
| [../CHANGELOG.md](../CHANGELOG.md) | 版本变更记录 |
