# 架构说明

## 1. 总览

```
┌──────────────────────────────┐        ┌───────────────────────────────┐
│  openresty / nginx (lua-waf) │        │  wafadmin (Go)                │
│                              │        │                               │
│  请求 → waf.check()          │        │  Vue 前端 (go:embed)          │
│   ├─ 12 步检测               │        │  transport/httpapi  ← REST    │
│   ├─ 命中 → 拦截 + 写日志    │        │  service            ← 业务    │
│   └─ 命中 → 可选自动封禁     │        │  repository         ← 存储    │
│                              │        │                               │
│  日志：file / mysql / redis ─┼───────►│  ingest（Redis 消费者）       │
└──────────────────────────────┘        └───────────────┬───────────────┘
                                                        │
                                     MySQL（日志/规则/站点/账号/配置）
```

职责边界：

- **lua-waf**：只做“判定与拦截”，不关心展示。规则文件与 `config.lua` 由后台下发，也可运维直接维护。
- **wafadmin**：管理面。读写配置与规则、展示统计、消费 Redis 队列写库；不做任何请求过滤。

## 2. 分层结构

| 层 | 包 | 职责 | 约束 |
|---|---|---|---|
| 组合根 | `internal/app` | 装配仓储、服务、路由、日志消费者 | 唯一知道“用 MySQL 还是内存”的地方 |
| 传输层 | `internal/transport/httpapi` | 路由、参数绑定、状态码、中间件（会话 / CSRF / 限流 / panic 恢复 / 访问日志） | 不做业务判断，错误统一转 `service.Fault` |
| 业务层 | `internal/service` | 校验、编排、事务边界、缓存 | 通过 `repository` 接口访问存储，不直接写 SQL |
| 仓储层 | `internal/repository` | 接口 + `mysql`、`memory` 两套实现 | 两套实现必须语义一致（便于 demo 与单测） |
| 领域模型 | `internal/model` | 结构体与规则类型目录 | 无依赖 |
| 基础设施 | `pkg/*` | captcha、crypto、geo、ratelimit、redis(RESP 客户端)、response、session、sqlwatch | 不依赖 internal |
| 集成 | `internal/nginx`、`internal/ingest` | 渲染 vhost / rule 文件并 reload；消费 Redis 队列 | 副作用集中在此 |

依赖方向单一：`transport → service → repository → model`，`pkg` 被任意层使用但自身不反向依赖。

## 3. 请求链路（以一个写接口为例）

```
POST /api/lua/config
  → AccessLogger → Recoverer → SecurityHeaders → CSRF → Authenticator.Private
  → LuaConfigHandler.save
      → LuaConfigService.Update
          → 校验（luaconfig_parse.normalizeLuaValue）
          → 按行回写（luaconfig_parse.rewriteLuaValues）
          → 备份 + 原子写文件（luaconfig.writeFile）
      → 返回最新视图（脱敏）
  → response.Writer 输出 { code:0, data:... }
```

错误统一模型：`service.Fault{Kind, Message}` → `code`（1001 参数非法 / 1002 未登录 / 1003 无权限 / 1004 不存在 / 1005 冲突 / 1500 内部错误），
对外只序列化 `Message`（内部原因写在日志里，不泄漏给前端）。

## 4. 数据模型（MySQL）

| 表 | 说明 |
|---|---|
| `waf_log` | 攻击日志：`client_ip`、`local_time`、`server_name`、`user_agent`、`attack_type`、`req_url`、`req_data`、`area`、`rule_tag` |
| `rules` | WAF 规则：`rule_type`、`rule_item`、`enabled`（默认 1） |
| `site` | 反代站点：域名、上游、证书、状态 |
| `user` | 管理员：账号、口令摘要、状态 |
| `setting` | 键值配置（如 `ai.config`） |
| `slow_sql` | 后台自身慢 SQL 采样 |

建表策略：表不存在则 `Sync` 建表；表已存在时**只补缺失列**（`ADD_MISSING_COLUMNS`），不修改、不删除，避免线上事故。

## 5. 关键设计决策

| 决策 | 原因 |
|---|---|
| 规则文件与数据库**取并集**（只增不删） | 规则可能由运维直接在服务器维护，后台不能覆盖；同时让后台能纳入新规则 |
| `config.lua` 采用**按行改值**而非重排 | 保留注释、顺序、缩进；出问题可用 `.bak` 回滚 |
| 日志入库提供三种模式 | 小流量用 file，简单场景直连 mysql，高并发用 redis + 后台消费（at-least-once） |
| 规则匹配大小写不敏感 + 输入归一化 | 大小写/编码是最常见的绕过手法，统一下沉比逐条规则写死更可靠 |
| AI 结果服务端缓存 | 分析单次常需 30~100 秒，跨设备/刷新要能复用，避免重复计费 |
| 前端内嵌进二进制 | 单文件交付，升级只需替换可执行文件 |
| `memory` 仓储与 `mysql` 同接口 | demo 模式、单测、以及“读不到库时的降级展示”都靠它 |

## 6. 并发与可靠性

- **Redis 消费者**：`BRPOPLPUSH` 原子取出到 processing 队列 → 批量写 MySQL → `LREM` 确认；
  启动时把 processing 里的残留回灌源队列（崩溃恢复），实现 at-least-once。
- **配置写入**：`LuaConfigService` 与 `SystemConfigService` 都加互斥锁 + 临时文件原子替换，避免并发保存互相覆盖。
- **看板指标**：`/api/system/ingest` 由消费者实时上报，前端 10 秒轮询绘制队列深度。
- **慢 SQL 采集**：`pkg/sqlwatch` 包装 xorm 执行，超过 `SLOW_SQL_MS` 记库并在后台展示、可交 AI 分析。
