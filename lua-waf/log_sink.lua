--[[
    日志输出统一入口（log sink）

    lua 侧只做「判定 + 输出」，两种输出模式由 config.config_log_mode 控制：

      1. file  —— 追加写本地日志文件（按天分文件，便于 rsyslog / 采集器收集）
      2. redis —— 把一条日志 JSON RPUSH 到 Redis 列表（config_redis_list_key），
                  由后台 wafadmin 的消费者批量写入 MySQL（at-least-once）

    设计约定：
      - lua 侧**不直连 MySQL**：落库统一交给后台 Go 进程（削峰 + 可观测）。
      - 请求链路绝不阻塞：写 Redis 只是发一条命令，失败也不抛异常。
      - 写 Redis 失败时回落写文件，日志不丢；同时打 WARN 便于排查
        （常见原因：缺 resty.redis、Redis 地址/口令错、Redis 不可用）。

    需要的共享内存（nginx.conf；file 模式不需要日志相关的 dict）：
      lua_shared_dict limit 100m;      -- CC / 自动封禁计数
      lua_shared_dict waf_bans 20m;    -- 自动封禁表
]]

local config = require("config")
local cjson = require("cjson.safe")

local _M = {}

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

-- 写本地日志文件（按天分文件）
function _M.write_file(record)
    local ok, err = pcall(function()
        local log_dir = config.config_log_dir
        local day = string.sub(ngx.localtime(), 1, 10)
        local path = log_dir .. day .. ".log"

        local payload = cjson.encode(record)
        if payload == nil then
            return
        end

        local file = io.open(path, "a+")
        if file == nil then
            ngx.log(ngx.ERR, "[waf] 打开日志文件失败: ", path)
            return
        end
        file:write(payload, "\n")
        file:close()
    end)
    if not ok then
        ngx.log(ngx.ERR, "[waf] 写入日志文件失败: ", tostring(err))
    end
end

-- Redis 故障时的「熔断」状态（每个 worker 各自一份，不需要共享内存）：
--   redis_fail_until —— 在这个时间点之前不再尝试写 Redis（直接写文件）
--   redis_warn_at    —— 上次打告警的时间，用于限制告警频率
-- 目的：口令错误 / Redis 宕机时，不要每个请求都去握手一次（会拖慢请求）并把 error.log 刷爆。
local REDIS_COOLDOWN = 5
local redis_fail_until = 0
local redis_warn_at = 0
local redis_skipped = 0

-- 首次写入时打印一次实际生效的模式，便于排查「配了 redis 却还在写文件」
local announced = false

local function announce(mode)
    if announced then
        return
    end
    announced = true
    ngx.log(ngx.WARN, "[waf] 日志输出模式=", tostring(mode),
        "（file=", tostring(config.config_log_dir),
        "；redis=", tostring(config.config_redis_host), ":", tostring(config.config_redis_port),
        " db=", tostring(config.config_redis_db),
        " key=", tostring(config.config_redis_list_key),
        "）落库由后台 wafadmin 消费，需开启 conf/app.ini 的 [log] INGEST_ENABLED = on")
end

-- 统一入口：按配置的日志模式写出
function _M.write(record)
    local normalized = _M.normalize(record)
    local mode = config.config_log_mode or "file"
    announce(mode)

    if mode == "redis" then
        local payload = cjson.encode(normalized)
        if payload == nil then
            _M.write_file(normalized)
            return
        end

        -- 熔断期内直接写文件，不再尝试 Redis（常见于口令错误/Redis 宕机）
        if redis_fail_until > 0 and ngx.now() < redis_fail_until then
            redis_skipped = redis_skipped + 1
            _M.write_file(normalized)
            return
        end

        -- 运行时加载：缺库时给出可读错误而不是抛异常
        local ok_load, log_redis = pcall(require, "log_redis")
        if not ok_load or log_redis == nil or type(log_redis) ~= "table" then
            ngx.log(ngx.WARN, "[waf] 加载 log_redis 失败：", tostring(log_redis), "，回落写文件")
            _M.write_file(normalized)
            return
        end

        local ok, err = log_redis.push(payload)
        if ok then
            -- 成功即解除熔断
            if redis_fail_until > 0 then
                ngx.log(ngx.WARN, "[waf] Redis 队列已恢复，结束回落写文件（期间跳过 ", redis_skipped, " 条）")
                redis_fail_until = 0
                redis_skipped = 0
            end
            return
        end

        -- 失败：进入熔断期，并按频率限制打告警（否则每个请求一条日志会刷爆 error.log）
        redis_fail_until = ngx.now() + REDIS_COOLDOWN
        if redis_warn_at == 0 or ngx.now() - redis_warn_at >= REDIS_COOLDOWN then
            redis_warn_at = ngx.now()
            ngx.log(ngx.WARN, "[waf] Redis 队列写入失败（", tostring(config.config_redis_host), ":",
                tostring(config.config_redis_port), "）：", tostring(err),
                "，日志已回落写文件；", REDIS_COOLDOWN, " 秒内不再重试 Redis（检查口令/地址/服务状态）")
        end
        _M.write_file(normalized)
        return
    end

    _M.write_file(normalized)
end

-- 读取计数器（调试用；无共享内存时返回 nil）
function _M.stats()
    local meta = ngx.shared["waf_log_meta"]
    if meta == nil then
        return nil
    end
    return {
        inserted = meta:get("inserted") or 0,
        failed = meta:get("failed") or 0
    }
end

return _M
