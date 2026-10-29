# 规则与检测说明

## 1. 规则集

规则存在数据库 `rules` 表，同时下发为 lua 侧 `rules/*.rule`（JSON 数组）。

```json
[{"Id":1,"RuleType":"args","RuleItem":"(?:\\b(?:and|or)\\b\\s*\\d+\\s*=\\s*\\d+)"}]
```

| RuleType | 文件 | 检测阶段 | 说明 |
|---|---|---|---|
| `args` | `args.rule` | 11 | GET 参数（含解码后内容） |
| `post` | `post.rule` | 12 | POST 表单/JSON 值 |
| `url` | `url.rule` | 8 | 请求 URI |
| `cookie` | `cookie.rule` | 10 | Cookie 值 |
| `header` | `header.rule` | 5 | 请求头（name: value 整体） |
| `useragent` | `useragent.rule` | 6 | User-Agent |
| `blackip` | `blackip.rule` | 4 | IP 黑名单（精确/网段） |
| `whiteip` | `whiteip.rule` | 1 | IP 白名单 |
| `whiteurl` | `whiteUrl.rule` | 7 | URL 白名单 |
| `whiteua` | `whiteua.rule` | 6 | UA 白名单 |

匹配全部使用 `ngx.re.find(subject, rule, "joi")`：**PCRE + JIT + 忽略大小写**。

## 2. 检测与拦截顺序（12 步）

```
1  白名单 IP        config_white_ip_check
2  封禁表           （自动封禁/手工封禁，命中即拦）
3  畸形请求         config_malformed_check
4  黑名单 IP        config_black_ip_check
5  请求头           config_header_check
6  User-Agent       config_user_agent_check（含 UA 白名单）
7  URL 白名单       config_white_url_check
8  URL              config_url_check
9  限频规则         ratelimit.rule（按 URL + 维度，见 §4）
10 CC / IP 频率     config_cc_check / config_ipcc_check
11 Cookie           config_cookie_check
12 参数 + POST      config_url_args_check / config_post_check
```

后台「安全概览」顶部可展开查看该顺序（`GET /api/rules/pipeline`）。

## 3. 规则文件同步（取并集）

后台「WAF 规则 → 从规则文件同步」把 `RULE_PATH` 与 `RULE_IMPORT_DIR` 里的 `*.rule` 合并入库：

- 文件里有、库中没有 → **新增**；
- 库里已有 → **保留不动**（不覆盖、不删除）；
- 跨文件/跨目录重复 → 只算一次，计入 `duplicates`；
- `ratelimit.rule` 会被跳过（它是限频规则的下发产物，schema 不同）。

返回字段与自证关系：

| 字段 | 含义 |
|---|---|
| `files` | 读取的规则文件数 |
| `in_files` | 去重后文件里的规则条数 = `added + kept` |
| `added` / `kept` | 本次新增 / 库里已存在 |
| `duplicates` / `skipped` | 重复跳过 / 无效条目（缺 RuleType 或 RuleItem） |
| `total` | 同步后库内规则总数 |

## 4. 按 URL 的精细化限频

与全局 CC/IP 频率的区别：全局只有一组阈值；限频规则可给**不同 URL 配不同阈值**，并按不同维度计数。

规则字段见 [CONFIG.md §3](CONFIG.md#3-限频规则数据库--rulesratelimitrule)，要点：

- 规则按 `id` 升序下发，lua **顺序匹配、第一条命中生效** → 更具体的 URL 放前面；
- 计数键：`rl:<规则ID>:<维度值>`，存 `ngx.shared.limit`，TTL = `window`；
- 取不到维度值（如规则按 `X-API-Key` 但请求没带）→ **不计数、放行**，不会合并成"空 key"桶；
- 命中日志：拦截 `attack_type=限频攻击`、观察 `attack_type=限频观察`，`rule_tag=ratelimit:<id>`，
  `req_data` 含维度值、窗口、实际次数与阈值。

### 常见配置

| 场景 | URL 匹配 | 维度 | 阈值 | 动作 |
|---|---|---|---|---|
| 登录防爆破 | `/api/login`（前缀） | IP | 10 次 / 60 秒 | 拦截 |
| 开放 API 按密钥限流 | `^/open/api/`（正则） | Header `X-API-Key` | 1000 次 / 60 秒 | 拦截 |
| 下载防刷 | `/dl/`（前缀） | Cookie `session` | 20 次 / 60 秒 | 拦截 |
| 全站单 IP 限速 | `/`（前缀） | IP | 120 次 / 60 秒 | 拦截 |
| 防盗链 | `\.(zip|mp4)$`（正则） | Referer | 30 次 / 60 秒 | 拦截 |
| 接口观察 | `/ping`（前缀） | URI | 100 次 / 60 秒 | 仅记录 |

## 5. 畸形请求检测

| 检查 | rule_tag | 拦截 |
|---|---|---|
| TE + CL 同时存在（TE.CL 走私） | `http.smuggling.te_cl` | 是 |
| 重复 Content-Length 取值不一致（CL.CL） | `http.smuggling.cl_cl` | 是 |
| 非法 Transfer-Encoding（非单一 chunked） | `http.smuggling.te` | 是 |
| Content-Length 非整数 | `http.malformed.cl` | 是 |
| 缺失/非法 Host | `http.malformed.host` | 是 |
| 原始报文含 NUL / 裸换行 | `http.malformed.nul` / `crlf` | 是 |
| 超长请求行 / 非规范百分号编码 / 控制字符 | `http.malformed.*` | 是 |
| URI 二次编码（`%25xx`） | `http.malformed.double_encode` | 否（只记录） |

> 注意：`ngx.req.get_headers(0, true)` 返回的头名**保留原始大小写**，索引前必须自行转小写。

## 6. 上传与请求体

- 超过 `config_post_max_size` 的请求体只记录不深检（避免影响正常大文件上传）；
- multipart 分段超过 `config_post_max_parts` 视为畸形；
- `config_upload_check` + `config_upload_ext_whitelist` 做上传后缀白名单校验（命中 `post.upload.ext`）。

## 7. 归一化（规避编码绕过）

`util.normalize()` 在匹配前对 URL / Cookie / 参数 / 请求头做统一处理：

1. 连续 3 次 URL 解码；
2. `%uXXXX` → 字符；
3. HTML 实体还原；
4. 删除 `/* … */` 注释（SQL 注入常见绕过）。

原始值与被归一化后的值都会参与规则匹配，命中即记录。