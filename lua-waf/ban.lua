--[[
    自动封禁（auto ban）

    默认关闭。开启后，命中攻击（由 util.log_record 通知）会按来源 IP 计数，
    窗口内达到阈值即写入封禁表 `waf_bans`，后续请求在 waf.check() 的 ban_check 阶段直接 403，
    到期自动解封（共享内存 TTL）。

    为什么默认关闭：
      检测到攻击就永久/长期封 IP 很容易误杀 NAT、CDN 回源、办公网出口等共享 IP，
      因此这里做成「阈值 + 窗口 + 过期时间」的软封禁，并由开关控制，按业务自行开启。

    共享内存（nginx.conf）：
      lua_shared_dict waf_bans 20m;
]]

local config = require("config")

local _M = {}

-- 封禁表所在共享内存
local DICT = "waf_bans"

-- 不计入自动封禁的事件（非攻击命中，或封禁自身的记录）
local EXCLUDE = {
    ["请求体超限"] = true,
    ["IP封禁中"] = true,
    ["自动封禁"] = true
}

local warned = false

-- 取共享内存（未声明时返回 nil 并只告警一次）
local function dict()
    local shared = ngx.shared[DICT]
    if shared == nil and not warned then
        warned = true
        ngx.log(ngx.WARN, "[waf] 未声明 lua_shared_dict ", DICT, "，自动封禁不会生效")
    end
    return shared
end

-- 取客户端真实 IP（避免依赖 util，防止循环 require）
local function client_ip()
    local ip = ngx.req.get_headers()["X_real_ip"]
    if ip == nil then
        ip = ngx.req.get_headers()["X_Forwarded_For"]
    end
    if ip == nil then
        ip = ngx.var.remote_addr
    end
    if ip == nil then
        return ""
    end
    if type(ip) == "table" then
        ip = ip[1]
    end
    ip = string.gsub(tostring(ip), "%s", "")
    -- 多级代理取第一个地址
    local first = string.match(ip, "^([^,]+)")
    if first ~= nil then
        ip = first
    end
    return ip
end

-- 开关是否开启
function _M.enabled()
    return config.config_auto_ban_check == "on"
end

-- 该攻击类型是否参与封禁（config_auto_ban_types 为 "*" 或空表示全部）
local function type_allowed(attack_type)
    if attack_type == nil then
        return false
    end
    if EXCLUDE[attack_type] then
        return false
    end

    local allowed = config.config_auto_ban_types
    if allowed == nil or allowed == "" or allowed == "*" then
        return true
    end

    for item in string.gmatch(allowed, "[^,]+") do
        local name = string.gsub(item, "^%s*(.-)%s*$", "%1")
        if name ~= "" and name == attack_type then
            return true
        end
    end
    return false
end

-- 封禁某个 IP
function _M.ban(ip, seconds, reason)
    local shared = dict()
    if shared == nil or ip == nil or ip == "" then
        return false
    end

    local ttl = tonumber(seconds) or tonumber(config.config_auto_ban_expire) or 600
    local ok, err = shared:set("ban:" .. ip, 1, ttl)
    if not ok then
        ngx.log(ngx.ERR, "[waf] 写入封禁表失败: ", tostring(err))
        return false
    end

    ngx.log(ngx.WARN, "[waf] 自动封禁 ", ip, "，时长 ", ttl, " 秒，原因: ", tostring(reason))
    return true
end

-- 解除封禁
function _M.unban(ip)
    local shared = dict()
    if shared == nil or ip == nil or ip == "" then
        return false
    end
    shared:delete("ban:" .. ip)
    shared:delete("hit:" .. ip)
    return true
end

-- 当前请求的 IP 是否处于封禁中
function _M.currently_banned()
    if not _M.enabled() then
        return false
    end
    local shared = dict()
    if shared == nil then
        return false
    end
    local ip = client_ip()
    if ip == "" then
        return false
    end
    return shared:get("ban:" .. ip) ~= nil
end

-- 命中攻击时调用：计数并在达到阈值时封禁
function _M.record(attack_type)
    if not _M.enabled() then
        return
    end
    if not type_allowed(attack_type) then
        return
    end

    local shared = dict()
    if shared == nil then
        return
    end

    local ip = client_ip()
    if ip == "" then
        return
    end

    local window = tonumber(config.config_auto_ban_window) or 60
    local threshold = tonumber(config.config_auto_ban_threshold) or 1
    local key = "hit:" .. ip

    local hits = shared:get(key)
    if hits == nil then
        shared:set(key, 1, window)
        hits = 1
    else
        hits = shared:incr(key, 1) or (hits + 1)
    end

    if hits >= threshold then
        _M.ban(ip, nil, attack_type)
        -- 封禁后清空命中计数，解封后重新统计
        shared:delete(key)
    end
end

-- 当前计数与封禁状态（排障用）
function _M.stats(ip)
    local shared = dict()
    if shared == nil then
        return nil
    end
    local target = ip or client_ip()
    return {
        ip = target,
        hits = shared:get("hit:" .. target) or 0,
        banned = shared:get("ban:" .. target) ~= nil
    }
end

return _M
