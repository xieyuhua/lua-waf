# 部署与运维

## 1. 环境要求

| 组件 | 版本 / 说明 |
|---|---|
| Go | 1.24+（仅编译时需要） |
| Node.js | 20+（仅在改前端时需要，`web/dist` 已随仓库提供） |
| MySQL | 5.7 / 8.0，字符集 `utf8` |
| openresty | 1.19+（带 lua-nginx-module），WAF 运行侧 |
| Redis | 5.0+（可选，仅 `config_log_mode = "redis"` 时使用） |

## 2. 首次部署

### 2.1 管理台

```bash
# 编译（先建前端产物）
cd web && npm install && npm run build && cd ..
go build -o wafadmin .

# 配置：至少改这几项
#   [security] SESSION_SECRET = 32 位以上随机串   ← 否则重启掉登录
#   [database] 账号/地址/库名
#   [waf] LUA_CONFIG / RULE_PATH / RULE_IMPORT_DIR
vi conf/app.ini

# 启动（前台；生产用 systemd / supervisor 托管）
./wafadmin -config conf/app.ini
```

首次启动会自动建表（表已存在时只补缺失列）。默认管理员由初始化逻辑写入，密码可用
`./wafadmin reset-password -u <账号>` 重置。

### 2.2 openresty 侧

1. 把仓库里的 `lua-waf/` 拷到服务器（默认 `/www/server/nginx/x-waf/`）；
2. 确认 `config.lua` 的 `config_rule_dir`、`config_log_dir` 与后台配置一致；
3. 在 vhost 里挂载：

```nginx
lua_shared_dict limit 50m;          # CC / 自动封禁计数
lua_package_path "/www/server/nginx/x-waf/?.lua;;";
init_by_lua_block { waf = require "waf" }
access_by_lua_block { waf.check() }
```

4. `nginx -t && nginx -s reload`。

## 2.3 lua 侧依赖（需要哪些库）

WAF 的 lua 代码只依赖 openresty 自带的 `ngx.*`，外加三个 `resty.*` 库：

| 库 | 用途 | 缺失时的表现 | 解决 |
|---|---|---|---|
| `resty.redis` | redis 日志模式写队列 | `log_sink` 回落写文件 + error.log 出现「Redis 队列写入失败: module 'resty.redis' not found」 | **已内置** `lua-waf/lib/resty/redis.lua`（纯 Lua，无需编译） |
| `resty.lrucache` | IP 归属地缓存 | 归属地解析变慢 / 报错 | **已内置** `lua-waf/lib/resty/lrucache.lua` |
| `resty.mysql` | nginx 直连 MySQL 落库 | `mysql` 模式不可用 | **未内置**（协议复杂）。推荐改用 redis 模式 + 后台消费；或 `opm get openresty/lua-resty-mysql` |

内置库的引入方式（`lua_package_path` 加上 `lib/`，且放在最前面）：

```nginx
lua_package_path "/www/server/nginx/x-waf/lib/?.lua;/www/server/nginx/x-waf/?.lua;;";
```

> 如果你的机器上已装官方 lua-resty-redis，并希望优先用它，把 `/usr/local/openresty/lualib/?.lua`
> 放到 `lib/` 之前即可；两者接口同名同参，可互换。

**为什么推荐 redis 模式而不是 nginx 直连 MySQL**：nginx 直连库会在流量高峰把数据库压力带进
worker（即使异步也需要维护连接池与重试），而 redis 模式把落库交给后台 `wafadmin` 批量写入，
既能削峰，也能在后台看到队列长度、失败数等可观测指标。

## 3. 日常操作

| 场景 | 操作 |
|---|---|
| 改运行参数 | 系统设置 → 运行配置 → 保存（多数热更新；`HTTP_PORT` 等需重启进程） |
| 改 WAF 配置 | 防护配置 → WAF 配置 → 保存 → 「仅 reload」或「保存并生效」 |
| 加/改规则 | WAF 规则 → 编辑 → 「下发全部规则」→ openresty 侧 reload |
| 规则文件在服务器上被改过 | WAF 规则 → 「从规则文件同步」（并集，不会覆盖库里已有规则） |
| 停掉某条误报规则 | 防护分析 → 看样例 → 停用 → 下发 → reload |
| 切换日志入库方式 | 见 README 5.2；切换后记得「保存并生效」并重启管理台（`[log] INGEST_ENABLED`） |

## 4. 日志入库链路运维

### 4.1 Redis + 后台消费（推荐）

```ini
[log]
INGEST_ENABLED = on
REDIS_ADDR = 127.0.0.1:6379
REDIS_LIST_KEY = waf:logs
INGEST_BATCH = 200
INGEST_BLOCK_MS = 2000
```

```lua
-- config.lua
config_log_mode = "redis",
config_redis_consume = "off",   -- 关键：不要和 nginx 抢消费
config_redis_list_key = "waf:logs",
```

看板「日志入库队列」卡片每 10 秒刷新：

| 指标 | 正常表现 | 异常含义 |
|---|---|---|
| 待处理（`LLEN waf:logs`） | 稳态接近 0，攻击高峰短暂上升 | 持续增长 = 消费跟不上（加大 `INGEST_BATCH` 或扩容） |
| 处理中（`waf:logs:processing`） | 应贴近 0 | 长期 > 0 = MySQL 写入失败，看「最近错误」 |
| 失败 / 丢弃 | 0 | > 0 说明有坏数据（JSON 解析失败）或写库异常 |
| 最近入库时间 | 每批更新 | 长时间不更新 = 消费者没跑（检查启动日志与 `enabled`） |

Redis 建议：开启 AOF（`appendonly yes`），内存策略设为 `noeviction`（避免队列被淘汰丢日志）。

### 4.2 「配了 redis，却还在写文件」怎么查

按顺序排查（用 WAF 配置页的「**生效自检**」按钮可一次看到前 3 项）：

1. **改的是不是同一份文件**：后台 `[waf] LUA_CONFIG` 必须指向 nginx 实际加载的 config.lua
   （通常 `/www/server/nginx/x-waf/config.lua`）。若配成相对路径 `lua-waf/config.lua`，
   后台编辑的是仓库副本，nginx 读的是另一份 → reload 也不会变。自检会提示「检测到另一份配置文件」。
2. **Redis 到底通不通**：lua 用 `lua-resty-redis` 连 Redis，连不上/口令错时 `log_sink` 会
   **打 WARN 并回落到写文件**（这是有意的“不丢日志”设计）。用「测试连接」确认；再查
   `nginx error.log` 里是否有 `[waf] Redis 队列写入失败`。
3. **依赖是否齐全**：`lua-resty-redis` 是否安装、`nginx.conf` 是否声明了
   `lua_shared_dict waf_log_queue 20m;` 与 `lua_shared_dict waf_log_meta 1m;`。
4. **消费方是否冲突**：`config_redis_consume = "on"`（nginx 消费）与
   `[log] INGEST_ENABLED = on`（后台消费）**只能开一个**。
5. **是否真的 reload**：改完必须 `nginx -t && nginx -s reload`；`config.lua` 是在
   `init_worker` 阶段加载的，不 reload 不会重新读取。

验证是否生效：`redis-cli -a <口令> LLEN waf:logs` 数值在涨 = nginx 已在写队列；
`ls -l <LOG_DIR>` 不再新增文件 = 文件模式已停止。

**现在可以在界面上一步看到结论**：WAF 配置 →「生效自检」，它会直接告诉你

- 日志模式（文件里实际的 `config_log_mode` 值）；
- Redis 是否连通、队列 `waf:logs` 当前长度（有攻击时应增长）；
- **日志目录最近是否还有新文件**（模式为 redis/mysql 却仍在 10 分钟内有写入 → 判定为「仍在写文件」，
  并提示这是 log_sink 回落，还是该目录其实是 nginx 的 access_log 目录）。

同时 lua 侧新增了一条提示：每个 worker 首次写日志时会打印

```
[waf] 日志输出模式=redis（file=/www/wwwlogs/waf；redis=127.0.0.1:6379 db=0；…）
```

`grep '日志输出模式' /www/server/nginx/logs/error.log` 就能确认**nginx 实际读到的模式**——
如果这里显示 `file`，说明 nginx 加载的 config.lua 不是后台改的那一份（或被旧文件覆盖）。

### 4.3 消费者没生效的排查

```bash
# 1) 看启动日志
[info] 日志入库消费者已启用：127.0.0.1:6379 <- waf:logs（批量 200）
[info] 日志入库消费者已启动：source=waf:logs batch=200
# 2) 环境变量是否覆盖了 ini
echo $WAF_LOG_INGEST
# 3) 接口直接看状态
curl -s 'http://127.0.0.1:5002/api/system/ingest'
```

## 5. 升级与回滚

```bash
# 升级
systemctl stop wafadmin
git pull && (cd web && npm run build) && go build -o wafadmin .
systemctl start wafadmin          # 自动补缺失列，不动已有数据

# 回滚
systemctl stop wafadmin
git checkout <上一个提交> && go build -o wafadmin .
systemctl start wafadmin
```

配置文件回滚：

- `conf/app.ini` → 后台「查看原文」可复制；建议改动前 `cp conf/app.ini conf/app.ini.bak`
- `lua-waf/config.lua` → 后台每次保存都会生成 `config.lua.bak`，直接 `cp config.lua.bak config.lua` 即可回滚
- 规则文件 → 每次「下发全部规则」前建议备份 `RULE_PATH` 目录

## 6. 常见问题（FAQ）

| 现象 | 原因 | 处理 |
|---|---|---|
| 重启后要重新登录 | `[security] SESSION_SECRET` 为空 → 每次随机 | 填固定值并重启 |
| 登录提示验证码错误 | 验证码 5 分钟过期 / 后端时间漂移 | 刷新验证码；校对服务器时间 |
| 「从规则文件同步」提示目录不可读 | 后台机器没有 nginx 规则目录 | 配 `[waf] RULE_IMPORT_DIR = lua-waf/rules` |
| 同步后某些规则显示但线上不生效 | 只同步了数据库，没下发+reload | 点「下发全部规则」→ `nginx -s reload` |
| Header 规则条数为 0 | 从未同步过 `header.rule` | 「从规则文件同步」后刷新 |
| 攻击日志不增长 | 日志模式/队列/消费者有问题 | 按第 4 节排查 |
| 攻击地图没有点 | 查询范围无数据，或 `area` 无法解析 | 切到「全部」；看「未定位」计数 |
| AI 分析报「调用大模型接口失败」 | 多为超时（分析类 30~100 秒） | AI 设置里超时 ≥180 秒；检查网关连通性 |
| AI 结果看不到 | 旧版本只存在浏览器本地 | 升级后结果存服务端，`GET /api/ai/results/{logId}` |
| 保存配置报 500 | 目标文件无写权限 | 检查 `LUA_CONFIG` / `app.ini` 所在目录权限与属主 |
| 慢 SQL 列表为空 | `[monitor] SLOW_SQL_MS` 阈值过高 | 调低阈值（如 200ms）观察 |

## 6.1 日志页/概览接口很慢或报「查询日志失败」

`waf_log` 在跑一段时间后常是百万级。若缺索引，`按类型统计`、`按 IP 排行`这类 `GROUP BY`
会退化成全表扫描（实测单次 20~45 秒），前端等不及就表现为超时 → 500。
后台启动时会**自动补建索引**（幂等，日志里能看到 `为 waf_log 补建索引 …`）：

| 索引 | 用途 |
|---|---|
| `idx_waf_log_local_time(local_time)` | 按日期/时间范围过滤（列表、趋势、概览） |
| `idx_waf_log_attack_type(attack_type)` | 攻击类型聚合 |
| `idx_waf_log_client_ip(client_ip)` + `idx_waf_log_ip_time(client_ip, local_time)` | 来源 IP 排行（Top N + 时间过滤） |
| `idx_waf_log_server_name(server_name)` | 站点筛选 |

同时做了三处查询优化：类型统计不再 `GROUP_CONCAT(area)`（取值上千种，代价极高）；
IP 排行改为「先聚合取 Top N 再回表取地区/类型」，且默认只统计近 30 天；筛选项改用
`SELECT DISTINCT attack_type` 并缓存 60 秒。

手工核对索引：

```sql
SHOW INDEX FROM waf_log;
```

## 7. 巡检清单（建议每日 / 每周）

每日：

1. 安全概览：今日拦截量、攻击类型分布是否异常；
2. 「日志入库队列」：待处理是否回落、失败是否为 0；
3. 防护分析 → 命中 Top：是否有异常飙升的规则（可能误报或新型攻击）；
4. 攻击日志：抽查最新攻击是否已被正确归类。

每周：

1. 规则命中样例抽查，处理误报；
2. 慢 SQL 统计：Top SQL 是否已优化；
3. 磁盘：MySQL 数据目录、`LOG_DIR`、`config.lua.bak` 大小；
4. 备份：数据库全量备份 + `conf/` 与 `lua-waf/` 目录备份。
