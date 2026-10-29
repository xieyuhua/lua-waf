# WAF 规则与检测链路

本文说明 openresty 侧（`lua-waf/`）的检测顺序、规则文件格式、匹配语义，以及后台如何参与规则管理。

---

## 1. 检测与拦截顺序（12 步）

`waf.lua` 的 `check()` 串行执行，**任一命中即结束**本次请求的后续检查：

| # | 阶段 | 函数 | 命中动作 | 规则 / 开关 |
|---|---|---|---|---|
| 1 | IP 白名单 | `white_ip_check` | 放行（跳过全部检测） | `whiteip.rule` · `config_white_ip_check` |
| 2 | 封禁名单 / 自动封禁 | `ban_check` | 拦截（返回封禁页，不写新日志） | 共享内存封禁表 · `config_auto_ban_check` |
| 3 | 畸形请求 | `malformed_request_check` | 拦截 | 代码实现 · `config_malformed_check` |
| 4 | IP 黑名单 | `black_ip_check` | 拦截 | `blackip.rule` · `config_black_ip_check` |
| 5 | 请求头检测 | `header_attack_check` | 拦截 | `header.rule` · `config_header_check` |
| 6 | User-Agent 检测 | `user_agent_attack_check` | 检测 | `useragent.rule`（`whiteua.rule` 可放行）· `config_user_agent_check` |
| 7 | URL 白名单 | `white_url_check` | 放行 | `whiteUrl.rule` · `config_white_url_check` |
| 8 | URL 检测 | `url_attack_check` | 检测 | `url.rule` · `config_url_check` |
| 9 | CC 频率 | `cc_attack_check` | 检测 | `config_cc_rate` · `config_cc_check` |
| 10 | Cookie 检测 | `cookie_attack_check` | 检测 | `cookie.rule` · `config_cookie_check` |
| 11 | GET 参数检测 | `url_args_attack_check` | 检测 | `args.rule` · `config_url_args_check` |
| 12 | 请求体检测 | `post_attack_check` | 检测 | `post.rule` · `config_post_check` / `config_post_multipart_check` |

后台「安全概览 → 检测与拦截顺序」与 `GET /api/rules/pipeline` 展示的就是这份顺序（数据源：`internal/service/rulepipeline.go`），改 lua 时需同步更新。

命中后的统一动作：

1. `util.log_record(...)` 记录攻击类型、请求地址、命中内容、规则标识；
2. 若 `config_waf_enable = "on"`，按 `config_waf_model` 输出（html 拦截页 / 302 跳转 / 幻影降级）；
3. 若开启自动封禁，按 IP 累计命中次数，达阈值写入封禁表（后续请求在第 2 步即被拦）。

---

## 2. 规则文件格式

规则文件位于 `config_rule_dir`（默认 `/www/server/nginx/x-waf/rules/`），每个类型一个 `*.rule`，内容是 JSON 数组：

```json
[
  {"Id": 4, "RuleType": "args", "RuleItem": "\\.\\./"},
  {"Id": 5, "RuleType": "args", "RuleItem": "(?i)\\$\\{jndi:"}
]
```

| 字段 | 说明 |
|---|---|
| `Id` | 规则编号（后台数据库主键；文件里仅作标识） |
| `RuleType` | 规则类型（= 文件名，如 `args`、`header`） |
| `RuleItem` | 正则片段（PCRE 语法） |

后台按类型下发时会把该类型下**启用中**的规则写入 `<类型>.rule`；停用的规则保留在库里但不下发。

---

## 3. 匹配语义（重要）

- **大小写不敏感**：所有匹配统一 `ngx.re.find(subject, rule, "joi")`，`o`=JIT 编译缓存，`i`=忽略大小写。
  因此规则里不需要也不应该靠大小写区分（`[A-Z]` 会被弱化）；需要大小写敏感时用内联 `(?-i)` 局部关闭。
- **归一化后再匹配**：URL、Cookie、GET 参数、请求头都会先经过 `util.normalize()`：
  最多 3 轮 URL 解码 → `%uXXXX` → HTML 实体（`&#x2e;`/`&#46;`）→ **整体删除 SQL 注释标记**（`un/**/ion` → `union`）。
  原值仍匹配一次（双保险）。
- **请求体分段**：`application/json`（含 `+json`）递归展开**键与字符串值**逐段匹配；`multipart/form-data` 按分段落检测，
  文件名、字段名、段内容都会过规则；`x-www-form-urlencoded` 按参数展开。
- **上传白名单**：开启 `config_upload_check` 后，multipart 文件名后缀不在 `config_upload_ext_whitelist` 内直接拦截（标识 `post.upload.ext`）。

实测编码绕过样本（归一化前后命中数）：

| 样本 | 归一化前 | 归一化后 |
|---|---|---|
| `%252e%252e%252fetc%252fpasswd` | 命中 | 命中 |
| `%u002e%u002e/…/etc/passwd` | 命中 | 命中 |
| `&#x2e;&#x2e;/&#x2e;&#x2e;/etc/passwd` | 命中 | 命中 |
| `x=%24%7Bjndi%3Aldap%3A%2F%2F…` | 漏 | **命中** |
| `id=1 and sle%65p(5)` | 漏 | **命中** |
| `id=1 un/**/ion sel/**/ect 1,2,3` | 漏 | **命中** |

---

## 4. 现有规则清单

| 文件 | 条数 | 覆盖的攻击面 |
|---|---|---|
| `args.rule` | 50 | SQL 注入（含 `union all select`、`extractvalue`、`into outfile`、`waitfor delay`）、命令执行、路径穿越（含双重编码）、SSRF/云元数据、SSTI、Log4Shell、Spring4Shell/OGNL、fastjson/jackson、原型链、GraphQL 内省、LDAP 注入、提示注入 |
| `post.rule` | 40 | 请求体侧的同类攻击 + 上传文件名危险后缀（含 RFC5987 `filename*=UTF-8''x.jsp`）、双扩展名、`x-httpd-php`、webshell 特征 |
| `url.rule` | 27 | 目录穿越、备份/源码后缀、CVE 探测路径（ProxyShell、MOVEit、vCenter、Confluence、OFBiz、PHPUnit、Laravel、ThinkPHP、Nacos、actuator、Druid、H2、Jenkins/Solr、Docker/K8s/etcd/`_cat`） |
| `cookie.rule` | 25 | 注入类特征、Shiro `rememberMe`、Java 序列化 `rO0AB`、Shellshock |
| `header.rule` | 12 | Host 注入、`X-Forwarded-For`/`X-Real-IP` 伪造内网地址、头部 Log4Shell、多级 XFF 堆叠、Referer `javascript:`/`<script>`、`X-Original-URL` 穿越、CRLF 注入、`X-Middleware-Subrequest` |
| `useragent.rule` | 3 | 扫描器（sqlmap/nikto/nuclei/gobuster…）、脚本客户端（python-requests/curl/go-http-client…） |
| `blackip.rule` | 38 | 静态 IP 黑名单 |
| `whiteip.rule` | 8 | IP 白名单 |
| `whiteUrl.rule` | 7 | URL 白名单 |
| `whiteua.rule` | 0 | UA 白名单（默认空；填了就跳过 UA 检测，用于内部拨测/监控探针） |

> `whiteua.rule` 是「UA 白名单」，命中后**不再做 UA 检测**，例如内部拨测工具：

```json
[{"Id": 1, "RuleType": "whiteua", "RuleItem": "(?i)^(monitor-probe|internal-checker)/"}]
```

---

## 5. 后台的规则管理能力

| 能力 | 入口 | 接口 |
|---|---|---|
| 按类型浏览 / 搜索 / 新增 / 编辑 / 删除 | WAF 规则 | `GET/POST /api/rules`、`PUT/DELETE /api/rules/{id}` |
| 单条规则启用/停用 | WAF 规则行内开关 | `PUT /api/rules/{id}/enabled` |
| 从规则文件同步（并集） | WAF 规则 → 「从规则文件同步」 | `POST /api/rules/sync-files` |
| 下发全部规则到 lua 目录 | WAF 规则 → 「下发全部规则」 | `POST /api/rules/sync` |
| 命中排行与样例 | 防护分析 | `GET /api/rules/stats`、`GET /api/rules/stats/samples` |
| 检测链路说明 | 安全概览顶部 / 防护分析 | `GET /api/rules/pipeline` |

### 同步（并集）语义

扫描 `RULE_PATH` 与 `RULE_IMPORT_DIR` 下所有 `*.rule`，合并后与数据库比对：

- 文件里有、库里没有 → **新增**；
- 库里已有 → **保持原样**（不覆盖、不删除、不改启停状态）；
- 跨目录/同文件重复 → 只算一次；坏文件跳过并在返回信息中提示；目录不可读不算错误。

---

## 6. 误报治理流程

1. **看排行**：防护分析 → 规则命中 Top（今天 / 7 天 / 30 天），关注命中量异常高的规则；
2. **看样例**：点「看样例」，抽屉里是该规则命中的真实日志（IP、时间、站点、URL、Body 截断）；
3. **判定**：
   - 样例是自家业务（内部拨测、健康检查、特殊参数名）→ 误报，直接「停用该规则」；
   - 确为攻击 → 保持启用，必要时收窄正则；
4. **上线**：停用/启用是即时写库的，但要「下发全部规则」+ `nginx -s reload` 才对线上生效；
5. **回归**：新增或修改规则后，先用一批攻击样本与正常样本验证（命中率 + 误杀），再下发。

> 已知需要留意的两条规则：
> - `useragent.rule` 的脚本客户端规则会拦 `python-requests`、`curl` 等，内部拨测请改用 `whiteua.rule` 白名单；
> - `args.rule` 的 `\$\{` 在 JSON 请求体中可能误伤，必要时收窄或停用。
## 按 URL 的精细化限频（ratelimit）

除了 `config.lua` 里的全局 CC / IP 频率（`config_cc_rate` / `config_ipcc_rate`），
还可以用「防护配置 → **限频规则**」给不同 URL 配不同阈值，并按不同维度计数。

### 规则字段

| 字段 | 说明 |
|---|---|
| URL 匹配 | `prefix` 前缀（如 `/api/login`）或 `regex` 正则（如 `^/open/api/`，lua 侧 `ngx.re.find` 忽略大小写） |
| 计数维度 | `ip` 来源 IP / `header` 请求头（如 `X-API-Key`）/ `cookie` / `arg` URL 参数 / `uri` 完整 URI |
| 阈值 | `limit` 次 / `window` 秒（窗口滑动由 `ngx.shared.limit` 的 TTL 控制） |
| 超限动作 | `block` 直接拦截，或 `log` 只记录（用于观察是否误伤） |

### 生效方式

1. 保存规则后后台自动下发到 `<RULE_PATH>/ratelimit.rule`（JSON 数组）；
2. lua 侧 `ratelimit.lua` 读取该文件（按 mtime 热加载），在请求链路中**按顺序匹配、第一条命中生效**；
3. 因此**更具体的规则应排在前面**（列表按 ID 升序下发；需要调整顺序时按想要的优先级依次新建）。

### 命中后的日志

- 拦截：`attack_type = 限频攻击`，`rule_tag = ratelimit:<规则ID>`，`req_data` 里带维度值、窗口、实际次数与阈值；
- 仅记录：`attack_type = 限频观察`，不拦截但会产生攻击日志，便于评估阈值。

### 示例

| 场景 | URL 匹配 | 维度 | 阈值 | 动作 |
|---|---|---|---|---|
| 登录防爆破 | `/api/login`（前缀） | IP | 10 次 / 60 秒 | 拦截 |
| 开放 API 按密钥限流 | `^/open/api/`（正则） | Header `X-API-Key` | 1000 次 / 60 秒 | 拦截 |
| 下载防刷 | `/dl/`（前缀） | Cookie `session` | 20 次 / 60 秒 | 拦截 |
| 全站探活接口观察 | `/ping`（前缀） | URI | 100 次 / 60 秒 | 仅记录 |

> 计数使用共享内存 `lua_shared_dict limit`（nginx.conf 中已有），重启 nginx 计数清零属正常。