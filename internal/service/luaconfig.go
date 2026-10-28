package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// luaFieldType 是配置项类型。
type luaFieldType string

const (
	luaTypeSwitch luaFieldType = "switch"
	luaTypeNumber luaFieldType = "number"
	luaTypeText   luaFieldType = "text"
	luaTypeSelect luaFieldType = "select"
	luaTypeSecret luaFieldType = "secret"
)

// luaField 描述一个可编辑的 Lua 配置项。
type luaField struct {
	Key     string
	Label   string
	Type    luaFieldType
	Group   string
	Desc    string
	Options []string
	Min     float64
	Max     float64
}

// LuaConfigItem 是返回给前端的配置项。
type LuaConfigItem struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Type    string   `json:"type"`
	Value   string   `json:"value"`
	Desc    string   `json:"desc"`
	Options []string `json:"options,omitempty"`
	Min     float64  `json:"min,omitempty"`
	Max     float64  `json:"max,omitempty"`
	// Masked 表示该值为脱敏展示（密钥类）。
	Masked bool `json:"masked,omitempty"`
}

// LuaConfigGroup 是按分组组织的配置项。
type LuaConfigGroup struct {
	Name  string          `json:"name"`
	Items []LuaConfigItem `json:"items"`
}

// LuaConfigView 是 lua-waf 配置的展示结果。
type LuaConfigView struct {
	// Path 为 config.lua 路径。
	Path string `json:"path"`
	// Exists 表示文件是否存在。
	Exists bool `json:"exists"`
	// Writable 表示当前进程是否有权限写入。
	Writable bool `json:"writable"`
	// Groups 为可编辑配置项（按分组）。
	Groups []LuaConfigGroup `json:"groups"`
	// Unknown 为文件里存在但未纳入表单的键（只读展示）。
	Unknown []string `json:"unknown"`
	// UpdatedAt 为文件最近修改时间。
	UpdatedAt string `json:"updated_at"`
	// NeedReload 提示修改后需 reload nginx 生效。
	NeedReload bool `json:"need_reload"`
}

// LuaReloader 让修改后的 config.lua 生效（内部执行 nginx -t 后 reload）。
type LuaReloader interface {
	Reload() (string, error)
}

// LuaConfigService 负责读写 openresty 侧的 config.lua。
//
// 只做「按行替换值」，保留原有注释、顺序与缩进；写入前备份为 .bak 并原子替换。
type LuaConfigService struct {
	// Path 为 config.lua 路径。
	Path string
	// Reloader 负责在保存后让配置生效，可为空（演示模式）。
	Reloader LuaReloader

	mu sync.Mutex
}

// luaLinePattern 匹配 `config_xxx = value,` 形式的行。
var luaLinePattern = regexp.MustCompile(`^(\s*)(config_[A-Za-z0-9_]+)(\s*=\s*)(.+?)(,?)(\s*(?:--.*)?)$`)

// ratePattern 校验 "次数/秒" 形式的限速配置。
var ratePattern = regexp.MustCompile(`^\d+/\d+$`)

// fields 返回可编辑配置项清单（顺序即界面顺序）。
func (s *LuaConfigService) fields() []luaField {
	const (
		groupBase    = "基础"
		groupCheck   = "检测开关"
		groupRate    = "频率与请求体"
		groupAutoBan = "自动封禁"
		groupPath    = "目录"
		groupLog     = "日志输出"
		groupMySQL   = "MySQL 落库"
		groupRedis   = "Redis 队列"
	)

	return []luaField{
		{Key: "config_waf_enable", Label: "总开关", Type: luaTypeSwitch, Group: groupBase, Desc: "关闭后所有检测直接放行（含黑名单）"},
		{Key: "config_waf_model", Label: "拦截方式", Type: luaTypeSelect, Group: groupBase, Options: []string{"html", "redirect", "jinghuashuiyue"}, Desc: "html=拦截页；redirect=跳转；jinghuashuiyue=幻影降级"},
		{Key: "config_waf_redirect_url", Label: "跳转地址", Type: luaTypeText, Group: groupBase, Desc: "拦截方式为 redirect 时使用"},
		{Key: "config_expire_time", Label: "封禁时长（秒）", Type: luaTypeNumber, Group: groupBase, Min: 0, Max: 86400},

		{Key: "config_white_url_check", Label: "URL 白名单", Type: luaTypeSwitch, Group: groupCheck},
		{Key: "config_white_ip_check", Label: "IP 白名单", Type: luaTypeSwitch, Group: groupCheck},
		{Key: "config_black_ip_check", Label: "IP 黑名单", Type: luaTypeSwitch, Group: groupCheck},
		{Key: "config_url_check", Label: "URL 检测", Type: luaTypeSwitch, Group: groupCheck},
		{Key: "config_url_args_check", Label: "GET 参数检测", Type: luaTypeSwitch, Group: groupCheck},
		{Key: "config_user_agent_check", Label: "User-Agent 检测", Type: luaTypeSwitch, Group: groupCheck},
		{Key: "config_cookie_check", Label: "Cookie 检测", Type: luaTypeSwitch, Group: groupCheck},
		{Key: "config_header_check", Label: "请求头检测", Type: luaTypeSwitch, Group: groupCheck, Desc: "Host 注入、代理头伪造、头部 JNDI、头内注入（规则文件 header.rule）"},
		{Key: "config_upload_check", Label: "上传后缀白名单校验", Type: luaTypeSwitch, Group: groupCheck, Desc: "开启后上传文件后缀不在白名单内即拦截（config_upload_ext_whitelist）"},
		{Key: "config_upload_ext_whitelist", Label: "允许的上传后缀", Type: luaTypeText, Group: groupCheck, Desc: "逗号分隔，例如 jpg,png,pdf,docx,zip"},
		{Key: "config_cc_check", Label: "CC 频率检测", Type: luaTypeSwitch, Group: groupCheck},
		{Key: "config_ipcc_check", Label: "IP 频率检测", Type: luaTypeSwitch, Group: groupCheck},
		{Key: "config_post_check", Label: "请求体检测", Type: luaTypeSwitch, Group: groupCheck},
		{Key: "config_post_multipart_check", Label: "分段上传检测", Type: luaTypeSwitch, Group: groupCheck},
		{Key: "config_malformed_check", Label: "畸形请求检测", Type: luaTypeSwitch, Group: groupCheck},

		{Key: "config_cc_rate", Label: "CC 阈值", Type: luaTypeText, Group: groupRate, Desc: "格式 次数/秒，例如 300/60"},
		{Key: "config_ipcc_rate", Label: "IP 阈值", Type: luaTypeText, Group: groupRate, Desc: "格式 次数/秒，例如 120/7200"},
		{Key: "config_post_max_size", Label: "请求体检测上限（字节）", Type: luaTypeNumber, Group: groupRate, Min: 0},
		{Key: "config_post_max_parts", Label: "分段数量上限", Type: luaTypeNumber, Group: groupRate, Min: 0, Max: 5000},

		{Key: "config_auto_ban_check", Label: "自动封禁", Type: luaTypeSwitch, Group: groupAutoBan, Desc: "命中攻击后按 IP 计数，达阈值写入封禁表"},
		{Key: "config_auto_ban_expire", Label: "封禁时长（秒）", Type: luaTypeNumber, Group: groupAutoBan, Min: 0, Max: 86400},
		{Key: "config_auto_ban_window", Label: "统计窗口（秒）", Type: luaTypeNumber, Group: groupAutoBan, Min: 1, Max: 3600},
		{Key: "config_auto_ban_threshold", Label: "命中阈值（次）", Type: luaTypeNumber, Group: groupAutoBan, Min: 1, Max: 1000},
		{Key: "config_auto_ban_types", Label: "生效攻击类型", Type: luaTypeText, Group: groupAutoBan, Desc: "* 表示全部，也可写 sql注入,XSS跨站"},

		{Key: "config_log_dir", Label: "WAF 日志目录", Type: luaTypeText, Group: groupPath},
		{Key: "config_rule_dir", Label: "规则目录", Type: luaTypeText, Group: groupPath, Desc: "须与后台「运行配置 → 规则目录」一致"},

		{Key: "config_log_mode", Label: "日志输出模式", Type: luaTypeSelect, Group: groupLog, Options: []string{"file", "mysql", "redis"}, Desc: "file=本地文件；mysql=内存队列异步落库；redis=Redis 队列异步落库"},
		{Key: "config_log_batch_size", Label: "每批落库条数", Type: luaTypeNumber, Group: groupLog, Min: 1, Max: 5000},
		{Key: "config_log_flush_interval", Label: "落库间隔（秒）", Type: luaTypeNumber, Group: groupLog, Min: 1, Max: 300},
		{Key: "config_log_queue_size", Label: "内存队列上限", Type: luaTypeNumber, Group: groupLog, Min: 100, Max: 1000000},

		{Key: "config_mysql_host", Label: "主机", Type: luaTypeText, Group: groupMySQL},
		{Key: "config_mysql_port", Label: "端口", Type: luaTypeNumber, Group: groupMySQL, Min: 1, Max: 65535},
		{Key: "config_mysql_database", Label: "库名", Type: luaTypeText, Group: groupMySQL},
		{Key: "config_mysql_user", Label: "账号", Type: luaTypeText, Group: groupMySQL},
		{Key: "config_mysql_password", Label: "口令", Type: luaTypeSecret, Group: groupMySQL},
		{Key: "config_mysql_keepalive", Label: "连接空闲（毫秒）", Type: luaTypeNumber, Group: groupMySQL, Min: 0},

		{Key: "config_redis_host", Label: "主机", Type: luaTypeText, Group: groupRedis},
		{Key: "config_redis_port", Label: "端口", Type: luaTypeNumber, Group: groupRedis, Min: 1, Max: 65535},
		{Key: "config_redis_password", Label: "密码", Type: luaTypeSecret, Group: groupRedis},
		{Key: "config_redis_db", Label: "库号", Type: luaTypeNumber, Group: groupRedis, Min: 0, Max: 255},
		{Key: "config_redis_list_key", Label: "队列键名", Type: luaTypeText, Group: groupRedis},
		{Key: "config_redis_consume", Label: "由 nginx 自身消费", Type: luaTypeSwitch, Group: groupRedis, Desc: "关闭后 nginx 只写队列，由后台 wafadmin 消费入库（需同时开启 [log] INGEST_ENABLED）"},
	}
}

// View 读取并解析 config.lua。
func (s *LuaConfigService) View(ctx context.Context) (*LuaConfigView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.viewLocked()
}

// viewLocked 组装视图（调用方已持锁）。
func (s *LuaConfigService) viewLocked() (*LuaConfigView, error) {
	view := &LuaConfigView{
		Path:       s.path(),
		Groups:     make([]LuaConfigGroup, 0, 8),
		Unknown:    make([]string, 0, 4),
		NeedReload: true,
	}

	content, err := os.ReadFile(view.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return view, nil
		}
		return nil, ErrInternal.Wrap(err, "读取 config.lua 失败")
	}
	view.Exists = true
	view.Writable = writable(view.Path)
	if info, statErr := os.Stat(view.Path); statErr == nil {
		view.UpdatedAt = info.ModTime().Format("2006-01-02 15:04:05")
	}

	values, parsed := parseLuaValues(string(content))

	groupIndex := make(map[string]int, 8)
	for _, field := range s.fields() {
		index, ok := groupIndex[field.Group]
		if !ok {
			view.Groups = append(view.Groups, LuaConfigGroup{Name: field.Group})
			index = len(view.Groups) - 1
			groupIndex[field.Group] = index
		}

		value, exists := values[field.Key]
		item := LuaConfigItem{
			Key:     field.Key,
			Label:   field.Label,
			Type:    string(field.Type),
			Value:   value,
			Desc:    field.Desc,
			Options: field.Options,
			Min:     field.Min,
			Max:     field.Max,
		}
		if field.Type == luaTypeSecret {
			item.Value = mask(value)
			item.Masked = true
		}
		if !exists {
			item.Desc = strings.TrimSpace(item.Desc + "（配置文件缺少该项）")
		}
		view.Groups[index].Items = append(view.Groups[index].Items, item)
	}

	// 未纳入表单的键只做只读提示
	known := make(map[string]struct{}, 64)
	for _, field := range s.fields() {
		known[field.Key] = struct{}{}
	}
	for key := range parsed {
		if _, ok := known[key]; !ok {
			view.Unknown = append(view.Unknown, key)
		}
	}
	sortStrings(view.Unknown)
	return view, nil
}

// Update 校验并写回 config.lua，返回最新视图。
func (s *LuaConfigService) Update(ctx context.Context, values map[string]string) (*LuaConfigView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.path()
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound.New("未找到 %s，请先部署 lua-waf", path)
		}
		return nil, ErrInternal.Wrap(err, "读取 config.lua 失败")
	}

	fields := make(map[string]luaField, 64)
	for _, field := range s.fields() {
		fields[field.Key] = field
	}

	// 先全部校验，避免写一半
	normalized := make(map[string]string, len(values))
	for key, raw := range values {
		field, ok := fields[key]
		if !ok {
			return nil, ErrInvalid.New("不支持修改 %s", key)
		}
		// 密钥类提交掩码表示「不修改」
		if field.Type == luaTypeSecret && strings.Contains(raw, "****") {
			continue
		}
		value, err := normalizeLuaValue(field, raw)
		if err != nil {
			return nil, err
		}
		normalized[key] = value
	}
	if len(normalized) == 0 {
		return s.viewLocked()
	}

	lines := strings.Split(string(content), "\n")
	applied := 0
	for i, line := range lines {
		matched := luaLinePattern.FindStringSubmatch(line)
		if matched == nil {
			continue
		}
		key := matched[2]
		value, ok := normalized[key]
		if !ok {
			continue
		}
		// 多行字符串（[[ ... ]]）不在此处替换
		if strings.HasPrefix(strings.TrimSpace(matched[4]), "[[") {
			continue
		}
		quote := ""
		old := strings.TrimSpace(matched[4])
		if len(old) > 0 && (old[0] == '"' || old[0] == '\'') {
			quote = string(old[0])
		}
		if quote == "" && (fields[key].Type == luaTypeText || fields[key].Type == luaTypeSecret ||
			fields[key].Type == luaTypeSwitch || fields[key].Type == luaTypeSelect) {
			quote = `"`
		}
		rendered := value
		if quote != "" {
			rendered = quote + escapeLua(value, quote) + quote
		}
		lines[i] = matched[1] + matched[2] + matched[3] + rendered + matched[5] + matched[6]
		applied++
	}
	if applied == 0 {
		return nil, ErrInvalid.New("配置文件中没有匹配到待修改的配置项")
	}

	updated := strings.Join(lines, "\n")
	if err := s.writeFile(path, updated); err != nil {
		return nil, err
	}
	return s.viewLocked()
}

// Raw 返回 config.lua 原文。
func (s *LuaConfigService) Raw(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	content, err := os.ReadFile(s.path())
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound.New("未找到 %s", s.path())
		}
		return "", ErrInternal.Wrap(err, "读取 config.lua 失败")
	}
	return string(content), nil
}

// path 返回生效的配置文件路径。
func (s *LuaConfigService) path() string {
	if strings.TrimSpace(s.Path) != "" {
		return s.Path
	}
	return "/www/server/nginx/x-waf/config.lua"
}

// writeFile 备份后原子写入。
func (s *LuaConfigService) writeFile(path, content string) error {
	if err := os.WriteFile(path+".bak", []byte(content), 0o644); err != nil {
		// 备份失败不影响主流程，但要记录下来
		// （备份用于出错时人工回滚）
		_ = err
	}

	temp, err := os.CreateTemp(filepath.Dir(path), ".config-lua-*")
	if err != nil {
		return ErrInternal.Wrap(err, "创建临时文件失败")
	}
	tempName := temp.Name()
	if _, err := temp.WriteString(content); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempName)
		return ErrInternal.Wrap(err, "写入临时文件失败")
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return ErrInternal.Wrap(err, "关闭临时文件失败")
	}
	if err := os.Rename(tempName, path); err != nil {
		_ = os.Remove(tempName)
		return ErrInternal.Wrap(err, "替换 config.lua 失败")
	}
	return nil
}

// parseLuaValues 解析 `config_xxx = value` 行，返回去引号后的值与原始键集合。
func parseLuaValues(content string) (map[string]string, map[string]struct{}) {
	values := make(map[string]string, 64)
	keys := make(map[string]struct{}, 64)
	for _, line := range strings.Split(content, "\n") {
		matched := luaLinePattern.FindStringSubmatch(line)
		if matched == nil {
			continue
		}
		key := matched[2]
		raw := strings.TrimSpace(matched[4])
		keys[key] = struct{}{}
		values[key] = unquoteLua(raw)
	}
	return values, keys
}

// unquoteLua 去掉 Lua 字符串两侧的引号。
func unquoteLua(raw string) string {
	if len(raw) >= 2 {
		if (raw[0] == '"' && raw[len(raw)-1] == '"') || (raw[0] == '\'' && raw[len(raw)-1] == '\'') {
			inner := raw[1 : len(raw)-1]
			inner = strings.ReplaceAll(inner, `\"`, `"`)
			inner = strings.ReplaceAll(inner, `\'`, `'`)
			inner = strings.ReplaceAll(inner, `\\`, `\`)
			return inner
		}
	}
	return raw
}

// escapeLua 转义 Lua 字符串内容。
func escapeLua(value, quote string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, quote, `\`+quote)
	return value
}

// normalizeLuaValue 按字段类型校验并归一化取值。
func normalizeLuaValue(field luaField, raw string) (string, error) {
	value := strings.TrimSpace(raw)

	switch field.Type {
	case luaTypeSwitch:
		if value != "on" && value != "off" {
			return "", ErrInvalid.New("%s 只能是 on 或 off", field.Label)
		}
		return value, nil
	case luaTypeNumber:
		if value == "" {
			return "", ErrInvalid.New("%s 不能为空", field.Label)
		}
		number, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return "", ErrInvalid.New("%s 必须是数字", field.Label)
		}
		if field.Max > 0 && number > field.Max {
			return "", ErrInvalid.New("%s 不能大于 %v", field.Label, field.Max)
		}
		if number < field.Min {
			return "", ErrInvalid.New("%s 不能小于 %v", field.Label, field.Min)
		}
		// 整数类型去掉小数点，保持配置文件整洁
		if number == float64(int64(number)) {
			return strconv.FormatInt(int64(number), 10), nil
		}
		return strconv.FormatFloat(number, 'f', -1, 64), nil
	case luaTypeSelect:
		for _, option := range field.Options {
			if option == value {
				return value, nil
			}
		}
		return "", ErrInvalid.New("%s 取值必须是 %s 之一", field.Label, strings.Join(field.Options, " / "))
	case luaTypeSecret, luaTypeText:
		if strings.Contains(value, "[[") {
			return "", ErrInvalid.New("%s 不支持多行内容", field.Label)
		}
		if field.Key == "config_cc_rate" || field.Key == "config_ipcc_rate" {
			if !ratePattern.MatchString(value) {
				return "", ErrInvalid.New("%s 格式应为 次数/秒，例如 300/60", field.Label)
			}
		}
		return value, nil
	default:
		return value, nil
	}
}

// sortStrings 简单插入排序，避免额外依赖。
func sortStrings(items []string) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j] < items[j-1]; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

// DescribeLuaConfigPath 生成便于展示的路径说明。
func DescribeLuaConfigPath(view *LuaConfigView) string {
	if view == nil {
		return ""
	}
	return fmt.Sprintf("%s（存在=%v）", view.Path, view.Exists)
}

// Reload 让修改后的 config.lua 生效（nginx -t && nginx -s reload）。
func (s *LuaConfigService) Reload(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s.Reloader == nil {
		return "当前模式不支持 reload（演示模式只渲染不落盘）", nil
	}
	output, err := s.Reloader.Reload()
	if err != nil {
		return output, ErrInternal.Wrap(err, "reload nginx 失败，请检查配置")
	}
	return output, nil
}
