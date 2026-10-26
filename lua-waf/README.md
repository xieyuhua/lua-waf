# lua-waf：openresty 侧 WAF

本目录是运行在 openresty（nginx + ngx_lua）中的 WAF 实现：拦截 SQL 注入、XSS、路径穿越、命令执行、
扫描器、CC 攻击等，并把命中记录写入日志，由 [wafadmin](../README.md) 后台可视化管理。

waf 与 wafadmin 必须部署在同一台服务器上：后台负责生成 vhost 与规则文件，waf 负责在请求链路上执行。

## 1. 文件说明

| 文件 | 作用 |
| --- | --- |
| `nginx.conf` | 参考主配置：声明 `lua_shared_dict`（limit / badGuys / waf_log_queue / waf_log_meta）、`init_by_lua_file`、`init_worker_by_lua_file`、`access_by_lua_file`，并 `include vhosts/*.conf` |
| `init.lua` | 配置加载时执行一次：`waf = require("waf")` 并 `waf.load_rules()` 把规则读入内存 |
| `init_worker.lua` | worker 启动时执行：启动异步日志消费者（`log_consumer.start()`） |
| `access.lua` | 请求入口（access 阶段）：解析真实 IP → CDN/蜘蛛 IP 放行 → URL 白名单 → `waf.check()` |
| `waf.lua` | 检测调度中心：各项检查函数与统一的 `check()` 顺序；规则表缓存在 `_M.RULES` |
| `malformed.lua` | **v2 新增**：畸形请求检测（HTTP 走私、裸 CRLF、非法编码、超长请求行、Host 校验） |
| `ban.lua` | **v2 新增**：自动封禁（命中攻击按 IP 计数，达阈值写入 `waf_bans` 并带 TTL，到期自动解封） |
| `body.lua` | **v2 新增**：请求体检测（urlencoded / multipart 分段 / JSON / 大请求体落盘） |
| `log_sink.lua` | **v2 新增**：日志输出统一入口，按 `config_log_mode` 选择写文件 / 内存队列 / Redis 队列，失败自动回落文件 |
| `log_mysql.lua` | **v2 新增**：批量 INSERT `waf_log`（lua-resty-mysql + 连接池 + `ngx.quote_sql_str` 转义） |
| `log_redis.lua` | **v2 新增**：Redis 列表入队 / 出队（lua-resty-redis + 连接池 + 可选密码与库号） |
| `log_consumer.lua` | **v2 新增**：`ngx.timer.every` 定时批量落库，共享内存锁保证单 worker 写入，失败自动回队 |
| `config.lua` | 全部开关与阈值 |
| `util.lua` | 工具：读取规则文件、取客户端 IP、构造日志记录并交给 `log_sink`、封禁 IP、输出拦截页 |
| `iputils.lua` / `ip2region.lua` / `iop.lua` / `bitop.lua` | IP 归属地解析（ip2region）与 CDN/蜘蛛 IP 段数据 |
| `ip2region.db` | IP 库数据文件 |
| `rules/*.rule` | 规则文件，由后台「下发全部规则」生成 |
| `sql/waf_log.sql` | `waf_log` 建表语句（mysql / redis 日志模式必需） |

## 2. 请求处理流程

```
access_by_lua_file access.lua
   ├─ real_ip = 从 X_real_ip / X_Forwarded_For 取真实 IP
   ├─ ip_open_list 命中（主流 CDN/蜘蛛）→ 直接放行
   ├─ URL 白名单（whiteUrl.rule）→ 放行
   └─ waf.check()  ← 命中即记录日志并输出拦截页
```

`waf.check()` 的检测顺序（先命中先返回）：

```
1. white_ip_check            IP 白名单（放行）
2. ban_check                 自动封禁表（拒绝）
3. malformed_request_check   畸形请求（走私 / 头部注入 / 非法编码）
4. black_ip_check            IP 黑名单（拦截）
5. user_agent_attack_check   UA 特征（扫描器）
6. white_url_check           URL 白名单（放行）
7. url_attack_check          URL 路径特征
8. cc_attack_check           CC 频率
9. cookie_attack_check       Cookie 特征
10. url_args_attack_check    GET 参数特征
11. post_attack_check        请求体特征（含 multipart 分段）
```

此外还有 `bad_guy_check`：`ngx.shared.badGuys` 中已被累计拦截的 IP，用于「幻影/诱饵」后端切换。

## 3. 规则文件

每个类型一个 JSON 文件，内容为数组，**键名必须保持首字母大写**（lua 侧按 `rule["RuleItem"]` 取值）：

```json
[{ "Id": 4, "RuleType": "args", "RuleItem": "\\.\\./" },
 { "Id": 7, "RuleType": "args", "RuleItem": "select.+(from|limit)" }]
```

| 规则类型 | 文件 | 作用 |
| --- | --- | --- |
| `args` | `args.rule` | GET 参数注入特征 |
| `post` | `post.rule` | 请求体注入特征（含 multipart 分段内容） |
| `cookie` | `cookie.rule` | Cookie 特征 |
| `url` | `url.rule` | URL 路径特征 |
| `useragent` | `useragent.rule` | 扫描器/爬虫 UA |
| `headers` | `headers.rule` | 自定义请求头特征（后台可维护，默认无内置规则） |
| `blackip` | `blackip.rule` | IP 黑名单 |
| `whiteip` | `whiteip.rule` | IP 白名单 |
| `whiteUrl` | `whiteUrl.rule` | URL 白名单 |

`RuleItem` 为 **PCRE** 片段（`ngx.re.find` 匹配），不是完整的 Lua 模式串。

## 4. v2 新增：请求体与畸形请求防御

### 4.1 body.lua（请求体 / POST 分段）

1. **统一读取**：先 `ngx.req.get_body_data()`；返回空说明请求体已写入临时文件（大请求体、分段上传），
   此时用 `ngx.req.get_body_file()` 读取并检测 —— 修复了 1.x 只扫 `get_post_args()` 导致
   **大包 / multipart 绕过**的问题。
2. **按类型分段**：
   - `application/x-www-form-urlencoded`：逐参数解码后检测键与值；
   - `multipart/form-data`：逐段解析 `Content-Disposition`，检测字段名、文件名与内容；
   - `application/json` 及其他：整体检测。
3. **分段畸形拦截**：缺失 `boundary`、分段头没有 `\r\n\r\n` 结束符、分段数量超过 `config_post_max_parts`、
   分段头含 NUL 或裸换行、字段名含 `"`/`;`/CRLF、上传文件名含 `/`、`\` 或 `../`。
4. **体量保护**：超过 `config_post_max_size` 只记录一条「请求体超限」日志，不做深度扫描，
   避免影响正常大文件上传。

### 4.2 malformed.lua（畸形请求）

| 检查 | 说明 | 规则标签 |
| --- | --- | --- |
| TE + CL 同时存在 | 请求走私 TE.CL | `http.smuggling.te_cl` |
| 重复 Content-Length 取值不一致 | 请求走私 CL.CL | `http.smuggling.cl_cl` |
| Content-Length 非纯数字 | 畸形头 | `http.malformed.cl` |
| Transfer-Encoding 非单一 `chunked` | 请求走私 | `http.smuggling.te` |
| 原始报文含 NUL | 头部注入 | `http.malformed.nul` |
| 头部出现裸 CR / LF | 头部注入 | `http.malformed.crlf` |
| 非规范百分号编码（`%` 后非两位十六进制） | 编码绕过 | `http.malformed.percent` |
| URI 或解码后出现控制字符 | 编码绕过 | `http.malformed.control*` |
| URI 存在 `%25xx` 二次编码 | 编码绕过（**默认只记日志不拦截**） | `http.malformed.double_encode` |
| 请求行超过 4096 字节 | 资源滥用 | `http.malformed.uri_len` |
| Host 缺失或含非法字符 | 畸形请求 | `http.malformed.host` |

> 二次编码默认只记录日志，避免误杀正常业务；如需拦截，把 `malformed.lua` 中
> `_M.report(..., false)` 的最后一个参数去掉即可。

## 5. 配置项（config.lua）

| 配置 | 默认值 | 说明 |
| --- | --- | --- |
| `config_waf_enable` | `"on"` | WAF 总开关（关时只记日志不拦截） |
| `config_log_dir` | `/www/wwwlogs/waf/` | 攻击日志目录 |
| `config_rule_dir` | `/www/server/nginx/x-waf/rules` | 规则文件目录，**必须与后台 `RULE_PATH` 一致** |
| `config_white_ip_check` / `config_black_ip_check` | `"on"` | 黑白名单开关 |
| `config_white_url_check` / `config_url_check` | `"on"` | URL 白名单 / URL 特征 |
| `config_url_args_check` | `"on"` | GET 参数 |
| `config_user_agent_check` | `"on"` | UA |
| `config_cookie_check` | `"on"` | Cookie |
| `config_post_check` | `"on"` | 请求体检测（v2 起覆盖 multipart 与大请求体） |
| `config_post_multipart_check` | `"on"` | multipart / 分段上传检测 |
| `config_post_max_size` | `8388608` | 请求体深度检测上限（8MB） |
| `config_post_max_parts` | `200` | multipart 分段数量上限 |
| `config_malformed_check` | `"on"` | 畸形请求检测 |
| `config_cc_check` / `config_cc_rate` | `"on"` / `"300/60"` | 单 URI 频率：60 秒 300 次 |
| `config_ipcc_check` / `config_ipcc_rate` | `"on"` / `"120/7200"` | 单 IP 频率：7200 秒 120 次 |
| `config_waf_model` | `"html"` | 拦截输出方式：`html` / `redirect` |
| `config_waf_redirect_url` | `""` | `redirect` 模式下的跳转地址 |
| `config_output_html` | 内置模板 | 拦截页 HTML，`%s` 为客户端 IP |
| `config_expire_time` | `600` | 封禁过期时间（秒） |
| `config_log_mode` | `"file"` | 日志输出模式：`file` / `mysql` / `redis` |
| `config_log_batch_size` | `200` | 每轮落库条数 |
| `config_log_flush_interval` | `2` | 落库间隔（秒） |
| `config_log_queue_size` | `20000` | 内存队列上限（条），超出丢弃并回落写文件 |
| `config_mysql_host` / `config_mysql_port` | `127.0.0.1` / `3306` | 落库 MySQL 地址 |
| `config_mysql_database` / `config_mysql_user` / `config_mysql_password` | `waf` / `waf` / `waf` | 落库库名与账号 |
| `config_mysql_keepalive` | `60000` | 连接池空闲时间（毫秒） |
| `config_redis_host` / `config_redis_port` | `127.0.0.1` / `6379` | Redis 地址（redis 模式） |
| `config_redis_password` / `config_redis_db` | `""` / `0` | Redis 密码与库号 |
| `config_redis_list_key` | `"waf:logs"` | Redis 队列键名 |
| `config_auto_ban_check` | `"off"` | 是否自动封禁命中攻击的来源 IP（**默认关闭**） |
| `config_auto_ban_expire` | `600` | 封禁时长（秒），到期自动解封 |
| `config_auto_ban_window` | `60` | 命中计数窗口（秒） |
| `config_auto_ban_threshold` | `1` | 窗口内命中多少次后封禁 |
| `config_auto_ban_types` | `"*"` | 参与封禁的攻击类型，逗号分隔；`*` 表示全部（排除「请求体超限」等非攻击事件） |

## 6. 日志输出模式（文件 / 高并发异步落库）

日志统一由 `util.log_record()` → `log_sink.write()` 输出，由 `config_log_mode` 决定去向：

| 模式 | 链路 | 适用场景 |
| --- | --- | --- |
| `file` | 直接追加写 `/www/wwwlogs/waf/x_waf.log`（一行一条 JSON） | 已有采集器（Filebeat/rsyslog/自研）读取文件入库，或仅做本地审计 |
| `mysql` | `RPUSH ngx.shared.waf_log_queue` → `ngx.timer` 每 2 秒 `LPOP` 一批 → 批量 `INSERT INTO waf_log` | **单机高并发**，不引入额外组件，写入不阻塞请求 |
| `redis` | `RPUSH waf:logs` → `ngx.timer` 每 2 秒 `LPOP` 一批 → 批量 `INSERT INTO waf_log` | **多台 WAF 汇聚**、Redis 削峰、MySQL 抖动时缓冲 |

三种模式都保证「请求链路不阻塞」：

- 写文件是本地 append（毫秒级），与 1.x 行为一致；
- `mysql` / `redis` 模式在请求中只做一次内存写入或一次 Redis 命令（复用连接池），
  真正的 INSERT 全部发生在 `ngx.timer` 轻线程里，按批合并（默认 200 条 / 2 秒）；
- **失败兜底**：共享内存未声明、队列满、Redis 连不上、MySQL 报错都不会丢日志 ——
  采集端回落写文件；落库失败会把整批记录重新入队（内存模式回队列尾部，redis 模式重新 RPUSH），下一轮重试；
- **单写者**：多个 worker 各有一个 timer，但通过 `ngx.shared.waf_log_meta` 的 `flush_lock`
  保证同一时刻只有一个 worker 执行批量 INSERT，避免锁竞争；
- **可观测**：`log_consumer.stats()` 返回 `inserted / failed / dropped / pending`，
  可直接在拦截页调试或写入日志观察。

### 启用步骤

1. 建表：`mysql -u root -p waf < sql/waf_log.sql`
2. `config.lua` 中把 `config_log_mode` 改为 `mysql`（或 `redis`），填写 MySQL / Redis 连接信息；
3. 确认 `nginx.conf` 已声明共享内存并加了 worker 初始化钩子：

```nginx
lua_shared_dict waf_log_queue 20m;
lua_shared_dict waf_log_meta  1m;
init_worker_by_lua_file /www/server/nginx/x-waf/init_worker.lua;
```

4. `nginx -t && nginx -s reload`，在 `error_log info` 中应看到
   `[waf] 日志输出模式: mysql，每 2 秒批量落库一次`。

### 调优建议

- 队列大小 `config_log_queue_size` 按峰值 QPS × 落库间隔 估算，`20m` 共享内存约可容纳数万条短日志；
- 单批 200~500 条、间隔 1~3 秒是比较稳的组合；MySQL 压力大时优先增大批量、再拉长间隔；
- Redis 模式注意 `maxmemory-policy`，不要设为 `allkeys-lru`（可能淘汰日志），建议 `noeviction` + 监控 `llen`；
- `waf_log` 表增长快时，可按月分表或定期归档（后台「攻击日志」只读最近数据）。

## 7. 自动封禁（可选）

**默认不封禁 IP。** 命中攻击时只拦截「当前这一次请求」（输出拦截页 + 记录日志），
不会把 IP 加入黑名单。要获得跨请求的封禁效果，目前有三条路径：

| 机制 | 触发条件 | 效果 | 生效范围 |
| --- | --- | --- | --- |
| IP 黑名单（`blackip.rule`） | 后台手工维护 / 日志页「封禁」按钮 | 一律 403 | 长期，直到规则被删除并下发 |
| 拦截频率封禁（`config_ipcc_rate`） | 同一 IP 在 7200 秒内被拦截超过 120 次 | 窗口内后续请求一律 403 | 窗口到期自动解除 |
| **自动封禁（`config_auto_ban_check`）** | 命中攻击按 IP 计数，窗口内达阈值 | 写入 `waf_bans`（TTL 到期自动解封），`ban_check` 阶段直接 403 | 可配置（默认 600 秒） |

开启自动封禁：

```lua
config_auto_ban_check = "on",      -- 打开开关
config_auto_ban_expire = 600,      -- 封多久（秒）
config_auto_ban_window = 60,       -- 统计窗口（秒）
config_auto_ban_threshold = 1,     -- 窗口内命中几次封禁（1 = 首次命中即封）
config_auto_ban_types = "*",       -- 参与的攻击类型；也可写成 "sql注入,XSS跨站,命令执行"
```

并在 `nginx.conf` 声明封禁表：

```nginx
lua_shared_dict waf_bans 20m;
```

实现要点：

- 计数键 `hit:<ip>`（带窗口 TTL），封禁键 `ban:<ip>`（带封禁 TTL），到期由共享内存自动清理，无需定时任务；
- 白名单 IP 优先于封禁（`white_ip_check` 在 `ban_check` 之前），避免误封自家出口；
- 「请求体超限」「IP封禁中」等非攻击事件不计入，防止日志类事件触发封禁；
- 排障：`require("ban").stats(ip)` 可查看 `hits` 与 `banned`。

> 注意：检测即封禁在 **NAT / CDN 回源 / 办公网出口** 场景容易误伤整片用户。
> 建议先用 `config_auto_ban_threshold = 3` + `window = 300` 观察一段时间日志，再收紧；
> 单次误判希望只拦当前请求时，保持默认关闭，改用后台日志页的「封禁」按钮人工处置。

## 8. 部署与生效

1. 把整个目录放到 `/www/server/nginx/x-waf/`，确保 `nginx.conf` 中
   `lua_package_path` 覆盖该目录（新增的 `body.lua`、`malformed.lua`、`log_*.lua` 必须与 `waf.lua` 同级）。
2. 在 `http {}` 中声明共享内存与入口：

```nginx
lua_shared_dict limit 100m;
lua_shared_dict badGuys 100m;
lua_shared_dict waf_bans      20m;   -- 自动封禁（config_auto_ban_check=on 时）
lua_shared_dict waf_log_queue 20m;   -- 仅 mysql 模式需要
lua_shared_dict waf_log_meta  1m;    -- 仅 mysql 模式需要
lua_code_cache on;
init_by_lua_file /www/server/nginx/x-waf/init.lua;
init_worker_by_lua_file /www/server/nginx/x-waf/init_worker.lua;
access_by_lua_file /www/server/nginx/x-waf/access.lua;
```

3. **规则/配置更新后不需要重启 nginx**：规则在 `init_by_lua` 阶段读入内存，
   `nginx -s reload` 会重建 worker 并重新加载；后台「下发全部规则」「站点同步」已自动执行
   `nginx -t` → `nginx -s reload`（检查不通过则不重载）。
   只有修改 `lua_shared_dict` 名称/大小、升级 nginx 二进制时才需要完全重启。
4. 若担心源码泄露，可用 luajit 预编译：

```bash
/www/server/nginx/luajit/bin/luajit -b access.lua access11.lua
```

5. 调试技巧：
   - 临时把 `lua_code_cache` 设为 `off`，改 lua 立即生效（性能差，仅调试用）；
   - `nginx -t` 校验语法；`error_log ... info;` 查看规则加载与日志模式启用情况；
   - 拦截页与命中记录都可在后台「攻击日志」中按 IP / 类型 / 时间范围查询。

## 9. 日志格式

`util.log_record()` 构造一条记录交给 `log_sink`，字段与后台 `waf_log` 表一一对应
（`file` 模式写一行 JSON；`mysql` / `redis` 模式按字段批量入库）：

```json
{"client_ip":"221.229.166.13","local_time":"2026-10-26 22:16:52","server_name":"www.example.com",
 "user_agent":"sqlmap/1.7.2","attack_type":"CC攻击","req_url":"/download?name=..%2F",
 "area":"中国-北京","req_data":"id=1' AND 1=1 --","rule_tag":"args->cc"}
```

后台按 `local_time` 做时间范围筛选、按 `area` 做攻击地图定位、按 `attack_type` 做类型统计。

## 10. 致谢

1. 感谢春哥开源的 [openresty](https://openresty.org)
2. 感谢 unixhot 开源的 [waf](https://github.com/unixhot/waf)
3. 感谢 lunny 开源的 [xorm](https://github.com/go-xorm/xorm)
4. 感谢 [ip2region](https://github.com/lionsoul2014/ip2region) 提供的 IP 归属地库
