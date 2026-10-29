# X-WAF 管理台

openresty Lua WAF + Go 管理后台（Web 端用 Vue 3 重构）。WAF 只负责**判定与拦截**，
日志统一写 Redis，由后台进程批量入库，管理台负责规则/配置/日志/统计/AI 分析的可视化运维。

- 前台拦截：`lua-waf/`（openresty + LuaJIT）
- 管理后台：Go（`internal/`）+ Vue 3（`web/`）
- 数据流：`openresty → Redis(waf:logs) → wafadmin 消费者 → MySQL.waf_log`

## 1. 能力总览

| 模块 | 说明 |
|---|---|
| 安全概览 | 今日攻击量、攻击类型分布、Top 来源、趋势、检测与拦截顺序（可折叠）、日志入库队列指标 |
| 攻击地图 | 按来源地区聚合的 2D 地图（今日/近 7 天/全部） |
| 攻击日志 | 分页筛选（类型/IP/站点/日期）、详情、按标签查命中样本、AI 分析单条日志 |
| 防护分析 | 规则命中 Top（7 天）+ 命中样本抽屉 |
| WAF 规则 | 8 类规则集的增删改查、启用/停用、关键词检索、从规则文件同步（并集）、检测顺序说明 |
| 限频规则 | 按 URL 精细化限频：不同 URL 不同阈值，可按 IP/Header/Cookie/参数/URI/站点/方法/Referer 计数 |
| WAF 配置 | 在线编辑 `config.lua`（4 个选项卡 / 37 项）、Redis 连通性测试、生效自检 |
| 站点管理 | 受保护域名与后端配置，一键同步生成 nginx vhost |
| 慢 SQL 统计 | 慢查询采集、趋势、AI 分析、清理 |
| 系统设置 | 运行配置（app.ini）、AI 配置、**告警通知（企业微信机器人，命中勾选规则时提醒）**、主题切换、账号管理 |

## 2. 目录结构

```
lua-waf/                    openresty 侧 WAF（部署到 nginx 的 x-waf 目录）
  waf.lua                   检测主流程（12 步顺序判定）
  body.lua                  POST/JSON/multipart/上传检测
  malformed.lua             畸形请求（请求走私、控制字符、非法 Host…）
  ratelimit.lua             按 URL 的精细化限频（读取 rules/ratelimit.rule）
  log_sink.lua              日志出口：file / redis（失败回落 file + 熔断）
  log_redis.lua             Redis 生产端（RPUSH）
  config.lua                37 项配置（后台可在线编辑）
  lib/resty/                内置纯 Lua 依赖（redis / lrucache），无需 opm
  rules/*.rule              规则文件（JSON 数组，{Id,RuleType,RuleItem}）
  sql/waf_log.sql           waf_log 建表脚本（后台已能自动建表，仅权限不足时手工执行）
  nginx.conf                参考配置（lua_package_path / shared dict / init 钩子）

internal/                   Go 后端（分层：transport → service → repository → model）
  transport/httpapi/        路由、中间件、handler
  service/                  业务逻辑（规则/配置/日志/统计/AI/限频/自检）
  repository/{mysql,memory} 存储实现（memory 用于 demo 与测试）
  ingest/                   Redis → MySQL 消费者
  nginx/                    vhost / 规则文件生成器
  app/                      依赖装配（组合根）
pkg/                        通用件（response、redis 客户端、crypto、geo、sqlwatch…）
web/                        Vue 3 + Vite + Element Plus 前端
conf/app.ini                运行配置模板
docs/                       文档
```

## 3. 快速开始

### 3.1 本地体验（演示模式，无需 MySQL / nginx）

```bash
WAF_RUN_MODE=demo WAF_HTTP_PORT=5002 WAF_SESSION_SECRET=demo-secret go run .
# 浏览器打开 http://127.0.0.1:5002 ，默认账号 admin / admin123
```

演示模式使用内存仓储与内置样例数据，不会写 nginx 配置、也不会真的拦截请求。

### 3.2 生产部署

```bash
# 1) 编译（前端产物已 embed 进二进制）
cd web && npm install && npm run build && cd ..
go build -o wafadmin .

# 2) 配置
vi conf/app.ini          # 数据库、[waf] 路径、[log] Redis、[security] SESSION_SECRET

# 3) 启动
./wafadmin -config conf/app.ini

# 4) 部署 lua 侧并重载 nginx
rsync -av lua-waf/lib/ lua-waf/*.lua lua-waf/rules/ /www/server/nginx/x-waf/
nginx -t && nginx -s reload
```

必看的三条：`[waf] LUA_CONFIG` 必须指向 nginx **实际加载**的 `config.lua`；
`[log] INGEST_ENABLED = on`；`[security] SESSION_SECRET` 一定要设置（否则每次重启登录态失效）。

## 4. 日志链路（唯一一种入库方式）

```
openresty --RPUSH--> Redis(waf:logs) --BRPOPLPUSH--> wafadmin 消费者 --批量 INSERT--> MySQL.waf_log
```

| 环节 | 关键配置 | 说明 |
|---|---|---|
| 生产 | `config.lua`：`config_log_mode = "redis"` + `config_redis_*` | 写失败自动回落本地文件（不丢日志） |
| 队列 | `config_redis_list_key` ↔ `[log] REDIS_LIST_KEY` | 两边必须一致 |
| 消费 | `conf/app.ini`：`[log] INGEST_ENABLED = on` | lua 侧**不直连 MySQL**，不再需要 lua-resty-mysql |
| 落库 | `waf_log` 表 | 后台启动自动建表 + 补索引 |

`config_log_mode = "file"` 时日志只写本地文件，后台日志页不会有数据（属预期）。

## 5. 部署形态与依赖

| 依赖 | 要求 |
|---|---|
| openresty | ≥ 1.19（需要 `ngx.re`、`ngx.timer`、`lua_shared_dict`） |
| Lua 库 | `resty.redis`、`resty.lrucache`（已内置 `lua-waf/lib/`，无需 opm；也可换成官方库） |
| MySQL | 5.7 / 8.0，`waf` 库 + 读写账号 |
| Redis | 任意版本，仅用 List 命令（RPUSH / BRPOPLPUSH / LREM / LLEN） |
| Go | 1.24+（仅编译期） |
| Node | 18+（仅前端构建期） |

nginx.conf 需要的最小声明：

```nginx
lua_package_path "/www/server/nginx/x-waf/lib/?.lua;/www/server/nginx/x-waf/?.lua;;";
lua_shared_dict limit         100m;   # CC / 限频计数
lua_shared_dict waf_bans       20m;   # 自动封禁表
init_by_lua_file        /www/server/nginx/x-waf/init.lua;
init_worker_by_lua_file /www/server/nginx/x-waf/init_worker.lua;
access_by_lua_file      /www/server/nginx/x-waf/access.lua;
```

## 6. 文档索引

| 文档 | 内容 |
|---|---|
| [docs/README.md](docs/README.md) | 文档索引（按角色找入口） |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | 分层设计、模块职责、数据流、关键取舍 |
| [docs/CONFIG.md](docs/CONFIG.md) | `conf/app.ini` 与 `lua-waf/config.lua` 全部配置项 |
| [docs/RULES.md](docs/RULES.md) | 规则集格式、下发与同步、限频规则、检测顺序 |
| [docs/API.md](docs/API.md) | 全部 HTTP 接口（含请求/响应示例） |
| [docs/OPS.md](docs/OPS.md) | 部署、依赖、巡检、故障排查手册 |
| [docs/ROADMAP.md](docs/ROADMAP.md) | 优化路线图（P2/P3 待办，含收益与风险评估） |
| [CHANGELOG.md](CHANGELOG.md) | 变更记录（按主题） |

## 7. 常用命令

```bash
go build -o wafadmin .            # 编译
go test ./...                     # 测试（含 memory 仓储的回归用例）
./wafadmin reset-password admin   # 重置后台密码（CLI）
curl -s localhost:5002/api/health # 健康检查
```
## 规则自检（命令行 / 后台）

```bash
./wafadmin ruletest -config conf/app.ini                 # 全量自检：找死规则
./wafadmin ruletest -config conf/app.ini -payload "id=1 union select 1"   # 载荷验证
./wafadmin ruletest -config conf/app.ini -strict -json    # CI 卡口
```

后台「WAF 规则」页右上角「规则自检」= 同样能力的可视化版本（全量自检 / 载荷验证）。
