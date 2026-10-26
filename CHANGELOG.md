# 变更记录

## 2.0.0

> 后端分层重构 + 后台 Vue 3 重写 + 慢 SQL / AI / 攻击地图等新能力。

### 架构

- 后端改为五层结构：`transport/httpapi → service → repository → model`，基础设施下沉到 `pkg/*` 与 `internal/nginx`；
  依赖方向单向，业务全部以**结构体 + 方法**实现，一个业务域一个文件。
- 组合根 `internal/app`：按「配置 → 仓储 → 服务 → 接入层」装配，负责首次初始化、优雅关闭。
- 仓储抽象为接口，提供 `mysql`（xorm）与 `memory`（演示/测试，含样例数据）两套实现。
- 配置改为 `默认值 < conf/app.ini < 环境变量`，支持 `WAF_*` 覆盖并在启动时校验。
- 修复 1.x 无法编译的问题（`routers/waflog.go` 空实现但 `main.go` 引用 4 个 handler）。
- 修正 `User` 结构体 `xrom` 标签拼写错误，账号名建立唯一索引。
- 移除 macaron 及其 session/csrf/captcha/cache 依赖，改用标准库 `net/http`（Go 1.22+ 路由）。

### 新增功能

- **慢 SQL 统计**：仓储调用计时（阈值 `monitor.SLOW_SQL_MS`）入库 + 外部签名接口上报；
  支持时间范围查询、总数/均值/P95/超 1s、耗时排行、明细、按天清理。
- **AI 能力**：OpenAI 兼容大模型配置（密钥脱敏存储）、连通性测试、对选中日志（1–20 条）生成研判结论。
- **攻击地图大屏**：地区文本 → 经纬度（内置 120+ 词表），世界地图涟漪点、境外占比、地区 Top10、实时拦截、自动刷新。
- **WAF 规则子菜单**：按「拦截策略 / 访问控制」拆分为 9 个子页面，带数量徽标。
- **攻击日志筛选**：关键词（IP/URL/UA/类型/规则/站点）+ IP + 站点 + 类型 + 时间范围组合；详情抽屉；勾选日志送 AI。
- 站点 nginx 配置预览接口；规则内容格式校验（PCRE/RE2 兼容性提示）。
- 演示模式（`RUN_MODE=demo`）：内置样例数据，无需 MySQL 与 openresty 即可体验全部界面。

### 安全

- 会话改为 HMAC-SHA256 签名的无状态 Cookie（HttpOnly + SameSite=Lax）。
- 双提交 Cookie 的 CSRF 防护（`waf_csrf` + `X-CSRF-Token`），写操作强制校验。
- 登录失败限流（同 IP 5 分钟 10 次）、图形验证码一次性有效。
- 统一安全响应头与 `Cache-Control: no-store`；口令最小长度 8 位（新增/修改时）。
- 口令摘要保持 1.x 算法不变（`md5(md5(raw)[5:10])`），老库账号可直接登录。

### 前端

- 全新 Vue 3 + Vite + Element Plus + ECharts 工程，替换 1.x 的 Bootstrap/Lumino 模板。
- 页面：登录、安全概览、攻击地图、站点管理、WAF 规则（子菜单）、账号管理、攻击日志、慢 SQL、AI 设置、账号设置。
- 亮/暗双主题、响应式布局、页面切换动效、骨架屏与空状态、表格操作列紧凑化。
- 构建产物通过 `//go:embed all:web/dist` 内嵌，保持单文件部署。

### lua-waf

- 新增 `body.lua`：请求体统一读取（含 `get_body_file` 临时文件场景），
  urlencoded / multipart 分段 / JSON 逐段检测；拦截缺失 boundary、分段数超限、分段头 NUL/裸换行、
  字段名注入、上传文件名路径穿越。
- 新增 `malformed.lua`：TE/CL 走私、重复 Content-Length、非单一 chunked、原始报文 NUL 与裸 CRLF、
  非规范百分号编码、解码后控制字符、超长请求行、Host 校验。
- `config.lua` 新增 `config_post_multipart_check`、`config_post_max_size`、`config_post_max_parts`、
  `config_malformed_check`；`waf.lua` 把畸形检测提前到检测链最前。
- 规则文件格式保持 `[{ "Id":.., "RuleType":.., "RuleItem":".." }]` 不变，兼容既有规则。

### 文档

- 重写 `README.md`（功能、快速开始、配置项、部署、规则生效与重载说明）。
- 新增 `docs/ARCHITECTURE.md`（分层约束、扩展流程、生效链路）、`docs/API.md`（接口与错误码）、
  `web/README.md`（前端工程）、`CHANGELOG.md`（本文件）。
- 重写 `lua-waf/README.md`（文件清单、检测顺序、规则格式、配置项、部署生效）。

### 工程

- `.gitignore` 覆盖二进制与前端依赖；移除仓库中误提交的 `server`、`wafadmin` 旧构建产物。
- 新增测试：`pkg/crypto`（口令兼容/签名）、`pkg/geo`（地区解析）、`internal/service`（站点校验、慢 SQL 口径）、
  `internal/transport/httpapi`（鉴权、CSRF、登录全流程、CRUD、规则子菜单、地图、看板、同步签名）。
- 移除 `models/`、`routers/`、`modules/`、`setting/`、`templates/`、`public/`（历史版本保留在 git 记录中）。

### 不兼容变更 / 迁移说明

| 项目 | 1.x | 2.0.0 |
| --- | --- | --- |
| 后台页面 | 服务端渲染模板（`/login/`、`/admin/index/`） | SPA（`/login`、`/dashboard`） |
| 接口形态 | 表单 POST + 重定向 | JSON `/api/*` + 会话 Cookie |
| CSRF | macaron csrf 中间件 | 双提交 Cookie + `X-CSRF-Token` |
| 入口 | `go run main.go` | `go run . -config conf/app.ini`，内嵌前端 |
| 配置 | 仅 ini | ini + 环境变量，`RUN_MODE` 支持 `demo/dev/prod` |
| 站点/规则下发 | 直接写文件后 reload | 写文件 → `nginx -t` → reload（检查失败则中止） |
| 数据库 | 自动建 site/user/rules | 追加 `slow_sql`、`setting` 表（`Sync2` 自动创建） |
| 口令算法 | `md5(md5(raw)[5:10])` | 保持不变（兼容老账号） |

数据库无需手工迁移：`Sync2` 会新增缺失的表与列；`waf_log` 仍由 openresty 侧写入，后台只读。
