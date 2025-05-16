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

waf和waf-admin必须同时部署在每一台云WAF服务器中。


## 后端服务器管理

当多台waf做负载均衡时，只需登录其中一台进行管理即可，多台waf的所有的配置信息会自动同步到所有的服务器中。

管理地址为：http://ip:5000/login/

管理后台的默认的账户及口令分别为：admin，admin，请管理员部署系统后第一时间修改密码，防止被攻击者使用默认口令登入胡乱改动waf的配置。

### 新增站点

在`Site Manager`选项中，可以新增一个后端服务器，需要填写以下内容：

- Site Name，表示要加入waf的网站的域名
- 80表示该网站监听的端口
- Backend，表示有多少个后台app server，可以写多个（换行分割），例如：
```bash
1.1.1.1:80
8.8.8.8:80
```

- SSL Status，表示是否启用ssl，参数为on或off（如果要启用的话，需要在nginx中配置有效的证书）
- Debug Level，表示日志级别，可选的参数有`debug, info, notice, warn, error, crit, alert, emerg`

### 站点配置同步

新增站点后，需要在后台同步站点信息后方可生效，同步的方式有2种：

1. 全部同步
1. 针对某一新增的站点进行同步

## waf规则管理

在`waf Rules`选项中，可以修改waf的规则，修改完后可以点击“同步全部策略”按钮，将最新的规则同步到所有的服务器并让openresty重新加载。