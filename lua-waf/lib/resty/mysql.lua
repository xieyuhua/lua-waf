--[[
    resty/mysql.lua —— 占位实现（未内置 MySQL 协议客户端）

    为什么没有内置：MySQL 协议需要完整实现握手、mysql_native_password / caching_sha2_password
    认证（含 SHA1/SHA256）、结果集解析，代码量与风险都远大于 Redis；
    而 WAF 的日志入库**推荐路径是 redis + 后台 wafadmin 消费**（见 docs/OPS.md 4.1），
    lua 侧完全不需要 MySQL 客户端。

    本文件的作用是「明确报错」而不是让 nginx 抛 Lua 异常：
    当 config_log_mode = "mysql" 但环境里没有 lua-resty-mysql 时，
    log_mysql 会拿到下面这条可读的错误，log_sink 会回落到写文件（不丢日志），
    同时在 error.log 里给出修复建议。

    如果你确实需要 nginx 直连 MySQL，任选其一：
      1) 安装官方库（纯 Lua，无需编译）：
         opm get openresty/lua-resty-mysql        # 或
         luarocks install lua-resty-mysql
         然后把它的 resty/mysql.lua 放到 lua_package_path 能搜到的地方。
      2) 改用 redis 模式（推荐）：
         config.lua: config_log_mode = "redis"、config_redis_consume = "off"
         conf/app.ini: [log] INGEST_ENABLED = on
]]

local _M = {}
_M._VERSION = "waf-mysql-stub 1.0"

local MESSAGE = "未安装 lua-resty-mysql（lua 侧无法直连 MySQL）。"
    .. "建议改用 redis 模式：config_log_mode = \"redis\" + config_redis_consume = \"off\"，"
    .. "并在后台 conf/app.ini 打开 [log] INGEST_ENABLED = on；"
    .. "或安装官方库：opm get openresty/lua-resty-mysql"

function _M.new()
    return nil, MESSAGE
end

return _M
