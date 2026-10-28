# HTTP 接口文档

- 基础前缀：`/api`
- 认证：登录后由服务端下发 `waf_session` Cookie；**所有写接口**还需带 CSRF 双提交：
  - Cookie `waf_csrf=<token>`
  - 请求头 `X-CSRF-Token: <token>`
- 响应统一结构：

```json
{ "code": 0, "message": "ok", "data": { } }
```

错误码：`1001` 参数非法、`1002` 未登录/会话失效、`1003` 无权限、`1004` 不存在、`1005` 冲突、`1500` 内部错误。

分页参数：`page`（从 1 开始）、`size`（默认取 `[pager] PAGEMax`）。

---

## 1. 认证与会话

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/auth/captcha` | 获取登录验证码（返回 id + base64 图片） |
| POST | `/api/auth/login` | 登录：`{ username, password, captcha_id, captcha }` |
| POST | `/api/auth/logout` | 退出登录 |
| GET | `/api/auth/profile` | 当前登录用户信息 |

---

## 2. 看板与攻击地图

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/dashboard/summary` | 概览：今日拦截、累计攻击、站点数、启用规则数、近 15 天趋势、今日类型分布 |
| GET | `/api/attack-map?range=today\|all` | 攻击地图快照：`points`（含经纬度与次数）、`status`、`target`、`top_countries`、`top_types`、`trend`、`recent` |

`range` 缺省表示全部时间；后台前端默认传 `today`（今日）。

---

## 3. 攻击日志

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/logs` | 分页查询：`keyword`、`type`、`ip`、`date`、`page`、`size` |
| GET | `/api/logs/{id}` | 单条详情 |
| GET | `/api/logs/dates` | 有日志的日期列表 |
| GET | `/api/logs/types` | 攻击类型聚合 |
| GET | `/api/logs/ips` | 来源 IP 聚合 |
| GET | `/api/logs/filters` | 筛选项字典（类型、站点等） |
| GET | `/api/logs/overview` | 日志页顶部统计 |

---

## 4. WAF 规则

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/rules/types` | 规则类型及各自条数（生成子菜单） |
| GET | `/api/rules?type=&keyword=&page=&size=` | 按类型分页查询 |
| POST | `/api/rules` | 新增：`{ rule_type, rule_item }` |
| PUT | `/api/rules/{id}` | 修改规则内容 |
| DELETE | `/api/rules/{id}` | 删除规则 |
| PUT | `/api/rules/{id}/enabled` | 启用/停用：`{ "enabled": false }`（停用后不下发，规则不删除） |
| POST | `/api/rules/validate` | 校验正则是否可编译 |
| POST | `/api/rules/sync` | 下发全部规则到 `RULE_PATH` |
| POST | `/api/rules/sync-files` | 规则文件与数据库**取并集**（缺少的新增，已有的保留） |
| GET | `/api/rules/pipeline` | 检测与拦截顺序（12 步，含动作/规则文件/开关/说明） |
| GET | `/api/rules/stats?days=7&limit=20` | 规则命中排行（按日志 `rule_tag` 聚合） |
| GET | `/api/rules/stats/samples?rule_tag=&days=&limit=` | 某条规则的命中样例（判断误杀） |

**sync-files 返回**：

```json
{ "dir": "/www/server/nginx/x-waf/rules/ + lua-waf/rules", "files": 10, "in_files": 210,
  "added": 0, "kept": 210, "skipped": 0, "message": "规则文件与数据库已一致，无需新增" }
```

**stats/samples 返回**：

```json
{ "rule_tag": "args->sql_inject", "days": 7, "total": 24, "rule_id": 8, "rule_enabled": true,
  "items": [ { "id": 1, "client_ip": "1.2.3.4", "local_time": "2026-10-28 10:00:00",
               "attack_type": "SQL注入", "req_url": "/x?id=1", "req_data": "...", "server_name": "a.com" } ] }
```

---

## 5. WAF 配置（config.lua）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/lua/config` | 按模块分组的配置项（密钥脱敏，含 `detected` 标记） |
| PUT | `/api/lua/config` | 保存：`{ "values": { "config_xxx": "值" } }`，只提交变化的项 |
| GET | `/api/lua/config/raw` | config.lua 原文 |
| POST | `/api/lua/config/test` | **保存前连通性自检**：用表单当前值测试 Redis / MySQL（返回 `redis`/`mysql` 各自的 `ok`、`latency_ms`、`message`） |
| POST | `/api/lua/reload` | 执行 `nginx -t && nginx -s reload` |

控件类型：`switch` / `number` / `text` / `select` / `secret` / `code`（多行模板，如拦截页 HTML）。
`code` 类型内容不能包含 `]]`（会破坏 Lua 长字符串）。

`values` 的每个值可以是字符串、数字或布尔（前端数字控件提交 number、开关提交 `on`/`off`），
后端会统一转成字符串写回 config.lua；服务端会拒绝未在 schema 中声明的键。

**测试连接返回示例**：

```json
{ "redis": { "target": "redis", "address": "127.0.0.1:6379", "ok": true, "latency_ms": 3,
             "message": "PING 成功，耗时 3ms（库 0）" },
  "mysql": { "target": "mysql", "address": "192.168.2.6:3306/waf", "ok": false, "latency_ms": 3001,
             "message": "连接失败：192.168.2.6:3306 连接超时，检查网络/防火墙/地址是否正确" } }
```

未配置 `config_redis_host` / `config_mysql_host` 的目标不会出现在结果里；口令传掩码 `****` 时会用已保存的值。

---

## 6. 系统设置

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/system/info` | 运行信息：版本、模式、前端版本、路径等 |
| GET | `/api/system/ingest` | 日志入库队列指标：`enabled`、`connected`、`queue_length`、`processing_length`、`consumed`、`written`、`failed`、`last_error`、`last_flush_at` |
| GET | `/api/system/config` | app.ini 解析结果（敏感项脱敏） |
| PUT | `/api/system/config` | 保存运行配置（写回 app.ini 并热更新） |
| GET | `/api/system/config/raw` | app.ini 原文 |
| POST | `/api/system/config/reload` | 从磁盘重载配置 |

---

## 7. AI 分析

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/ai/config` | 读取 AI 配置（api_key 脱敏） |
| PUT | `/api/ai/config` | 保存 AI 配置 |
| POST | `/api/ai/test` | 连通性测试 |
| POST | `/api/ai/analyze` | 分析攻击日志：`{ log_ids: [], question, refresh }` |
| GET | `/api/ai/results/{logId}` | 取该日志最近一次分析结果（服务端缓存） |
| POST | `/api/ai/slow-sql/analyze` | 分析慢 SQL：`{ slow_sql_ids: [], question, refresh }` |
| GET | `/api/ai/slow-sql/results/{id}` | 取该慢 SQL 最近一次分析结果 |

分析接口可能耗时 30~100 秒，前置网关/反代超时需放宽到 ≥ 180 秒；
返回体里的 `truncated` / `finish_reason` 表示内容是否被模型截断。

---

## 8. 站点与同步

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/sites` | 站点列表（分页/筛选） |
| GET | `/api/sites/all` | 全量站点（下拉用） |
| GET | `/api/sites/{id}` | 站点详情 |
| POST | `/api/sites` | 新增站点 |
| PUT | `/api/sites/{id}` | 修改站点 |
| DELETE | `/api/sites/{id}` | 删除站点 |
| GET | `/api/sites/{id}/preview` | 预览渲染后的 vhost 配置 |
| POST | `/api/sites/{id}/sync` | 下发单个站点 |
| POST | `/api/sites/sync` | 下发全部站点 |
| POST | `/api/sync/site`、`/api/sync/rule`、`/api/sync/slow-sql` | 多节点同步入口（带 `API_KEY`） |
| GET | `/api/site/sync/`、`/api/rule/sync/` | 旧版同步兼容接口 |

---

## 9. 慢 SQL

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/slow-sql` | 慢 SQL 列表（支持时间范围、关键字、分页） |
| GET | `/api/slow-sql/stats` | 统计聚合（条数、耗时分布、按 SQL 归并） |
| POST | `/api/slow-sql/purge` | 清理历史慢 SQL |

---

## 10. 账号管理

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/users` | 管理员列表 |
| POST | `/api/users` | 新增 |
| PUT | `/api/users/{id}` | 修改 |
| DELETE | `/api/users/{id}` | 删除 |
| POST | `/api/users/password` | 修改自己的口令 |

---

## 11. 其它

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/health` | 健康检查：`{"code":0,"data":{"status":"ok"}}` |

前端为单页应用：未匹配 `/api` 的路径全部回落到 `index.html`。
