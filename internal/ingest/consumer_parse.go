package ingest

// 本文件是日志消费里的「纯函数」部分：解析 Redis 里的日志 JSON、错误翻译。
// 与 consumer.go 的循环/队列逻辑分开，便于阅读与单测。

import (
	"encoding/json"
	"strings"
	"time"

	"wafadmin/internal/model"
)

// parseRecord 把 lua 写入的 JSON 解析为日志模型。
func parseRecord(raw string) (model.WafLog, bool) {
	var payload struct {
		ClientIp   string `json:"client_ip"`
		LocalTime  string `json:"local_time"`
		ServerName string `json:"server_name"`
		UserAgent  string `json:"user_agent"`
		AttackType string `json:"attack_type"`
		ReqUrl     string `json:"req_url"`
		ReqData    string `json:"req_data"`
		Area       string `json:"area"`
		RuleTag    string `json:"rule_tag"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return model.WafLog{}, false
	}
	if strings.TrimSpace(payload.ClientIp) == "" && strings.TrimSpace(payload.ReqUrl) == "" {
		return model.WafLog{}, false
	}

	record := model.WafLog{
		ClientIp:   payload.ClientIp,
		LocalTime:  payload.LocalTime,
		ServerName: payload.ServerName,
		UserAgent:  payload.UserAgent,
		AttackType: payload.AttackType,
		ReqUrl:     payload.ReqUrl,
		ReqData:    payload.ReqData,
		Area:       payload.Area,
		RuleTag:    payload.RuleTag,
	}
	if record.LocalTime == "" {
		record.LocalTime = time.Now().Format("2006-01-02 15:04:05")
	}
	return record, true
}

// containsRecord 判断原始 JSON 是否对应某条已解析记录（用于 LREM 精确删除）。
func containsRecord(raw string, records []model.WafLog) bool {
	record, ok := parseRecord(raw)
	if !ok {
		return false
	}
	for _, item := range records {
		if item.ClientIp == record.ClientIp && item.LocalTime == record.LocalTime &&
			item.ReqUrl == record.ReqUrl && item.RuleTag == record.RuleTag {
			return true
		}
	}
	return false
}

// explainInsertError 把常见建表/权限问题翻译成可操作的提示。
func explainInsertError(err error) string {
	if err == nil {
		return ""
	}

	message := err.Error()
	switch {
	case strings.Contains(message, "1146"), strings.Contains(message, "doesn't exist"):
		return "（waf_log 表不存在：重启 wafadmin 会自动建表；若数据库账号无建表权限，请手工执行 lua-waf/sql/waf_log.sql）"
	case strings.Contains(message, "1142"), strings.Contains(message, "denied"):
		return "（数据库账号权限不足：需要该库的 INSERT/CREATE 权限）"
	case strings.Contains(message, "1406"), strings.Contains(message, "Data too long"):
		return "（字段超长：检查 waf_log 表字段长度是否与 model/waflog.go 一致）"
	}
	return ""
}
