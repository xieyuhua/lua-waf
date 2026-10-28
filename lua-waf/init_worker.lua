--[[
    worker 初始化钩子（由 nginx.conf 的 init_worker_by_lua_file 调用）

    说明：日志落库由后台 wafadmin 消费 Redis 完成
    （conf/app.ini 的 [log] INGEST_ENABLED = on），lua 侧不再启动消费者、也不再直连 MySQL。
    这里保留钩子是为了后续需要 per-worker 初始化时（如预热缓存）直接使用。
]]

local _M = {}

-- 进程级初始化（当前无需动作）
function _M.start()
    return true
end

_M.start()

return _M