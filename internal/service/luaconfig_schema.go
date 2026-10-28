package service

// 本文件是「WAF 配置」板块的**配置项声明表**（schema）。
//
// 设计要点：
//  1. 每一项对应 openresty 侧 config.lua 里的一个 `config_xxx = ...` 全局变量；
//     后台只做「按行改值」，不改结构，因此这里只声明“有哪些项、怎么校验、怎么展示”。
//  2. 声明顺序即界面顺序；同一个 Group 的项会自动归到同一个模块（前端以选项卡展示）。
//  3. Type 决定前端控件与后端校验规则（见 luaconfig_value.go 的 normalizeLuaValue）：
//     switch=开关(on/off)、number=数字、text=单行文本、select=枚举、secret=密钥（读时脱敏）。
//  4. Desc 会显示在界面字段下方，写“人话”，用于解释这项到底影响什么、什么时候需要改。

// luaFieldType 是配置项的控件/校验类型。
type luaFieldType string

const (
	// luaTypeSwitch 为开关：值只能是 "on" / "off"。
	luaTypeSwitch luaFieldType = "switch"
	// luaTypeNumber 为数字：按 Min/Max 校验，整数会去掉小数点。
	luaTypeNumber luaFieldType = "number"
	// luaTypeText 为单行文本：不支持 `[[ ]]` 多行内容。
	luaTypeText luaFieldType = "text"
	// luaTypeSelect 为枚举：取值必须在 Options 内。
	luaTypeSelect luaFieldType = "select"
	// luaTypeSecret 为密钥：读取时脱敏为 `abcd****wxyz`，提交掩码表示“不修改”。
	luaTypeSecret luaFieldType = "secret"
	// luaTypeCode 为多行内容（Lua 长字符串 `[[ ... ]]`）：用于拦截页 HTML 等模板。
	luaTypeCode luaFieldType = "code"
)

// luaField 描述一个可编辑的 config.lua 配置项。
type luaField struct {
	// Key 为 config.lua 中的变量名（必须与 lua 侧一致）。
	Key string
	// Label 为界面显示名。
	Label string
	// Type 为控件/校验类型。
	Type luaFieldType
	// Group 为所属模块（前端一个选项卡）。
	Group string
	// Desc 为字段说明，显示在控件下方。
	Desc string
	// Options 为枚举可选值（Type=select 时必填）。
	Options []string
	// Min 为数字下限（含）。
	Min float64
	// Max 为数字上限（含）；0 表示不限。
	Max float64
}

// 配置模块（选项卡）名称。顺序即前端选项卡顺序。
const (
	// groupBase 基础：总开关、拦截方式、封禁时长等全局行为。
	groupBase = "基础"
	// groupCheck 检测开关：各个检测阶段的开关。
	groupCheck = "检测开关"
	// groupRate 频率与请求体：CC/IP 频率阈值、请求体检测上限。
	groupRate = "频率与请求体"
	// groupAutoBan 自动封禁：命中攻击后的自动封禁策略。
	groupAutoBan = "自动封禁"
	// groupPath 目录：规则目录等路径配置。
	groupPath = "目录"
	// groupOutput 拦截页面：拦截页 HTML 模板（多行）。
	groupOutput = "拦截页面"
	// groupLog 日志输出：日志模式与批量参数。
	groupLog = "日志输出"
	// groupRedis Redis 队列：写 Redis 队列用的连接参数与消费方。
	groupRedis = "Redis 队列"
)

// luaFields 返回全部可编辑配置项（顺序即界面顺序）。
//
// 需要注意：这里的 Key 必须与 lua-waf/config.lua 中的变量名逐字一致，
// 否则后台会读不到值（界面会提示“配置文件缺少该项”），也无法写回。
func luaFields() []luaField {
	return []luaField{
		// ---------- 基础：全局开关与拦截动作 ----------
		{
			Key: "config_waf_enable", Label: "总开关", Type: luaTypeSwitch, Group: groupBase,
			Desc: "关闭后所有检测直接放行（连黑名单也不生效），仅用于临时排障，请谨慎关闭",
		},
		{
			Key: "config_waf_model", Label: "拦截方式", Type: luaTypeSelect, Group: groupBase,
			Options: []string{"html", "redirect", "jinghuashuiyue"},
			Desc:    "html=返回拦截页；redirect=302 跳到指定地址；jinghuashuiyue=幻影降级（返回伪装内容）",
		},
		{
			Key: "config_waf_redirect_url", Label: "跳转地址", Type: luaTypeText, Group: groupBase,
			Desc: "仅当拦截方式为 redirect 时使用，建议填本站的一个静态页",
		},
		{
			Key: "config_expire_time", Label: "封禁时长（秒）", Type: luaTypeNumber, Group: groupBase,
			Min: 0, Max: 86400, Desc: "自动封禁/黑名单命中的封禁有效期，0 表示不封禁",
		},

		// ---------- 拦截页面：拦截时返回给浏览器的 HTML（多行模板） ----------
		{
			Key: "config_output_html", Label: "拦截页 HTML 模板", Type: luaTypeCode, Group: groupOutput,
			Desc: "拦截方式为 html 时返回给浏览器的页面，支持 %s 占位（依次为 IP、请求地址、拦截次数）；不要包含 ]]，保存后建议 reload 预览",
		},

		// ---------- 检测开关：与 waf.lua 的 check() 顺序一一对应 ----------
		{
			Key: "config_white_ip_check", Label: "IP 白名单", Type: luaTypeSwitch, Group: groupCheck,
			Desc: "命中 whiteip.rule 的 IP 直接放行，跳过后续全部检测",
		},
		{
			Key: "config_black_ip_check", Label: "IP 黑名单", Type: luaTypeSwitch, Group: groupCheck,
			Desc: "命中 blackip.rule 的 IP 直接拦截",
		},
		{
			Key: "config_white_url_check", Label: "URL 白名单", Type: luaTypeSwitch, Group: groupCheck,
			Desc: "命中 whiteUrl.rule 的路径跳过 URL / 参数 / 请求体检测",
		},
		{
			Key: "config_url_check", Label: "URL 检测", Type: luaTypeSwitch, Group: groupCheck,
			Desc: "对请求路径匹配 url.rule（穿越、备份文件、CVE 探测路径等）",
		},
		{
			Key: "config_url_args_check", Label: "GET 参数检测", Type: luaTypeSwitch, Group: groupCheck,
			Desc: "对解码后的查询串匹配 args.rule（SQL 注入、命令执行、SSRF、SSTI 等）",
		},
		{
			Key: "config_user_agent_check", Label: "User-Agent 检测", Type: luaTypeSwitch, Group: groupCheck,
			Desc: "对 UA 匹配 useragent.rule（扫描器、脚本客户端）；可用 whiteua.rule 放行内部探针",
		},
		{
			Key: "config_cookie_check", Label: "Cookie 检测", Type: luaTypeSwitch, Group: groupCheck,
			Desc: "对 Cookie 匹配 cookie.rule（注入、Shiro rememberMe、Java 序列化等）",
		},
		{
			Key: "config_header_check", Label: "请求头检测", Type: luaTypeSwitch, Group: groupCheck,
			Desc: "遍历全部请求头匹配 header.rule（Host 注入、XFF 伪造、头部 JNDI、CRLF 注入等）",
		},
		{
			Key: "config_post_check", Label: "请求体检测", Type: luaTypeSwitch, Group: groupCheck,
			Desc: "对请求体匹配 post.rule；JSON 会按字段展开、表单按参数展开后逐段检测",
		},
		{
			Key: "config_post_multipart_check", Label: "分段上传检测", Type: luaTypeSwitch, Group: groupCheck,
			Desc: "multipart 逐段检测（分段头畸形、文件名穿越、段内内容）",
		},
		{
			Key: "config_upload_check", Label: "上传后缀白名单校验", Type: luaTypeSwitch, Group: groupCheck,
			Desc: "开启后上传文件后缀不在白名单内即拦截（配合下面的「允许的上传后缀」使用）",
		},
		{
			Key: "config_upload_ext_whitelist", Label: "允许的上传后缀", Type: luaTypeText, Group: groupCheck,
			Desc: "逗号分隔，例如 jpg,png,pdf,docx,zip；留空表示不限制",
		},
		{
			Key: "config_cc_check", Label: "CC 频率检测", Type: luaTypeSwitch, Group: groupCheck,
			Desc: "按「IP + URI」在时间窗口内计数，超过阈值判定 CC 攻击",
		},
		{
			Key: "config_ipcc_check", Label: "IP 频率检测", Type: luaTypeSwitch, Group: groupCheck,
			Desc: "按 IP 维度计数，适合拦截单 IP 的大范围扫描",
		},
		{
			Key: "config_malformed_check", Label: "畸形请求检测", Type: luaTypeSwitch, Group: groupCheck,
			Desc: "请求行/头非法（超长、控制字符、重复 Host、异常编码）在解析前直接拦掉",
		},

		// ---------- 频率与请求体：阈值类参数 ----------
		{
			Key: "config_cc_rate", Label: "CC 阈值", Type: luaTypeText, Group: groupRate,
			Desc: "格式 次数/秒，例如 300/60 表示 60 秒内同一 IP+URI 超过 300 次即拦截",
		},
		{
			Key: "config_ipcc_rate", Label: "IP 阈值", Type: luaTypeText, Group: groupRate,
			Desc: "格式 次数/秒，例如 120/7200 表示 2 小时内同 IP 超过 120 次即拦截",
		},
		{
			Key: "config_post_max_size", Label: "请求体检测上限（字节）", Type: luaTypeNumber, Group: groupRate,
			Min: 0, Desc: "超过该长度的请求体只记录不深检，避免影响正常大文件上传",
		},
		{
			Key: "config_post_max_parts", Label: "分段数量上限", Type: luaTypeNumber, Group: groupRate,
			Min: 0, Max: 5000, Desc: "multipart 分段数上限，超过视为畸形请求",
		},

		// ---------- 自动封禁：命中攻击后累计封禁 ----------
		{
			Key: "config_auto_ban_check", Label: "自动封禁", Type: luaTypeSwitch, Group: groupAutoBan,
			Desc: "命中攻击后按 IP 计数，达到阈值写入封禁表，后续请求在第 2 步就被拦掉",
		},
		{
			Key: "config_auto_ban_window", Label: "统计窗口（秒）", Type: luaTypeNumber, Group: groupAutoBan,
			Min: 1, Max: 3600, Desc: "在多少秒内累计命中次数",
		},
		{
			Key: "config_auto_ban_threshold", Label: "命中阈值（次）", Type: luaTypeNumber, Group: groupAutoBan,
			Min: 1, Max: 1000, Desc: "窗口内命中多少次后开始封禁",
		},
		{
			Key: "config_auto_ban_types", Label: "参与自动封禁的攻击类型", Type: luaTypeText, Group: groupAutoBan,
			Desc: "逗号分隔的攻击类型白名单，例如 sql注入,cc攻击；留空表示所有类型都参与计数",
		},
		{
			Key: "config_auto_ban_expire", Label: "封禁时长（秒）", Type: luaTypeNumber, Group: groupAutoBan,
			Min: 0, Max: 86400, Desc: "自动封禁的有效期，0 表示不封禁（仅计数）",
		},

		// ---------- 目录 ----------
		{
			Key: "config_rule_dir", Label: "规则目录", Type: luaTypeText, Group: groupPath,
			Desc: "lua 侧读取 *.rule 的目录，须与后台「系统设置 → 运行配置 → RULE_PATH」一致",
		},
		{
			Key: "config_log_dir", Label: "日志目录", Type: luaTypeText, Group: groupPath,
			Desc: "file 模式下的日志写入目录，须与后台「运行配置 → LOG_DIR」一致",
		},

		// ---------- 日志输出：只保留模式选择（落库交给后台 wafadmin 消费） ----------
		{
			Key: "config_log_mode", Label: "日志输出模式", Type: luaTypeSelect, Group: groupLog,
			Options: []string{"file", "redis"},
			Desc:    "file=写本地日志文件；redis=写 Redis 队列，由后台 wafadmin 消费入库（推荐；lua 侧不再直连 MySQL）",
		},

		// ---------- Redis 队列：写队列参数（消费在后台） ----------
		{Key: "config_redis_host", Label: "主机", Type: luaTypeText, Group: groupRedis},
		{Key: "config_redis_port", Label: "端口", Type: luaTypeNumber, Group: groupRedis, Min: 1, Max: 65535},
		{Key: "config_redis_password", Label: "密码", Type: luaTypeSecret, Group: groupRedis,
			Desc: "留空或保持脱敏值表示不修改"},
		{Key: "config_redis_db", Label: "库号", Type: luaTypeNumber, Group: groupRedis, Min: 0, Max: 255},
		{
			Key: "config_redis_list_key", Label: "队列键名", Type: luaTypeText, Group: groupRedis,
			Desc: "须与后台 [log] REDIS_LIST_KEY 一致，否则后台消费不到日志",
		},
	}
}

// luaFieldIndex 返回 Key → 字段 的索引，便于按 Key 查校验规则。
func luaFieldIndex() map[string]luaField {
	fields := luaFields()
	index := make(map[string]luaField, len(fields))
	for _, field := range fields {
		index[field.Key] = field
	}
	return index
}

// knownLuaKeys 返回受支持的配置键集合（用于识别“未纳入表单”的键）。
func knownLuaKeys() map[string]struct{} {
	fields := luaFields()
	keys := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		keys[field.Key] = struct{}{}
	}
	return keys
}
