--[[
    worker 初始化钩子

    由 nginx.conf 的 init_worker_by_lua_file 调用：
    启动异步日志消费者（仅当 config_log_mode 为 mysql / redis 时生效）。
]]

local ok, err = pcall(function()
    require("log_consumer").start()
end)

if not ok then
    ngx.log(ngx.ERR, "[waf] 初始化日志消费者失败: ", tostring(err))
end
