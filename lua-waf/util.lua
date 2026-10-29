--[[


]]

local io = require("io")
local cjson = require("cjson.safe")
local string = require("string")
local config = require("config")
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
-- 统一交给 log_sink 处理：按 config_log_mode 决定写文件、写内存队列还是写 Redis 队列
function _M.log_record(config_log_dir, attack_type, url, data, ruletag)
    local client_IP = _M.get_client_ip()
    local user_agent = _M.get_user_agent()
    local server_name = ngx.var.server_name
    local local_time = ngx.localtime()
    local area = _M.parse_ipinfo(client_IP)

    local record = {
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

    if url == nil then
        record.req_url = '-'
    end

    -- 日志写入不得影响请求：内部失败只记录错误日志，不抛异常
    local ok, err = pcall(function()
        require("log_sink").write(record)
    end)
    if not ok then
        ngx.log(ngx.ERR, "[waf] 记录 WAF 日志失败: ", tostring(err))
    end

    -- 命中攻击后按需自动封禁来源 IP（开关 config_auto_ban_check，默认关闭）
    local ok_ban, err_ban = pcall(function()
        require("ban").record(attack_type)
    end)
    if not ok_ban then
        ngx.log(ngx.ERR, "[waf] 自动封禁处理失败: ", tostring(err_ban))
    end
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

-- 归一化可疑输入：多轮 URL 解码 + %uXXXX + HTML 实体 + 去除 SQL 注释，便于规则匹配
-- 目的：抵御双重编码、大小写/注释插入等绕过手法
function _M.normalize(text)
    if text == nil or type(text) ~= "string" or text == "" then
        return ""
    end

    local out = text

    -- 最多解码 3 轮，兼容 %252e%252e 之类的双重/三重编码
    for _ = 1, 3 do
        local decoded = ngx.unescape_uri(out)
        if decoded == nil or decoded == out then
            break
        end
        out = decoded
    end

    -- %uXXXX（IIS/旧版 Tomcat 风格）
    out = ngx.re.gsub(out, [[%u([0-9a-fA-F]{4})]], function(m)
        return string.char(tonumber(m[1], 16) % 256)
    end, "jo")

    -- HTML 实体
    out = ngx.re.gsub(out, [[&#x([0-9a-fA-F]+);]], function(m)
        return string.char(tonumber(m[1], 16) % 256)
    end, "jo")
    out = ngx.re.gsub(out, [[&#([0-9]+);]], function(m)
        return string.char(tonumber(m[1], 10) % 256)
    end, "jo")
    out = out:gsub("&lt;", "<"):gsub("&gt;", ">"):gsub("&quot;", '"'):gsub("&apos;", "'"):gsub("&amp;", "&")

    -- 去掉 SQL 注释标记，避免 /*!50000select*/ 之类绕过
    out = ngx.re.gsub(out, [[/\*![0-9]*]], "", "jo")
    out = ngx.re.gsub(out, [[/\*.*?\*/]], "", "jo")

    return out
end

-- ===== 规则匹配（带字面量预筛）=====
-- 背景：每类规则几十条，每条都跑一次 PCRE（即使有 JIT）也是浪费；
-- 绝大多数规则都包含一段「必然出现在任何匹配中」的固定字面量（如 "union select"、".git"）。
-- 这里先把规则里的字面量抽出来做一次朴素的子串判断，不命中就直接跳过正则。
--
-- 安全性：只有规则**不含**以下结构时才抽取，避免误判导致漏检：
--   分组 () / 交替 | / 字符类 [ ] / 可选 ? / 重复 * { } / 锚点 ^ $
-- 抽取到的字面量长度需 >= 3，且统一转小写后比较（与 "i" 标志一致）。

local LITERAL_CACHE = {}

-- 从规则里抽取「必须出现」的字面量，没有则返回 nil
local function required_literal(rule)
    local cached = LITERAL_CACHE[rule]
    if cached ~= nil then
        if cached == false then
            return nil
        end
        return cached
    end

    if string.find(rule, "[|%(%)%[%]%?%*{^$]", 1) then
        LITERAL_CACHE[rule] = false
        return nil
    end

    local best, run = nil, {}
    local i, n = 1, #rule
    local function flush()
        if #run >= 3 then
            local text = table.concat(run)
            if best == nil or #text > #best then
                best = text
            end
        end
        run = {}
    end

    while i <= n do
        local byte = string.sub(rule, i, i)
        if byte == "\\" then
            -- 转义：取被转义的那个字符作为字面量（如 \. -> .）
            local escaped = string.sub(rule, i + 1, i + 1)
            if escaped ~= "" and not string.find(escaped, "%w") then
                run[#run + 1] = escaped
                i = i + 2
            else
                -- \d \w \s 这类是字符类，无法确定字面量
                flush()
                i = i + 2
            end
        elseif byte == "+" then
            -- 前一个字符必须出现至少一次，但它已经算进 run 里了，这里只断开
            flush()
            i = i + 1
        else
            run[#run + 1] = byte
            i = i + 1
        end
    end
    flush()

    if best == nil then
        LITERAL_CACHE[rule] = false
        return nil
    end

    best = string.lower(best)
    LITERAL_CACHE[rule] = best
    return best
end

-- 规则匹配：先做字面量预筛，再走 PCRE（flags 通常为 "joi"）
function _M.rulematch(subject, rule, flags)
    if subject == nil or rule == nil or rule == "" then
        return nil
    end

    local literal = required_literal(rule)
    if literal ~= nil then
        if not string.find(string.lower(subject), literal, 1, true) then
            return nil
        end
    end

    return ngx.re.find(subject, rule, flags or "joi")
end

return _M
