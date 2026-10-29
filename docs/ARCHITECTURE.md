# 架构说明

## 1. 总体结构

```
                    ┌───────────────────────────────┐
   浏览器  ─────────►│ openresty (lua-waf)           │
                    │  判定 + 拦截 + 写日志          │
                    └───────────┬───────────────────┘
                                │ RPUSH waf:logs
                                ▼
                    ┌───────────────────────────────┐
                    │ Redis（队列，削峰/跨机汇聚）    │
                    └───────────┬───────────────────┘
                                │ BRPOPLPUSH → 批量 INSERT
                                ▼
   ┌────────────────────────────────────────────────────────┐
   │ wafadmin（Go）：transport → service → repository → model│
   │   REST API + Vue 3 前端（embed 进二进制）               │
   └───────────────┬────────────────────────────────────────┘
                   │ xorm
                   ▼
              MySQL（waf 库：waf_log / rules / site / …）
```

lua 侧只做「判定 + 输出」，**不直连 MySQL**：数据库抖动不会拖慢 nginx，落库可观测（队列长度、失败数、最近入库时间）。

## 2. Go 后端分层

| 层 | 目录 | 职责 | 约束 |
|---|---|---|---|
| transport | `internal/transport/httpapi` | 路由、鉴权/CSRF 中间件、请求解析、统一响应 | 不写业务规则，只做参数与响应转换 |
| service | `internal/service` | 业务规则、校验、编排、下发文件、AI 调用 | 只依赖 repository 接口与 pkg，不直接 SQL |
| repository | `internal/repository/{mysql,memory}` | 存储实现（MySQL via xorm / 内存实现用于 demo 与测试） | 双实现必须行为一致，公共行为有回归测试 |
| model | `internal/model` | 数据结构与 xorm/JSON 标签 | 无业务逻辑 |
| app | `internal/app` | 组合根：按运行模式装配 store/service/handler | 唯一知道"用哪个实现"的地方 |

外部依赖收敛在 `pkg/`：`response`（统一响应与错误码）、`redis`（自研 RESP 客户端）、
`crypto`（双 md5 口令）、`geo`（IP 归属地）、`sqlwatch`（慢 SQL 观察器）。

## 3. 运行模式

| 模式 | 存储 | nginx 侧行为 | 用途 |
|---|---|---|---|
| `prod`（默认） | MySQL | 真实写 vhost/规则文件并 reload | 生产 |
| `demo` | 内存 + 样例数据 | DryRun，不落盘 | 本地体验、演示 |

切换：`WAF_RUN_MODE=demo` 或 `[server] RUN_MODE`。

## 4. 关键数据流

### 4.1 请求拦截（lua）

`access.lua → waf.check()` 顺序判定（详见 `docs/RULES.md`）：

```
白名单IP → 封禁 → 畸形请求 → 黑名单IP → 请求头 → UA → 白名单URL
→ URL → 限频规则 → CC/IP频率 → Cookie → 参数 → POST
```

命中后：写日志（`log_sink`）→ 按 `config_waf_model` 输出拦截页/跳转/幻影降级。

### 4.2 日志入库

```
log_sink.write()
  ├─ mode=file  → 按天追加写本地文件
  └─ mode=redis → RPUSH config_redis_list_key
                  失败：打 WARN（5 秒限频）→ 回落写文件 → 熔断 5 秒
wafadmin/ingest
  ├─ BRPOPLPUSH 源队列 → 处理中队列（at-least-once）
  ├─ 批量 INSERT waf_log（失败则留在处理中队列下轮重试）
  ├─ 成功后 LREM 处理中队列
  └─ 启动时把上次残留的处理中记录回收回源队列
```

### 4.3 规则下发

| 方向 | 触发 | 产物 |
|---|---|---|
| 后台 → nginx | 保存规则 / 手动同步 / 站点同步 | `rules/*.rule`（JSON 数组）、vhost 配置，随后 `nginx -t && nginx -s reload` |
| 文件 → 数据库 | 「从规则文件同步」 | 取并集：文件里有、库里没有的新增；库里已有的保留不动 |
| 后台 → nginx | 保存限频规则 | `rules/ratelimit.rule`（`{id,pattern,match,key,limit,window,action}`） |

### 4.4 配置在线编辑

`WAF 配置`页读写 `[waf] LUA_CONFIG` 指向的 `config.lua`：
按行替换值（保留注释/顺序/缩进）→ 备份 `.bak` → 临时文件 + rename 原子替换 → 可选 `nginx -t && reload`。
保存前可用「测试连接」（Redis PING）与「生效自检」（文件是否被 nginx 真正加载、日志链路是否通）。

## 5. 关键取舍

| 决策 | 原因 |
|---|---|
| lua 不直连 MySQL | 削峰、避免数据库抖动影响 nginx、可观测；lua 侧少一个依赖 |
| Redis 队列而非本地文件采集 | 多机汇聚、天然削峰、消费端可水平扩展 |
| 处理中队列 + 回收 | at-least-once：宁可重复（可去重）也不丢日志 |
| 规则文件与数据库"取并集" | 运维可能直接改服务器文件，人工调整也可能在后台，双向都不能被覆盖 |
| 只增不改的表结构策略 | 避免 xorm `Sync` 在线上表做危险 ALTER；只在表缺失时创建、缺列时补列 |
| memory 仓储 | demo 无依赖运行 + 单测不需要外部服务 |
| 前端 embed 进二进制 | 单文件部署，避免静态资源路径与跨域问题 |

## 6. 前端结构

| 目录 | 说明 |
|---|---|
| `web/src/views/` | 页面（概览/地图/日志/规则/限频/配置/站点/慢SQL/设置） |
| `web/src/components/` | 布局、侧边栏、配置面板、AI 对话框等 |
| `web/src/api/` | 各模块接口封装（统一 `client.js`，自动带 CSRF） |
| `web/src/stores/` | 轻量 reactive 单例（规则类型、主题等） |

主题通过 CSS 变量 + `html.dark` 切换；响应式使用 `utils/responsive.js` 与栅格自适应。