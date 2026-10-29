package service

// 本文件是「WAF 配置」板块的**服务入口**：读取 config.lua → 组装视图 → 校验并写回 → reload。
//
// 分层说明（本板块共 3 个文件）：
//   - luaconfig_schema.go：配置项声明（有哪些项、属于哪个模块、怎么校验/展示、中文说明）
//   - luaconfig_parse.go：文本解析与校验（按行取值、按行回写、值的类型校验）
//   - luaconfig.go（本文件）：对外服务（View / Update / Raw / Reload）与文件读写
//
// 为什么要拆：配置项会持续增加（目前 40+ 项），声明与读写逻辑混在一起后，
// 加一个配置要同时改「类型定义、控件类型、校验、按行替换」四处，容易漏；
// 拆开后新增配置只需要在 schema 里加一条，其余逻辑通用。

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"wafadmin/internal/config"
	"wafadmin/internal/repository"
)

// LuaConfigItem 是返回给前端的单个配置项。
type LuaConfigItem struct {
	// Key 为 config.lua 中的变量名。
	Key string `json:"key"`
	// Label 为界面显示名。
	Label string `json:"label"`
	// Type 为控件类型：switch / number / text / select / secret / code。
	Type string `json:"type"`
	// Value 为当前值（密钥类已脱敏）。
	Value string `json:"value"`
	// Desc 为字段说明。
	Desc string `json:"desc"`
	// Options 为枚举可选值。
	Options []string `json:"options,omitempty"`
	// Min 为数字下限。
	Min float64 `json:"min,omitempty"`
	// Max 为数字上限。
	Max float64 `json:"max,omitempty"`
	// Masked 表示该值为脱敏展示（密钥类），提交时原样回传表示不修改。
	Masked bool `json:"masked,omitempty"`
	// Detected 表示该键在 config.lua 中实际存在（false 时界面会提示缺少该项）。
	Detected bool `json:"detected"`
}

// LuaConfigGroup 是按模块分组的配置项（前端一个选项卡）。
type LuaConfigGroup struct {
	// Name 为模块名。
	Name string `json:"name"`
	// Items 为该模块下的配置项。
	Items []LuaConfigItem `json:"items"`
}

// LuaConfigView 是 WAF 配置页的完整视图。
type LuaConfigView struct {
	// Path 为 config.lua 路径。
	Path string `json:"path"`
	// Exists 表示文件是否存在。
	Exists bool `json:"exists"`
	// Writable 表示当前进程是否有写权限。
	Writable bool `json:"writable"`
	// Groups 为可编辑配置项（按模块分组）。
	Groups []LuaConfigGroup `json:"groups"`
	// Unknown 为文件里存在但未纳入表单的键（只读展示）。
	Unknown []string `json:"unknown"`
	// UpdatedAt 为文件最近修改时间。
	UpdatedAt string `json:"updated_at"`
	// NeedReload 提示保存后需要 reload nginx 才生效。
	NeedReload bool `json:"need_reload"`
}

// LuaReloader 让修改后的 config.lua 生效（内部执行 nginx -t && reload）。
type LuaReloader interface {
	Reload() (string, error)
}

// LuaConfigService 负责读写 openresty 侧的 config.lua。
//
// 写入策略：只做「按行替换值」，保留原有注释、顺序与缩进；写前备份 .bak，写时先写临时文件再原子替换。
type LuaConfigService struct {
	// Path 为 config.lua 路径（为空时用默认路径）。
	Path string
	// Reloader 负责保存后让配置生效，可为空（演示模式）。
	Reloader LuaReloader
	// Logs 用于「生效自检」里查最近一条攻击日志，判断日志链路是否真的在入库（可为空）。
	Logs repository.LogRepository
	// Cfg 为后台运行配置（用于自检里核对 [log] 消费相关配置）。
	Cfg *config.Config

	// mu 串行化读写，避免并发保存互相覆盖。
	mu sync.Mutex
}

// View 读取并解析 config.lua，返回按模块分组的视图。
func (s *LuaConfigService) View(ctx context.Context) (*LuaConfigView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.viewLocked()
}

// viewLocked 组装视图（调用方需已持锁）。
func (s *LuaConfigService) viewLocked() (*LuaConfigView, error) {
	view := &LuaConfigView{
		Path:       s.path(),
		Groups:     make([]LuaConfigGroup, 0, 8),
		Unknown:    make([]string, 0, 4),
		NeedReload: true,
	}

	content, err := os.ReadFile(view.Path)
	if err != nil {
		// 文件不存在时返回空视图（前端会提示未找到 config.lua），不视为错误
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

	// 按 schema 声明顺序聚合成模块，Group 相同即同一模块
	groupIndex := make(map[string]int, 8)
	for _, field := range luaFields() {
		index, ok := groupIndex[field.Group]
		if !ok {
			view.Groups = append(view.Groups, LuaConfigGroup{Name: field.Group})
			index = len(view.Groups) - 1
			groupIndex[field.Group] = index
		}

		value, exists := values[field.Key]
		item := LuaConfigItem{
			Key:      field.Key,
			Label:    field.Label,
			Type:     string(field.Type),
			Value:    value,
			Desc:     field.Desc,
			Options:  field.Options,
			Min:      field.Min,
			Max:      field.Max,
			Detected: exists,
		}
		// 密钥类脱敏展示，避免口令直接暴露在前端/日志里
		if field.Type == luaTypeSecret {
			item.Value = mask(value)
			item.Masked = true
		}
		if !exists {
			item.Desc = strings.TrimSpace(item.Desc + "（配置文件中缺少该项，保存时会自动补写）")
		}
		view.Groups[index].Items = append(view.Groups[index].Items, item)
	}

	// 文件里存在但表单未覆盖的键，只在界面做只读提示（例如自定义变量）
	known := knownLuaKeys()
	for key := range parsed {
		if _, ok := known[key]; !ok {
			view.Unknown = append(view.Unknown, key)
		}
	}
	sortStrings(view.Unknown)
	return view, nil
}

// Update 校验并写回 config.lua，返回最新视图。
//
// 流程：读取原文 → 逐项校验（先全部校验再写，避免写一半）→ 按行替换 → 备份 + 原子写入。
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

	fields := luaFieldIndex()

	normalized := make(map[string]string, len(values))
	for key, raw := range values {
		field, ok := fields[key]
		if !ok {
			return nil, ErrInvalid.New("不支持修改 %s", key)
		}
		// 密钥类：前端回传脱敏值表示「不修改」
		if field.Type == luaTypeSecret && containsMask(raw) {
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

	updated, applied := rewriteLuaValues(string(content), normalized, fields)
	if applied == 0 {
		return nil, ErrInvalid.New("配置文件中没有匹配到待修改的配置项")
	}

	if err := s.writeFile(path, updated); err != nil {
		return nil, err
	}
	return s.viewLocked()
}

// Raw 返回 config.lua 原文（供「查看原文」弹窗使用）。
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

// path 返回实际使用的配置文件路径。
func (s *LuaConfigService) path() string {
	if strings.TrimSpace(s.Path) != "" {
		return s.Path
	}
	return "/www/server/nginx/x-waf/config.lua"
}

// writeFile 先备份 .bak，再写临时文件并原子替换目标文件。
// 备份失败不影响主流程（备份仅用于人工回滚）。
func (s *LuaConfigService) writeFile(path, content string) error {
	if err := os.WriteFile(path+".bak", []byte(content), 0o644); err != nil {
		// 备份失败不阻断保存：文件仍会被正常写入
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

// DescribeLuaConfigPath 生成便于展示的路径说明（调试/日志用）。
func DescribeLuaConfigPath(view *LuaConfigView) string {
	if view == nil {
		return ""
	}
	return fmt.Sprintf("%s（存在=%v）", view.Path, view.Exists)
}
