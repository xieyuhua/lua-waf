# 配置说明

本项目有两层配置，**互不重叠**：

| 层 | 文件 | 谁在跑 | 用途 |
|---|---|---|---|
| 管理台配置 | `conf/app.ini` | Go 后台（`wafadmin`） | 端口、数据库、路径、会话、日志消费 |
| WAF 配置 | `lua-waf/config.lua` | openresty（lua） | 检测开关、阈值、日志模式、拦截页模板 |

两层配置都能在后台在线修改：

- `conf/app.ini` → 系统设置 → 运行配置（保存后热更新，标注「需重启」的项除外）
- `lua-waf/config.lua` → 防护配置 → WAF 配置（保存后点「仅 reload」或「保存并生效」让 nginx 重载）

---

## 1. `conf/app.ini`

### 1.1 顶层

| 键 | 默认 | 说明 |
|---|---|---|
| `RUN_MODE` | `prod` | `prod` / `dev` / `demo`。`demo` 用内存仓储 + 演示数据，下发配置只 DryRun |

### 1.2 `[server]`

| 键 | 默认 | 说明 |
|---|---|---|
| `HTTP_PORT` | `5002` | 管理台监听端口（**需重启**生效） |
| `NGINX_BIN` | `/www/server/nginx/sbin/nginx` | nginx 可执行文件路径，用于 `-t` 与 `-s reload` |
| `NGINX_VHOSTS` | `/www/server/panel/vhost/nginx/` | 反代站点 vhost 配置目录 |
| `API_SERVERS` | `127.0.0.1` | 多节点部署时其它管理台节点（逗号分隔） |

### 1.3 `[database]`

| 键 | 默认 | 说明 |
|---|---|---|
| `USER` / `PASSWD` | `waf` / `waf` | 数据库账号 |
| `HOST` / `PORT` | `127.0.0.1` / `3306` | 数据库地址 |
| `NAME` | `waf` | 库名 |
| `AUTO_MIGRATE` | `true` | 表不存在时自动建表 |
| `ADD_MISSING_COLUMNS` | `true` | 表已存在时**只补缺失列**（不修改、不删除），老库升级用 |
| `SHOW_SQL` | `false` | 是否打印执行的 SQL（排障用，量大时关闭） |

> `rules` 表的 `enabled` 列就是靠 `ADD_MISSING_COLUMNS` 自动补齐的，老数据默认 `1`（启用）。

### 1.4 `[waf]`

| 键 | 默认 | 说明 |
|---|---|---|
| `LUA_CONFIG` | `/www/server/nginx/x-waf/config.lua` | WAF 配置文件路径（WAF 配置页读写它） |
| `RULE_PATH` | `/www/server/nginx/x-waf/rules/` | 规则目录：「下发全部规则」把规则写到这里 |
| `RULE_IMPORT_DIR` | `lua-waf/rules` | 「从规则文件同步」的候选目录（后台与 nginx 不同机时用） |
| `LOG_DIR` | `/www/wwwlogs/waf` | file 模式下的 WAF 日志目录 |
| `MAP_TARGET` | `中国-北京` | 攻击地图的「防护目标」位置 |

> 从规则文件同步会同时扫描 `RULE_PATH` 与 `RULE_IMPORT_DIR`，**合并**后与数据库取并集：
> 文件里有、库里没有的新增；库里已有的保持不动（不覆盖、不删除）。

### 1.5 `[security]`

| 键 | 默认 | 说明 |
|---|---|---|
| `SESSION_SECRET` | 空 | 会话签名密钥。**为空时每次启动随机生成，重启即掉登录**，生产务必填固定值 |
| `SESSION_TTL` | `12h` | 会话有效期 |
| `SESSION_SECURE` | `false` | 走 HTTPS 时设为 `true`（Cookie 加 Secure） |
| `CAPTCHA_TTL` | `5m` | 登录验证码有效期 |
| `API_KEY` | — | 多节点同步密钥，各节点必须一致 |

### 1.6 `[log]` — 日志入库消费者（Redis 模式）

| 键 | 默认 | 说明 |
|---|---|---|
| `INGEST_ENABLED` | `off` | 是否启用后台消费 Redis 队列 |
| `REDIS_ADDR` | `127.0.0.1:6379` | Redis 地址 |
| `REDIS_PASSWORD` | 空 | Redis 口令 |
| `REDIS_DB` | `0` | 库号 |
| `REDIS_LIST_KEY` | `waf:logs` | 队列键（`lua-waf/config.lua` 的 `config_redis_list_key` 必须一致） |
| `INGEST_BATCH` | `200` | 单批取出条数 |
| `INGEST_BLOCK_MS` | `2000` | `BRPOPLPUSH` 阻塞毫秒数 |

### 1.7 `[monitor]`

| 键 | 默认 | 说明 |
|---|---|---|
| `SLOW_SQL_MS` | `200` | 后台自身 SQL 超过该耗时即记入「慢 SQL 统计」 |

### 1.8 环境变量（优先级高于 ini）

| 变量 | 对应配置 |
|---|---|
| `WAF_RUN_MODE` | `RUN_MODE` |
| `WAF_HTTP_PORT` | `[server] HTTP_PORT` |
| `WAF_CONFIG` | 配置文件路径（等价 `-config`） |
| `WAF_DB_HOST` / `WAF_DB_PORT` / `WAF_DB_USER` / `WAF_DB_PASSWD` / `WAF_DB_NAME` | `[database]` |
| `WAF_MAP_TARGET` | `[waf] MAP_TARGET` |
| `WAF_LUA_CONFIG` | `[waf] LUA_CONFIG` |
| `WAF_RULE_IMPORT_DIR` | `[waf] RULE_IMPORT_DIR` |
| `WAF_LOG_INGEST` | `[log] INGEST_ENABLED`（`on` / `off`） |
| `WAF_REDIS_ADDR` / `WAF_REDIS_PASSWORD` | `[log]` |
| `WAF_SESSION_SECRET` | `[security] SESSION_SECRET`（脚本化测试用） |

命令行子命令：

```bash
./wafadmin version                 # 版本
./wafadmin list-users              # 列出管理员
./wafadmin reset-password -u admin # 重置口令
./wafadmin help
```

---

## 2. `lua-waf/config.lua`（WAF 配置，共 37 项）

后台「防护配置 → WAF 配置」按模块选项卡展示，这里按同样顺序列出。
**lua 侧不再直连 MySQL**：`config_log_mode` 只有 `file` / `redis` 两种，日志落库统一由后台 `wafadmin` 消费 Redis 完成。

### 2.1 基础（4）

| 键 | 说明 |
|---|---|
| `config_waf_enable` | 总开关，关闭后所有检测直接放行（含黑名单），仅用于临时排障 |
| `config_waf_model` | 拦截方式：`html` 拦截页 / `redirect` 跳转 / `jinghuashuiyue` 幻影降级 |
| `config_waf_redirect_url` | redirect 模式的跳转地址 |
| `config_expire_time` | 封禁时长（秒），0 表示不封禁 |

### 2.2 拦截页面（1）

| 键 | 说明 |
|---|---|
| `config_output_html` | 拦截页 HTML 模板（Lua 长字符串 `[[ ... ]]`），支持 `%s` 占位；**不能包含 `]]`** |

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

### 2.4 频率与请求体（4）

`config_cc_rate`（`次数/秒`）、`config_ipcc_rate`、`config_post_max_size`（字节）、`config_post_max_parts`。

### 2.5 自动封禁（5）

`config_auto_ban_check`、`config_auto_ban_window`、`config_auto_ban_threshold`、`config_auto_ban_types`、`config_auto_ban_expire`。

### 2.6 目录（2）

`config_rule_dir`（须与 `[waf] RULE_PATH` 一致）、`config_log_dir`（file 模式日志目录，须与 `[waf] LOG_DIR` 一致）。

### 2.7 日志输出（1）

| 键 | 说明 |
|---|---|
| `config_log_mode` | `file` = 写本地日志文件；`redis` = 写 Redis 队列（推荐），由后台消费入库 |

### 2.8 Redis 队列（5）

| 键 | 说明 |
|---|---|
| `config_redis_host` / `config_redis_port` | Redis 地址 |
| `config_redis_password` | 口令（接口层脱敏） |
| `config_redis_db` | 库号 |
| `config_redis_list_key` | 队列键名，须与后台 `[log] REDIS_LIST_KEY` 一致 |

> 已移除的配置：`config_mysql_*`（6 项）、`config_redis_consume`（消费方改由后台固定承担）、
> `config_log_batch_size` / `config_log_flush_interval` / `config_log_queue_size`（落库参数由后台消费者控制）。
## 3. 后台「AI 设置」配置项

保存在数据库 `setting` 表（键 `ai.config`），不是 ini 文件：

| 字段 | 说明 |
|---|---|
| `base_url` | OpenAI 兼容接口地址，如 `http://host:7863/v1` |
| `model` | 模型名 |
| `api_key` | 密钥（接口返回脱敏） |
| `max_tokens` | 最大输出 tokens；`-1` 表示不传（由模型决定），上限 32768 |
| `temperature` | 采样温度 |
| `timeout_seconds` | 请求超时；分析类会强制不低于 180 秒，上限 900 |
| `cache_minutes` | 分析结果缓存分钟数（默认 10） |

---

## 4. 配置修改的生效方式

| 改了什么 | 生效方式 |
|---|---|
| `conf/app.ini` 运行参数 | 保存即热更新；标「需重启」的（如 `HTTP_PORT`）需重启进程 |
| `lua-waf/config.lua` | WAF 配置页保存后点「仅 reload」或「保存并生效」执行 `nginx -t && nginx -s reload` |
| `rules/*.rule` 文件 | 同上（需要 reload 才重新加载规则） |
| 数据库里的规则（后台增删改） | 点「下发全部规则」写回 `*.rule`，再 reload |
| AI 配置 | 保存即生效（下次调用使用） |
