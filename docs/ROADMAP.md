# 优化路线图（待办）

按优先级记录**尚未实施**的优化项，下次升级时按此清单推进。
已完成项见 [CHANGELOG.md](../CHANGELOG.md)。

图例：**收益** ★ 越多越值得做；**风险** 指改动可能引入回归的程度。

---

## P1（本轮已完成，仅记录结论）

| 项 | 结果 |
|---|---|
| 拆 `internal/service/ai.go`（584 行） | ✅ 拆为 `ai.go`(355) + `ai_types.go`(154) + `ai_prompt.go`(95)，与既有 `aicache.go`/`ai_slowsql.go` 并列 |
| 拆 `web/src/views/AttackMapView.vue`（831 行） | ✅ 视图 287 行；图配置抽到 `composables/useAttackMapCharts.js`(184)，样式抽到 `views/attackmap.css`(392) |
| repository 里 116 处 `ctx.Err()` 三行守卫 | ⏸ **主动放弃**：纯格式样板，收敛后不改变任何行为，却要改动 116 处、制造巨大 diff 噪音。若将来 xorm 升级到支持 `Session.Context(ctx)`，可直接删掉这些守卫（那才是真正的收益） |

---

## P2（下次升级建议做）

### P2-1 双仓储行为一致性测试｜收益 ★★★｜风险 低

`repository/mysql` 与 `repository/memory` 各有一套实现（各约 1000 行），语义靠人工保证：
分页边界、关键字过滤、`Count` 与 `List` 的 WHERE 是否一致（历史上就出过 `Count` 重置条件导致计数错的 bug）。

建议：把「分页/过滤/计数」写成**表驱动用例**，同一套 `repository.Repositories` 接口断言跑两遍（mysql 用 testcontainers 或可选的 `WAF_TEST_MYSQL_DSN`，memory 直接跑）。
一旦两边行为分叉，测试立即失败。

### P2-2 拆分 `internal/app/app.go`（324 行）｜收益 ★★｜风险 低

现在一个文件里混了：仓储装配、服务装配、nginx 管理器、HTTP 客户端等。
建议拆 `wire_repos.go` / `wire_services.go` / `wire_http.go`，组合根只留 `New()`。

### P2-3 前端小工具收敛｜收益 ★★｜风险 极低

- 时间格式化：`SlowSqlView`、`UsersView` 各 3 处手写 `toLocaleString`，统一走 `utils/format.js`；
- 路径/ID 解析：`handler_ai.go`、`handler_ratelimit.go` 各写一份 `PathValue("id")` 解析，抽 `httpapi/params.go` 的 `pathID(r, name)`；
- 表格空状态、筛选栏在两个以上页面重复，可抽 `components/DataTableToolbar.vue` 之类的通用件。

### P2-4 前端按需加载 ECharts｜收益 ★★｜风险 低

`echarts` 打包 1.03 MB（gzip 343 KB），目前只有攻击地图与概览用到。
建议改为按需引入（`echarts/core` + 具体 chart/geo 组件）或 `defineAsyncComponent` 懒加载，首屏可减 300 KB 左右。

---

## P3（架构级，择机）

### P3-1 `waf_log` 的容量治理｜收益 ★★★｜风险 中

现状：约 145 万行且持续增长，单表查询虽已加索引，但 3~5 年后归档/DDL 都会很痛。

方案（择一或组合）：

1. **按天分区**（`PARTITION BY RANGE (TO_DAYS(local_time))`），便于 `DROP PARTITION` 快速清理；
2. **冷热分离**：`waf_log`（近 30 天）+ `waf_log_history`（更早），后台查询默认查热表，历史表按需查；
3. **定时归档任务**：后台加一个每日任务，把 `local_time < now-N` 搬到历史表并删除。

配套：`/api/logs` 增加「时间范围必填或默认近 30 天」的约束，避免误查全表。

### P3-2 统计结果缓存｜收益 ★★｜风险 低

概览/地图每次进页面都全量计算（当前 0.3~1.8 秒）。筛选项已有 60 秒缓存机制，
可复用同一套给 `/api/logs/overview`、`/api/dashboard/summary`、`/api/attack-map` 加 30~60 秒结果缓存，
并在响应里带 `cached_at` 让前端提示数据新鲜度。

### P3-3 规则匹配的规模化｜收益 ★｜风险 中

当前每类规则几十条 + `ngx.re` 的 PCRE JIT 足够快；规则上到几百条时建议：
按「首关键词」或「规则分组」预筛，避免每条规则都跑一次正则。
（openresty 会自动缓存编译后的正则，暂不需手工做编译缓存。）

### P3-4 归属地解析整体下线｜收益 ★★｜风险 中

lua 侧 `ip2region.lua`(450) + `bitop.lua`(340) + `ip2region.db`(11.9 MB) 只用于日志里的 `area` 字段。
若改为「后端入库时解析」（Go 侧 `pkg/geo` 已有能力），可整体删掉这三个文件，
nginx 侧体积与 `init` 阶段开销都会下降；代价是 area 变成入库时补全。

---

## 已知的非阻塞小问题

| 项 | 说明 |
|---|---|
| `config.lua` 里 `cc rate the xxx of xxx seconds` 之类历史注释 | 上游遗留英文占位注释，不影响功能，可在下次改配置时顺手清理 |
| `waf_log` 无数据保留策略 | 与 P3-1 同源，短期靠人工归档 |
| 前端 `element-plus` 全量引入（1.17 MB / gzip 375 KB） | 若首屏要求更激进，可换 unplugin 按需引入 |