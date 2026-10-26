--[[
    日志输出统一入口（log sink）

    两种输出模式，由 config.config_log_mode 控制：

      1. file  —— 直接追加写本地日志文件（与 1.x 行为一致，供采集器/rsyslog 收集）
      2. mysql —— 高并发异步：先写入 ngx.shared.waf_log_queue 内存队列，
                  由 ngx.timer 定时批量取出去重后 INSERT 到 MySQL
      3. redis —— 高并发异步：先 RPUSH 到 Redis 列表，由 ngx.timer 定时 LPOP
                  批量取出后 INSERT 到 MySQL（便于跨机器汇聚、削峰）

    可靠性约定：
      - 请求链路上绝不阻塞：落库全部在 timer（轻线程）中完成；
      - 队列满 / Redis 不可用 / 共享内存未声明时，自动回落到文件，绝不丢日志；
      - 落库失败会把本批次重新入队，等待下一轮重试（内存模式）或重新 push 回 Redis。

    需要的共享内存（nginx.conf）：
      lua_shared_dict waf_log_queue 20m;   -- 日志缓冲队列（lpush/rpush 需要 openresty >= 0.10.6）
      lua_shared_dict waf_log_meta   1m;   -- 计数器与落库锁
]]

local config = require("config")
local cjson = require("cjson.safe")

local _M = {}

-- 共享内存与队列键名
_M.QUEUE_DICT = "waf_log_queue"
_M.META_DICT = "waf_log_meta"
_M.QUEUE_KEY = "logs"

-- 各字段入库前的最大长度（与 waf_log 表字段长度保持一致）
local LIMITS = {
    client_ip = 64,
    local_time = 32,
    server_name = 190,
    user_agent = 512,
    attack_type = 64,
    req_url = 1024,
    area = 64,
    req_data = 1024,
    rule_tag = 190
}

-- 截断字符串，避免超长请求体撑爆字段
local function trim(value, limit)
    if value == nil then
        return ""
    end
    local text = tostring(value)
    if limit ~= nil and #text > limit then
        return string.sub(text, 1, limit)
    end
    return text
end

-- 归一化一条日志记录（字段名与 waf_log 表一致）
function _M.normalize(record)
    record = record or {}
    return {
        client_ip = trim(record.client_ip, LIMITS.client_ip),
        local_time = trim(record.local_time or ngx.localtime(), LIMITS.local_time),
        server_name = trim(record.server_name, LIMITS.server_name),
        user_agent = trim(record.user_agent, LIMITS.user_agent),
        attack_type = trim(record.attack_type, LIMITS.attack_type),
        req_url = trim(record.req_url, LIMITS.req_url),
        area = trim(record.area, LIMITS.area),
        req_data = trim(record.req_data, LIMITS.req_data),
        rule_tag = trim(record.rule_tag, LIMITS.rule_tag)
    }
end

-- 追加写本地日志文件
function _M.write_file(record)
    local log_dir = config.config_log_dir
    local path = string.format("%s/%s_waf.log", log_dir, "x")
    local ok, err = pcall(function()
        local file = io.open(path, "a+")
        if file == nil then
            return
        end
        file:write(cjson.encode(record) or "", "\n")
        file:flush()
        file:close()
    end)
    if not ok then
        ngx.log(ngx.ERR, "[waf] 写入日志文件失败: ", tostring(err))
    end
end

-- 放入共享内存队列；容量不足时返回 false
function _M.push_memory(payload)
    local queue = ngx.shared[_M.QUEUE_DICT]
    if queue == nil then
        return false, "共享内存 " .. _M.QUEUE_DICT .. " 未声明"
    end

    local max_size = tonumber(config.config_log_queue_size) or 20000
    local length = queue:llen(_M.QUEUE_KEY) or 0
    if length >= max_size then
        local meta = ngx.shared[_M.META_DICT]
        if meta ~= nil then
            meta:incr("dropped", 1, 0)
        end
        return false, "队列已满"
    end

    local ok, err = queue:rpush(_M.QUEUE_KEY, payload)
    if not ok then
        local meta = ngx.shared[_M.META_DICT]
        if meta ~= nil then
            meta:incr("dropped", 1, 0)
        end
        return false, err
    end
    return true
end

-- 从共享内存队列取出一批（返回已解码的记录表）
function _M.drain_memory(limit)
    local queue = ngx.shared[_M.QUEUE_DICT]
    local records = {}
    if queue == nil then
        return records
    end

    for _ = 1, limit do
        local payload = queue:lpop(_M.QUEUE_KEY)
        if payload == nil then
            break
        end
        local record = cjson.decode(payload)
        if record ~= nil then
            records[#records + 1] = record
        else
            local meta = ngx.shared[_M.META_DICT]
            if meta ~= nil then
                meta:incr("dropped", 1, 0)
            end
        end
    end
    return records
end

-- 落库失败时把本批记录放回队列
function _M.requeue_memory(records)
    local queue = ngx.shared[_M.QUEUE_DICT]
    if queue == nil then
        return
    end
    for _, record in ipairs(records) do
        local payload = cjson.encode(record)
        if payload ~= nil then
            queue:rpush(_M.QUEUE_KEY, payload)
        end
    end
end

-- 统一入口：按配置的日志模式写出
function _M.write(record)
    local normalized = _M.normalize(record)
    local mode = config.config_log_mode or "file"

    if mode == "mysql" then
        local payload = cjson.encode(normalized)
        if payload == nil then
            _M.write_file(normalized)
            return
        end
        local ok, err = _M.push_memory(payload)
        if ok then
            return
        end
        ngx.log(ngx.WARN, "[waf] 内存队列写入失败（", tostring(err), "），回落写文件")
        _M.write_file(normalized)
        return
    end

    if mode == "redis" then
        local payload = cjson.encode(normalized)
        if payload == nil then
            _M.write_file(normalized)
            return
        end
        local ok, err = require("log_redis").push(payload)
        if ok then
            return
        end
        ngx.log(ngx.WARN, "[waf] Redis 队列写入失败（", tostring(err), "），回落写文件")
        _M.write_file(normalized)
        return
    end

    _M.write_file(normalized)
end

-- 读取计数器（供调试接口或日志使用）
function _M.stats()
    local meta = ngx.shared[_M.META_DICT]
    local queue = ngx.shared[_M.QUEUE_DICT]
    if meta == nil then
        return nil
    end
    return {
        inserted = meta:get("inserted") or 0,
        failed = meta:get("failed") or 0,
        dropped = meta:get("dropped") or 0,
        pending = queue and queue:llen(_M.QUEUE_KEY) or 0
    }
end

return _M
