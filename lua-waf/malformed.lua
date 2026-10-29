--[[
    畸形请求检测模块（malformed request guard）

    覆盖以下场景：
      1. Transfer-Encoding 与 Content-Length 同时出现（HTTP 请求走私 TE.CL）
      2. 重复且取值不一致的 Content-Length（请求走私 CL.CL）
      3. 非法 Transfer-Encoding（非单一 chunked，如 "chunked, chunked"）
      4. 原始报文中的 NUL 字节与裸 CR/LF（头部注入）
      5. 非规范百分号编码、二次编码（%25xx）与解码后的控制字符
      6. 超长请求行、缺失或非法的 Host 头

    说明：命中后统一由 waf.lua 记录日志并输出拦截页面。
    开关：config.config_malformed_check
]]

local config = require("config")
local util = require("util")

local _M = {}

-- 请求行最大长度（字节），超过视为畸形
local MAX_URI_LENGTH = 4096

-- 报告一次违规：block 为 false 时只记录日志不拦截
function _M.report(attack_type, detail, rule_tag, block)
    util.log_record(config.config_log_dir, attack_type, ngx.var.request_uri, detail, rule_tag)
    if block == false then
        return false
    end
    if config.config_waf_enable == "on" then
        util.waf_output()
        return true
    end
    return false
end

-- 取值统一转为字符串
local function to_text(value)
    if type(value) == "table" then
        return table.concat(value, ",")
    end
    if value == nil then
        return ""
    end
    return tostring(value)
end

-- 把 header 表统一成小写键名（保留重复头的数组形式）
local function normalize_headers(raw)
    local headers = {}
    if type(raw) ~= "table" then
        return headers
    end

    for key, value in pairs(raw) do
        headers[string.lower(key)] = value
    end
    return headers
end

-- Transfer-Encoding / Content-Length 冲突检查
function _M.header_conflict_check(headers)
    local transfer_encoding = headers["transfer-encoding"]
    local content_length = headers["content-length"]

    -- 1. TE 与 CL 同时存在：不同实现取值优先级不一致，可被用于绕过 WAF
    if transfer_encoding ~= nil and content_length ~= nil then
        return _M.report("畸形请求", "Transfer-Encoding与Content-Length同时存在", "http.smuggling.te_cl")
    end

    -- 2. 重复 Content-Length 且取值不一致
    if type(content_length) == "table" then
        local first = nil
        for _, value in ipairs(content_length) do
            local trimmed = string.gsub(tostring(value), "%s", "")
            if first == nil then
                first = trimmed
            elseif first ~= trimmed then
                return _M.report("畸形请求", "重复Content-Length取值不一致", "http.smuggling.cl_cl")
            end
        end
    end

    -- 3. Content-Length 必须为正整数（重复头时逐个校验，取值一致的重复头是合法的）
    if type(content_length) == "table" then
        for _, value in ipairs(content_length) do
            local trimmed = string.gsub(tostring(value), "%s", "")
            if trimmed ~= "" and not ngx.re.find(trimmed, "^\\d+$", "jo") then
                return _M.report("畸形请求", "Content-Length取值非法: " .. trimmed, "http.malformed.cl")
            end
        end
    else
        local content_length_text = to_text(content_length)
        if content_length_text ~= "" and not ngx.re.find(content_length_text, "^\\d+$", "jo") then
            return _M.report("畸形请求", "Content-Length取值非法: " .. content_length_text, "http.malformed.cl")
        end
    end

    -- 4. Transfer-Encoding 仅允许 chunked
    if transfer_encoding ~= nil then
        local raw = string.lower(string.gsub(to_text(transfer_encoding), "%s", ""))
        if raw ~= "chunked" then
            return _M.report("畸形请求", "非法Transfer-Encoding: " .. raw, "http.smuggling.te")
        end
    end

    return false
end

-- 原始报文检查：NUL 字节、裸 CR/LF
function _M.raw_header_check()
    local raw = ngx.req.raw_header()
    if raw == nil or raw == "" then
        return false
    end

    if string.find(raw, "\0", 1, true) then
        return _M.report("畸形请求", "请求头包含NUL字节", "http.malformed.nul")
    end

    local stripped = string.gsub(raw, "\r\n", "")
    if string.find(stripped, "\r", 1, true) or string.find(stripped, "\n", 1, true) then
        return _M.report("畸形请求", "请求头包含裸换行", "http.malformed.crlf")
    end

    return false
end

-- 请求行与 URI 检查
function _M.request_line_check()
    local uri = ngx.var.request_uri
    if uri == nil or uri == "" then
        return _M.report("畸形请求", "缺失请求行", "http.malformed.request_line")
    end

    if #uri > MAX_URI_LENGTH then
        return _M.report("畸形请求", "请求行过长(" .. #uri .. "字节)", "http.malformed.uri_len")
    end

    -- 非规范百分号编码：% 后必须跟两位十六进制
    if ngx.re.find(uri, "%(?![0-9A-Fa-f]{2})", "jo") then
        return _M.report("畸形请求", "非规范百分号编码: " .. uri, "http.malformed.percent")
    end

    -- URI 中的原始控制字符
    if ngx.re.find(uri, "[\\x00-\\x1f\\x7f]", "jo") then
        return _M.report("畸形请求", "URI包含控制字符", "http.malformed.control")
    end

    -- 解码后的控制字符（%00、%0d%0a 等），属于头部/参数注入的前置特征
    local decoded = ngx.unescape_uri(uri)
    if decoded ~= nil and ngx.re.find(decoded, "[\\x00-\\x1f\\x7f]", "jo") then
        return _M.report("畸形请求", "解码后URI包含控制字符", "http.malformed.control_decoded")
    end

    -- 二次编码（%25xx）多为绕过检测的手段，先记录日志，可由开关决定是否拦截
    if ngx.re.find(uri, "%25[0-9A-Fa-f]{2}", "jo") then
        return _M.report("畸形请求", "URI存在二次编码", "http.malformed.double_encode", false)
    end

    return false
end

-- Host 头检查
function _M.host_check(headers)
    local host = headers["host"]
    if host == nil or host == "" then
        -- 兜底：某些阶段 get_headers 取不到时，用 nginx 变量判断，避免误杀正常请求
        host = ngx.var.http_host or ngx.var.host
    end
    if host == nil or host == "" then
        return _M.report("畸形请求", "缺失Host头", "http.malformed.host")
    end

    local text = to_text(host)
    if #text > 255 or not ngx.re.find(text, "^[A-Za-z0-9\\-._:\\[\\]]+$", "jo") then
        return _M.report("畸形请求", "Host头非法: " .. text, "http.malformed.host")
    end

    return false
end

-- 入口：按顺序执行全部畸形检查
function _M.check()
    if config.config_malformed_check ~= "on" then
        return false
    end

    -- 第二个参数为 true 时保留同名头的原始取值（便于识别重复头），
    -- 但注意：raw=true 时**头名不会被转小写**（可能是 Host / Content-Length），
    -- 所以这里必须自己建一份小写键名的表，否则 headers["host"] 恒为 nil，
    -- 会把所有请求都判成「缺失Host头」—— 曾经就踩过这个坑。
    local headers = normalize_headers(ngx.req.get_headers(0, true))

    if _M.request_line_check() then return true end
    if _M.header_conflict_check(headers) then return true end
    if _M.host_check(headers) then return true end
    if _M.raw_header_check() then return true end

    return false
end

return _M
