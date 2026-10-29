--[[

]]

local rulematch = ngx.re.find
local unescape = ngx.unescape_uri

local config = require("config")
local util = require("util")
local body = require("body")
local malformed = require("malformed")
-- 按 URL 的精细化限频（rules/ratelimit.rule 由后台下发；缺文件时静默跳过）
local ok_ratelimit, ratelimit = pcall(require, "ratelimit")
if not ok_ratelimit then
    ratelimit = nil
end
local ban = require("ban")

local _M = {
    RULES = {}
}

function _M.load_rules()
    _M.RULES = util.get_rules(config.config_rule_dir)
    for k, v in pairs(_M.RULES)
    do
        ngx.log(ngx.INFO, string.format("%s Rule Set", k))
        for kk, vv in pairs(v)
        do
            ngx.log(ngx.INFO, string.format("index:%s, Rule:%s", kk, vv))
        end
    end
    return _M.RULES
end

function _M.get_rule(rule_file_name)
    return _M.RULES[rule_file_name]
end

-- allow white ip check
function _M.white_ip_check()
    
    if config.config_white_ip_check == "on" then
        local IP_WHITE_RULE = _M.get_rule('whiteip.rule')
        local WHITE_IP = util.get_client_ip()
        if IP_WHITE_RULE ~= nil then
            for _, rule in pairs(IP_WHITE_RULE) do
                if rule ~= "" and rulematch(WHITE_IP, rule, "joi") then
                    return true
                end
            end
        end
    end
    
 -- 检查是否禁用
    util.forbid(true)
    return false
end

-- Bad guys check
function _M.bad_guy_check()
    local client_ip = util.get_client_ip()
    local ret = false
    if client_ip ~= "" then
        ret = ngx.shared.badGuys.get(client_ip)
        if ret ~= nil and ret > 0 then
            ret = true
        end
    end
    return ret
end


-- deny black ip
function _M.black_ip_check()
    if config.config_black_ip_check == "on" then
        local IP_BLACK_RULE = _M.get_rule('blackip.rule')
        local BLACK_IP = util.get_client_ip()
        -- BLACK_IP = '106.91.161.188'
        if IP_BLACK_RULE ~= nil then
            for _, rule in pairs(IP_BLACK_RULE) do
                if rule ~= "" and rulematch(BLACK_IP, rule, "joi") then
                    -- util.log_record(config.config_log_dir, '黑名单ip', ngx.var_request_uri, "_", "_")
                    if config.config_waf_enable == "on" then
                        ngx.exit(403)
                        return true
                    end
                end
            end
        end
    end
    return false
end

-- allow white url
function _M.white_url_check()
    if config.config_white_url_check == "on" then
        local URL_WHITE_RULES = _M.get_rule('whiteUrl.rule')
        local REQ_URI = ngx.var.request_uri
        if URL_WHITE_RULES ~= nil then
            for _, rule in pairs(URL_WHITE_RULES) do
                if rule ~= "" and (rulematch(REQ_URI, rule, "joi") or rulematch(util.normalize(REQ_URI), rule, "joi")) then
                    return true
                end
            end
        end
    end
    return false
end

-- 按 URL 的精细化限频（规则来自 rules/ratelimit.rule）
function _M.ratelimit_check()
    if ratelimit == nil then
        return false
    end

    local hit, attack_type, detail, rule_tag = ratelimit.check()
    if hit then
        util.log_record(config.config_log_dir, attack_type, ngx.var.request_uri, detail, rule_tag)
        if config.config_waf_enable == "on" then
            util.waf_output()
            return true
        end
    end
    return false
end

-- deny cc attack
function _M.cc_attack_check()
    if config.config_cc_check == "on" then
        local ATTACK_URI = ngx.var.uri
        local CC_TOKEN = util.get_client_ip() .. ATTACK_URI
        local limit = ngx.shared.limit
        local CCcount = tonumber(string.match(config.config_cc_rate, '(.*)/'))
        local CCseconds = tonumber(string.match(config.config_cc_rate, '/(.*)'))
        local req, _ = limit:get(CC_TOKEN)
        if req then
            if req > CCcount then
                util.log_record(config.config_log_dir, 'cc攻击', ngx.var.request_uri, "-", "-")
                if config.config_waf_enable == "on" then
                    ngx.exit(403)
                end
            else
                limit:incr(CC_TOKEN, 1)
            end
        else
            limit:set(CC_TOKEN, 1, CCseconds)
        end
    end
    return false
end

-- UA 白名单：命中的 UA 不做扫描器/脚本客户端检测（用于内部拨测、监控探针）
function _M.white_ua_check()
    local WHITE_UA_RULES = _M.get_rule('whiteua.rule')
    if WHITE_UA_RULES == nil then
        return false
    end

    local UA = ngx.var.http_user_agent
    if UA == nil or UA == "" then
        return false
    end

    for _, rule in pairs(WHITE_UA_RULES) do
        if rule ~= "" and rulematch(UA, rule, "joi") then
            return true
        end
    end
    return false
end

-- deny header attack（Host 注入 / 代理头伪造 / 头部 JNDI / 头内注入）
function _M.header_attack_check()
    if config.config_header_check == "on" then
        local HEADER_RULES = _M.get_rule('header.rule')
        if HEADER_RULES == nil then
            return false
        end

        local headers = ngx.req.get_headers(0, true)
        for name, value in pairs(headers) do
            local values = value
            if type(value) ~= "table" then
                values = { value }
            end
            for _, single in pairs(values) do
                local line = name .. ": " .. tostring(single)
                local normalized = util.normalize(line)
                for _, rule in pairs(HEADER_RULES) do
                    if rule ~= "" and (rulematch(line, rule, "joi") or rulematch(normalized, rule, "joi")) then
                        util.log_record(config.config_log_dir, '请求头攻击', ngx.var.request_uri, name, rule)
                        if config.config_waf_enable == "on" then
                            util.waf_output()
                            return true
                        end
                    end
                end
            end
        end
    end
    return false
end
-- deny cookie
function _M.cookie_attack_check()
    if config.config_cookie_check == "on" then
        local COOKIE_RULES = _M.get_rule('cookie.rule')
        local USER_COOKIE = ngx.var.http_cookie
        if USER_COOKIE ~= nil then
            for _, rule in pairs(COOKIE_RULES) do
                if rule ~= "" and (rulematch(USER_COOKIE, rule, "joi") or rulematch(util.normalize(USER_COOKIE), rule, "joi")) then
                    util.log_record(config.config_log_dir, 'Cookie攻击', ngx.var.request_uri, "-", rule)
                    if config.config_waf_enable == "on" then
                        util.waf_output()
                        return true
                    end
                end
            end
        end
    end
    return false
end

-- deny url
function _M.url_attack_check()
    if config.config_url_check == "on" then
        local URL_RULES = _M.get_rule('url.rule')
        local REQ_URI = ngx.var.request_uri
        for _, rule in pairs(URL_RULES) do
            if rule ~= "" and (rulematch(REQ_URI, rule, "joi") or rulematch(util.normalize(REQ_URI), rule, "joi")) then
                util.log_record(config.config_log_dir, '黑名单url', REQ_URI, "-", rule)
                if config.config_waf_enable == "on" then
                    util.waf_output()
                    return true
                end
            end
        end
    end
    return false
end

-- deny url args
function _M.url_args_attack_check()
    if config.config_url_args_check == "on" then
        local ARGS_RULES = _M.get_rule('args.rule')
        for _, rule in pairs(ARGS_RULES) do
            local REQ_ARGS = ngx.req.get_uri_args()
            for key, val in pairs(REQ_ARGS) do
                local ARGS_DATA = {}
                if type(val) == 'table' then
                    ARGS_DATA = table.concat(val, " ")
                else
                    ARGS_DATA = val
                end
                
                if ARGS_DATA and type(ARGS_DATA) ~= "boolean" and rule ~= "" and rulematch(util.normalize(unescape(ARGS_DATA)), rule, "joi") then
                    util.log_record(config.config_log_dir, 'get参数攻击', ngx.var.request_uri, "-", rule)
                    if config.config_waf_enable == "on" then
                        util.waf_output()
                        return true
                    end
                end
            end
        end
    end
    return false
end

-- deny user agent
function _M.user_agent_attack_check()
    if config.config_user_agent_check == "on" then
        -- UA 白名单优先：内部拨测/监控探针可在此放行（rules/whiteua.rule）
        if _M.white_ua_check() then
            return false
        end
        local USER_AGENT_RULES = _M.get_rule('useragent.rule')
        local USER_AGENT = ngx.var.http_user_agent
        if USER_AGENT ~= nil then
            for _, rule in pairs(USER_AGENT_RULES) do
                if rule ~= "" and rulematch(USER_AGENT, rule, "joi") then
                    util.log_record(config.config_log_dir, 'user-agent攻击', ngx.var.request_uri, "-", rule)
                    if config.config_waf_enable == "on" then
                        util.waf_output()
                        return true
                    end
                end
            end
        end
    end
    return false
end

-- 自动封禁检查：命中封禁表直接拒绝（封禁由 ban.record 在攻击命中时写入）
function _M.ban_check()
    if config.config_auto_ban_check ~= "on" then
        return false
    end
    if not ban.currently_banned() then
        return false
    end

    util.log_record(config.config_log_dir, 'IP封禁中', ngx.var.request_uri, "-", "auto_ban")
    if config.config_waf_enable == "on" then
        local expire = tonumber(config.config_auto_ban_expire) or 600
        ngx.status = ngx.HTTP_FORBIDDEN
        ngx.header.content_type = "text/html"
        ngx.say(string.format(config.config_output_html, util.get_client_ip(), expire))
        ngx.exit(ngx.HTTP_FORBIDDEN)
    end
    return false
end

-- deny malformed request (HTTP 走私 / 头部注入 / 非法编码)
function _M.malformed_request_check()
    if config.config_malformed_check ~= "on" then
        return false
    end
    -- malformed.check 内部已完成日志记录与拦截输出
    return malformed.check()
end

-- deny post（含 urlencoded / multipart 分段 / json / 大请求体落盘场景）
function _M.post_attack_check()
    if config.config_post_check ~= "on" then
        return false
    end

    local hit, attack_type, detail, rule_tag = body.check(_M.get_rule('post.rule'))
    if hit then
        util.log_record(config.config_log_dir, attack_type, ngx.var.request_uri, detail, rule_tag)
        if config.config_waf_enable == "on" then
            util.waf_output()
            return true
        end
    end
    return false
end

-- start change to jinghuashuiyue mode, set in vhosts's location segument
function _M.start_jingshuishuiyue()
    local host = util.get_server_host()
    ngx.var.target = string.format("proxy_%s", host)
    if host and _M.bad_guy_check() then
        ngx.var.target = string.format("unreal_%s", host)
    end
end

-- waf start
function _M.check()
    if _M.white_ip_check() then
    elseif _M.ban_check() then
    elseif _M.malformed_request_check() then
    elseif _M.black_ip_check() then
    elseif _M.header_attack_check() then
    elseif _M.user_agent_attack_check() then
    elseif _M.white_url_check() then
    elseif _M.url_attack_check() then
    elseif _M.ratelimit_check() then
    elseif _M.cc_attack_check() then
    elseif _M.cookie_attack_check() then
    elseif _M.url_args_attack_check() then
    elseif _M.post_attack_check() then
    else
        return
    end
end

return _M
