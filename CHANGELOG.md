# 变更记录

按主题汇总（新→旧）。细节以 git 提交为准。

## 告警通知（企业微信机器人）

- 新增「系统设置 → 告警通知」：填企业微信群机器人 Webhook，**勾选需要提醒的规则**（可按类型筛选、
  关键词搜索、批量勾选当前筛选），命中这些规则时推送消息；支持静默期（同规则 N 秒内只提醒一次）
  与 `@all`；提供「发送测试」用于保存前试通。
- 配置存 `setting` 表（键 `alert.config`），接口 `GET/PUT /api/alert/config`、`POST /api/alert/test`。
- 触发点在日志入库消费者：一批日志成功入库后匹配 `rule_tag`，推送失败只记日志，不影响入库。

## 日志归档与归属地

- **按月归档而不是删除**：`[log] RETENTION_DAYS`（后台「系统设置 → 日志保留天数」可改）控制
  `waf_log` 只保留最近 N 天，更早的数据先 `INSERT INTO waf_log_YYYYMM`（`CREATE TABLE ... LIKE waf_log`）
  再从 `waf_log` 删除，历史可查；每批 5 万条，按月份提交，失败不影响其它月份。
- **IP 归属地解析下放到后端**：新增 `pkg/geo/ip2region.go`（ip2region v1 数据文件读取 + 归属地拼装，
  规则与 lua 侧一致）与 `[waf] IP2REGION_DB`；后台启动自检（解析 8.8.8.8），通过则消费者入库时补全 `area`，
  lua 侧不必再为每个 worker 加载 12MB 数据文件；数据文件不可用时自动回退 lua 侧解析。

## 优化（P2 / P3 落地）

- **仓储一致性测试**：新增 `repositorytest` 套件，同一组断言同时跑 MySQL 与内存实现
  （分页/过滤/计数/CRUD/排序），MySQL 侧用 `WAF_TEST_MYSQL_ADDR` 开关启用。
- **组合根拆分**：`app.go` 拆为 `app.go` / `wire_repos.go` / `wire_services.go`。
- **统一工具**：handler 路径 ID 解析收敛到 `httpapi.pathID`；前端 ECharts 改为按需引入
  （chunk 1036 KB → 653 KB，gzip 343 → 218 KB）。
- **统计缓存**：概览 / 看板 / 攻击地图结果缓存 45 秒（`statsCache`），刷新页面不再重复聚合。
- **日志保留策略**：新增 `[log] RETENTION_DAYS` 与 `RetentionService`，每日分批清理超期 `waf_log`（默认关闭）。
- **规则匹配预筛**：lua 侧 `util.rulematch` 先做字面量子串判断，未命中直接跳过 PCRE；
  同时移除 `util.lua` 里已废弃的 `resty.mysql` 依赖。

## 代码整理

- 删除死代码（`mysql/ratelimit.go` 未使用的 `sortRules`）；新增 `service/secret.go` 统一「脱敏值判定」，
  替换 4 处各自手写的 `strings.Contains(..., "****")`。
- 拆分 `service/ai.go`（584 → 355 行）：`ai_types.go`（配置与 DTO）/ `ai_prompt.go`（提示词与错误翻译）。
- 拆分 `views/AttackMapView.vue`（831 → 287 行）：图配置抽到 `composables/useAttackMapCharts.js`，
  样式抽到 `views/attackmap.css`。
- 拆分 `ingest/consumer.go`（402 → 335 行）：解析/错误翻译挪到 `consumer_parse.go`。
- 新增 [docs/ROADMAP.md](docs/ROADMAP.md)：P2/P3 优化待办（双仓储一致性测试、app.go 拆分、
  waf_log 容量治理与分区、统计缓存、IP 归属地下线等）。

## 日志链路

- **lua 只写 Redis，Go 统一入库**：移除 `log_mysql.lua` / `log_consumer.lua` / `lib/resty/mysql.lua`；
  `log_sink` 只保留 `file` / `redis` 两种模式；配置项 47 → 37。
- **消费者可诊断**：启动时 PING Redis 并打印就绪/失败结论（含积压条数、队列键）；
  入库错误翻译（1146 表不存在 / 1142 权限不足 / 1406 字段超长）。
- **Redis 失败熔断**：口令错或 Redis 宕机时 5 秒内不再重试、WARN 限频，避免拖慢请求与刷爆 error.log；
  恢复后自动解除。
- **`waf_log` 自动建表**：加入自动建表清单（含 5 个索引）；建表失败不阻塞启动并给出手工建表指引。

## WAF 配置

- 在线编辑 `config.lua`：按行替换、保留注释与顺序、`.bak` 备份、原子写入。
- 选项卡合并为 4 个：基础(15) / 拦截页面(1) / 检测开关(15) / 日志输出(6)。
- 「测试连接」（Redis PING）与「生效自检」（路径一致性、日志模式、Redis、目录、最近入库），
  结果按行展示 ok/warn/error。
- 修复混类型保存报 1001（前端数字控件提交 number，后端按 `map[string]any` 接收后统一转字符串）。
- `[waf] LUA_CONFIG` 默认改为 nginx 实际路径，修复"改的是仓库副本、reload 不生效"。

## 规则

- 规则文件同步取并集：文件有库里没有的新增，库里已有的保留。
- 同步统计口径修正：`in_files = added + kept`，重复条目计入 `duplicates`，新增 `total`；
  跳过 `ratelimit.rule`。
- 新增规则启用/停用、命中统计（按 `rule_tag` 聚合）与命中样本。
- 新增请求头检测阶段与归一化（3 次解码 / `%uXXXX` / HTML 实体 / 去注释）。
- 畸形请求检测：TE.CL、CL.CL、非法 TE/CL、NUL、裸换行、控制字符、超长请求行、非法 Host；
  **修复 `get_headers(0,true)` 头名未小写导致所有请求被判「缺失Host头」**。

## 限频

- 新增「限频规则」：按 URL 精细化限频，维度支持 `ip / header / cookie / arg / uri / site / method / referer`，
  可拦截或仅记录；自动下发 `rules/ratelimit.rule`。
- 页面优化：概览数字、空状态场景模板、URL/阈值/动作的视觉区分。

## 日志与统计

- 修复日志接口 20~45 秒超时报 1500：自动补建 `waf_log` 索引（5 个）、
  去掉 `GROUP_CONCAT(area)`、IP 排行两段式 + 默认近 30 天、筛选项 DISTINCT + 60 秒缓存。
- 攻击地图默认「今日」；新增「检测与拦截顺序」折叠面板（概览顶部）。
- 慢 SQL 统计支持时间筛选与 AI 分析；AI 分析结果可回看（缓存 10 分钟）。

## 安全与体验

- 后台口令双 md5、验证码、登录限流、双提交 CSRF、会话签名 Cookie。
- Vue 3 重构后台：统一设计系统、4 套主题、深浅色切换、移动端自适应。
- 站点管理一键下发 vhost 并 reload；配置/规则下发前校验（`nginx -t`）。
- 内置纯 Lua 依赖 `resty.redis` / `resty.lrucache`，无需 opm；缺库时不再哑失败。