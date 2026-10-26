# X-WAF

X-WAF是一款适用中、小企业的云WAF系统，让中、小企业也可以非常方便地拥有自己的免费云WAF。

优先级： 站点管理->蜘蛛ip->地区管理限制->ip白名单->ip黑名单->user-agent->白名单地址->黑名单地址->cc攻击->cookie过滤->url参数过滤->post参数过滤

```
    -- 一秒钟5次
    config_cc_rate = "5/1",
    config_ipcc_check = 'on',
    -- ip请求，超过120次，则封禁2小时
    config_ipcc_rate = "120/7200",
```

加密为二进制

/www/server/nginx/luajit/bin/luajit -b access.lua access11.lua


# 主要特性

- 支持对常见WEB攻击的防御，如sql注入、xss、路径穿越，阻断扫描器的扫描等
- 对持对CC攻击的防御
- waf为反向模式，后端保护的服务器可直接用内网IP，不需暴露在公网中
- 支持主流厂商CDN/蜘蛛IP，站点控制，基于ip2region的ip地址信息查看
- 支持IP、URL、Referer、User-Agent、Get、Post、Cookies参数型的防御策略
- 安装、部署与维护非常简单
- 支持在线管理waf规则
- 支持在线管理后端服务器
- 多台waf的配置可自动同步
- 跨平台，支持在linux、unix、mac和windows操作系统中部署

# 架构简介
x-waf由waf自身与Waf管理后台组成：

- [waf](https://github.com/xsec-lab/x-waf)：基于openresty + lua开发。
- [waf管理后台](https://github.com/xsec-lab/x-waf-admin)：采用golang + xorm + macrom开发的，支持二进制的形式部署。

waf和waf-admin必须同时部署在每一台云WAF服务器中。


## v2 新增：请求体与畸形请求防御

| 文件 | 作用 |
| --- | --- |
| `body.lua` | 请求体检测：统一读取（含 `get_body_file` 临时文件场景）、按 Content-Type 分段解析 urlencoded / multipart / json，逐段套用 `post.rule`；拦截缺失 boundary、分段数量超限、分段头 NUL/裸换行、字段名注入、上传文件名路径穿越 |
| `malformed.lua` | 畸形请求检测：TE/CL 冲突、重复 Content-Length、非单一 chunked、原始报文 NUL 与裸 CR/LF、非规范百分号编码、解码后控制字符、超长请求行、缺失/非法 Host |

开关（`config.lua`）：

```lua
config_post_check = "on",              -- 请求体检测总开关
config_post_multipart_check = "on",    -- multipart / 分段上传检测
config_post_max_size = 8388608,        -- 深度检测上限（8MB），超过只记录不阻断
config_post_max_parts = 200,           -- multipart 分段数量上限
config_malformed_check = "on",         -- 畸形请求检测总开关
```

调用顺序在 `waf.lua` 的 `_M.check()` 中：畸形请求检测 → 白名单 IP → 黑名单 IP → UA → 白名单 URL →
URL → CC → Cookie → URL 参数 → POST/请求体。


## 致谢

1. 感谢春哥开源的[openresty](https://openresty.org)
1. 感谢unixhot开源的[waf](https://github.com/unixhot/waf)
1. 感谢无闻开源的[macron](https://go-macaron.com/)和[peach](https://peachdocs.org/)
1. 感谢lunny开源的[xorm](https://github.com/go-xorm/xorm)
