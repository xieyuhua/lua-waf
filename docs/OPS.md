# 运维手册

## 1. 部署

### 1.1 编译与启动

```bash
cd web && npm ci && npm run build && cd ..   # 前端产物 embed 进二进制
go build -o wafadmin .
./wafadmin -config conf/app.ini
```

后台默认监听 `[server] HTTP_PORT`（默认 5002）。健康检查：`GET /api/health`。

### 1.2 lua 侧部署

```bash
rsync -av --delete lua-waf/lib/ /www/server/nginx/x-waf/lib/
rsync -av lua-waf/*.lua lua-waf/rules/ /www/server/nginx/x-waf/
nginx -t && nginx -s reload
```

注意：服务器上的 `config.lua` 以实际那一份为准，首次部署建议先备份再覆盖。

### 1.3 数据库

后台启动时自动建表（缺失则创建、已存在则只补缺失列）：
`site / rules / user / slow_sql / setting / ratelimit_rule / waf_log`，
并自动补 `waf_log` 的查询索引（`local_time`、`attack_type`、`client_ip`、`server_name`、`(client_ip, local_time)`）。

若数据库账号没有建表权限，用 root 执行一次：

```bash
mysql -u root -p waf < lua-waf/sql/waf_log.sql
```

## 2. 配置要点

详见 [CONFIG.md](CONFIG.md)。最常踩的三条：

1. `[waf] LUA_CONFIG` 必须是 **nginx 实际加载**的 config.lua 绝对路径；
2. redis 日志模式必须 `[log] INGEST_ENABLED = on`，且 `REDIS_*` 与 `config.lua` 的 `config_redis_*` 一致；
3. `[security] SESSION_SECRET` 一定要设置，否则每次重启登录态全部失效。

## 3. 日常运维

### 3.1 规则维护

- 后台改规则 → 自动下发 `rules/*.rule` + reload；
- 运维直接改服务器文件 → 后台「WAF 规则 → 从规则文件同步」取并集入库；
- 同步口径：`去重后条数 = 新增 + 已存在`，`库内共 N 条` 应等于侧边栏各类型数字之和。

### 3.2 配置维护

`WAF 配置`页按 4 个选项卡组织（基础 / 拦截页面 / 检测开关 / 日志输出）。
保存前建议先「测试连接」（Redis PING）与「生效自检」（逐行给出 ok/warn/error）。

### 3.3 巡检清单

| 频率 | 检查项 |
|---|---|
| 每日 | 概览「日志入库队列」：`queue_length` 是否堆积、`failed` 是否增长、`last_flush_at` 是否新鲜 |
| 每日 | `nginx error.log` 是否出现 `Redis 队列写入失败` / `日志输出模式` 行 |
| 每周 | `waf_log` 行数与磁盘占用；慢 SQL 统计；规则命中 Top 是否有异常规则 |
| 每周 | 后台账号、`SESSION_SECRET`、数据库/Redis 口令是否仍有效 |

## 4. 故障排查

### 4.1 日志页/概览接口很慢或报「查询日志失败」

`waf_log` 百万级时缺索引会退化成全表扫描（单次 20~45 秒），前端等不及即报 500。
后台启动会自动补索引，也可手工核对 `SHOW INDEX FROM waf_log;`。

已做的优化：类型统计不再 `GROUP_CONCAT(area)`；IP 排行改两段式且默认只统计近 30 天；
筛选项改 `SELECT DISTINCT attack_type` 并缓存 60 秒。实测概览 45s → 1.8s、IP 排行 33s → 0.4s。

### 4.2 报 `Table 'waf.waf_log' doesn't exist`

表不存在（新库/换库）。后台重启会自动建表；若打印 `[error] 创建表 waf_log 失败：…`，
说明账号无建表权限，用 root 执行 `lua-waf/sql/waf_log.sql`，建好后消费自动恢复（无需重启）。

### 4.3 「配了 redis，却还在写文件」

1. **改的是不是同一份文件**：`[waf] LUA_CONFIG` 必须指向 nginx 实际加载的 config.lua
   （相对路径会解析到进程工作目录，往往是仓库副本）；
2. **Redis 通不通**：口令错/未启动时 `log_sink` 回落写文件（不丢日志），
   看 error.log 是否有 `Redis 队列写入失败（…）：WRONGPASS`；
3. **依赖是否齐全**：`resty.redis`（已内置 `lib/resty/redis.lua`）、`lua_shared_dict limit`；
4. **是否 reload**：`nginx -t && nginx -s reload`，否则 lua 仍用旧配置。

验证：`redis-cli -a <口令> LLEN waf:logs` 在涨 = nginx 已在写队列；日志目录不再新增文件 = 文件模式已停。

### 4.4 「Redis 里的日志一直没被消费」

按启动日志逐条核对：

| 日志 | 含义 | 处理 |
|---|---|---|
| `日志入库消费者未启用（INGEST_ENABLED=off）` | 没人消费 | `[log] INGEST_ENABLED = on` 后重启 |
| `无法连接 Redis（…）` | 地址/端口/网络 | 改 `REDIS_ADDR`，确认可达 |
| `Redis PING 失败（…）：NOAUTH/WRONGPASS` | 口令不一致 | 两边口令对齐 |
| `queue_length` 不降、`consumed` 不涨 | 队列键不一致 | `REDIS_LIST_KEY` 与 `config_redis_list_key` 对齐 |

`processing_length > 0` 且不降说明有一批卡在处理中，重启会自动回收到源队列重试（不丢）。

### 4.5 所有请求被拦截 / 被判成同一攻击类型

先用 `GET /api/rules/pipeline` 与 error.log 里的 `[waf] 日志输出模式=…` 定位阶段。
历史坑：`ngx.req.get_headers(0, true)` 返回的头名**不转小写**，按小写索引会恒为 nil
（曾导致所有请求被判「缺失Host头」）；自定义 lua 若出现同类现象，先查这一点。

### 4.6 nginx 报 `connect() failed (13: Permission denied) while logging to syslog`

与 WAF 无关：某处 `access_log syslog:server=unix:/tmp/xxx.sock` 的 socket 权限不足
（常见于面板的站点流量统计）。`ls -l` 看 socket 属主/权限，给 nginx worker 用户可写权限，
或改成 `access_log /path/site.log main;` / `syslog:server=127.0.0.1:514`。

### 4.6b 怀疑「规则没生效 / 被绕过」

先用规则自检工具定位，不必凭感觉加规则：

```bash
# 全量自检：列出永远命中不了的死规则、写错的正则
./wafadmin ruletest -config conf/app.ini

# 只查某一类
./wafadmin ruletest -config conf/app.ini -type args

# 载荷验证：这条参数/URL 到底会被哪条规则拦住（按 lua 检查顺序）
./wafadmin ruletest -config conf/app.ini -payload "id=1 union select 1&path=../../etc/passwd"

# CI 卡口：发现死规则就失败
./wafadmin ruletest -config conf/app.ini -strict
```

后台「WAF 规则」页右上角也有 **规则自检** 按钮，支持「全量自检」与「载荷验证」两种模式。

判定口径：

| 状态 | 含义 | 要不要处理 |
|---|---|---|
| `dead` | 连按规则自己生成的样例都命中不了 | **必须处理**（规则写坏了） |
| `bad-regex` | 正则不合法 | **必须处理** |
| `go-incompat` | Go/RE2 不支持（反向引用、前瞻） | 确认 lua 侧是否真生效（PCRE 支持） |
| `narrow` | 有效，但内置攻击样例没覆盖该形态 | 不用，属正常 |
| `skipped` | IP 名单/URL 白名单，无固定样例 | 不用 |

### 4.7 `waf_log` 越来越大

三层手段：

0. **归档（推荐）**：后台「系统设置 → 日志保留天数」设 N（或 `[log] RETENTION_DAYS = N`）。
   `waf_log` 只保留最近 N 天；更早的数据**不删除**，按月归档到 `waf_log_YYYYMM`
   （如 `waf_log_202610`，结构与 `waf_log` 完全一致，查历史直接查对应月份表）。
   每天执行一次，按 5 万条一批处理，日志里会打印
   `已归档超期日志 X 条（早于 …），归档表：waf_log_202609, …`。

1. **查询侧**（已内置）：`local_time` / `attack_type` / `client_ip` / `server_name` / `(client_ip, local_time)`
   索引自动补齐；类型统计不再 `GROUP_CONCAT(area)`；IP 排行默认只看近 30 天。
2. **容量侧**：`conf/app.ini` 的 `[log] RETENTION_DAYS`（默认 0 = 不清理）。
   设为 N 后，后台每天删除 `local_time` 早于「今天-N 天」的日志，日志里会打印
   `已清理超期日志 X 条（早于 …）`。

```ini
[log]
RETENTION_DAYS = 180      ; waf_log 保留 180 天，更早的按月归档
```

查历史：

```sql
SELECT * FROM waf_log_202609 WHERE client_ip = '1.2.3.4' ORDER BY id DESC LIMIT 20;
```

> 归档是分批的（每批 5 万行，先 INSERT 到月份表再 DELETE），不会一次锁表。
> 需要「按天分区」这种更细的粒度时见 `docs/ROADMAP.md`。

### 4.8 统计接口的缓存

概览 / 看板 / 攻击地图的结果缓存 **45 秒**（键分别为 `days`、`summary`、`range`）。
所以刚发生攻击后，最多 45 秒才会出现在概览数字里——这是有意的取舍（避免每次刷新都做多次聚合）。
需要立刻看最新数据时，直接查「攻击日志」页（该页不缓存）。

## 5. 备份与恢复

| 对象 | 建议 |
|---|---|
| MySQL | 每日 `mysqldump waf`；`waf_log` 可按天保留，历史归档 |
| `config.lua` / `rules/*.rule` | 后台改配置前会生成 `.bak`；建议随代码仓库版本管理 |
| `conf/app.ini` | 含口令，单独保管 |

恢复顺序：MySQL → `conf/app.ini` → lua 文件与规则 → 启动后台 → `nginx -s reload`。

## 6. 常用 CLI

```bash
./wafadmin reset-password admin     # 重置管理员口令（按提示输入新口令）
./wafadmin list-users               # 列出后台账号
./wafadmin version                  # 版本
./wafadmin -config conf/app.ini     # 正常启动（默认读 WAF_CONFIG 或 conf/app.ini）
```