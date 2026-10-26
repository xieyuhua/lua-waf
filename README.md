# X-WAF 管理台（wafadmin v2）

X-WAF 是面向中小企业的云 WAF 系统：openresty + lua 负责流量拦截，本仓库的 `wafadmin`
负责站点、规则、攻击日志、慢 SQL、AI 研判与攻击地图的可视化管理。

v2 相比 1.x 做了两件大事：

1. **后端按分层架构重构**：配置 / 领域模型 / 仓储 / 服务 / 接入层严格分离，全部业务以结构体 + 方法实现，
   补充了单元测试与集成测试；修复了 1.x 中 `waflog` 路由缺失导致无法编译的问题。
2. **后台改用 Vue 3 重写**：Vite + Element Plus + ECharts，支持亮/暗双主题、响应式布局与攻击地图大屏；
   构建产物通过 `//go:embed` 内嵌进二进制，仍是单文件部署。

## 文档索引

| 文档 | 内容 |
| --- | --- |
| 本文件 | 功能总览、快速开始、配置项、部署提示 |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | 分层设计、扩展流程、错误与安全设计、配置下发链路 |
| [docs/API.md](docs/API.md) | 全部 HTTP 接口的请求/响应结构与错误码 |
| [web/README.md](web/README.md) | 前端工程结构、命令、接口约定、设计系统 |
| [lua-waf/README.md](lua-waf/README.md) | openresty 侧 WAF：检测顺序、规则格式、请求体/畸形检测、配置项 |
| [CHANGELOG.md](CHANGELOG.md) | 1.x → 2.0.0 变更清单 |

**环境要求**：Go ≥ 1.24（构建二进制）、Node ≥ 20 与 npm（仅重建前端时需要）。

---

## 一、主要特性

| 模块 | 能力 |
| --- | --- |
| 安全概览 | 今日/累计拦截、站点与规则统计、15 天趋势、攻击类型分布、来源 IP Top5、最新拦截、最近慢 SQL |
| 攻击地图大屏 | 世界地图 + 攻击来源涟漪点、境外占比、地区 Top10、实时拦截滚动、自动刷新 |
| 站点管理 | 域名/端口/后端地址/诱饵地址/SSL/日志级别 CRUD，nginx 配置预览，一键下发与集群同步 |
| WAF 规则 | 按规则类型拆分子菜单（URL 参数、POST、Cookie、URL、UA、Header、IP 黑白名单、URL 白名单），增删改查、格式校验、一键下发 |
| 攻击日志 | 多条件筛选（关键词/IP/站点/类型/时间范围）、详情抽屉、勾选日志交给 AI 研判 |
| 慢 SQL 统计 | 时间范围查询（预设 + 自定义）、总数/均值/P95/超 1s 统计、耗时排行、明细与清理 |
| AI 设置 | OpenAI 兼容接口配置（地址/密钥/模型/温度/超时/提示词），连通性测试 |
| 账号管理 | 后台账号 CRUD、口令修改、登录失败限流、图形验证码、CSRF 防护 |
| 运行配置 | 网页编辑 `conf/app.ini`（端口、路径、密钥、分页、慢 SQL 阈值、数据库），保存即写回文件并热更新，标注需重启项，支持查看原文与从磁盘重载 |
| 外观设置 | 亮色 / 暗色 / 跟随系统 + 6 套主题色，顶栏一键切换，设置本地持久化 |

---

## 二、架构总览

```
                       ┌──────────────────────────────────────┐
  浏览器 (Vue 3 SPA) →  │ transport/httpapi  接入层（路由/中间件） │
                       └───────────────┬──────────────────────┘
                                       │ 只做协议转换与参数绑定
                       ┌───────────────▼──────────────────────┐
                       │ service   业务层（各业务域一个结构体）  │
                       └───────────────┬──────────────────────┘
                                       │ 只依赖仓储接口
                       ┌───────────────▼──────────────────────┐
                       │ repository  仓储层（接口 + 两种实现）   │
                       │   ├─ mysql  (xorm, 带慢 SQL 观测)      │
                       │   └─ memory (演示/测试, 内置样例数据)   │
                       └───────────────┬──────────────────────┘
                                       │
                       ┌───────────────▼──────────────────────┐
                       │ model 领域模型 + pkg 基础设施(无业务)   │
                       └──────────────────────────────────────┘

额外支撑：nginx 包负责 vhost/规则文件渲染与 reload；pkg/sqlwatch 把慢查询投递给服务层。
```

### 目录结构

```
.
├── main.go                      # 入口：解析配置、内嵌前端资源、启动应用、优雅退出
├── conf/app.ini                 # 配置文件（demo/dev/prod 三种运行模式）
├── internal/
│   ├── app/app.go               # 组合根：按层装配依赖、初始化数据、启动 HTTP 服务
│   ├── config/config.go         # 配置加载与校验（默认值 < ini < 环境变量）
│   ├── model/                   # 领域模型（一个业务域一个文件）
│   │   ├── site.go  rule.go  ruletype.go  user.go  waflog.go  slowsql.go
│   │   ├── setting.go  query.go           # 分页/查询条件
│   │   └── defaultrules.go                # 内置 WAF 规则数据
│   ├── repository/              # 仓储契约（按业务域拆文件）+ 两种实现
│   │   ├── site.go rule.go user.go log.go setting.go store.go
│   │   ├── mysql/               # xorm 实现，含慢 SQL 观测
│   │   └── memory/              # 内存实现（演示模式/测试）
│   ├── service/                 # 业务层：认证/站点/规则/用户/日志/看板/慢SQL/AI/地图/同步
│   ├── nginx/                   # vhost 与规则文件渲染、nginx -t / reload
│   └── transport/httpapi/       # 路由、中间件、各业务域 handler、SPA 托管
├── pkg/                         # 可复用基础设施（无业务语义）
│   ├── crypto/  session/  response/  captcha/  ratelimit/  geo/  sqlwatch/
├── lua-waf/                     # openresty 侧 WAF（规则、access 流程、POST/畸形检测）
└── web/                         # Vue 3 前端工程，构建产物 dist 被内嵌
```

### 分层约束（新增功能请遵守）

1. **一个业务域一个文件**：站点、规则、用户、日志、慢 SQL、AI、地图、同步互不混写。
2. **只用结构体 + 方法**：不使用游离的业务函数；构造函数之外的逻辑都挂在结构体上。
3. **依赖方向单向**：`transport → service → repository → model`；
   `model` 不依赖任何其它层，`repository` 不依赖 `service`。
4. **仓储必须成对实现**：新增查询时同时补 `mysql` 与 `memory` 两个实现，保证演示模式与测试可用。
5. **接入层不写业务**：解析参数、调用服务、映射错误码，仅此而已。

---

## 三、快速开始

### 1. 演示模式（无需数据库，开箱即用）

```bash
# 后端（内嵌已构建的前端）
go build -o wafadmin . && ./wafadmin -config conf/app.ini
# 浏览器访问 http://127.0.0.1:5002 ，账号 admin / admin
```

`conf/app.ini` 默认 `RUN_MODE = demo`：使用内存仓储 + 内置样例数据（站点、74 条规则、约 260 条攻击日志、
86 条慢 SQL），此模式下**只渲染不写入** nginx 配置文件与规则文件。

### 2. 生产模式（MySQL）

```ini
RUN_MODE = prod
[database]
USER = waf
PASSWD = ********
HOST = 127.0.0.1:3306
NAME = waf
```

首次启动会自动建表（`site` / `rules` / `user` / `slow_sql` / `setting`），
并在空库时写入初始账号 `admin / admin` 与 74 条内置规则，请立即修改口令。

### 3. 前端开发模式

```bash
cd web
npm install
npm run dev        # http://127.0.0.1:5173 ，/api 反向代理到 127.0.0.1:5002
npm run build      # 产出 web/dist，随后 go build 即可内嵌
npm run gen:map    # 重新生成攻击地图所需的 world.geo.json（基于 world-atlas）
```

### 4. 测试

```bash
go test ./...                 # 单元测试 + HTTP 接口集成测试（内存仓储）
go vet ./...
```

覆盖点：口令摘要历史兼容、接口签名、地区解析、站点校验、慢 SQL 时间范围与统计口径、
登录（验证码单次有效/错误口令/CSRF/未登录）、站点 CRUD、规则子菜单、攻击地图、看板。

---

## 四、配置说明（conf/app.ini）

| 段 | 键 | 说明 |
| --- | --- | --- |
| 根 | `RUN_MODE` | `demo`（内存+演示数据）/ `dev` / `prod`（MySQL） |
| server | `HTTP_PORT` | 后台监听端口，默认 5002 |
| server | `API_KEY` | 节点同步签名密钥，多台 WAF 必须一致 |
| server | `NGINX_BIN` / `NGINX_VHOSTS` | nginx 可执行文件与 vhost 目录 |
| server | `SSL_CERT_DIR` | 站点证书目录（`<域名>/fullchain.pem`、`privkey.pem`） |
| server | `API_SERVERS` | 集群内其它节点，逗号分隔 |
| waf | `RULE_PATH` / `LOG_DIR` | 规则文件目录 / WAF 日志目录 |
| pager | `PAGEMax` | 默认分页大小 |
| security | `SESSION_SECRET` | 会话签名密钥，留空则每次启动随机（重启需重新登录） |
| security | `SESSION_TTL` / `SESSION_SECURE` / `CAPTCHA_TTL` | 会话有效期 / HTTPS 下设为 true / 验证码有效期 |
| monitor | `SLOW_SQL_MS` | 后台自身 SQL 慢查询阈值（毫秒） |
| database | `AUTO_MIGRATE` | 是否允许自动建表（默认 true）。**仅在表不存在时创建，已存在的表不做任何结构同步** |

> 建表策略：启动时逐张检查 `site` / `rules` / `user` / `slow_sql` / `setting`，
> 存在的表直接跳过（不会 ALTER、不会重建主键），不存在的表才创建；`waf_log` 由 openresty 侧维护，后台只读。
> 关闭 `AUTO_MIGRATE` 后若缺表则启动报错，需手工初始化。

环境变量可覆盖：`WAF_CONFIG`、`WAF_RUN_MODE`、`WAF_HTTP_PORT`、`WAF_API_KEY`、
`WAF_SESSION_SECRET`、`WAF_DB_HOST`、`WAF_DB_USER`、`WAF_DB_PASSWD`、`WAF_DB_NAME`。

---

## 五、接口一览

统一响应：`{ "code": 0, "message": "ok", "data": ... }`，`code != 0` 表示业务错误。
除登录与验证码外均需会话 Cookie；非 GET 请求需带 `X-CSRF-Token`（值与 `waf_csrf` Cookie 相同）。
完整请求/响应字段、校验规则与错误码见 [docs/API.md](docs/API.md)。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/health` | 健康检查（无需登录） |
| GET | `/api/auth/captcha` | 获取图形验证码（base64 PNG） |
| POST | `/api/auth/login` | 登录（验证码 + 口令） |
| POST | `/api/auth/logout` | 注销 |
| GET | `/api/auth/profile` | 当前账号 |
| GET | `/api/dashboard/summary` | 看板汇总 |
| GET | `/api/attack-map?range=today\|all` | 攻击地图快照 |
| GET/POST/PUT/DELETE | `/api/sites`、`/api/sites/{id}` | 站点 CRUD |
| GET | `/api/sites/{id}/preview` | 预览渲染后的 vhost |
| POST | `/api/sites/sync`、`/api/sites/{id}/sync` | 下发站点配置 |
| GET | `/api/rules/types` | 规则类型 + 数量（子菜单） |
| GET/POST/PUT/DELETE | `/api/rules`、`/api/rules/{id}` | 规则 CRUD |
| POST | `/api/rules/validate` | 规则格式校验 |
| POST | `/api/rules/sync` | 下发全部规则 |
| GET/POST/PUT/DELETE | `/api/users`、`/api/users/{id}` | 账号 CRUD |
| POST | `/api/users/password` | 修改自身口令 |
| GET | `/api/logs` | 攻击日志分页（keyword/client_ip/server_name/attack_type/day/from/to） |
| GET | `/api/logs/overview`、`/ips`、`/types`、`/dates`、`/filters`、`/{id}` | 日志统计与筛选 |
| GET | `/api/slow-sql`、`/api/slow-sql/stats` | 慢 SQL 列表（时间范围）与统计 |
| POST | `/api/slow-sql/purge?days=30` | 清理历史慢 SQL |
| GET/PUT | `/api/ai/config` | AI 配置读取/保存（密钥脱敏） |
| POST | `/api/ai/test` | 连通性测试 |
| POST | `/api/ai/analyze` | 日志 AI 研判（`log_ids` / `question`） |
| GET | `/api/system/info` | 运行信息 |
| GET/PUT | `/api/system/config` | 读取 / 保存运行配置（写回 ini 并热更新） |
| POST | `/api/system/config/reload` | 从磁盘重新加载配置 |
| GET | `/api/system/config/raw` | 查看配置文件原文 |
| GET | `/api/sync/site`、`/api/sync/rule` | 节点同步回调（签名校验，兼容 1.x `/api/site/sync/`、`/api/rule/sync/`） |
| POST | `/api/sync/slow-sql` | 外部上报慢 SQL（签名校验） |

签名算法与 1.x 一致：`hash = md5(API_KEY + md5(timestamp + API_KEY))`，时间戳有效期 ±10 分钟。

---

## 六、慢 SQL 统计

两条采集通道：

1. **后台自身采集**：仓储层每次数据库访问都会计时，超过 `monitor.SLOW_SQL_MS`（默认 200ms）即写入
   `slow_sql` 表（来源标记 `admin`）。实现见 `pkg/sqlwatch` + `repository/mysql/*`。
2. **外部上报**：openresty/采集器调用签名接口，来源标记 `agent`：

```bash
TS=$(date +%s)
HASH=$(printf "%s%s" "$API_KEY" "$(printf "%s%s" "$TS" "$API_KEY" | md5sum | cut -d' ' -f1)" | md5sum | cut -d' ' -f1)
curl -X POST "http://127.0.0.1:5002/api/sync/slow-sql?timestamp=$TS&hash=$HASH" \
  -H 'Content-Type: application/json' \
  -d '{"statement":"SELECT * FROM orders WHERE ...","duration_ms":1830,"database":"shop","operation":"select","client_ip":"10.0.0.5"}'
```

时间查询规则：`from` / `to` 支持 `YYYY-MM-DD`、`YYYY-MM-DD HH:mm:ss`、RFC3339；
**只传日期时按整天计算**（`to` 自动补到 23:59:59）。统计口径：P95 为耗时升序 95 分位，
排行按“语句归一化”分组。

---

## 七、AI 日志研判

在「AI 设置」中填写 OpenAI 兼容接口（如 `https://api.openai.com/v1`、`https://api.deepseek.com/v1`）、
API Key、模型名并启用，即可在攻击日志页勾选 1~20 条日志点击「AI 分析」：
后端把日志字段（时间/来源/地区/站点/攻击类型/命中规则/路径/UA/请求体）拼装为提示词，
要求模型输出「结论 / 依据 / 风险等级 / 处置建议」。接口地址、密钥只保存在 `setting` 表，
返回前端时密钥脱敏显示。

---

## 八、lua-waf 增强：POST 分段与畸形请求防御

新增两个独立模块（`lua-waf/body.lua`、`lua-waf/malformed.lua`），由 `waf.check()` 统一调度：

**请求体检测（body.lua）**

- 统一读取请求体：`ngx.req.get_body_data()` 为空时回落到 `get_body_file()` 读取临时文件，
  解决**大请求体 / 分段上传绕过检测**的老问题（1.x 只扫 `get_post_args`）。
- 按 Content-Type 分段：`x-www-form-urlencoded` 逐参数解码、`multipart/form-data` 逐段解析、
  JSON 与其它类型整体检测。
- multipart 分段畸形：缺失 `boundary`、分段数量超过 `config_post_max_parts`、分段头含 NUL/裸换行、
  字段名注入（`"`、`;`、CR/LF）、上传文件名包含路径穿越 → 直接拦截。
- 超过 `config_post_max_size`（默认 8MB）只记录日志不深度扫描，避免影响正常上传。

**畸形请求检测（malformed.lua）**

- HTTP 请求走私：`Transfer-Encoding` 与 `Content-Length` 同时出现、重复 `Content-Length` 取值不一致、
  非单一 `chunked` 的 `Transfer-Encoding`。
- 头部注入：原始报文中的 NUL 字节、裸 CR/LF（非 CRLF 换行）。
- 编码畸形：非规范百分号编码、解码后的控制字符、URI 中原始控制字符、超长请求行、缺失/非法 Host。
- 二次编码（`%25xx`）默认只记录日志（避免误杀），可通过改造 `malformed.report` 转为拦截。

开关都在 `lua-waf/config.lua`：`config_post_multipart_check`、`config_post_max_size`、
`config_post_max_parts`、`config_malformed_check`。

**日志输出双模式（`config_log_mode`）**

| 模式 | 链路 | 适用 |
| --- | --- | --- |
| `file` | 追加写 `/www/wwwlogs/waf/x_waf.log`（一行一条 JSON） | 已有采集器读文件入库 / 本地审计 |
| `mysql` | 共享内存队列 → `ngx.timer` 每 2 秒批量 `INSERT INTO waf_log` | 单机高并发，不引入额外组件 |
| `redis` | Redis 列表 → `ngx.timer` 批量 `INSERT INTO waf_log` | 多台 WAF 汇聚、削峰、MySQL 抖动缓冲 |

请求链路不阻塞（内存写入 / 一次 Redis 命令），落库全部在 timer 中批量完成；
队列满、Redis 不可用、共享内存未声明都会**自动回落写文件**，落库失败会把整批记录回队重试；
多 worker 通过共享内存锁保证单写者。启用需建表（`lua-waf/sql/waf_log.sql`）、
在 `nginx.conf` 声明 `waf_log_queue`/`waf_log_meta` 并加上 `init_worker_by_lua_file`。

---

## 九、部署提示

1. `lua-waf/` 目录放入 `/www/server/nginx/x-waf/`，确保 `lua_package_path` 覆盖该目录
   （新增的 `body.lua`、`malformed.lua` 与 `waf.lua` 同级）。
2. 后台通过「站点管理 → 同步」生成 vhost 到 `NGINX_VHOSTS`，通过「WAF 规则 → 下发全部规则」
   生成 `*.rule` 到 `RULE_PATH`，两次操作都会执行 `nginx -t` 后再 `-s reload`。
3. 多台 WAF 部署时，把彼此的 `API_SERVERS` 填全并保持 `API_KEY` 一致，即可一键同步。
4. 生产环境务必：修改默认口令、设置 `SESSION_SECRET`、启用 HTTPS 后把 `SESSION_SECURE = true`。

### 规则/配置更新后要不要重启 nginx？

**不需要重启，`nginx -s reload` 即可，而且后台已经自动执行。**

规则在 `init_by_lua`（配置加载阶段）一次性读入 worker 内存（`waf.load_rules()` → `_M.RULES`），
因此改了磁盘上的 `.rule` 文件必须让 worker 重建才能生效；`reload` 会重建 worker 并重新执行
`init_by_lua`，而 `restart` 只是更粗暴地做同一件事并会打断连接。

后台下发链（`internal/service/sync.go` → `internal/nginx/runner.go`）：

```
写入 <type>.rule / <域名>.conf  →  nginx -t  →  通过才 nginx -s reload
                                          └─ 不通过则中止并向前端返回错误
```

需要**完全重启**的少数情况：

- 修改 `lua_shared_dict` 的名称或大小（共享内存尺寸变更 reload 不生效）；
- 升级 / 替换 nginx 二进制（`-s upgrade` 或重启）；
- `nginx.conf` 主进程级参数（worker 数、pid 路径等）调整且 reload 后行为异常时。

规则「没生效」的排查顺序：

1. 是否点了「下发全部规则」（只改数据库不发文件等于没生效）；
2. 后台 `conf/app.ini` 的 `RULE_PATH` 是否与 `lua-waf/config.lua` 的 `config_rule_dir` 完全一致（含尾斜杠）；
3. 多台 WAF 是否每台都同步成功（看接口返回的 `nodes`）；
4. 客户端是否仍压在老 worker 的长连接上（upstream 配了 `keepalive 32`，几十秒内沿用旧规则属正常）；
5. 调试期可临时把 `lua_code_cache` 设为 `off`，改 lua 即时生效（仅调试，性能损失大）。

## 十、许可

与上游项目保持一致，仅供授权范围内使用。
