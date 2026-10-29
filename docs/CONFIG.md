# 配置说明

配置分两处：

| 文件 | 作用 | 谁改 |
|---|---|---|
| `conf/app.ini` | 后台进程（Go）自身配置：数据库、路径、Redis 消费、安全 | 运维改，重启生效（部分支持后台热更新） |
| `lua-waf/config.lua` | openresty 侧 WAF 行为（37 项） | 后台「WAF 配置」在线改，reload 生效 |

---

## 1. `conf/app.ini`

所有键都支持环境变量覆盖（见 §1.8）。分节如下。

### 1.1 `[server]`

| 键 | 默认 | 说明 |
|---|---|---|
| `RUN_MODE` | `prod` | `prod` / `demo`（demo=内存仓储 + 样例数据 + 不写 nginx） |
| `HTTP_PORT` | `5002` | 监听端口 |
| `API_KEY` | `change-me-please` | 多节点同步配置时的共享密钥，所有节点必须一致 |
| `NGINX_BIN` | `/www/server/nginx/sbin/nginx` | nginx 可执行文件路径（用于 `-t` / reload） |
| `NGINX_VHOSTS` | `/www/server/panel/vhost/nginx/` | vhost 配置输出目录 |
| `SSL_CERT_DIR` | `/www/server/panel/vhost/cert` | 站点证书目录（`<域名>/fullchain.pem`、`privkey.pem`） |
| `API_SERVERS` | `127.0.0.1` | 集群内其它 WAF 节点（逗号分隔） |

### 1.2 `[database]`

| 键 | 说明 |
|---|---|
| `HOST` / `PORT` / `USER` / `PASSWORD` / `NAME` | MySQL 连接信息（仅 MySQL） |
| `AUTO_MIGRATE` | `true`：表缺失时自动创建（默认） |
| `ADD_MISSING_COLUMNS` | `true`：已存在的表只补缺失列，不做破坏性变更 |
| `SHOW_SQL` | 是否打印 SQL（也可在后台「系统设置」热切换） |

### 1.3 `[waf]`

| 键 | 说明 |
|---|---|
| `LUA_CONFIG` | **nginx 实际加载的 config.lua 绝对路径**（如 `/www/server/nginx/x-waf/config.lua`）。配成相对路径会改到仓库副本，reload 也不生效 |
| `MAP_TARGET` | 攻击地图的防护目标位置（如 `中国-北京`） |
| `RULE_PATH` | lua 侧规则目录（下发目标，需与 nginx 一致） |
| `RULE_IMPORT_DIR` | 「从规则文件同步」的候选目录（后台与 nginx 不同机时用本机副本） |
| `LOG_DIR` | file 模式的日志目录（仅用于展示/校验） |
| `IP2REGION_DB` | ip2region 数据文件路径（后端解析 IP 归属地；不可用时自动回退到 lua 侧解析） |

### 1.4 `[log]`

| 键 | 默认 | 说明 |
|---|---|---|
| `INGEST_ENABLED` | `on` | **redis 日志模式必须为 on**，否则没人消费队列 |
| `REDIS_ADDR` | `127.0.0.1:6379` | Redis 地址 |
| `REDIS_PASSWORD` | 空 | 口令，必须与 `config.lua` 的 `config_redis_password` 一致 |
| `REDIS_DB` | `0` | 库号 |
| `REDIS_LIST_KEY` | `waf:logs` | 队列键，必须与 `config_redis_list_key` 一致 |
| `INGEST_BATCH` | `200` | 单批入库条数 |
| `INGEST_BLOCK_MS` | `2000` | BRPOPLPUSH 阻塞毫秒数 |
| `RETENTION_DAYS` | `0` | 日志保留天数：`waf_log` 只保留最近 N 天，更早的数据**不删除**，按月归档到 `waf_log_YYYYMM`；0=不归档（后台「系统设置」也能改） |

### 1.5 `[security]`

| 键 | 说明 |
|---|---|
| `SESSION_SECRET` | 会话签名密钥。**必须设置**，留空则每次重启随机生成（登录态全部失效） |
| `SESSION_TTL` | 会话有效期（默认 12h） |
| `SESSION_SECURE` | HTTPS 访问时设为 `true`（Cookie 加 Secure） |
| `CAPTCHA_TTL` | 验证码有效期 |

### 1.6 `[pager]`

| 键 | 说明 |
|---|---|
| `PAGEMax` | 单页最大条数（默认 10，接口 size 会被夹到该值） |

### 1.7 `[monitor]`

| 键 | 说明 |
|---|---|
| `SLOW_SQL_MS` | 慢 SQL 阈值（毫秒），超过则被观察器记录 |

### 1.8 环境变量覆盖

| 环境变量 | 覆盖 |
|---|---|
| `WAF_RUN_MODE` | `[server] RUN_MODE` |
| `WAF_HTTP_PORT` | `[server] HTTP_PORT` |
| `WAF_SESSION_SECRET` | `[security] SESSION_SECRET` |
| `WAF_LUA_CONFIG` | `[waf] LUA_CONFIG`（本地联调常用 `lua-waf/config.lua`） |
| `WAF_RULE_PATH` / `WAF_RULE_IMPORT_DIR` | 规则目录 |
| `WAF_LOG_DIR` | `[waf] LOG_DIR` |
| `WAF_LOG_INGEST` | `[log] INGEST_ENABLED` |
| `WAF_REDIS_ADDR` / `WAF_REDIS_PASSWORD` / `WAF_REDIS_DB` / `WAF_REDIS_LIST_KEY` | `[log]` 对应项 |
| `WAF_DB_HOST` / `WAF_DB_PORT` / … | `[database]` 对应项 |

---

## 2. `lua-waf/config.lua`（37 项，后台分 4 个选项卡）

后台「防护配置 → WAF 配置」按模块选项卡展示；字段顺序与文件一致，
保存时**只按行替换值**（保留注释、顺序与缩进），写前备份 `.bak`。

### 2.1 基础（15）

> 合并了「总开关/拦截方式」「频率与请求体」「自动封禁」「目录」四组。

| 键 | 说明 |
|---|---|
| `config_waf_enable` | 总开关；关闭后所有检测直接放行（含黑名单），仅用于临时排障 |
| `config_waf_model` | 拦截方式：`html` 拦截页 / `redirect` 跳转 / `jinghuashuiyue` 幻影降级 |
| `config_waf_redirect_url` | redirect 模式的跳转地址 |
| `config_expire_time` | 封禁时长（秒），0 表示不封禁 |
| `config_cc_rate` | CC 阈值，格式 `次数/秒`（如 `300/60`：60 秒内同一 IP+URI 超 300 次拦截） |
| `config_ipcc_rate` | IP 阈值，格式 `次数/秒`（如 `120/7200`） |
| `config_post_max_size` | 请求体检测上限（字节），超过只记录不深检 |
| `config_post_max_parts` | multipart 分段上限，超过视为畸形 |
| `config_auto_ban_check` | 自动封禁开关 |
| `config_auto_ban_window` | 自动封禁统计窗口（秒） |
| `config_auto_ban_threshold` | 窗口内命中次数阈值 |
| `config_auto_ban_types` | 参与自动封禁的攻击类型（逗号分隔，`*` 为全部） |
| `config_auto_ban_expire` | 封禁时长（秒） |
| `config_rule_dir` | 规则目录，须与 `[waf] RULE_PATH` 一致 |
| `config_log_dir` | file 模式日志目录，须与 `[waf] LOG_DIR` 一致 |

### 2.2 拦截页面（1）

| 键 | 说明 |
|---|---|
| `config_output_html` | 拦截页 HTML 模板（Lua 长字符串 `[[ … ]]`），支持 `%s` 占位；**不能包含 `]]`** |

### 2.3 检测开关（15）

| 键 | 对应阶段 |
|---|---|
| `config_white_ip_check` | 1 IP 白名单 |
| `config_black_ip_check` | 4 IP 黑名单 |
| `config_white_url_check` | 7 URL 白名单 |
| `config_url_check` | 8 URL 检测 |
| `config_url_args_check` | 11 GET 参数检测 |
| `config_user_agent_check` | 6 User-Agent 检测 |
| `config_cookie_check` | 10 Cookie 检测 |
| `config_header_check` | 5 请求头检测 |
| `config_post_check` | 12 请求体检测 |
| `config_post_multipart_check` | 12 multipart 分段检测 |
| `config_upload_check` | 12 上传后缀白名单校验 |
| `config_upload_ext_whitelist` | 允许的上传后缀（逗号分隔） |
| `config_cc_check` | 9 CC 频率检测 |
| `config_ipcc_check` | 9 IP 频率检测 |
| `config_malformed_check` | 3 畸形请求检测 |

### 2.4 日志输出（6）

> 合并了「日志输出」与「Redis 队列」两组：选 `redis` 时下面的 Redis 参数即日志队列。

| 键 | 说明 |
|---|---|
| `config_log_mode` | `file` 写本地文件；`redis` 写 Redis 队列，由后台消费入库（推荐） |
| `config_redis_host` / `config_redis_port` | Redis 地址 |
| `config_redis_password` | 口令（接口返回脱敏，提交脱敏值表示不修改） |
| `config_redis_db` | 库号 |
| `config_redis_list_key` | 队列键名，须与 `[log] REDIS_LIST_KEY` 一致 |

> 已移除：`config_mysql_*`（lua 不再直连 MySQL）、`config_redis_consume`（消费固定由后台承担）、
> `config_log_batch_size` / `config_log_flush_interval` / `config_log_queue_size`（批量参数由后台控制）。

---

## 3. 限频规则（数据库 + `rules/ratelimit.rule`）

后台「防护配置 → 限频规则」维护，保存后自动下发到 `RULE_PATH/ratelimit.rule`。

| 字段 | 取值 | 说明 |
|---|---|---|
| URL 匹配 | 前缀 / 正则 | 正则在 lua 侧用 `ngx.re.find(..., "joi")` |
| 计数维度 | `ip` / `header` / `cookie` / `arg` / `uri` / `site` / `method` / `referer` | 后三种不需要维度名 |
| 阈值 | `limit` 次 / `window` 秒 | 计数存 `ngx.shared.limit`，TTL=窗口 |
| 超限动作 | `block` 拦截 / `log` 仅记录 | 观察期建议先 log |

---

## 4. AI 配置（数据库 `setting` 表，后台可编辑）

| 键 | 说明 |
|---|---|
| `ai.config` | JSON：`base_url`、`api_key`、`model`、`temperature`、`timeout_seconds`、`prompt` |
| 分析缓存 | 同一条日志/慢 SQL 的分析结果缓存 10 分钟（`/api/ai/results/{logId}` 可回看） |

超时：分析类请求下限 180s、上限 900s，失败自动重试一次；错误会翻译成可读原因。
---

## 5. 告警通知（企业微信机器人）

后台「系统设置 → 告警通知」维护，配置存在数据库 `setting` 表（键 `alert.config`），**无需改 ini**。

| 字段 | 说明 |
|---|---|
| `enabled` | 总开关 |
| `webhook_url` | 企业微信群机器人地址（群 → 群机器人 → 添加机器人后复制） |
| `rule_ids` | **勾选的规则 ID**：只有命中这些规则才推送（对应日志里的 `rule_tag`） |
| `interval_sec` | 静默期，同一规则在该时间内只提醒一次（默认 60，范围 10~3600） |
| `mention_all` | 消息末尾是否附加 `@all` |

消息内容示例：

```
【X-WAF 告警】命中关注的规则
1. 2026-10-29 11:40:12｜get参数攻击
   站点：www.example.com
   URL：/api/order?id=1 union select 1
   来源：1.2.3.4 中国北京北京电信
   规则：union\s+select
```

> 单条消息最多列 8 条命中（其余折叠为「另有 N 条」）；推送在后台消费进程里进行，失败只记日志、不影响入库。
