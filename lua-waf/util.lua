--[[


]]

local io = require("io")
local cjson = require("cjson.safe")
local string = require("string")
local config = require("config")
local mysql = require "resty.mysql"
local Ip2Region = require("ip2region")

local _M = {
    version = "0.1",
    RULE_TABLE = {},
    RULE_FILES = {
        "args.rule",
        "blackip.rule",
        "cookie.rule",
        "post.rule",
        "url.rule",
        "useragent.rule",
        "whiteip.rule",
        "whiteUrl.rule"
    }
}



function _M.parse_ipinfo(ip)
    
    ip2region = nil
    ip2region_file = io.open("/www/server/nginx/x-waf/ip2region.db", "r")
    if ip2region_file then
        io.close(ip2region_file)
        ip2region = Ip2Region.new("/www/server/nginx/x-waf/ip2region.db")
    end
    
    local ipinfo = ''
    if ip2region then
        local data = ip2region:memorySearch(ip)
        local country = nil
        local region = nil
        local city = nil
        local isp = nil
        local tmp = {}
        if data and data.region then
            string.gsub(data.region, "[^|]+", function(w) table.insert(tmp, w) end )
            for key, value in pairs(tmp) do
                if key == 1 and value ~= '0' then
                    country = value
                end
                if key == 3 and value ~= '0' then
                    region = value
                end
                if key == 4 and value ~= '0' then
                    city = value
                end
                if key == 5 and value ~= '0' then
                    isp = value
                end
            end
            
            if country~=nill then
                ipinfo = ipinfo..country
            end
            if region~=nill then
                ipinfo = ipinfo..region
            end   
            if city~=nill then
                ipinfo = ipinfo..city
            end
            if isp~=nill then
                ipinfo = ipinfo..isp
            end
        end
    end
    return ipinfo
end


-- Get all rule file name
function _M.get_rule_files(rules_path)
    local rule_files = {}
    for _, file in ipairs(_M.RULE_FILES) do
        if file ~= "" then
            local file_name = rules_path .. '/' .. file
            ngx.log(ngx.DEBUG, string.format("rule key:%s, rule file name:%s", file, file_name))
            rule_files[file] = file_name
        end
    end
    return rule_files
end


-- Load WAF rules into table when on nginx's init phase
function _M.get_rules(rules_path)
    local rule_files = _M.get_rule_files(rules_path)
    if rule_files == {} then
        return nil
    end

    for rule_name, rule_file in pairs(rule_files) do
        local t_rule = {}
        local file_rule_name = io.open(rule_file)
        local json_rules = file_rule_name:read("*a")
        file_rule_name:close()
        local table_rules = cjson.decode(json_rules)
        if table_rules ~= nil then
            ngx.log(ngx.INFO, string.format("%s:%s", table_rules, type(table_rules)))
            for _, table_name in pairs(table_rules) do
                -- ngx.log(ngx.INFO, string.format("Insert table:%s, value:%s", t_rule, table_name["RuleItem"]))
                table.insert(t_rule, table_name["RuleItem"])
            end
        end
        ngx.log(ngx.INFO, string.format("rule_name:%s, value:%s", rule_name, t_rule))
        _M.RULE_TABLE[rule_name] = t_rule
    end
    return (_M.RULE_TABLE)
end

-- Get the client IP
function _M.get_client_ip()
    local CLIENT_IP = ngx.req.get_headers()["X_real_ip"]
    if CLIENT_IP == nil then
        CLIENT_IP = ngx.req.get_headers()["X_Forwarded_For"]
    end
    if CLIENT_IP == nil then
        CLIENT_IP = ngx.var.remote_addr
    end
    if CLIENT_IP == nil then
        CLIENT_IP = ""
    end
    return CLIENT_IP
end

-- Get the client user agent
function _M.get_user_agent()
    local USER_AGENT = ngx.var.http_user_agent
    if USER_AGENT == nil then
        USER_AGENT = "unknown"
    end
    return USER_AGENT
end

-- get server's host
function _M.get_server_host()
    local host = ngx.req.get_headers()["Host"]
    return host
end

-- WAF log record for json
function _M.log_record(config_log_dir, attack_type, url, data, ruletag)
    local log_path = config_log_dir
    local client_IP = _M.get_client_ip()
    local user_agent = _M.get_user_agent()
    local server_name = ngx.var.server_name
    local local_time = ngx.localtime()
    local area = _M.parse_ipinfo(client_IP)
    -- local area = client_IP
    local log_json_obj = {
        client_ip = client_IP,
        local_time = local_time,
        server_name = server_name,
        user_agent = user_agent,
        attack_type = attack_type,
        req_url = url,
        area = area,
        req_data = data,
        rule_tag = ruletag,
    }
    
    if url==nil then
        url = '-'
    end
    
    local log_line = cjson.encode(log_json_obj)
    -- local log_name = string.format("%s/%s_waf.log", log_path, ngx.today())
    local log_name = string.format("%s/%s_waf.log", log_path, "x")

    local file, err = io.open(log_name, "a+")
    if err ~= nil then ngx.log(ngx.DEBUG, "file err:" .. err) end
    if file == nil then
        return
    end

    file:write(string.format("%s\n", log_line))
    file:flush()
    file:close()
end

-- forbid ip response
function _M.forbid(flag)
    -- 如果多次请求被拦截，那么就封锁更长时间，但是也不是黑名单
    if config.config_ipcc_check == "on" then
        local IPCC_TOKEN = _M.get_client_ip()
        local limit = ngx.shared.limit
        local IPCCcount = tonumber(string.match(config.config_ipcc_rate, '(.*)/'))
        local IPCCseconds = tonumber(string.match(config.config_ipcc_rate, '/(.*)'))
        local req, _ = limit:get(IPCC_TOKEN)
        if req then
            if req > IPCCcount then
                if config.config_waf_enable == "on" then
                    -- ngx.say(IPCC_TOKEN..':'..req)
                    ngx.say('由于ip:'..IPCC_TOKEN.."非法请求次数过多，现被封禁2小时!")
                    ngx.exit(403)
                end
            else
                if flag then
                else
                    limit:incr(IPCC_TOKEN, 1)
                end
            end
        else
            limit:set(IPCC_TOKEN, 1, IPCCseconds)
        end
    end
    
end
-- WAF response
function _M.waf_output()
  -- 检查是否禁用
    _M.forbid(false)
    if config.config_waf_model == "redirect" then
        ngx.redirect(config.config_waf_redirect_url, 301)
    elseif config.config_waf_model == "jinghuashuiyue" then
        local bad_guy_ip = _M.get_client_ip()
        _M.set_bad_guys(bad_guy_ip, config.config_expire_time)
    else
        local IPCC_TOKEN = _M.get_client_ip()
        local limit = ngx.shared.limit
        local req, _ = limit:get(IPCC_TOKEN)
        local seedcode = math.random(99999999)
        local fingerprint = ngx.md5("x-waf" ..  _M.get_client_ip() .. seedcode)
        local expire = ngx.cookie_time(ngx.time() + 5 * 60)
        ngx.header['Set-Cookie'] = {
            'seedcode=' .. seedcode .. '; path=/; Expires=' .. expire,
            'fingerprint=' .. fingerprint .. '; path=/; Expires=' .. expire
        }
        ngx.header.content_type = "text/html"
        ngx.status = ngx.HTTP_FORBIDDEN
        ngx.say(string.format(config.config_output_html, _M.get_client_ip(), req))
        ngx.exit(ngx.status)
    end
end

-- set bad guys ip to ngx.shared dict
function _M.set_bad_guys(bad_guy_ip, expire_time)
    local badGuys = ngx.shared.badGuys
    local req, _ = badGuys:get(bad_guy_ip)
    if req then
        badGuys:incr(bad_guy_ip, 1)
    else
        badGuys:set(bad_guy_ip, 1, expire_time)
    end
end

return _M
