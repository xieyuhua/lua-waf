--[[

]]

-- WAF config file, enable = "on", disable = "off"

local _M = {
    -- waf status
    config_waf_enable = "on",
    -- 日志目录（file 模式写这里，按天分文件；redis 模式下不使用）
    config_log_dir = "/www/wwwlogs/waf/",
    -- 日志输出模式（只支持两种）：
    --   file  = 追加写本地日志文件，由 rsyslog / 采集器收集
    --   redis = 把日志 JSON RPUSH 到 Redis 队列，由后台 wafadmin 消费并批量写入 MySQL（推荐）
    config_log_mode = "redis",

    -- Redis（redis 模式必填；后台 conf/app.ini 的 [log] REDIS_ADDR / REDIS_LIST_KEY 必须与此一致）
    config_redis_host = "127.0.0.1",
    config_redis_port = 6379,
    config_redis_password = "",
    config_redis_db = 0,
    config_redis_list_key = "waf:logs",

    -- rule setting
    config_rule_dir = "/www/server/nginx/x-waf/rules",
    -- enable/disable white url
    config_white_url_check = "on",
    -- enable/disable white ip
    config_white_ip_check = "on",
    -- enable/disable block ip
    config_black_ip_check = "on",
    -- enable/disable url filtering
    config_url_check = "on",
    -- enalbe/disable url args filtering
    config_url_args_check = "on",
    -- enable/disable user agent filtering
    config_user_agent_check = "on",
    -- enable/disable cookie deny filtering
    config_cookie_check = "on",

-- 请求头检测（Host 注入 / 代理头伪造 / 头部 JNDI / 头内注入），规则文件 header.rule
config_header_check = "on",
    -- enable/disable cc filtering
    config_cc_check = "on",
    -- cc rate the xxx of xxx seconds
    config_cc_rate = "300/60",
    -- enable/disable cc filtering
    config_ipcc_check = 'on',
    -- ipcc rate the xxx of xxx seconds，#########ip请求，100次，否则就封禁2小时
    config_ipcc_rate = "120/7200",
    -- enable/disable post filtering
    config_post_check = "on",
    -- enable/disable multipart / 分段上传检测（逐段检查字段名、文件名与内容）
    config_post_multipart_check = "on",
    -- 请求体深度检测上限（字节），超过上限只记录日志不做深度扫描，避免影响大文件上传
    config_post_max_size = 8388608,
    -- multipart 分段数量上限，超过视为异常分段
    config_post_max_parts = 200,

-- 上传文件后缀白名单校验（config_upload_check = "on" 时生效）
-- 允许清单用逗号分隔；不在清单内的上传直接拦截（可用于阻止 webshell 上传）
config_upload_check = "off",
config_upload_ext_whitelist = "jpg,jpeg,png,gif,webp,bmp,pdf,doc,docx,xls,xlsx,ppt,pptx,txt,csv,zip,rar,7z,mp4,mp3,wav",
    -- enable/disable 畸形请求检测（TE/CL 走私、重复 Content-Length、裸 CRLF、非法编码等）
    config_malformed_check = "on",
    -- 检测到攻击是否自动封禁来源 IP（默认关闭：NAT / CDN 回源等共享 IP 场景易误杀）
    config_auto_ban_check = "off",
    -- 封禁时长（秒）
    config_auto_ban_expire = 600,
    -- 统计窗口（秒）与窗口内触发封禁的命中次数
    config_auto_ban_window = 60,
    config_auto_ban_threshold = 1,
    -- 参与自动封禁的攻击类型，逗号分隔；"*" 表示除内置排除项外全部参与
    config_auto_ban_types = "*",
    -- config waf output redirect/html/jinghuashuiyue
    config_waf_model = "html",
    -- if config_waf_output ,setting url
    config_waf_redirect_url = "",
    config_expire_time = 600,
    config_output_html = [[
    <html>
    <head>
    <meta charset="UTF-8">
    <title>网站防火墙</title>
    </head>
      <body>
        <div>
      <div class="table">
        <div>
          <div class="cell">
            您的IP为: %s
          </div>
          <div class="cell">
             <p class="t1">您的请求带有不合法参数，已被网站管理员设置拦截%s次！</p>
          </div>
        </div>
      </div>
    </div>
      </body>
    </html>
    ]],

}

return _M
