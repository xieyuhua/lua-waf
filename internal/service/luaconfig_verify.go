package service

// 本文件负责「WAF 配置生效自检」：判断后台改的 config.lua 到底是不是 nginx 正在读的那一份，
// 以及日志模式相关的前置条件是否满足。
//
// 典型问题（本文件就是为了让它可见）：
//  1. 后台的 [waf] LUA_CONFIG 指向仓库内的相对路径（如 lua-waf/config.lua），
//     而 nginx 实际加载的是 /www/server/nginx/x-waf/config.lua —— 改的是另一份文件，reload 也不会生效；
//  2. 日志模式改成 redis 后，nginx 侧仍写文件：多为 lua-resty-redis 未安装、Redis 连不上/口令错，
//     log_sink 会打 WARN 并回落到文件；
//  3. redis 模式下 config_redis_consume 与后台 [log] INGEST_ENABLED 同时开启 → 两边抢消费或重复入库；
//  4. 规则目录 / 日志目录与后台配置不一致 → 规则或日志写到预期之外的位置。

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"wafadmin/internal/model"
	"wafadmin/pkg/redis"
)

// LuaVerifyItem 是一条自检结果。
type LuaVerifyItem struct {
	// Key 为检查项标识。
	Key string `json:"key"`
	// Label 为检查项名称。
	Label string `json:"label"`
	// Level 为结果级别：ok / warn / error。
	Level string `json:"level"`
	// Message 为详细说明（含建议动作）。
	Message string `json:"message"`
}

// LuaVerifyView 是自检结果集合。
type LuaVerifyView struct {
	// Path 为后台正在编辑的 config.lua 路径。
	Path string `json:"path"`
	// Exists 表示该文件是否存在。
	Exists bool `json:"exists"`
	// Items 为逐项检查结果。
	Items []LuaVerifyItem `json:"items"`
	// Mode 为文件里的日志模式（便于快速确认）。
	Mode string `json:"mode"`
	// RedisConsume 为文件里的「nginx 自身消费」开关。
	RedisConsume string `json:"redis_consume"`
	// Verdict 为总体结论：ok / warn / error。
	Verdict string `json:"verdict"`
}

// Verify 执行配置生效自检。
func (s *LuaConfigService) Verify(ctx context.Context) (*LuaVerifyView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	path := s.path()
	view := &LuaVerifyView{Path: path, Items: make([]LuaVerifyItem, 0, 8)}

	add := func(key, label, level, message string) {
		view.Items = append(view.Items, LuaVerifyItem{Key: key, Label: label, Level: level, Message: message})
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			add("file", "配置文件", "error",
				"后台配置的路径不存在："+path+"。请把 [waf] LUA_CONFIG 改成 nginx 实际加载的 config.lua（通常是 /www/server/nginx/x-waf/config.lua）")
			view.Verdict = "error"
			return view, nil
		}
		return nil, ErrInternal.Wrap(err, "读取 config.lua 失败")
	}
	view.Exists = true

	values, _ := parseLuaValues(string(content))
	view.Mode = values["config_log_mode"]
	view.RedisConsume = values["config_redis_consume"]

	// 1) 路径是否像「nginx 侧」的绝对路径
	if !filepath.IsAbs(path) {
		add("abs_path", "配置路径", "warn",
			"当前配置的是相对路径 "+path+"（解析为 "+absoluteOrSelf(path)+"）。后台与 nginx 同机部署时，建议改成 nginx 实际加载的绝对路径")
	} else {
		add("abs_path", "配置路径", "ok", "使用绝对路径："+path)
	}

	// 2) 规则目录同目录下是否还有另一份 config.lua（很可能才是 nginx 读的那份）
	if rewriteDir := strings.TrimSpace(values["config_rule_dir"]); rewriteDir != "" {
		sibling := filepath.Join(filepath.Dir(strings.TrimRight(rewriteDir, "/\\")), "config.lua")
		if sameFile(sibling, path) {
			add("same_file", "与 nginx 规则目录同源", "ok", "config.lua 与 config_rule_dir 在同一目录："+sibling)
		} else if _, statErr := os.Stat(sibling); statErr == nil {
			add("same_file", "与 nginx 规则目录同源", "error",
				"检测到另一份配置文件："+sibling+"（与 config_rule_dir 同目录）。若 nginx 加载的是它，后台改的是 "+path+" 就不会生效")
		} else {
			add("same_file", "与 nginx 规则目录同源", "warn",
				"config_rule_dir 同目录下没有 config.lua（"+sibling+"），请确认 nginx 实际加载的是 "+path)
		}
	}

	// 3) 日志模式与前置条件
	mode := strings.ToLower(strings.TrimSpace(view.Mode))
	switch mode {
	case "file":
		add("log_mode", "日志模式", "ok", "file 模式：日志写本地文件，后台日志页不会增长（这是预期行为）")
	case "mysql":
		add("log_mode", "日志模式", "ok", "mysql 模式：nginx 直接批量写库；需要 config_mysql_* 可用")
	case "redis":
		redisPort := values["config_redis_port"]
		if redisPort == "" {
			redisPort = "6379"
		}
		address := fmt.Sprintf("%s:%s", values["config_redis_host"], redisPort)

		test := s.testRedis(values["config_redis_host"], redisPort, values["config_redis_password"], values["config_redis_db"])
		if test != nil && test.Ok {
			add("redis_conn", "Redis 连通性", "ok", test.Message+"（"+address+"）")
		} else if test != nil {
			add("redis_conn", "Redis 连通性", "error",
				test.Message+"（"+address+"）。log_sink 在写 Redis 失败时会**回落到写文件**，这就是「配了 redis 还在写文件」的常见原因")
		}

		consume := strings.ToLower(strings.TrimSpace(view.RedisConsume))
		if consume == "off" {
			add("consume", "消费方", "warn",
				"config_redis_consume = off：由后台 wafadmin 消费，需要 conf/app.ini 的 [log] INGEST_ENABLED = on 且 REDIS_LIST_KEY 与 config_redis_list_key 一致")
		} else {
			add("consume", "消费方", "ok", "config_redis_consume = on：nginx 自己消费队列落库（此时后台看板的「已入库」不会增长，属正常）")
		}

		add("redis_ext", "lua-resty-redis", "warn",
			"若上面连通性 OK 但仍写文件，请检查 nginx error.log 是否有「[waf] Redis 队列写入失败」（多为 lua-resty-redis 未安装或 shared dict 未声明）")
	case "":
		add("log_mode", "日志模式", "error", "config.lua 里没有 config_log_mode，将按 file 模式处理")
	default:
		add("log_mode", "日志模式", "error", "无法识别的 config_log_mode = "+view.Mode+"（只能是 file / mysql / redis）")
	}

	// 4) 目录一致性：规则目录 / 日志目录是否与后台配置一致
	if dir := strings.TrimSpace(values["config_rule_dir"]); dir != "" {
		if info, statErr := os.Stat(dir); statErr != nil || !info.IsDir() {
			add("rule_dir", "规则目录", "warn", "config_rule_dir 目录不存在或不可访问："+dir+"（若 nginx 不在本机可忽略）")
		} else {
			add("rule_dir", "规则目录", "ok", "存在："+dir)
		}
	}

	// 4.1) 日志目录「最近是否还在被写」——直接回答「配置了 redis 为什么还写文件」
	if dir := strings.TrimSpace(values["config_log_dir"]); dir != "" {
		if newest, name, err := newestFile(dir); err == nil && !newest.IsZero() {
			age := time.Since(newest)
			if mode != "file" && age < 10*time.Minute {
				add("log_dir_write", "日志目录写入", "error",
					fmt.Sprintf("当前模式为 %s，但 %s 下最近仍有写入（%s，%s 前）。说明日志仍被写到文件：要么是 log_sink 回落（看 nginx error.log 的「[waf] Redis 队列写入失败」），要么这个目录其实是 nginx 的 access_log 目录",
						defaultString(mode, "未知"), dir, name, age.Round(time.Second)))
			} else if mode != "file" {
				add("log_dir_write", "日志目录写入", "ok",
					fmt.Sprintf("%s 下最近一次写入在 %s 前（%s），已停止写文件", dir, age.Round(time.Minute), name))
			} else {
				add("log_dir_write", "日志目录写入", "ok", fmt.Sprintf("file 模式，最近写入 %s（%s 前）", name, age.Round(time.Minute)))
			}
		} else if err != nil {
			add("log_dir_write", "日志目录写入", "warn", "无法读取日志目录 "+dir+"（若 nginx 不在本机可忽略）："+err.Error())
		}
	}

	// 4.2) redis 模式下报告队列长度。
	// 注意：config_redis_consume = on 时 nginx 自己会消费队列，LLEN 长期为 0 属正常，
	// 此时真正的判据是「MySQL 里最近有没有新日志」，见 4.3。
	if mode == "redis" {
		redisPort := values["config_redis_port"]
		if redisPort == "" {
			redisPort = "6379"
		}
		address := fmt.Sprintf("%s:%s", values["config_redis_host"], redisPort)
		if client, err := redis.Dial(redis.Options{
			Addr:     address,
			Password: values["config_redis_password"],
			DB:       atoiDefault(values["config_redis_db"], 0),
			Timeout:  3 * time.Second,
		}); err == nil {
			key := defaultString(values["config_redis_list_key"], "waf:logs")
			length, lerr := client.LLen(key)
			_ = client.Close()
			if lerr == nil {
				consumeOn := strings.ToLower(strings.TrimSpace(view.RedisConsume)) != "off"
				level := "ok"
				message := fmt.Sprintf("队列 %s 当前长度 %d", key, length)
				if consumeOn {
					message += "（config_redis_consume = on：nginx 自己消费，长度通常接近 0，属正常；以下面的「最近入库」为准）"
				} else if length == 0 {
					level = "warn"
					message += "；nginx 只写不消费，若线上有攻击仍为 0，说明 nginx 没在写队列（模式没生效或回落写文件）"
				} else {
					message += "（nginx 只写不消费，长度应持续增长）"
				}
				add("redis_queue", "Redis 队列", level, message)
			}
		}
	}
	// 4.3) 最近入库时间：日志链路的最终判据（不管 file/mysql/redis 模式）
	if s.Logs != nil {
		if latest, okTime, err := s.latestLogTime(ctx); err == nil && okTime {
			age := time.Since(latest)
			level := "ok"
			message := fmt.Sprintf("最近一条攻击日志入库于 %s（%s 前）", latest.Format("2006-01-02 15:04:05"), age.Round(time.Minute))
			if age > 30*time.Minute {
				level = "warn"
				message += "；半小时内没有新日志，若线上有攻击请检查 nginx 侧是否回落写文件（error.log 搜「Redis 队列写入失败」）"
			}
			add("db_recent", "最近入库", level, message)
		}
	}
	// 5) 汇总结论
	view.Verdict = "ok"
	for _, item := range view.Items {
		if item.Level == "error" {
			view.Verdict = "error"
			break
		}
		if item.Level == "warn" && view.Verdict == "ok" {
			view.Verdict = "warn"
		}
	}
	return view, nil
}

// absoluteOrSelf 返回绝对路径（失败时返回原值）。
func absoluteOrSelf(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// sameFile 判断两个路径是否指向同一个文件（大小写与相对路径都做了归一）。
func sameFile(left, right string) bool {
	absLeft, errLeft := filepath.Abs(left)
	absRight, errRight := filepath.Abs(right)
	if errLeft != nil || errRight != nil {
		return left == right
	}
	return strings.EqualFold(filepath.Clean(absLeft), filepath.Clean(absRight))
}

// newestFile 返回目录下最新的普通文件及其修改时间。
func newestFile(dir string) (time.Time, string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return time.Time{}, "", err
	}

	var newest time.Time
	var name string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
			name = entry.Name()
		}
	}
	return newest, name, nil
}

// latestLogTime 返回最近一条攻击日志的入库时间（解析失败时 ok=false）。
func (s *LuaConfigService) latestLogTime(ctx context.Context) (time.Time, bool, error) {
	items, _, err := s.Logs.List(ctx, model.LogQuery{ListQuery: model.ListQuery{Page: 1, Size: 1}})
	if err != nil {
		return time.Time{}, false, err
	}
	if len(items) == 0 {
		return time.Time{}, false, nil
	}

	raw := strings.TrimSpace(items[0].LocalTime)
	if raw == "" {
		return time.Time{}, false, nil
	}
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006/01/02 15:04:05"} {
		if parsed, parseErr := time.ParseInLocation(layout, raw, time.Local); parseErr == nil {
			return parsed, true, nil
		}
	}
	return time.Time{}, false, nil
}
