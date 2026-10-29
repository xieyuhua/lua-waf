# 文档索引

| 想知道什么 | 看这篇 |
|---|---|
| 这个项目是什么、怎么跑起来 | [../README.md](../README.md) |
| 代码怎么分层、数据怎么流动、为什么这么设计 | [ARCHITECTURE.md](ARCHITECTURE.md) |
| `app.ini` / `config.lua` 每个配置项什么意思 | [CONFIG.md](CONFIG.md) |
| 规则格式、检测顺序、限频规则怎么写 | [RULES.md](RULES.md) |
| 某个接口的路径/参数/返回 | [API.md](API.md) |
| 上线、巡检、出问题怎么排查 | [OPS.md](OPS.md) |
| 版本变更 | [../CHANGELOG.md](../CHANGELOG.md) |
| 还有哪些待优化、下次升级做什么 | [ROADMAP.md](ROADMAP.md) |

## 按角色

- **运维**：README §3 快速开始 → OPS §1 部署 → OPS §3 巡检 → 出问题查 OPS §4
- **安全运营**：RULES 全篇（规则与顺序）→ CONFIG §3 限频规则 → 后台「防护分析 / 攻击日志」
- **开发**：ARCHITECTURE（分层与取舍）→ API（接口契约）→ CONFIG（配置项来源）

## 关键约定（先记住这几条）

1. lua 侧**只写 Redis、不直连 MySQL**，落库由 `wafadmin` 消费者完成；
2. `[waf] LUA_CONFIG` 必须是 **nginx 实际加载**的 config.lua 绝对路径；
3. redis 模式必须 `[log] INGEST_ENABLED = on`，且 Redis 口令/库号/队列键两边一致；
4. `[security] SESSION_SECRET` 必须设置，否则重启后登录态全失效；
5. 规则文件与数据库**取并集**：文件里有库里没有的新增，库里已有的保留不动。