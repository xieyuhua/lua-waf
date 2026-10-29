--[[
    按 URL 的精细化限频（ratelimit）

    与全局 CC/IP 频率的区别：
      - 全局（config_cc_check / config_ipcc_check）只有一组阈值，按「IP+URI」或「IP」计数；
      - 本模块按**规则**限频：每条规则 = URL 匹配 + 计数维度（IP / Header / Cookie / 参数 / URI）+ 阈值 + 窗口。

    规则来源：$config_rule_dir/ratelimit.rule（JSON 数组，由后台「防护配置 → 限频规则」下发），例如：

        [
          {"id":1,"name":"登录接口","pattern":"/api/login","match":"prefix",
           "key":"ip","key_name":"","limit":10,"window":60,"action":"block","enabled":true},
          {"id":2,"name":"开放API","pattern":"^/open/api/","match":"regex",
           "key":"header","key_name":"X-API-Key","limit":1000,"window":60,"action":"block","enabled":true}
        ]

    计数使用 ngx.shared.limit（与 CC 共用字典；键前缀 rl:<规则ID>:<维度值>，TTL = window 秒）。

    说明：
      - 规则按顺序匹配，**命中第一条**就生效（所以把更具体的 URL 放前面）；
      - action = "block" 命中即拦截；action = "log" 只记日志不拦（用于先观察误伤）；
      - 规则文件为空或读取失败时不影响其它检测。
]]

local config = require("config")
local cjson = require("cjson.safe")

local _M = {}

-- 共享字典名（与 nginx.conf 里的 lua_shared_dict limit 一致）
_M.DICT = "limit"

-- 规则缓存：文件未变化时复用（按 mtime + 大小判断）
local cache = {
    rules = nil,
    path = nil,
    mtime = nil,
    size = nil
}

-- 读取规则文件（带 mtime 缓存）
local function load_rules()
    local path = (config.config_rule_dir or "") .. "ratelimit.rule"
    local file = io.open(path, "rb")
    if file == nil then
        return cache.rules or {}
    end

    -- LuaJIT 里不能把方法引用当值用，这里显式判断
    local mtime = nil
    if type(file.getlastmodified) == "function" then
        mtime = file:getlastmodified()
    end
    local content = file:read("*a")
    file:close()

    local size = #(content or "")
    if cache.path == path and cache.mtime == mtime and cache.size == size then
        return cache.rules or {}
    end

    local decoded = cjson.decode(content or "")
    if type(decoded) ~= "table" then
        cache.path, cache.mtime, cache.size, cache.rules = path, mtime, size, {}
        return {}
    end

    local rules = {}
    for _, item in ipairs(decoded) do
        if type(item) == "table" and item.enabled ~= false then
            rules[#rules + 1] = item
        end
    end

    cache.path, cache.mtime, cache.size, cache.rules = path, mtime, size, rules
    return rules
end

-- URL 匹配：prefix（前缀）/ regex（正则，退化为子串匹配）
local function match_url(uri, rule)
    local pattern = rule.pattern or ""
    if pattern == "" then
        return false
    end

    if (rule.match or "prefix") == "regex" then
        if ngx.re and ngx.re.find then
            return ngx.re.find(uri, pattern, "joi") ~= nil
        end
        -- 没有 ngx.re 时退化为子串匹配，避免因为正则引擎问题漏检
        return string.find(uri, pattern, 1, true) ~= nil
    end

    return string.sub(uri, 1, #pattern) == pattern
end

-- 解析 Cookie 中的某个键
local function cookie_value(name)
    local raw = ngx.var.http_cookie
    if raw == nil or name == nil or name == "" then
        return nil
    end
    for pair in string.gmatch(raw, "[^;]+") do
        local key, value = string.match(pair, "^%s*([^=]+)=(.*)$")
        if key ~= nil and key == name then
            return value
        end
    end
    return nil
end

-- 按维度取计数值
local function resolve_key(rule, uri)
    local kind = rule.key or "ip"

    if kind == "ip" then
        local util = require("util")
        return util.get_client_ip()
    elseif kind == "uri" then
        return uri
    elseif kind == "header" then
        local name = string.lower(rule.key_name or "")
        if name == "" then
            return nil
        end
        return ngx.var["http_" .. string.gsub(name, "-", "_")]
    elseif kind == "cookie" then
        return cookie_value(rule.key_name)
    elseif kind == "arg" then
        local name = rule.key_name or ""
        if name == "" then
            return nil
        end
        return ngx.var["arg_" .. name]
    end

    return nil
end

-- 主入口：返回 命中(bool), 攻击类型, 详情, 规则标识
function _M.check()
    local rules = load_rules()
    if #rules == 0 then
        return false
    end

    local dict = ngx.shared[_M.DICT]
    if dict == nil then
        -- 未声明 lua_shared_dict limit 时无法计数，直接跳过（不影响其它检测）
        return false
    end

    local uri = ngx.var.uri or ngx.var.request_uri or "/"

    for _, rule in ipairs(rules) do
        if match_url(uri, rule) then
            local value = resolve_key(rule, uri)
            if value ~= nil and value ~= "" then
                local limit = tonumber(rule.limit) or 0
                local window = tonumber(rule.window) or 60
                if limit > 0 then
                    local key = "rl:" .. tostring(rule.id or 0) .. ":" .. tostring(value)
                    local count, err = dict:incr(key, 1, 0, window)

                    if count == nil then
                        -- 字典写入失败（极少见）：记录一次日志后放行
                        ngx.log(ngx.WARN, "[waf] 限频计数失败: ", tostring(err), " key=", key)
                    elseif count > limit then
                        local detail = string.format("%s=%s 在 %ds 内请求 %d 次，超过阈值 %d（规则：%s）",
                            tostring(rule.key), tostring(value), window, count, limit, tostring(rule.name or rule.id))
                        local tag = "ratelimit:" .. tostring(rule.id or 0)

                        if (rule.action or "block") == "log" then
                            -- 只记录不拦截
                            require("util").log_record(config.config_log_dir, "限频观察", ngx.var.request_uri, detail, tag)
                            return false
                        end
                        return true, "限频攻击", detail, tag
                    end
                end
            end

            -- 命中规则即结束（第一条匹配的规则生效）
            return false
        end
    end

    return false
end

return _M
