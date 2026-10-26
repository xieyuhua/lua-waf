--[[


]]
local iputils = require('iputils')
local util    = require("util")
local realip =  util.get_client_ip()
require("iop")
local cjson = require("cjson.safe")
local string = require("string")
local io = require("io")

-- 主流厂商CDN/蜘蛛IP
local in_open  = iputils.ip_in_cidrs(realip, iputils.parse_cidrs(ip_open_list))
if in_open then
    return 
end


-- 过滤ftp数据处理
local config = require("config")
if config.config_white_url_check == "on" then
    local URL_WHITE_RULES = waf.get_rule('whiteUrl.rule')
    local REQ_URI = ngx.var.request_uri
    if URL_WHITE_RULES ~= nil then
        for _, rule in pairs(URL_WHITE_RULES) do
            if rule ~= "" and string.find(REQ_URI, rule) then
                
                    local log_data = ngx.var.body_bytes_sent
                    t = {}
                    if ngx.var.http_cookie then
                        s = ngx.var.http_cookie
                        for k, v in string.gmatch(s, "(%w+_%w+_%w+_%w+)=([%w%/%.=_-]+)") do
                            t[k] = v
                        end
                    end
                    
                return true
            end
        end
    end
end


-- waf
waf.check()

