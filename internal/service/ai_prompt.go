package service

// 本文件是 AI 模块的「提示词与错误翻译」：把日志转成提示词，
// 以及把底层网络错误翻译成用户能看懂的原因。

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"wafadmin/internal/model"
)

// buildLogPrompt 把日志拼装为分析提示词。
func buildLogPrompt(logs []model.WafLog, question string) string {
	var builder strings.Builder
	builder.WriteString("以下是 WAF 命中的攻击日志：\n\n")
	for index, item := range logs {
		builder.WriteString(fmt.Sprintf("### 日志 %d\n", index+1))
		builder.WriteString(fmt.Sprintf("- 时间: %s\n", item.LocalTime))
		builder.WriteString(fmt.Sprintf("- 来源 IP: %s（%s）\n", item.ClientIp, fallback(item.Area, "未知")))
		builder.WriteString(fmt.Sprintf("- 站点: %s\n", fallback(item.ServerName, "未知")))
		builder.WriteString(fmt.Sprintf("- 攻击类型: %s\n", fallback(item.AttackType, "未知")))
		builder.WriteString(fmt.Sprintf("- 命中规则: %s\n", fallback(item.RuleTag, "未知")))
		builder.WriteString(fmt.Sprintf("- 请求路径: %s\n", truncateText(item.ReqUrl, 2000)))
		builder.WriteString(fmt.Sprintf("- User-Agent: %s\n", truncateText(fallback(item.UserAgent, "无"), 600)))
		if strings.TrimSpace(item.ReqData) != "" {
			builder.WriteString(fmt.Sprintf("- 请求体: %s\n", truncateText(item.ReqData, 2000)))
		}
		builder.WriteString("\n")
	}
	builder.WriteString("### 请回答\n")
	builder.WriteString(question)
	return builder.String()
}

// fallback 返回非空值。
func fallback(value, def string) string {
	if strings.TrimSpace(value) == "" {
		return def
	}
	return value
}

// truncateText 截断过长文本。
func truncateText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

// describeTransportError 把传输层错误翻译成可操作的提示，并附带原始错误信息。
func describeTransportError(err error, endpoint string, timeout time.Duration) string {
	raw := err.Error()
	lower := strings.ToLower(raw)

	hint := ""
	switch {
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(lower, "context deadline exceeded"):
		hint = fmt.Sprintf("请求超时（%s 内模型未返回），可在「AI 设置」调大「请求超时」，或改用更快的模型", timeout)
	case errors.Is(err, context.Canceled):
		hint = "请求已被取消（页面关闭或服务重启）"
	case strings.Contains(lower, "connection refused"):
		hint = "连接被拒绝：模型服务未启动或端口不对"
	case strings.Contains(lower, "no such host") || strings.Contains(lower, "server misbehaving"):
		hint = "域名解析失败：请检查接口地址"
	case strings.Contains(lower, "i/o timeout") || strings.Contains(lower, "dial tcp") && strings.Contains(lower, "timeout"):
		hint = "连接超时：网络不可达或被防火墙拦截"
	case strings.Contains(lower, "eof") || strings.Contains(lower, "reset by peer") || strings.Contains(lower, "broken pipe"):
		hint = "连接被对端中断：模型服务可能过载或主动断开"
	case strings.Contains(lower, "x509") || strings.Contains(lower, "certificate"):
		hint = "TLS 证书校验失败：可改用 http 内网地址，或为接口配置受信任证书"
	case strings.Contains(lower, "unsupported protocol scheme"):
		hint = "接口地址缺少 http:// 或 https:// 前缀"
	case strings.Contains(lower, "too many redirects"):
		hint = "接口发生多次重定向：请确认地址是否指向 /v1 接口"
	default:
		hint = "网络请求失败"
	}
	return fmt.Sprintf("%s（%s；原始错误：%s）", hint, endpoint, raw)
}

// isRetryableTransportError 判断是否为可重试的瞬时传输错误。
func isRetryableTransportError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}
	lower := strings.ToLower(err.Error())
	for _, keyword := range []string{"eof", "reset by peer", "broken pipe", "connection reset", "temporary failure"} {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}
