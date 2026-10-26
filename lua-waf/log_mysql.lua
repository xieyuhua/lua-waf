--[[
    批量写入 waf_log 表（lua-resty-mysql）

    要点：
      - 一次批量 INSERT，多行 VALUES，显著降低 MySQL 压力；
      - 使用连接池（set_keepalive），避免每个批次重新握手；
      - 所有值经 ngx.quote_sql_str 转义，防止日志内容造成 SQL 注入；
      - 表结构见 lua-waf/sql/waf_log.sql。
]]

local config = require("config")
local mysql = require("resty.mysql")

local _M = {}

-- 建表语句涉及的列（顺序与 INSERT 保持一致）
local COLUMNS = "client_ip, local_time, server_name, user_agent, attack_type, req_url, area, req_data, rule_tag"

-- 建立连接（带超时与连接池）
function _M.connect()
    local db, err = mysql:new()
    if not db then
        return nil, err or "无法创建 mysql 连接"
    end

    db:set_timeouts(500, 1000, 5000)
    local ok, connect_err = db:connect({
        host = config.config_mysql_host or "127.0.0.1",
        port = tonumber(config.config_mysql_port) or 3306,
        database = config.config_mysql_database or "waf",
        user = config.config_mysql_user or "waf",
        password = config.config_mysql_password or "",
        charset = "utf8mb4"
    })
    if not ok then
        return nil, connect_err
    end
    return db
end

-- 批量插入一批记录，返回成功与否与错误信息
function _M.insert(records)
    if records == nil or #records == 0 then
        return true
    end

    local db, err = _M.connect()
    if db == nil then
        return false, err
    end

    local rows = {}
    for _, record in ipairs(records) do
        rows[#rows + 1] = string.format(
            "(%s,%s,%s,%s,%s,%s,%s,%s,%s)",
            ngx.quote_sql_str(record.client_ip or ""),
            ngx.quote_sql_str(record.local_time or ""),
            ngx.quote_sql_str(record.server_name or ""),
            ngx.quote_sql_str(record.user_agent or ""),
            ngx.quote_sql_str(record.attack_type or ""),
            ngx.quote_sql_str(record.req_url or ""),
            ngx.quote_sql_str(record.area or ""),
            ngx.quote_sql_str(record.req_data or ""),
            ngx.quote_sql_str(record.rule_tag or "")
        )
    end

    local sql = "INSERT INTO waf_log (" .. COLUMNS .. ") VALUES " .. table.concat(rows, ",")
    local result, query_err = db:query(sql)

    -- 复用连接：keepalive 时间由配置控制（毫秒），连接池大小 100
    local keepalive = tonumber(config.config_mysql_keepalive) or 60000
    db:set_keepalive(keepalive, 100)

    if not result then
        return false, query_err
    end
    return true
end

return _M
