# X-WAF 云防火墙管理台

> 面向 **openresty / nginx + lua** 的 WAF 管理后台：规则管理、实时看板、攻击地图、防护分析、慢 SQL 审计、AI 研判，全部打包在一个二进制里（Go + 内嵌 Vue 前端）。

| 关键词 | 说明 |
|---|---|
| 后端 | Go 1.24，标准库 `net/http`（Go 1.22 路由），分层架构 `transport → service → repository → model` |
| 前端 | Vue 3 + Vite + Element Plus + ECharts，构建产物通过 `//go:embed` 内嵌进二进制 |
| 防护侧 | `lua-waf/`（waf.lua / body.lua / util.lua / config.lua / rules/*.rule）运行在 openresty 侧 |
| 存储 | MySQL（日志、规则、站点、账号、配置），可选 Redis 作为日志队列 |
| 交付 | 单个 `wafadmin` 可执行文件 + `conf/app.ini` + `lua-waf/` 目录 |

---

## 1. 功能一览

| 菜单 | 能做什么 |
|---|---|
| 安全概览 | 今日拦截 / 累计攻击 / 站点 / 规则数；近 15 天趋势、攻击类型分布、日志入库队列指标；顶部可展开「检测与拦截顺序」 |
| 攻击地图 | 来源地 → 防护目标的攻击连线大屏，默认展示**今日**数据，支持全部时间、主题切换、缩放漫游 |
| 防护配置 → 站点管理 | 反向代理站点（vhost）增删改、下发 nginx 配置、预览 |
| 防护配置 → 防护分析 | 规则命中 Top（今天 / 7 天 / 30 天）、命中日志样例、一键停用 / 启用规则 |
| 防护配置 → WAF 规则 | 按类型维护规则（args / post / cookie / url / ua / header / 黑白名单），逐条启停、从规则文件同步、下发全部规则 |
| 防护配置 → WAF 配置 | `config.lua` 在线编辑，按模块选项卡（基础 / 拦截页面 / 检测开关 / 频率与请求体 / 自动封禁 / 目录 / 日志输出 / MySQL / Redis），保存后可一键 reload |
| 防护配置 → 账号管理 | 管理员账号维护、修改口令 |
| 审计分析 → 攻击日志 | 多条件筛选、详情抽屉、AI 分析、IP 维度统计 |
| 审计分析 → 慢 SQL 统计 | 慢 SQL 列表、趋势聚合、AI 优化建议 |
| 系统设置 | 运行配置（app.ini 在线改）、外观设置（主题 / 暗色）、AI 设置（大模型接入）、账号设置 |

---

## 2. 快速开始

### 2.1 本地跑起来（演示模式，无需 MySQL / nginx）

```bash
go build -o wafadmin.exe .
WAF_RUN_MODE=demo WAF_HTTP_PORT=5003 ./wafadmin.exe
# 打开 http://127.0.0.1:5003
```

`demo` 模式使用**内存仓储 + 内置演示数据**，下发 nginx 配置走 `DryRun`（只渲染不落盘），适合本地看界面与联调。

### 2.2 生产部署

```bash
# 1) 准备配置：改 RUN_MODE=prod、数据库、路径、SESSION_SECRET
cp conf/app.ini conf/app.ini.local

# 2) 编译前端（构建产物会被内嵌）
cd web && npm install && npm run build && cd ..

# 3) 编译后端并启动
go build -o wafadmin .
./wafadmin -config conf/app.ini

# 4) 把 lua-waf/ 部署到 openresty 侧，并在 vhost 中 access_by_lua 调用 waf.check()
```

细节见 [docs/OPS.md](docs/OPS.md)。

---

## 3. 目录结构

```
lua-waf 管理台
├── main.go                    入口（子命令分发 / HTTP 服务）
├── conf/app.ini               运行配置（可在线编辑）
├── internal/
│   ├── app/                   组合根：装配仓储、服务、路由
│   ├── cli/                   子命令：version / reset-password / list-users / help
│   ├── config/                app.ini 解析、写回、校验
│   ├── ingest/                Redis 日志队列消费者（批量写 MySQL，at-least-once）
│   ├── model/                 领域模型 + 内置规则类型目录
│   ├── nginx/                 站点配置与规则文件渲染、nginx reload
│   ├── repository/            仓储接口 + mysql / memory 两套实现
│   ├── service/               业务逻辑（规则、日志、站点、AI、看板、WAF 配置…）
│   └── transport/httpapi/     HTTP 路由、中间件（会话 / CSRF / 限流 / 恢复）
├── pkg/                       captcha / crypto / geo / ratelimit / redis / response / session / sqlwatch
├── lua-waf/                   openresty 侧 WAF（waf.lua、body.lua、util.lua、config.lua、rules/）
├── web/                       Vue 3 前端源码（构建产物 web/dist 内嵌）
└── docs/                      架构、配置、规则、接口、运维文档
```

---

## 4. 文档索引

| 文档 | 内容 |
|---|---|
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | 分层架构、模块职责、请求链路、关键设计决策 |
| [docs/CONFIG.md](docs/CONFIG.md) | `app.ini` 全部配置项 + 环境变量 + `config.lua` 47 项 WAF 配置说明 |
| [docs/RULES.md](docs/RULES.md) | 规则文件格式、12 步检测链路、匹配语义、规则清单、误报治理 |
| [docs/API.md](docs/API.md) | 全部 HTTP 接口（分组 + 示例） |
| [docs/OPS.md](docs/OPS.md) | 部署、升级、reload、日志链路排障、备份回滚、FAQ |
| [CHANGELOG.md](CHANGELOG.md) | 变更记录 |

---

## 5. 三条核心链路（速查）

### 5.1 请求防护链路（openresty 侧）

```
请求 → 1 IP 白名单(放行) → 2 封禁名单/自动封禁 → 3 畸形请求 → 4 IP 黑名单
     → 5 请求头检测 → 6 User-Agent → 7 URL 白名单(放行) → 8 URL 检测
     → 9 CC 频率 → 10 Cookie → 11 GET 参数 → 12 请求体（JSON 按字段 / multipart 按分段）
     → 命中：写日志 → 拦截（html / redirect / 幻影降级）→ 可选自动封禁
```

所有规则统一以 `ngx.re.find(..., "joi")` 执行（PCRE + JIT + **大小写不敏感**）；
URL / Cookie / 参数 / 请求头在匹配前会做 `util.normalize()`（多轮 URL 解码 + `%uXXXX` + HTML 实体 + 去 SQL 注释）。
详见 [docs/RULES.md](docs/RULES.md)。

### 5.2 日志入库链路（lua 只写，Go 负责入库）

| 模式 | 配置 | 说明 |
|---|---|---|
| 文件 | `config_log_mode = "file"` | lua 写本地日志文件，后台不读 |
| Redis + 后台消费 | `config_log_mode = "redis"` + `conf/app.ini` 的 `[log] INGEST_ENABLED = on` | lua 只 RPUSH 进队列，`wafadmin` 批量写 MySQL（at-least-once）——**推荐** |

> lua 侧已**移除直连 MySQL** 的能力（不再需要 `lua-resty-mysql`）：落库统一交给后台，
> 削峰、可观察（队列长度 / 失败数 / 最近入库时间），也不会因为数据库抖动拖慢 nginx。

看板「日志入库队列」卡片会实时显示待处理 / 处理中 / 已入库 / 失败、队列键与最近错误。

### 5.3 AI 研判链路

日志或慢 SQL → 后台拼装提示词（命中规则、请求内容、SQL 与执行信息）→ 调用 OpenAI 兼容接口 →
结果写服务端缓存（默认 10 分钟）→ 前端展示 Markdown 与截断提示。配置入口：系统设置 → AI 设置。

---

## 6. 安全设计要点

- 登录：图形验证码 + 登录频率限制；会话为 HMAC 签名 Cookie（`waf_session`）；
- CSRF：双提交（`waf_csrf` cookie + `X-CSRF-Token` 头），写接口强制校验；
- 口令：`md5(md5(raw)[5:10])` 兼容历史库；忘记密码用 `./wafadmin reset-password` 重置；
- 敏感配置（MySQL / Redis 口令、AI api_key）在接口层**脱敏**（`abcd****wxyz`），提交掩码表示不修改；
- SQL 全部走仓储层参数化；慢 SQL 由 `pkg/sqlwatch` 采集。

---

## 7. 常见问题（速查）

| 现象 | 处理 |
|---|---|
| 重启后要重新登录 | `conf/app.ini` 的 `[security] SESSION_SECRET` 为空会每次随机生成，填固定值即可 |
| 「从规则文件同步」提示目录不可读 | 后台机器没有 nginx 规则目录，配置 `[waf] RULE_IMPORT_DIR = lua-waf/rules` |
| 规则改了不生效 | 后台点「下发全部规则」→ openresty 侧 `nginx -t && nginx -s reload` |
| 攻击日志不增长 | 检查 `config_log_mode`、Redis 队列长度、看板「日志入库队列」失败数与最近错误 |
| AI 分析报「调用大模型接口失败」 | 多为超时：检查 AI 设置的超时时间（分析类最少 180 秒）与网关连通性 |

更多见 [docs/OPS.md](docs/OPS.md)。
