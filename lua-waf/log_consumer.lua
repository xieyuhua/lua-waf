--[[
    异步日志消费者（ngx.timer）

    在 init_worker 阶段由 init_worker.lua 启动，每个 worker 一个 ngx.timer.every；
    通过共享内存里的 flush_lock 保证同一时刻只有一个 worker 真正落库，
    避免多 worker 并发 INSERT 造成锁竞争。

    每轮流程：
      1. 抢锁（add + 过期时间，进程异常退出后自动释放）
      2. 按模式取一批记录（内存队列 LPOP / Redis LPOP）
      3. 批量 INSERT 到 waf_log
      4. 成功计数；失败则把本批记录重新入队并计数，等待下一轮
      5. 释放锁
]]

local config = require("config")
local log_sink = require("log_sink")
local log_mysql = require("log_mysql")
local log_redis = require("log_redis")
local cjson = require("cjson.safe")

local _M = {}

-- 落库锁
local LOCK_KEY = "flush_lock"

-- 当前日志输出模式
function _M.mode()
    return config.config_log_mode or "file"
end

-- 取一批待落库记录
function _M.collect()
    local batch = tonumber(config.config_log_batch_size) or 200

    if _M.mode() == "mysql" then
        return log_sink.drain_memory(batch)
    end

    if _M.mode() == "redis" then
        local records, err = log_redis.drain(batch)
        if records == nil then
            ngx.log(ngx.WARN, "[waf] 从 Redis 获取日志失败: ", tostring(err))
            return {}
        end
        return records
    end

    return {}
end

-- 落库失败时归还本批记录
function _M.requeue(records)
    if records == nil or #records == 0 then
        return
    end

    if _M.mode() == "mysql" then
        log_sink.requeue_memory(records)
        return
    end

    for _, record in ipairs(records) do
        local payload = cjson.encode(record)
        if payload ~= nil then
            log_redis.push(payload)
        end
    end
end

-- 定时任务：批量落库
function _M.flush(premature)
    if premature then
        return
    end
    if _M.mode() == "file" then
        return
    end

    local meta = ngx.shared[log_sink.META_DICT]
    local interval = tonumber(config.config_log_flush_interval) or 2

    -- 抢锁：同一时间只允许一个 worker 落库
    if meta ~= nil then
        local locked = meta:add(LOCK_KEY, 1, interval * 3)
        if not locked then
            return
        end
    end

    local records = _M.collect()
    if records ~= nil and #records > 0 then
        local ok, err = log_mysql.insert(records)
        if ok then
            if meta ~= nil then
                meta:incr("inserted", #records, 0)
            end
        else
            ngx.log(ngx.ERR, "[waf] 批量写入 waf_log 失败: ", tostring(err))
            if meta ~= nil then
                meta:incr("failed", 1, 0)
            end
            _M.requeue(records)
        end
    end

    if meta ~= nil then
        meta:delete(LOCK_KEY)
    end
end

-- 启动消费者（init_worker 阶段调用）
function _M.start()
    if _M.mode() == "file" then
        ngx.log(ngx.INFO, "[waf] 日志输出模式: file（本地文件）")
        return
    end

    local interval = tonumber(config.config_log_flush_interval) or 2
    local ok, err = ngx.timer.every(interval, _M.flush)
    if not ok then
        ngx.log(ngx.ERR, "[waf] 启动日志落库定时器失败: ", tostring(err))
        return
    end

    ngx.log(ngx.INFO, "[waf] 日志输出模式: ", _M.mode(), "，每 ", interval, " 秒批量落库一次")
end

-- 运行统计（排障用）
function _M.stats()
    return log_sink.stats()
end

return _M
