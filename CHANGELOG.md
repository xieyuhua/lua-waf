# 变更记录

> 版本号对应 `main.go` 中的 `version`。同一版本内按**主题**归类，不逐条罗列小改动。

---

## 2.0.0（当前）

分两大部分：**后端分层重构 + 后台 Vue 3 重写**，以及随后持续叠加的检测能力与运维能力。

### 架构与工程

- 后端改为五层结构 `transport/httpapi → service → repository → model`，基础设施下沉到 `pkg/*` 与 `internal/nginx`，
  依赖方向单向；业务按域一文件一结构体（`service/*.go`）。
- 组合根 `internal/app`：按「配置 → 仓储 → 服务 → 接入层」装配，负责首次初始化与优雅关闭。
- 仓储抽象为接口，提供 `mysql`（xorm）与 `memory`（演示/测试，含内置样例数据）两套实现。
- 配置为 `默认值 < conf/app.ini < 环境变量`，支持 `WAF_*` 覆盖并在启动时校验。
- 移除 macaron 及其 session/csrf/captcha/cache 依赖，改标准库 `net/http`（Go 1.22 路由）。
- 前端 Vue 3 + Vite + Element Plus + ECharts 重写；产物经 `//go:embed` 内嵌进二进制，单文件交付。
- 文档重整：`README` + `docs/{ARCHITECTURE,CONFIG,RULES,API,OPS}.md`。

### WAF 检测能力（lua 侧）

- 新增**请求头检测**阶段（`header.rule`）：Host 注入、`X-Forwarded-For`/`X-Real-IP` 伪造内网地址、头部 Log4Shell、
  多级 XFF 堆叠、Referer `javascript:`、`X-Original-URL` 穿越、CRLF 注入等。检测链路变为 12 步。
- 新增**参数归一化** `util.normalize()`：多轮 URL 解码 + `%uXXXX` + HTML 实体 + 去 SQL 注释，
  编码绕过样本命中率 5/8 → 8/8。
- **请求体字段级检测**：JSON 递归展开键与字符串值、multipart 逐段检测、上传后缀白名单（`post.upload.ext`）。
- **UA 白名单** `whiteua.rule`：内部拨测/监控探针可跳过 UA 检测。
- 规则匹配统一 `ngx.re.find(..., "joi")`（PCRE + JIT + 大小写不敏感），IP 黑白名单也统一为不敏感。
- 规则加固覆盖近五年攻击面：Log4Shell/Spring4Shell、fastjson/jackson、SQLi 变体、命令执行、SSRF/云元数据、
  XXE、原型链、GraphQL 内省、SSTI、Shiro/JNDI、以及 ProxyShell/MOVEit/OFBiz/PHPUnit/Laravel/ThinkPHP/
  Nacos/actuator/Druid/H2/Jenkins/Solr/Docker/K8s 等 CVE 探测路径。
  当前：args 50 / post 40 / url 27 / cookie 25 / header 12 / useragent 3 / blackip 38 / whiteip 8 / whiteUrl 7。

### 规则与配置管理

- 规则**逐条启停**（`rules.enabled`，老库自动补列且保持启用），停用规则保留不下发。
- **规则文件与数据库取并集**（`POST /api/rules/sync-files`）：文件有库里没有的新增，库里已有的不覆盖不删除；
  支持 `RULE_PATH` 与 `RULE_IMPORT_DIR` 多目录合并，跨目录去重、坏文件容错。
- **规则命中统计与样例**（防护分析）：按日志 `rule_tag` 聚合排行（今天/7/30 天），点击查看真实命中日志并可一键停用。
- **检测与拦截顺序**说明（`GET /api/rules/pipeline`），在安全概览顶部可展开查看（默认收起）。
- **WAF 配置独立菜单 + 模块选项卡**：`config.lua` 47 项按 9 个模块展示；
  新增 `code` 控件支持**拦截页 HTML 在线编辑**（Lua 长字符串 `[[ ... ]]` 读写、拒绝 `]]`）；
  后端按职责拆分 `luaconfig_schema.go` / `luaconfig_parse.go` / `luaconfig.go`。
- 攻击地图默认展示**今日**数据；`config.lua` 在线编辑 + 一键 reload。

### 日志与数据

- **Redis → Go 消费入库**（at-least-once）：`BRPOPLPUSH` + processing 队列 + 崩溃回灌；新增 `internal/ingest` 与
  `pkg/redis`（零依赖 RESP 客户端）。
- 安全概览新增「**日志入库队列**」卡片：待处理/处理中/已入库/失败、状态徽标、队列深度折线、未启用时的三步指引。
- 修复 xorm `Count()` 重置 WHERE 导致的「列表返回全量、总数却是过滤后」（日志/站点/账号/慢 SQL 仓储统计与列表分表查询）。
- 慢 SQL 统计支持时间范围、按 SQL 归并；后台自身慢 SQL 由 `pkg/sqlwatch` 采集展示。

### AI 研判

- 攻击日志与慢 SQL 均支持 AI 分析；结果服务端缓存（默认 10 分钟）并跨设备可见。
- 输出不再被截断：提高最大 tokens、`finish_reason=length` 时自动续写、截断状态可见。
- 修复「结果只有标题没有正文」（模型正文为空 / 推理模型 `reasoning_content`）。
- 修复「调用大模型接口失败」实为超时：分析类超时下限 180 秒、瞬时错误重试一次、错误信息可诊断。

### 界面与体验

- 全站移动端自适应（侧栏抽屉、网格降列、表格列裁剪、图表自适应）；主题明暗切换与主题色。
- 系统设置合并为选项卡页（运行配置 / 外观设置 / AI 设置 / 账号设置），WAF 配置独立成菜单。
- 攻击地图支持缩放/漫游、攻击状态条、攻击箭头、按 IP 归属地精确落点；地理库覆盖 483 个城市/地区/国家。

### 安全

- 登录验证码 + 登录频率限制；HMAC 签名会话 Cookie；写接口 CSRF 双提交。
- 口令 `md5(md5(raw)[5:10])` 兼容历史库；新增 CLI `reset-password` / `list-users` / `version`。
- 敏感配置（MySQL/Redis 口令、AI api_key）接口层脱敏，提交掩码表示不修改。

### 修复（WAF 配置保存与连通性自检）

- **修复保存 WAF 配置报「请求参数格式不正确」**：接口原先按 `map[string]string` 绑定，
  而前端数字控件（`el-input-number`）提交的是 JSON 数字，只要保存涉及任一瞬间数值字段就会 400/1001；
  改为 `map[string]any` 接住后统一转字符串（数字按十进制、布尔按 `on`/`off`），前端也统一转字符串提交。
- **新增「测试连接」**：`POST /api/lua/config/test` 用表单当前值测试 Redis（PING）与 MySQL（Ping），
  WAF 配置页工具条新增按钮，保存前即可确认地址/口令是否可用；错误信息按类型翻译
  （拒绝连接 / 超时 / 域名解析失败 / Access denied / Unknown database / Redis 口令错误），口令支持掩码复用已保存值。

### 改进（自检判据更准）

- 「生效自检」在 `config_redis_consume = on` 时不再把队列长度 0 当作异常（nginx 自己消费，长度本就接近 0），
  改为以「**最近入库时间**」作为日志链路的最终判据：直接查 `waf_log` 最新一条攻击日志的时间，
  30 分钟内有新增即判定链路正常；同时给出「日志目录最近是否还有写入」的结论。

### 修复（日志接口超时报 1500）

- **`/api/logs`、`/api/logs/filters`、`/api/logs/overview` 报「查询日志失败 / 统计攻击类型失败」**：
  根因是 `waf_log`（百万行）缺索引 + 查询里 `GROUP_CONCAT(DISTINCT area)`（area 上千种取值），
  单次 20~45 秒，客户端等不及超时即 500。修复：
  ① 启动时**自动补建索引**（local_time / attack_type / client_ip / server_name / (client_ip,local_time)，幂等）；
  ② 类型统计去掉 `GROUP_CONCAT(area)`；③ IP 排行改两段式（先取 Top N 再回表）并默认只统计近 30 天；
  ④ 筛选项改 `SELECT DISTINCT attack_type` + 60 秒缓存。
  实测：类型统计 27s→0.6s、IP 排行 33s→0.4s、概览 45s→1.8s、看板 4.7s→0.04s。

### 修复（日志模式不生效 / 改错文件）

- **`[waf] LUA_CONFIG` 默认改回 nginx 实际路径**（`/www/server/nginx/x-waf/config.lua`）：
  之前是相对路径 `lua-waf/config.lua`，后台改的是仓库副本，nginx reload 后自然不生效（本地联调用 `WAF_LUA_CONFIG` 覆盖）。
- **新增「生效自检」**（`GET /api/lua/config/verify` + WAF 配置页按钮）：逐项检查
  ① 配置路径是否绝对路径、② 规则目录同目录下是否还有另一份 config.lua（很可能是 nginx 真正加载的那份）、
  ③ 日志模式取值与 Redis 连通性（连接失败会提示「log_sink 会回落到写文件」）、
  ④ `config_redis_consume` 与后台 `[log] INGEST_ENABLED` 是否冲突、⑤ 规则目录是否存在，
  并给出 ok/warn/error 与总结论。

### 已知问题 / 注意

- `conf/app.ini` 的 `SESSION_SECRET` 为空时每次重启会失效会话，生产必须填固定值。
- `args.rule` 的 `\$\{` 规则在 JSON 请求体中可能误伤，必要时收窄或停用。
- `useragent.rule` 的脚本客户端规则会拦 `python-requests`、`curl`，内部拨测请用 `whiteua.rule` 白名单。
- 曾短暂引入 APISIX 插件下发（镜像复制 / key 限流），已按需求移除，相关代码与文档不再保留。

---

## 1.x（历史）

1.x 为旧版（macaron 框架 + 模板渲染），主要能力：攻击日志查看、规则文件管理、站点管理。
2.0.0 重写时保留其数据结构（`waf_log`、`rules`、`site`、`user`、`slow_sql`）与口令算法，
并新增「表已存在时只补缺失列」的兼容策略，便于从 1.x 平滑升级。
