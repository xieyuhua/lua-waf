# 变更记录

按主题汇总（新→旧）。细节以 git 提交为准。

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