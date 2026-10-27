# 变更记录

## 2.0.0

> 后端分层重构 + 后台 Vue 3 重写 + 慢 SQL / AI / 攻击地图等新能力。

### 架构

- 后端改为五层结构：`transport/httpapi → service → repository → model`，基础设施下沉到 `pkg/*` 与 `internal/nginx`；
  依赖方向单向，业务全部以**结构体 + 方法**实现，一个业务域一个文件。
- 组合根 `internal/app`：按「配置 → 仓储 → 服务 → 接入层」装配，负责首次初始化、优雅关闭。
- 仓储抽象为接口，提供 `mysql`（xorm）与 `memory`（演示/测试，含样例数据）两套实现。
- 配置改为 `默认值 < conf/app.ini < 环境变量`，支持 `WAF_*` 覆盖并在启动时校验。
- 修复 1.x 无法编译的问题（`routers/waflog.go` 空实现但 `main.go` 引用 4 个 handler）。
- 修正 `User` 结构体 `xrom` 标签拼写错误，账号名建立唯一索引。
- 移除 macaron 及其 session/csrf/captcha/cache 依赖，改用标准库 `net/http`（Go 1.22+ 路由）。

### 修复

- **列表筛选/搜索不生效（日志、站点、账号、慢 SQL）**：xorm 的 `Count()` 执行后会重置 session 上的
  WHERE 条件（`queryRows` 里 `defer resetStatement()`），而各仓储把统计与列表复用在同一个 session 上，
  导致「总数是过滤后的、列表却是全量」。改为统计与列表各用一个独立 session。
  新增 `internal/repository/mysql/log_test.go`（`WAF_TEST_MYSQL_DSN` 时连真实库）覆盖
  攻击类型 / 来源 IP / 关键词 / 分页 / 站点关键词过滤，日志表不存在时才建表、
  已存在时绝不做 Sync2，避免测试改动线上结构。
- **跨设备/跨浏览器看不到历史研判结果**：新增 `GET /api/ai/results/{logId}` 与
  `AnalyzeCache.FindByLog`，日志详情抽屉在本地无结果时自动拉服务端缓存并落到本地，手机可直接查看电脑上的分析；
  侧边栏底部显示 `X-WAF v版本 · fe-构建指纹`（`/api/system/info` 新增 `frontend_version`），
  便于确认浏览器加载的是哪一版前端。
- **移动端看不到 AI 研判结果**：长结论会把弹窗撑到 1300px 以上，而 `el-dialog__body`
  是 `overflow: visible`，屏幕外的内容无法滚动查看。现在 ≤900px 时 `.el-dialog` 限高 92vh、
  改为 flex 布局并让正文 `overflow-y: auto`（页脚按钮固定），同时结果出现后自动滚到「研判结果」区；
  另新增 `utils/markdown.js`（先转义再转换标题/列表/代码/加粗）把结论渲染为易读排版，长链接强制换行。
- **AI 配置保存一次后无法再修改**：`setting` 表的 `Set` 用 `Insert` 写入，第二次保存同键会撞主键
  （`Error 1062 Duplicate entry`），前端表现为"改不动"。改为「先更新，未命中再插入；并发撞主键退化为更新」，
  并新增 `internal/repository/mysql/setting_test.go`（`WAF_TEST_MYSQL_DSN` 存在时连真实库验证连续覆盖）
  与 `internal/service/ai_test.go`（反复保存、掩码保留密钥、默认值裁剪、非法配置校验）。

### 新增功能

- **AI 分析内容不再被截断**：① 「最大输出 tokens」默认 1024 → **4096**，上限 8192 → **32768**，
  并支持填 `-1` 表示不限制（请求不再发送 `max_tokens`）；② 模型输出达上限（`finish_reason=length`）时
  **自动续写最多 2 轮**并把内容拼接完整、用量累计；③ 仍被截断时结果带 `truncated` 标记，
  弹窗与详情抽屉显示「输出被截断」提示，引导调大上限后「忽略缓存重新请求」；
  ④ 提示词里的输入截断同步放宽（请求路径/请求体 300 → 2000，UA 200 → 600，慢 SQL 语句 2000 → 8000），
  让模型看到更完整的上下文。
- **慢 SQL 也支持 AI 分析（优化方案）**：慢 SQL 列表新增勾选列与「AI 优化建议（N）」按钮、行内「AI / AI建议」入口，
  详情抽屉内直接展示结果；提示词要求模型按「结论 / 原因分析 / 优化方案（含 `CREATE INDEX` 与 SQL 改写）/
  风险提示 / 验证方式」输出，并明确要求信息不足时不要编造字段名。
  后端新增 `SlowSQLRepository.Get`、`AIService.AnalyzeSlowSQL`、`CachedSlowSQLResult` 与
  `POST /api/ai/slow-sql/analyze`、`GET /api/ai/slow-sql/results/{id}`；
  `AnalyzeCache` 增加场景维度（log / slow_sql），同一 ID 的两类记录缓存互不串味。
  前端 `AiAnalyzeDialog` 支持 `scene` 属性复用同一套弹窗（标题/计数/预览标签/占位文案随场景变化），
  本地缓存键同步加场景前缀；Markdown 渲染补上 ``` 代码块（SQL 提示更易读）。
- **系统设置全部合并为一个页面**：`运行配置 / WAF 配置 / 外观设置 / AI 设置 / 账号设置` 五个选项卡
  集中在「系统设置」（侧边栏因此只保留一个入口），选项卡状态写入 `?tab=app|waf|appearance|ai|account`；
  `/settings/lua`、`/settings/appearance`、`/settings/ai`、`/settings/account` 与顶栏「账号设置」入口
  全部重定向到对应选项卡，旧链接不失效；各面板抽出到 `components/config/*Panel.vue`，
  统一「说明 + 操作按钮」工具栏，移动端选项卡可横向滑动、无横向溢出。
- **「运行配置」与「WAF 配置」合并为「系统设置」一个页面**：不再在侧边栏占两个菜单项，
  改为页面内选项卡切换（`运行配置（app.ini）` / `WAF 配置（config.lua）`），
  选项卡状态写入地址栏 `?tab=app|waf`，刷新/分享链接可定位；旧地址 `/settings/lua` 自动重定向到
  `/settings/system?tab=waf`；两个面板抽出为 `components/config/{AppConfigPanel,LuaConfigPanel}.vue`，
  各自保留原有保存/重载/reload 按钮，移动端选项卡与工具栏自动换行。

### 修复

- **AI 研判结果只显示模型/耗时/tokens、正文一片空白**：根因是模型返回的正文字段为空
  （推理类模型把内容放在 `reasoning_content`，或 `max_tokens` 用尽 `finish_reason=length`），
  后端此前原样返回并写入缓存，前端只渲染标题行。
  现在：① 解析响应时若 `content` 为空自动回退 `reasoning_content` / `reasoning` 并标注「以下为思考过程」；
  ② 正文仍为空返回明确错误「模型返回内容为空（finish_reason=…），请更换模型或调大最大输出 tokens」；
  ③ 空正文不再写入服务端缓存；④ 前端遇到历史空缓存条目时显示提示 + 引导「忽略缓存重新请求」，
  并在读取本地缓存时丢弃无正文的旧条目。

### 新增功能

- **攻击地图支持缩放 + 归属地精确到城市**：`geo.roam` 打开（滚轮 / 双指缩放、拖拽平移，max 8 倍），
  地图右上角新增放大 / 缩小 / 重置按钮，刷新数据后保留当前视野；
  地理词表重构为三层匹配（城市 → 省/州 → 国家，共 483 条），解析前剥离运营商与云厂商标记
  （电信/移动/联通/阿里云/腾讯云/Google Cloud/AWS/Cloudflare…），「保留 / 内网IP / 亚太地区」等占位值
  判为不可定位并在状态条提示「未定位 N 个来源」。对线上 144.9 万条日志实测：
  可定位率从原词表的大幅漏配提升到 **98.88%**（剩余 1.12% 为「保留」与空值），
  且能落到 `山景城 / 洛杉矶 / 休斯顿 / 芝加哥 / 阿姆斯特丹 / 泗水 / 万隆` 等城市粒度。
- **lua-waf 配置在线编辑**（系统设置 → WAF 配置）：网页读写 `config.lua`，按 8 个分组展示
  总开关/检测开关/限速/请求体与畸形检测/自动封禁/目录/日志模式/MySQL/Redis 共 40 余项；
  只替换对应行的值（保留注释、顺序与引号风格，数字保持裸值），保存前备份 `config.lua.bak` 并原子替换；
  非法取值（开关非 on/off、数字越界、下拉越界、限速格式错误、未知键）直接拒绝且不改文件；
  密钥类脱敏回显、提交掩码表示不修改；多行 `config_output_html` 等列为「未纳入表单」只读；
  支持「保存」「保存并生效（nginx -t + reload）」「仅 reload」「查看原文」。
  路径由 `[waf] LUA_CONFIG` 指定（默认 `/www/server/nginx/x-waf/config.lua`，支持 `WAF_LUA_CONFIG`）。
- **攻击地图：攻击状态 + 攻击箭头 + 主题跟随**：新增状态条（正常/告警/高危、今日攻击、近 5 分钟、
  每分钟速率、最近攻击时间，数据来自新接口字段 `status`），地图新增 `lines` 箭头表现
  「来源 → 防护目标」的攻击链路（线宽随次数、箭头流动效果），目标点由 `[waf] MAP_TARGET`
  （默认 中国-北京）解析得到；整页配色改为跟随主题（亮/暗 + 主题色），不再固定深色大屏。
- **AI 分析结果缓存（默认 10 分钟）**：
  - 服务端 `service.AnalyzeCache`：键 = 接口地址 + 模型 + 系统提示词 + 排序后的日志 ID + 提问，
    命中直接返回上次结论（`cached: true` + `cached_at`），不再消耗 token；带条目上限与过期清理；
  - 前端 `stores/ai.js`：同样参数命中本地缓存时连请求都不发，结果存 `localStorage`，
    刷新页面 10 分钟内仍可回看；
  - 缓存时长在「AI 设置 → 结果缓存（分钟）」可调（1~120）；支持 `refresh: true` 强制刷新；
    保存 AI 配置会立即清空服务端缓存。
- **AI 研判结果的三处查看入口**：分析弹窗内「研判结果」区、日志行「看结果」按钮、
  日志详情抽屉「AI 研判结果」区块。
- **全站移动端自适应**：新增断点体系（1400/1200/900/640/360）与 `web/src/utils/responsive.js`；
  侧栏在 ≤900px 变抽屉、网格自动降列、页头与筛选区堆叠、弹窗/抽屉按视口宽（92%/88%）、
  统计卡手机两列、图表高度自适应；表格次要列在手机上自动隐藏、操作列固定右侧并可点，
  其余列横向滑动。新增 `mobile-check.mjs` 遍历 11 个路由验证 390px/768px 无横向溢出（截图复核）。
- **命令行重置账号口令**：`wafadmin reset-password -config conf/app.ini [-user admin] [-password xxx] [-create]`
  与 `wafadmin list-users`、`version`、`help`；子命令不启动 HTTP 服务，复用业务层校验与摘要算法
  （`md5(md5(raw)[5:10])`，兼容 1.x 老库），不带口令时随机生成 16 位并回显。
  `main.go` 拆出 `serve()`，首个参数以 `-` 开头时仍按原方式启动服务。
- **修复主键标签（严重）**：`xorm:"pk autoincrement"` 中的 `autoincrement` 不是 xorm 关键字
  （它只认 `autoincr`），该词会被当成列名 —— 导致 `user`/`rules`/`waf_log` 查询报
  `Unknown column 'autoincrement'`，新建表的出主键没有自增。现统一为 `pk autoincr 'id'`，
  并新增 `internal/model/model_test.go` 作为回归防线。
- **表名对齐 1.x 老库**：`Rule` 的表名固定为 `rules`（老库是复数），`SlowSQL` 为 `slow_sql`，其余为 `site`/`user`/`setting`/`waf_log`。
- **已存在表补列开关** `database.ADD_MISSING_COLUMNS`（默认 false）：开启后对已有表只补缺失列
  （xorm `Sync` 语义，只增不改删），用于 1.x 老库缺 `last_login`/`created`/`updated` 的场景；
  关闭时启动日志会打印检测到的问题列与待执行的 `ALTER` 语句。
- **SQL 日志**：新增 `mysql.sqlLogger`（实现 xorm `core.ILogger`），开启后每条 SQL 以
  `[sql] INFO  [SQL] SELECT ... ["waf", "site"] - took: 1.294ms` 形式打印语句、参数与耗时
  （参数由 xorm 的 `[]interface {}{...}` 简化为 `[...]`）；关闭时连 PING 等 Info 也静默，只保留 Error/Warn。
  开关为 `database.SHOW_SQL`（默认 dev 开启 / prod 关闭，可用 `WAF_DB_SHOW_SQL` 覆盖），
  后台「运行配置 → 数据库」可热切换，无需重启（改连接信息仍需重启）。
- **运行配置编辑**（系统设置 → 运行配置）：网页读写 `conf/app.ini`，保存时先备份 `.bak` 再原子替换；
  可热更新的项立即生效，需重启的项（运行模式、端口、数据库、会话密钥）在响应中列出提示；
  支持查看原文与「从磁盘重载」。密钥字段一律脱敏回显，留空/掩码表示保持原值。
- **主题切换**（系统设置 → 外观设置）：亮色 / 暗色 / 跟随系统 + 6 套主题色，
  同步驱动品牌渐变、Element Plus 主色色阶与 ECharts 调色板；顶栏提供调色板抽屉与一键亮暗切换，
  设置持久化在浏览器本地。
- **建表策略调整**：启动时逐表检查，**已存在的表不再做结构同步**（不再 ALTER，避免
  `Multiple primary key defined` 之类的同步错误），只有缺失的表才创建；新增 `database.AUTO_MIGRATE` 开关，
  关闭后缺表直接报错、要求手工初始化。
- **慢 SQL 统计**：仓储调用计时（阈值 `monitor.SLOW_SQL_MS`）入库 + 外部签名接口上报；
  支持时间范围查询、总数/均值/P95/超 1s、耗时排行、明细、按天清理。
- **AI 能力**：OpenAI 兼容大模型配置（密钥脱敏存储）、连通性测试、对选中日志（1–20 条）生成研判结论。
- **攻击地图大屏**：地区文本 → 经纬度（内置 120+ 词表），世界地图涟漪点、境外占比、地区 Top10、实时拦截、自动刷新。
- **WAF 规则子菜单**：按「拦截策略 / 访问控制」拆分为 9 个子页面，带数量徽标。
- **攻击日志筛选**：关键词（IP/URL/UA/类型/规则/站点）+ IP + 站点 + 类型 + 时间范围组合；详情抽屉；勾选日志送 AI。
- 站点 nginx 配置预览接口；规则内容格式校验（PCRE/RE2 兼容性提示）。
- 演示模式（`RUN_MODE=demo`）：内置样例数据，无需 MySQL 与 openresty 即可体验全部界面。

### 安全

- 会话改为 HMAC-SHA256 签名的无状态 Cookie（HttpOnly + SameSite=Lax）。
- 双提交 Cookie 的 CSRF 防护（`waf_csrf` + `X-CSRF-Token`），写操作强制校验。
- 登录失败限流（同 IP 5 分钟 10 次）、图形验证码一次性有效。
- 统一安全响应头与 `Cache-Control: no-store`；口令最小长度 8 位（新增/修改时）。
- 口令摘要保持 1.x 算法不变（`md5(md5(raw)[5:10])`），老库账号可直接登录。

### 前端

- 全新 Vue 3 + Vite + Element Plus + ECharts 工程，替换 1.x 的 Bootstrap/Lumino 模板。
- 页面：登录、安全概览、攻击地图、站点管理、WAF 规则（子菜单）、账号管理、攻击日志、慢 SQL、AI 设置、账号设置。
- 亮/暗双主题、响应式布局、页面切换动效、骨架屏与空状态、表格操作列紧凑化。
- 构建产物通过 `//go:embed all:web/dist` 内嵌，保持单文件部署。

### lua-waf

- **自动封禁（可选，默认关闭）**：新增 `ban.lua`，命中攻击（由 `util.log_record` 通知）按来源 IP 计数，
  窗口内达阈值即写入 `ngx.shared.waf_bans`（带 TTL，到期自动解封），`waf.check()` 新增 `ban_check` 阶段直接 403；
  支持按攻击类型过滤（`config_auto_ban_types`）；白名单 IP 优先，非攻击事件（请求体超限/封禁中）不计入。
  开关与阈值为 `config_auto_ban_check / expire / window / threshold`。
  > 说明：默认「检测到攻击只拦当前请求」，这是为避免 NAT、CDN 回源等共享 IP 被误封。
- **后台一键封禁**：攻击日志列表与详情抽屉新增「封禁」按钮，把来源 IP 写入 `blackip` 规则并立即下发
  （转义点号后作为正则片段）。
- **日志输出双模式**（`config_log_mode`）：
  - `file`：保持 1.x 的本地文件追加写，供采集器读取；
  - `mysql`：日志先入 `ngx.shared.waf_log_queue` 内存队列，`ngx.timer` 每 N 秒批量
    `INSERT INTO waf_log`（默认 200 条 / 2 秒），单机高并发不阻塞请求；
  - `redis`：日志 `RPUSH` 到 Redis 列表，定时批量落库，适合多机汇聚与削峰。
  - 失败兜底：队列满 / Redis 不可用 / 共享内存未声明时自动回落写文件；落库失败整批回队重试；
    多 worker 通过共享内存锁保证单写者；`log_consumer.stats()` 提供 inserted/failed/dropped/pending 计数。
  - 新增文件：`log_sink.lua`、`log_mysql.lua`、`log_redis.lua`、`log_consumer.lua`、`init_worker.lua`、
    `sql/waf_log.sql`；`util.log_record` 改为统一走 `log_sink`。
- 新增 `body.lua`：请求体统一读取（含 `get_body_file` 临时文件场景），
  urlencoded / multipart 分段 / JSON 逐段检测；拦截缺失 boundary、分段数超限、分段头 NUL/裸换行、
  字段名注入、上传文件名路径穿越。
- 新增 `malformed.lua`：TE/CL 走私、重复 Content-Length、非单一 chunked、原始报文 NUL 与裸 CRLF、
  非规范百分号编码、解码后控制字符、超长请求行、Host 校验。
- `config.lua` 新增 `config_post_multipart_check`、`config_post_max_size`、`config_post_max_parts`、
  `config_malformed_check`；`waf.lua` 把畸形检测提前到检测链最前。
- 规则文件格式保持 `[{ "Id":.., "RuleType":.., "RuleItem":".." }]` 不变，兼容既有规则。

### 文档

- 重写 `README.md`（功能、快速开始、配置项、部署、规则生效与重载说明）。
- 新增 `docs/ARCHITECTURE.md`（分层约束、扩展流程、生效链路）、`docs/API.md`（接口与错误码）、
  `web/README.md`（前端工程）、`CHANGELOG.md`（本文件）。
- 重写 `lua-waf/README.md`（文件清单、检测顺序、规则格式、配置项、部署生效）。

### 工程

- `.gitignore` 覆盖二进制与前端依赖；移除仓库中误提交的 `server`、`wafadmin` 旧构建产物。
- 新增测试：`pkg/crypto`（口令兼容/签名）、`pkg/geo`（地区解析）、`internal/service`（站点校验、慢 SQL 口径）、
  `internal/transport/httpapi`（鉴权、CSRF、登录全流程、CRUD、规则子菜单、地图、看板、同步签名）。
- 移除 `models/`、`routers/`、`modules/`、`setting/`、`templates/`、`public/`（历史版本保留在 git 记录中）。

### 不兼容变更 / 迁移说明

| 项目 | 1.x | 2.0.0 |
| --- | --- | --- |
| 后台页面 | 服务端渲染模板（`/login/`、`/admin/index/`） | SPA（`/login`、`/dashboard`） |
| 接口形态 | 表单 POST + 重定向 | JSON `/api/*` + 会话 Cookie |
| CSRF | macaron csrf 中间件 | 双提交 Cookie + `X-CSRF-Token` |
| 入口 | `go run main.go` | `go run . -config conf/app.ini`，内嵌前端 |
| 配置 | 仅 ini | ini + 环境变量，`RUN_MODE` 支持 `demo/dev/prod` |
| 站点/规则下发 | 直接写文件后 reload | 写文件 → `nginx -t` → reload（检查失败则中止） |
| 数据库 | 自动建 site/user/rules | 追加 `slow_sql`、`setting` 表（`Sync2` 自动创建） |
| 口令算法 | `md5(md5(raw)[5:10])` | 保持不变（兼容老账号） |

数据库无需手工迁移：`Sync2` 会新增缺失的表与列；`waf_log` 仍由 openresty 侧写入，后台只读。
