--[[
    Redis 队列（lua-resty-redis）

    生产端：access 阶段把日志 RPUSH 到 config_redis_list_key；
    消费端：ngx.timer 定时 LPOP 一批，交给 log_mysql 批量落库。

    适用于多台 WAF 汇聚到同一个 Redis 再统一入库，或需要削峰的场景。
    连接池：set_keepalive 复用连接，避免每次请求都握手。
]]

local config = require("config")
local redis = require("resty.redis")
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

-- 取出一批日志，返回已解码的记录表
function _M.drain(limit)
    local client, err = _M.connect()
    if client == nil then
        return nil, err
    end

    local records = {}
    for _ = 1, limit do
        local value, pop_err = client:lpop(_M.key())
        if value == nil or value == false then
            break
        end
        local record = cjson.decode(value)
        if record ~= nil then
            records[#records + 1] = record
        else
            -- 无法解析的脏数据直接丢弃，避免阻塞队列
            ngx.log(ngx.WARN, "[waf] Redis 中存在无法解析的日志数据")
        end
        if pop_err ~= nil then
            break
        end
    end

    client:set_keepalive(10000, 100)
    return records
end

-- 队列当前长度（排障用）
function _M.length()
    local client, err = _M.connect()
    if client == nil then
        return nil, err
    end
    local length = client:llen(_M.key())
    client:set_keepalive(10000, 100)
    return length
end

return _M
