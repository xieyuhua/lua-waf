--[[
    Redis 队列（lua-resty-redis）

    本模块只做「生产」：access/重写阶段把日志 RPUSH 到 config_redis_list_key。
    消费完全交给后台 wafadmin（[log] INGEST_ENABLED = on），lua 侧不做任何入库。

    连接池：set_keepalive 复用连接，避免每次请求都握手。
    失败语义：连接/鉴权/写入失败时返回 false + 原因，由 log_sink 决定回落到写文件。
]]

local config = require("config")
-- 运行时加载 Redis 客户端：优先用环境里的 lua-resty-redis，
-- 没有则回落到内置实现 lua-waf/lib/resty/redis.lua（需把 lib 加进 lua_package_path）
local redis = require("resty.redis")
if redis == nil then
    return {
        connect = function() return nil, "缺少 resty.redis" end,
        push = function() return false, "缺少 resty.redis" end,
        drain = function() return nil, "缺少 resty.redis" end,
        length = function() return nil, "缺少 resty.redis" end
    }
end
local cjson = require("cjson.safe")

local _M = {}

-- 建立连接（含可选密码与库号）
function _M.connect()
    local client = redis:new()
    client:set_timeouts(200, 500, 1000)

    local ok, err = client:connect(
        config.config_redis_host or "127.0.0.1",
        tonumber(config.config_redis_port) or 6379
    )
    if not ok then
        return nil, err
    end

    local password = config.config_redis_password or ""
    if password ~= "" then
        local auth_ok, auth_err = client:auth(password)
        if not auth_ok then
            client:close()
            return nil, auth_err
        end
    end

    local db = tonumber(config.config_redis_db) or 0
    if db > 0 then
        local select_ok, select_err = client:select(db)
        if not select_ok then
            client:close()
            return nil, select_err
        end
    end

    return client
end

-- 队列键
function _M.key()
    return config.config_redis_list_key or "waf:logs"
end

-- 入队一条日志（payload 为 JSON 字符串）
function _M.push(payload)
    local client, err = _M.connect()
    if client == nil then
        return false, err
    end

    local length, push_err = client:rpush(_M.key(), payload)
    client:set_keepalive(10000, 100)

    if length == nil then
        return false, push_err
    end
    return true
end

-- 队列当前长度（排障用）

return _M
