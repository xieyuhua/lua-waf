--[[
    resty/redis.lua —— 零依赖的 Redis 客户端（纯 Lua，仅用 ngx.socket.tcp）

    为什么自带一份：不少 openresty 环境没有安装 lua-resty-redis（或版本过旧），
    而 WAF 的 redis 日志模式只需要极少数命令。这里实现一个“够用、可读、无 C 依赖”的客户端，
    放在 lua-waf/lib/ 下，通过 lua_package_path 引入即可：

        lua_package_path "/www/server/nginx/x-waf/lib/?.lua;/www/server/nginx/x-waf/?.lua;;";

    如果你的机器上已经装了官方 lua-resty-redis，把上面路径里的 lib 放到 $prefix 之后即可让官方版优先：
        lua_package_path "/www/server/nginx/x-waf/?.lua;/usr/local/openresty/lualib/?.lua;;";

    支持的接口（与官方保持同名同参，便于替换）：
        new / set_timeouts / set_timeout / connect / auth / select / ping / exec
        get / set / setex / incr / expire / del / exists
        rpush / lpush / lpop / rpop / llen / lrange
        close / set_keepalive / get_reused_times
]]

local tcp = ngx.socket.tcp

local _M = {}
_M._VERSION = "waf-redis 1.0"

-- 新建客户端；用法与官方一致：local redis = require("resty.redis"); local red = redis:new()
function _M.new(_, opts)
    opts = opts or {}
    return setmetatable({
        host = opts.host,
        port = opts.port or 6379,
        connect_timeout = opts.connect_timeout or 1000,
        send_timeout = opts.send_timeout or 1000,
        read_timeout = opts.read_timeout or 1000,
        sock = nil,
        reused = 0
    }, { __index = _M })
end

function _M.set_timeouts(self, connect_timeout, send_timeout, read_timeout)
    self.connect_timeout = connect_timeout or self.connect_timeout
    self.send_timeout = send_timeout or self.send_timeout
    self.read_timeout = read_timeout or self.read_timeout
end

function _M.set_timeout(self, timeout)
    self.connect_timeout = timeout
    self.send_timeout = timeout
    self.read_timeout = timeout
end

-- 编码为 RESP 数组（*N\r\n$len\r\narg\r\n...）
local function encode(args)
    local parts = { "*" .. #args .. "\r\n" }
    for _, arg in ipairs(args) do
        local value = tostring(arg)
        parts[#parts + 1] = "$" .. #value .. "\r\n" .. value .. "\r\n"
    end
    return table.concat(parts)
end

-- 读取一行（以 \r\n 结尾）
local function read_line(sock)
    local line, err = sock:receive("*l")
    if not line then
        return nil, err or "connection closed"
    end
    return line
end

-- 解析一次回复，返回 值, 错误
local function read_reply(sock)
    local line, err = read_line(sock)
    if not line then
        return nil, err
    end

    local prefix = string.sub(line, 1, 1)
    local rest = string.sub(line, 2)

    if prefix == "+" then
        return rest
    elseif prefix == "-" then
        return nil, rest
    elseif prefix == ":" then
        return tonumber(rest)
    elseif prefix == "$" then
        local length = tonumber(rest)
        if length == nil or length < 0 then
            return ngx.null
        end
        local data, data_err = sock:receive(length + 2)
        if not data then
            return nil, data_err
        end
        return string.sub(data, 1, length)
    elseif prefix == "*" then
        local count = tonumber(rest) or 0
        local items = {}
        for i = 1, count do
            local item, item_err = read_reply(sock)
            if item_err ~= nil and item == nil then
                return nil, item_err
            end
            items[i] = item
        end
        return items
    end

    return nil, "未知的 Redis 回复：" .. line
end

-- 建立连接（失败返回 nil, err）
function _M.connect(self, host, port)
    host = host or self.host
    port = port or self.port

    local sock, err = tcp()
    if not sock then
        return nil, err
    end

    sock:settimeouts(self.connect_timeout, self.send_timeout, self.read_timeout)

    local ok, connect_err = sock:connect(host, port)
    if not ok then
        return nil, "连接 Redis 失败: " .. tostring(connect_err)
    end

    self.sock = sock
    self.host = host
    self.port = port
    return 1
end

-- 发送命令并读取回复（方法名不能叫 do，do 是 Lua 关键字）
function _M.exec(self, ...)
    if not self.sock then
        return nil, "Redis 连接未建立"
    end

    local args = { ... }
    local payload = encode(args)

    local sent, send_err = self.sock:send(payload)
    if not sent then
        return nil, "发送 Redis 命令失败: " .. tostring(send_err)
    end
    return read_reply(self.sock)
end

-- 常用命令
function _M.ping(self) return self:exec("PING") end
function _M.auth(self, password) return self:exec("AUTH", password) end
function _M.select(self, db) return self:exec("SELECT", db) end
function _M.get(self, key) return self:exec("GET", key) end
function _M.set(self, key, value) return self:exec("SET", key, value) end
function _M.setex(self, key, ttl, value) return self:exec("SETEX", key, ttl, value) end
function _M.incr(self, key) return self:exec("INCR", key) end
function _M.expire(self, key, ttl) return self:exec("EXPIRE", key, ttl) end
function _M.del(self, key) return self:exec("DEL", key) end
function _M.exists(self, key) return self:exec("EXISTS", key) end
function _M.rpush(self, key, ...) return self:exec("RPUSH", key, ...) end
function _M.lpush(self, key, ...) return self:exec("LPUSH", key, ...) end
function _M.lpop(self, key) return self:exec("LPOP", key) end
function _M.rpop(self, key) return self:exec("RPOP", key) end
function _M.llen(self, key) return self:exec("LLEN", key) end

-- 区间读取（返回数组）
function _M.lrange(self, key, start, stop)
    return self:exec("LRANGE", key, start, stop)
end

-- 关闭连接
function _M.close(self)
    if self.sock then
        self.sock:close()
        self.sock = nil
    end
    return 1
end

-- 放入连接池（与官方签名一致；idle_timeout 单位毫秒，pool_size 为池大小）
function _M.set_keepalive(self, idle_timeout, pool_size)
    if not self.sock then
        return nil, "closed"
    end
    local ok, err = self.sock:setkeepalive(idle_timeout, pool_size)
    self.sock = nil
    if not ok then
        return nil, err
    end
    return 1
end

-- 当前连接是否来自连接池（这里不做区分，固定返回 0，便于调用方判断“是否新建”）
function _M.get_reused_times(self)
    return self.reused or 0
end

return _M
