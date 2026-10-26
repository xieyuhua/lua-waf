# 接口文档

所有接口以 `/api` 为前缀，返回统一信封：

```json
{ "code": 0, "message": "ok", "data": {} }
```

- `code = 0` 表示成功，非 0 为业务错误（见文末错误码表）。
- 除 `GET /api/auth/captcha`、`POST /api/auth/login`、`GET /api/health` 与 `/api/sync/*` 外，均需登录会话 Cookie `waf_session`。
- 除安全方法（GET/HEAD/OPTIONS）外，必须携带请求头 `X-CSRF-Token`，其值等于 Cookie `waf_csrf`。

分页响应统一结构：

```json
{ "items": [], "total": 128, "page": 1, "size": 20, "pages": 7 }
```

---

## 1. 健康检查

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/health` | 存活探测，返回 `{"status":"ok"}` |

---

## 2. 认证 `/api/auth`

### GET /api/auth/captcha

获取图形验证码（一次性，校验后失效）。

```json
{ "captcha_id": "440e928b5a28...", "image": "data:image/png;base64,iVBOR...", "expires_in": 300 }
```

### POST /api/auth/login

```json
{ "username": "admin", "password": "admin", "captcha_id": "440e928b...", "captcha_code": "8BHC" }
```

成功：下发 `waf_session`（HttpOnly）并返回

```json
{
  "user": { "id": 1, "username": "admin", "last_login": "2026-10-26T10:04:06+08:00", "created": "2026-09-26T10:04:06+08:00" },
  "redirect": "/admin/dashboard"
}
```

失败：验证码错误 `400/1001`，口令错误或限流命中 `401/1002`、`403/1003`。
同一来源 IP 5 分钟内最多 10 次尝试。

### POST /api/auth/logout

清除会话，返回 `{"message":"已退出登录"}`。

### GET /api/auth/profile

当前登录账号：`{ "id":1, "username":"admin", "last_login":"...", "created":"..." }`

---

## 3. 概览与系统 `/api/dashboard`、`/api/system`

### GET /api/dashboard/summary

```json
{
  "sites": 3, "rules": 74, "users": 2,
  "logs_total": 258, "logs_today": 21,
  "slow_sql_total": 86, "slow_sql_today": 12,
  "trend": [{ "date": "2026-10-12", "num": 19 }],
  "today_types": [{ "attack_type": "CC攻击", "num": 4, "area": "中国-北京" }],
  "today_ips": [{ "client_ip": "118.25.6.78", "area": "中国-江苏-南京", "num": 5, "attack_type": "SQL注入,XSS跨站" }],
  "recent_logs": [{ "id": 258, "client_ip": "221.229.166.13", "attack_type": "CC攻击", "req_url": "/download?name=..." }],
  "recent_slow_sql": [{ "id": 86, "duration_ms": 4914, "statement": "SELECT * FROM rules ..." }],
  "generated_at": "2026-10-26 10:07:50"
}
```

### GET /api/attack-map?range=today|all

攻击地图大屏数据。

```json
{
  "points": [{
    "name": "荷兰", "lat": 52.13, "lon": 5.29, "count": 76,
    "domestic": false, "area": "荷兰-阿姆斯特丹",
    "ips": ["45.83.12.7"], "attack_types": ["命令执行", "文件包含"]
  }],
  "total": 258, "domestic": 149, "overseas": 109, "foreign_rate": 42.2,
  "top_countries": [{ "name": "荷兰", "count": 76, "domestic": false }],
  "top_types": [{ "attack_type": "路径穿越", "num": 36 }],
  "trend": [{ "date": "2026-10-12", "num": 19 }],
  "recent": [],
  "site_count": 3,
  "range": "全部时间",
  "updated_at": "2026-10-26 10:11:13"
}
```

`points` 中无法解析地区的来源会归入 `name = "未知"`、`lat/lon = 0`，前端不渲染该点。

### GET /api/system/info

```json
{
  "version": "2.0.0", "run_mode": "demo", "demo": true, "page_size": 10, "http_port": 5002,
  "nginx_bin": "/www/server/nginx/sbin/nginx",
  "vhosts": "/www/server/panel/vhost/nginx/",
  "rule_path": "/www/server/nginx/x-waf/rules/",
  "api_servers": ["127.0.0.1"],
  "nginx_dry_run": true
}
```

`nginx_dry_run = true` 表示演示模式下只渲染配置、不写文件也不 reload。

### GET /api/system/config

读取运行配置（`security.SESSION_SECRET`、`API_KEY`、数据库口令均脱敏）。

```json
{
  "source": "conf/app.ini",
  "file": "conf/app.ini",
  "writable": true,
  "config": {
    "run_mode": "prod", "http_port": 5002,
    "api_key": "chan********ease", "api_servers": ["127.0.0.1"],
    "nginx_bin": "/www/server/nginx/sbin/nginx",
    "nginx_vhosts": "/www/server/panel/vhost/nginx/",
    "ssl_cert_dir": "/www/server/panel/vhost/cert",
    "rule_path": "/www/server/nginx/x-waf/rules/",
    "waf_log_dir": "/www/wwwlogs/waf",
    "page_size": 10,
    "session_secret": "********", "session_ttl": "12h0m0s", "session_secure": false,
    "captcha_ttl": "5m0s", "slow_sql_ms": 200,
    "database": { "host": "127.0.0.1", "port": 3306, "user": "waf", "password": "********", "name": "waf", "show_sql": false, "add_missing_columns": false }
  },
  "restart_required": []
}
```

### PUT /api/system/config

请求体与 `config` 同构；字符串字段留空表示保持原值，密钥字段传脱敏串（含 `****`）也视为保持原值。
保存后写入 ini（先备份 `app.ini.bak`，再临时文件 rename），并**立即热更新**可热更新的项。

```json
{ "page_size": 20, "slow_sql_ms": 300, "session_ttl": "8h", "api_servers": ["127.0.0.1", "10.0.0.9"] }
```

响应会把本次需要重启才生效的字段列在 `restart_required`：

```json
{ "file": "conf/app.ini", "writable": true, "config": { ... }, "restart_required": ["HTTP_PORT（监听端口）"] }
```

热更新范围：nginx 路径与演示开关、分页大小、慢 SQL 阈值、节点密钥、验证码有效期、
会话有效期/密钥/安全标志（改密钥会使既有会话失效）、**`database.show_sql`（SQL 日志开关）**。
需重启：`RUN_MODE`、`HTTP_PORT`、`database.*`（除 `show_sql`）、`security.SESSION_SECRET`。

### POST /api/system/config/reload

丢弃内存中的配置，从磁盘重新读取并应用（用于手工改过 ini 后同步）。响应同 `GET`。

### GET /api/system/config/raw

返回配置文件原文 `{ "content": "; wafadmin 配置文件\n..." }`。

---

## 4. 站点 `/api/sites`

站点对象：

```json
{
  "id": 1, "site_name": "www.example.com", "port": 80,
  "backend_addr": ["10.0.0.11:8080", "10.0.0.12:8080"],
  "unreal_addr": ["1.1.1.1"],
  "ssl": "off", "debug_level": "error",
  "last_change": "2026-10-19T10:04:06+08:00", "version": 1
}
```

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/sites?page&size&keyword` | 分页查询（keyword 匹配域名） |
| GET | `/api/sites/all` | 全部站点（下拉选择用，返回数组） |
| GET | `/api/sites/{id}` | 详情 |
| POST | `/api/sites` | 新增 |
| PUT | `/api/sites/{id}` | 修改（`version` 冲突时返回 400） |
| DELETE | `/api/sites/{id}` | 删除（同时清理 vhost 文件） |
| GET | `/api/sites/{id}/preview` | 预览渲染后的 nginx 配置 → `{ "content": "..." }` |
| POST | `/api/sites/sync` | 下发全部站点 |
| POST | `/api/sites/{id}/sync` | 下发单个站点 |

新增/修改请求体：

```json
{
  "site_name": "www.example.com",
  "port": 80,
  "backend_addr": ["10.0.0.11:8080"],
  "unreal_addr": [],
  "ssl": "off",
  "debug_level": "error"
}
```

校验规则：域名符合 `^(\*\.)?([a-z0-9-]+\.)*[a-z0-9-]+$` 且 ≤190 字符；端口 1–65535；
`backend_addr` 至少一条且形如 `ip:port` / `domain:port`；`ssl` 取 `on/off`；
`debug_level` 取 `debug|info|notice|warn|error|crit|alert|emerg`；域名不可重复。

同步响应：

```json
{
  "applied": 3,
  "message": "本机配置已生效",
  "nodes": [{ "server": "127.0.0.1", "status": "success", "message": "{\"Status\":0,\"Message\":\"successful\"}" }]
}
```

---

## 5. 规则 `/api/rules`

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/rules/types` | 规则类型 + 数量（用于子菜单） |
| GET | `/api/rules?type=&keyword=` | 按类型/关键词查询（不分页，返回 `{items,total,type}`） |
| POST | `/api/rules` | 新增 `{rule_type, rule_item}` |
| PUT | `/api/rules/{id}` | 修改 `{rule_item}`（`rule_type` 可选） |
| DELETE | `/api/rules/{id}` | 删除 |
| POST | `/api/rules/validate` | 格式校验 `{rule_type, rule_item}` → `{warning}` |
| POST | `/api/rules/sync` | 生成 `<type>.rule` 文件并 reload |

`GET /api/rules/types` 返回：

```json
[
  { "key": "args", "name": "URL 参数规则", "group": "拦截策略", "desc": "针对 GET 参数的注入特征匹配", "file": "args.rule", "count": 23 },
  { "key": "blackip", "name": "IP 黑名单", "group": "访问控制", "desc": "命中即拦截的来源 IP", "file": "blackip.rule", "count": 2 }
]
```

规则对象：`{ "id": 4, "rule_type": "args", "rule_item": "\\.\\./" }`。
`rule_item` 为 PCRE 片段，长度上限 255；`validate` 只对 Go 正则无法编译的 PCRE 语法给出提示，不阻断保存。

---

## 6. 账号 `/api/users`

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/users?page&size&keyword` | 分页查询（不含口令） |
| POST | `/api/users` | 新增 `{username, password}` |
| PUT | `/api/users/{id}` | 修改 `{username, password}`（password 留空表示不改） |
| DELETE | `/api/users/{id}` | 删除（不可删自己、不可删最后一个账号） |
| POST | `/api/users/password` | 修改自身口令 `{old_password, new_password}` |

用户名 3–32 位 `[A-Za-z0-9_.-]`；新口令至少 8 位。

---

## 7. 攻击日志 `/api/logs`

日志对象：

```json
{
  "id": 258, "client_ip": "221.229.166.13", "local_time": "2026-10-26 22:16:52",
  "server_name": "www.example.com", "user_agent": "sqlmap/1.7.2",
  "attack_type": "CC攻击", "req_url": "/download?name=..%2F..%2Fwindows%2Fwin.ini",
  "area": "中国-北京", "req_data": "id=1' AND 1=1 --", "rule_tag": "args->cc"
}
```

### GET /api/logs

查询参数（全部可选，组合为 AND）：

| 参数 | 说明 |
| --- | --- |
| `page` / `size` | 分页，size 上限 200 |
| `keyword` | 关键词，匹配 client_ip / req_url / user_agent / attack_type / rule_tag / server_name |
| `client_ip` | 来源 IP（模糊匹配） |
| `server_name` | 站点（精确匹配） |
| `attack_type` | 攻击类型（精确匹配） |
| `day` | 精确到天，`2026-10-26` |
| `from` / `to` | 时间范围，`YYYY-MM-DD HH:mm:ss`（字符串比较） |

### 其他

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/logs/overview?days=15` | `{total,today,types,ips,trend,top}` 统计摘要 |
| GET | `/api/logs/filters` | 筛选项 `{attack_types:[], sites:[]}` |
| GET | `/api/logs/ips?day=&limit=` | 按 IP 聚合 `[{client_ip,area,num,attack_type}]` |
| GET | `/api/logs/types?day=` | 按类型聚合 `[{attack_type,num,area}]` |
| GET | `/api/logs/dates?days=15` | 每日趋势 `[{date,num}]`（升序、补齐空缺） |
| GET | `/api/logs/{id}` | 单条详情 |

---

## 8. 慢 SQL `/api/slow-sql`

记录对象：

```json
{
  "id": 86, "occurred_at": "2026-10-26T09:20:00+08:00",
  "source": "agent", "database": "waf", "operation": "insert",
  "statement": "INSERT INTO waf_log (client_ip, local_time, ...) VALUES (?, ?, ...)",
  "duration_ms": 702, "rows": 1, "client_ip": "10.0.0.15", "trace_id": "trace-000086"
}
```

### GET /api/slow-sql

| 参数 | 说明 |
| --- | --- |
| `page` / `size` / `keyword` | 分页与语句关键词 |
| `from` / `to` | 记录时间范围；支持 `YYYY-MM-DD`（按整天算）、`YYYY-MM-DD HH:mm:ss`、RFC3339 |
| `source` | `admin`（后台自身采集）/ `agent`（外部上报） |
| `min_ms` | 最小耗时（毫秒） |
| `sort_by` | `occurred_at`（默认）/ `duration` |
| `order` | `desc`（默认）/ `asc` |

### GET /api/slow-sql/stats

参数同上，返回：

```json
{
  "total": 86, "avg_ms": 2610, "max_ms": 4914, "p95_ms": 4764, "over_1s": 67,
  "top_items": [{ "statement": "SELECT * FROM rules WHERE rule_type = ?", "operation": "select", "count": 12, "avg_ms": 2970, "max_ms": 4914 }]
}
```

口径：P95 为耗时升序 95 分位；`top_items` 按“语句归一化（去多余空白、截断 120 字符）”分组，最多 10 条。

### POST /api/slow-sql/purge?days=30

删除早于 N 天的记录，返回 `{ "removed": 12, "days": 30 }`。

---

## 9. AI `/api/ai`

### GET /api/ai/config

```json
{
  "enabled": false, "configured": false,
  "config": {
    "enabled": false, "base_url": "https://api.openai.com/v1", "api_key": "",
    "model": "gpt-4o-mini", "temperature": 0.2, "max_tokens": 1024,
    "timeout_seconds": 60, "cache_minutes": 10, "system_prompt": "你是一名资深的 Web 应用防火墙（WAF）安全分析师。...",
    "updated_at": "2026-10-26 10:20:00"
  }
}
```

`config.api_key` 返回时为脱敏值（形如 `sk-a********1234`）。

### PUT /api/ai/config

请求体同 `config`；`api_key` 传空或含 `********` 时保留已保存的密钥。

### POST /api/ai/test

连通性测试，返回 `{ "content": "OK", "model": "gpt-4o-mini", "elapsed_ms": 830, "usage": {...} }`。

### POST /api/ai/analyze

```json
{ "log_ids": [258, 257], "question": "这些请求是否属于同一攻击源？", "refresh": false }
```

`refresh: true` 跳过缓存，强制重新调用模型。

响应：

```json
{
  "content": "结论：真实攻击……", "model": "gpt-4o-mini",
  "elapsed_ms": 2310, "log_count": 2,
  "usage": { "prompt_tokens": 800, "completion_tokens": 320, "total_tokens": 1120 },
  "cached": false
}
```

缓存：键为「接口地址 + 模型 + 系统提示词 + 排序后的日志 ID + 提问」，有效期取
`ai.config.cache_minutes`（默认 10 分钟，1~120）。命中时返回上次结果并带
`"cached": true, "cached_at": "2026-10-26 15:20:31"`，不再调用模型；保存 AI 配置会清空缓存。

`log_ids` 1–20 条，`question` 可空（使用默认研判指令）。返回：

```json
{
  "content": "## 结论\n...",
  "model": "gpt-4o-mini",
  "elapsed_ms": 6420,
  "log_count": 2,
  "usage": { "prompt_tokens": 800, "completion_tokens": 420, "total_tokens": 1220 }
}
```

---

## 10. 节点同步 `/api/sync`（签名鉴权，不使用会话）

签名：`hash = md5(API_KEY + md5(timestamp + API_KEY))`，`timestamp` 为 Unix 秒，允许 ±10 分钟偏差。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/sync/site?timestamp=&hash=&id=` | 本机生成 vhost（可指定 id）并 reload |
| GET | `/api/sync/rule?timestamp=&hash=` | 本机生成规则文件并 reload |
| POST | `/api/sync/slow-sql?timestamp=&hash=` | 外部上报一条慢 SQL |

1.x 兼容路径：`GET /api/site/sync/`、`GET /api/rule/sync/`（响应为 `{"Status":0,"Message":"successful"}`）。

`/api/sync/slow-sql` 请求体：

```json
{
  "occurred_at": "2026-10-26 09:20:00",
  "database": "shop", "operation": "select",
  "statement": "SELECT * FROM orders WHERE status = 1",
  "duration_ms": 1830, "rows": 12000,
  "client_ip": "10.0.0.5", "trace_id": "trace-abc123"
}
```

生成签名示例：

```bash
TS=$(date +%s)
INNER=$(printf "%s%s" "$TS" "$API_KEY" | md5sum | cut -d' ' -f1)
HASH=$(printf "%s%s" "$API_KEY" "$INNER" | md5sum | cut -d' ' -f1)
curl -X POST "http://127.0.0.1:5002/api/sync/slow-sql?timestamp=$TS&hash=$HASH" \
  -H 'Content-Type: application/json' \
  -d '{"statement":"SELECT 1","duration_ms":1500,"database":"waf","operation":"select"}'
```

---

## 11. 错误码

| code | HTTP | 含义 | 典型场景 |
| --- | --- | --- | --- |
| 0 | 200 | 成功 | — |
| 1001 | 400 | 参数不合法 | 校验失败、时间格式错误、乐观锁冲突 |
| 1002 | 401 | 未授权 | 未登录、会话过期、口令错误 |
| 1003 | 403 | 禁止 | CSRF 校验失败、登录尝试过于频繁、删除自己 |
| 1004 | 404 | 不存在 | 站点/规则/账号/日志不存在 |
| 1005 | 409 | 冲突 | 域名或账号重名 |
| 1500 | 500 | 内部错误 | 数据库异常、AI 接口调用失败、nginx 重载失败 |

错误响应示例：

```json
{ "code": 1005, "message": "站点域名 www.example.com 已存在" }
```
