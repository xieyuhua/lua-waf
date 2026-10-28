--[[
    请求体检测模块（POST 分段 / multipart / JSON）

    设计要点：
      1. 统一读取请求体：chunked 传输由 nginx 先解分块，此处读取解码后的完整内容；
         超过 client_body_buffer_size 时请求体会落在临时文件，这里同样会读取并检测，
         避免“大请求体或分段上传绕过 WAF”的常见问题。
      2. 按 Content-Type 分段处理：x-www-form-urlencoded / multipart/form-data / JSON / 原始体。
      3. multipart 逐段检查：缺失 boundary、分段数量超限、分段头 NUL 或裸换行、
         filename 路径穿越、name 注入等畸形特征一律拦截。
      4. 每段内容与文件名都会套用 post.rule 规则，命中即拦截。

    开关：config.config_post_check / config.config_post_multipart_check
          config.config_post_max_size / config.config_post_max_parts
]]

local config = require("config")
local util = require("util")
local cjson = require("cjson.safe")

local _M = {}

-- 单段内容在日志中的最大展示长度
local MAX_LOG_LENGTH = 200

-- 读取请求体，返回：内容, 是否来自临时文件, 实际长度
function _M.read(max_size)
    ngx.req.read_body()

    local data = ngx.req.get_body_data()
    if data ~= nil then
        local length = #data
        if length > max_size then
            return string.sub(data, 1, max_size), false, length
        end
        return data, false, length
    end

    -- 请求体已落盘（大请求体 / 分段上传）
    local file = ngx.req.get_body_file()
    if file == nil then
        return "", false, 0
    end

    local handle = io.open(file, "rb")
    if handle == nil then
        ngx.log(ngx.WARN, "[waf] 读取请求体临时文件失败: ", file)
        return "", true, 0
    end

    local content = handle:read("*a") or ""
    handle:close()

    local length = #content
    if length > max_size then
        return string.sub(content, 1, max_size), true, length
    end
    return content, true, length
end

-- 解析 multipart 分段，返回：分段列表, 错误信息
function _M.parse_multipart(raw, boundary)
    local parts = {}
    local delimiter = "--" .. boundary
    local max_parts = tonumber(config.config_post_max_parts) or 200
    local position = 1

    while true do
        local start_pos = string.find(raw, delimiter, position, true)
        if start_pos == nil then
            break
        end

        -- 结束边界 "--"
        local after = start_pos + #delimiter
        if string.sub(raw, after, after + 1) == "--" then
            break
        end

        local header_start = after + 2
        local header_end = string.find(raw, "\r\n\r\n", header_start, true)
        if header_end == nil then
            return parts, "分段缺少头部结束符"
        end

        local header_text = string.sub(raw, header_start, header_end - 1)
        local next_pos = string.find(raw, delimiter, header_end + 4, true)
        local content_end = #raw
        if next_pos ~= nil then
            content_end = next_pos - 3
        end

        parts[#parts + 1] = {
            headers = header_text,
            name = string.match(header_text, 'name="([^"]*)"'),
            filename = string.match(header_text, 'filename="([^"]*)"'),
            value = string.sub(raw, header_end + 4, content_end)
        }

        if #parts > max_parts then
            return parts, "分段数量超过上限(" .. max_parts .. ")"
        end
        if next_pos == nil then
            break
        end
        position = next_pos
    end

    return parts, nil
end

-- 检查分段头是否畸形
function _M.inspect_part(part)
    local header_text = part.headers or ""

    if string.find(header_text, "\0", 1, true) then
        return "分段头包含NUL字节", "post.multipart.nul"
    end

    local stripped = string.gsub(header_text, "\r\n", "")
    if string.find(stripped, "\r", 1, true) or string.find(stripped, "\n", 1, true) then
        return "分段头包含裸换行", "post.multipart.crlf"
    end

    -- Content-Disposition 字段名注入
    if part.name ~= nil and ngx.re.find(part.name, '[\\r\\n;]|"', "jo") then
        return "分段字段名非法: " .. part.name, "post.multipart.name"
    end

    -- 文件名路径穿越
    if part.filename ~= nil and part.filename ~= "" then
        if ngx.re.find(part.filename, "\\.\\./|\\\\|/", "jo") then
            return "上传文件名包含路径: " .. part.filename, "post.multipart.filename"
        end
    end

    return nil, nil
end

-- 用给定的 post 规则检查一段文本，返回命中的规则
function _M.match_rules(text, rules)
    if text == nil or text == "" or rules == nil then
        return nil
    end
    for _, rule in ipairs(rules) do
        if rule ~= "" and ngx.re.find(text, rule, "joi") then
            return rule
        end
    end
    return nil
end

-- 把 JSON 体展开为待检测片段：递归取出所有键与字符串值
-- 目的：拦住 {"cmd":"<编码后的 payload>"} 这类"藏在字段值里"的攻击
function _M.json_segments(raw, limit)
    local segments = {}
    local decoded = cjson.decode(raw)
    if decoded == nil then
        return segments
    end

    local function walk(node, depth)
        if #segments >= limit or depth > 8 then
            return
        end
        local kind = type(node)
        if kind == "string" then
            segments[#segments + 1] = node
        elseif kind == "table" then
            for key, value in pairs(node) do
                if #segments >= limit then
                    return
                end
                if type(key) == "string" then
                    segments[#segments + 1] = key
                end
                walk(value, depth + 1)
            end
        elseif kind == "number" or kind == "boolean" then
            segments[#segments + 1] = tostring(node)
        end
    end

    walk(decoded, 1)
    return segments
end

-- 校验上传文件后缀是否在允许清单内，返回：错误信息, 规则标识
function _M.check_upload_ext(parts)
    if config.config_upload_check ~= "on" then
        return nil, nil
    end

    local list = config.config_upload_ext_whitelist or ""
    local allowed = {}
    for ext in string.gmatch(list, "[^,%s]+") do
        allowed[string.lower(ext)] = true
    end
    if next(allowed) == nil then
        return nil, nil
    end

    for _, part in ipairs(parts) do
        local filename = part.filename
        if filename ~= nil and filename ~= "" then
            local base = string.match(filename, "([^/\\]+)$") or filename
            local ext = string.lower(string.match(base, "%.([%w]+)$") or "")
            if ext == "" or allowed[ext] == nil then
                return "上传文件类型不在允许清单: " .. base, "post.upload.ext"
            end
        end
    end
    return nil, nil
end

-- 把 urlencoded / json 体展开为待检测的片段
function _M.split_segments(raw, content_type)
    local segments = {}

    if string.find(content_type, "application/x-www-form-urlencoded", 1, true) then
        for pair in string.gmatch(raw, "[^&]+") do
            local key, value = string.match(pair, "^([^=]*)=(.*)$")
            segments[#segments + 1] = ngx.unescape_uri(pair)
            if value ~= nil and ngx.unescape_uri(value) ~= nil then
                segments[#segments + 1] = ngx.unescape_uri(value)
            end
            if key ~= nil and #key > 0 then
                segments[#segments + 1] = ngx.unescape_uri(key)
            end
        end
        return segments
    end

    -- JSON：字段级展开（递归取键与字符串值）+ 原始体兜底
    if string.find(content_type, "application/json", 1, true) or string.find(content_type, "+json", 1, true) then
        local max_segments = tonumber(config.config_post_max_parts) or 200
        local leaves = _M.json_segments(raw, max_segments)
        for _, item in ipairs(leaves) do
            segments[#segments + 1] = item
        end
        segments[#segments + 1] = raw
        return segments
    end

    -- 其他类型：整体检测，同时兜底检测转义后的内容
    segments[#segments + 1] = raw
    return segments
end

-- 入口：检测请求体，返回：命中, 攻击类型, 详情, 规则
function _M.check(post_rules)
    if config.config_post_check ~= "on" then
        return false
    end

    local max_size = tonumber(config.config_post_max_size) or 8388608
    local raw, from_file, actual_size = _M.read(max_size)
    if raw == "" then
        return false
    end

    if actual_size > max_size then
        -- 超过深度检测上限：只记录，不阻断，避免影响正常大文件上传
        util.log_record(
            config.config_log_dir,
            "请求体超限",
            ngx.var.request_uri,
            string.format("请求体 %d 字节，超过检测上限 %d 字节", actual_size, max_size),
            "post.size"
        )
    end

    local content_type = string.lower(ngx.var.content_type or ngx.var.http_content_type or "")
    local segments = {}

    -- 1. multipart 分段上传
    if string.find(content_type, "multipart/form-data", 1, true) then
        if config.config_post_multipart_check ~= "on" then
            return false
        end

        local boundary = string.match(ngx.var.http_content_type or "", 'boundary="?([^";,]+)"?')
        if boundary == nil or boundary == "" then
            return true, "分段畸形", "multipart请求缺少boundary", "post.multipart.boundary"
        end

        local parts, err = _M.parse_multipart(raw, boundary)
        if err ~= nil then
            return true, "分段畸形", err, "post.multipart.parse"
        end
        if #parts == 0 then
            return true, "分段畸形", "multipart请求无有效分段", "post.multipart.empty"
        end

        -- 上传文件类型白名单校验（config_upload_check / config_upload_ext_whitelist）
        local upload_err, upload_tag = _M.check_upload_ext(parts)
        if upload_err ~= nil then
            return true, "上传攻击", upload_err, upload_tag
        end

        for _, part in ipairs(parts) do
            local detail, tag = _M.inspect_part(part)
            if detail ~= nil then
                return true, "分段畸形", detail, tag
            end

            if part.name ~= nil then
                segments[#segments + 1] = part.name
            end
            if part.filename ~= nil and part.filename ~= "" then
                segments[#segments + 1] = part.filename
            end
            if part.value ~= nil and part.value ~= "" then
                segments[#segments + 1] = string.sub(part.value, 1, max_size)
            end
        end
    else
        -- 2. urlencoded / json / 原始体
        segments = _M.split_segments(raw, content_type)
    end

    -- 3. 逐段套用 post 规则（同时对归一化后的内容再做一次，抵御编码绕过）
    for _, segment in ipairs(segments) do
        local matched = _M.match_rules(segment, post_rules)
        local subject = segment
        if matched == nil then
            local normalized = util.normalize(segment)
            if normalized ~= segment then
                matched = _M.match_rules(normalized, post_rules)
                subject = normalized
            end
        end

        if matched ~= nil then
            local detail = string.sub(subject, 1, MAX_LOG_LENGTH)
            if from_file then
                detail = "[临时文件] " .. detail
            end
            return true, "post参数攻击", detail, matched
        end
    end

    return false
end

return _M
