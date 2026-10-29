# API 说明

- 基础路径：`http://<host>:<port>/api`
- 鉴权：登录后下发 `waf_session`（HttpOnly Cookie）；写操作需 **双提交 CSRF**：
  Cookie `waf_csrf` + 请求头 `X-CSRF-Token`
- 统一响应：`{"code":0,"message":"ok","data":…}`；`code` 非 0 表示业务错误

| code | 含义 |
|---|---|
| 0 | 成功 |
| 1001 | 参数不合法 |
| 1002 | 未登录 |
| 1003 | 无权限 |
| 1004 | 不存在 |
| 1005 | 冲突 |
| 1500 | 服务器内部错误 |

## 1. 认证与账号

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/auth/captcha` | 图形验证码（登录用） |
| POST | `/api/auth/login` | 登录（含验证码，失败次数受限流） |
| POST | `/api/auth/logout` | 退出 |
| GET | `/api/auth/profile` | 当前账号信息 |
| GET/POST | `/api/users` | 账号列表 / 新建 |
| PUT | `/api/users/{id}` | 修改账号 |
| DELETE | `/api/users/{id}` | 删除账号 |
| POST | `/api/users/password` | 修改自己的口令 |

口令存储：`md5(md5(raw)[5:10])`；忘记口令用 CLI `-reset-password`。

## 2. 日志

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/logs` | 分页查询，参数：`page`、`size`、`attack_type`、`client_ip`、`server_name`、`day`、`from`、`to` |
| GET | `/api/logs/{id}` | 单条详情 |
| GET | `/api/logs/overview?days=15` | 概览：总量、今日、类型分布、Top 来源、趋势、最近日志 |
| GET | `/api/logs/filters` | 筛选项：攻击类型、站点（缓存 60 秒） |
| GET | `/api/logs/types` / `/api/logs/ips` / `/api/logs/dates` | 类型/来源/日期聚合 |

> 大表优化：`TypeStats` 不再 `GROUP_CONCAT(area)`；`IPStats` 两段式且默认只统计近 30 天。

## 3. 统计与地图

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/dashboard/summary` | 安全概览数据 |
| GET | `/api/attack-map?range=today\|7d\|all` | 攻击地图：按地区聚合 + 源站列表（默认 `today`） |

## 4. WAF 规则

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/rules/types` | 规则类型与各类型条数（侧边栏子菜单用） |
| GET | `/api/rules?type=args&keyword=` | 按类型/关键词查询 |
| POST | `/api/rules` | 新建 |
| PUT | `/api/rules/{id}` | 修改 |
| PUT | `/api/rules/{id}/enabled` | 启用/停用 |
| DELETE | `/api/rules/{id}` | 删除 |
| POST | `/api/rules/validate` | 校验正则合法性 |
| POST | `/api/rules/sync` | 下发规则文件到 nginx |
| POST | `/api/rules/sync-files` | **从规则文件同步（取并集）**，返回 `files/in_files/added/kept/duplicates/skipped/total` |
| GET | `/api/rules/pipeline` | 检测与拦截顺序说明 |
| GET | `/api/rules/stats?days=7&limit=20` | 规则命中 Top（按 `rule_tag` 聚合，默认 7 天） |
| GET | `/api/rules/stats/samples?rule_tag=&days=7` | 某条规则的命中样本 |

## 5. 限频规则

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/ratelimit` | 列表（含 `key_label`、`rate_text` 与 `rule_dir`） |
| POST | `/api/ratelimit` | 新建并下发 |
| PUT | `/api/ratelimit/{id}` | 更新并下发 |
| PUT | `/api/ratelimit/{id}/enabled` | 启用/停用 |
| DELETE | `/api/ratelimit/{id}` | 删除 |
| POST | `/api/ratelimit/sync` | 手动重新下发 `ratelimit.rule` |

请求体示例：

```json
{
  "name": "开放 API 按密钥限流",
  "pattern": "^/open/api/",
  "match": "regex",
  "key": "header",
  "key_name": "X-API-Key",
  "limit": 1000,
  "window": 60,
  "action": "block",
  "enabled": true
}
```

校验：正则必须可编译；`header/cookie/arg` 维度必须给 `key_name`；`limit` 1~1000000；`window` 1~86400；
`action` 只能是 `block` / `log`。

## 6. WAF 配置（config.lua）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/lua/config` | 按 4 个选项卡返回 37 项（含 `type/label/desc/value/masked`，密钥脱敏） |
| PUT | `/api/lua/config` | 保存；`values` 的值可以是字符串/数字/布尔，后端统一转字符串后按行替换 |
| GET | `/api/lua/config/raw` | 读原文（在线编辑用） |
| POST | `/api/lua/config/test` | **保存前连通性测试**：Redis PING，返回 `ok/latency_ms/message` |
| GET | `/api/lua/config/verify` | **生效自检**：路径/文件一致性、日志模式、Redis 连通、目录、最近入库 |
| POST | `/api/lua/reload` | 执行 `nginx -t && nginx -s reload` |

自检返回示例：

```json
{ "path": "/www/server/nginx/x-waf/config.lua", "mode": "redis", "verdict": "warn",
  "items": [ { "key": "redis_conn", "label": "Redis 连通性", "level": "error",
               "message": "PING 失败（口令或权限可能不对）：WRONGPASS…" } ] }
```

`level`：`ok` / `warn` / `error`；`verdict` 取最严重的一项。

## 7. 站点

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/sites` / `/api/sites/all` | 分页 / 全部（下拉用） |
| GET | `/api/sites/{id}` / `/api/sites/{id}/preview` | 详情 / vhost 预览 |
| POST | `/api/sites` · PUT `/api/sites/{id}` · DELETE `/api/sites/{id}` | 增改删 |
| POST | `/api/sites/{id}/sync` / `/api/sites/sync` | 单站 / 全量同步（生成 vhost + reload） |
| GET | `/api/sync/site` / `/api/site/sync/` | 兼容旧的同步入口 |
| GET | `/api/sync/rule` / `/api/rule/sync/` | 兼容旧的规则同步入口 |

## 8. 慢 SQL

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/slow-sql` | 分页查询（时间筛选、关键词） |
| GET | `/api/slow-sql/stats` | 统计（趋势、Top） |
| POST | `/api/slow-sql/purge` | 清理 |
| POST | `/api/sync/slow-sql` | 采集/同步 |

## 9. AI

| 方法 | 路径 | 说明 |
|---|---|---|
| GET/PUT | `/api/ai/config` | AI 配置（可编辑，密钥脱敏） |
| POST | `/api/ai/test` | 连通性测试 |
| POST | `/api/ai/analyze` | 分析日志（body：`log_id`） |
| GET | `/api/ai/results/{logId}` | 回看日志分析结果（缓存 10 分钟） |
| POST | `/api/ai/slow-sql/analyze` | 分析慢 SQL |
| GET | `/api/ai/slow-sql/results/{id}` | 回看慢 SQL 分析结果 |

## 10. 系统

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/health` | 健康检查（无需登录） |
| GET | `/api/system/info` | 版本、运行模式、路径等 |
| GET | `/api/system/config` / `/api/system/config/raw` | 运行配置（值脱敏） / 原文 |
| PUT | `/api/system/config` | 保存运行配置 |
| POST | `/api/system/config/reload` | 重新加载配置（部分项热生效） |
| GET | `/api/system/ingest` | 日志入库队列指标：`enabled/connected/queue_length/processing_length/consumed/written/failed/last_error/last_flush_at` |