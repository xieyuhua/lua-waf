--[[
    resty/lrucache.lua —— 极简 LRU 缓存（纯 Lua，无 C 依赖）

    用途：iputils.lua 用它缓存 IP 归属地解析结果（ip2region 查询开销较大）。
    很多 openresty 环境没有 lua-resty-lrucache，这里给一份够用的替代实现，
    接口与官方保持一致：new / get / set / delete / flush_all / count / capacity。

    放到 lua-waf/lib/ 并通过 lua_package_path 引入即可：
        lua_package_path "/www/server/nginx/x-waf/lib/?.lua;/www/server/nginx/x-waf/?.lua;;";

    说明：每个 worker 各自持有一份（Lua VM 级缓存），不做跨 worker 共享，
    与官方 lua-resty-lrucache 的语义一致。
]]

local _M = {}
_M._VERSION = "waf-lrucache 1.0"

-- 新建缓存：size 为最大条目数
function _M.new(_, size)
    size = tonumber(size) or 1000
    if size <= 0 then
        size = 1000
    end

    return setmetatable({
        size = size,
        data = {},   -- key -> value
        stamp = {},  -- key -> 最近访问序号（单调递增）
        seq = 0,
        n = 0
    }, { __index = _M })
end

-- 读取（不存在返回 nil, nil）
function _M.get(self, key)
    local value = self.data[key]
    if value == nil then
        return nil, nil
    end
    self.seq = self.seq + 1
    self.stamp[key] = self.seq
    return value, nil
end

-- 淘汰最久未使用的条目
local function evict(self)
    local oldest_key, oldest = nil, nil
    for key, stamp in pairs(self.stamp) do
        if oldest == nil or stamp < oldest then
            oldest = stamp
            oldest_key = key
        end
    end
    if oldest_key ~= nil then
        self.data[oldest_key] = nil
        self.stamp[oldest_key] = nil
        self.n = self.n - 1
    end
end

-- 写入（expire 参数为兼容官方签名，这里忽略过期时间）
function _M.set(self, key, value, expire)
    if self.data[key] == nil then
        if self.n >= self.size then
            evict(self)
        end
        self.n = self.n + 1
    end
    self.data[key] = value
    self.seq = self.seq + 1
    self.stamp[key] = self.seq
    return 1
end

-- 删除
function _M.delete(self, key)
    if self.data[key] ~= nil then
        self.data[key] = nil
        self.stamp[key] = nil
        self.n = self.n - 1
    end
    return 1
end

-- 清空
function _M.flush_all(self)
    self.data = {}
    self.stamp = {}
    self.seq = 0
    self.n = 0
end

-- 当前条目数
function _M.count(self)
    return self.n
end

-- 容量
function _M.capacity(self)
    return self.size
end

return _M
