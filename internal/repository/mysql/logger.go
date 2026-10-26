package mysql

import (
	"fmt"
	"log"
	"strings"

	"xorm.io/core"
)

// sqlLogger 把 xorm 的 SQL 日志转发到标准 log 输出（带 [sql] 前缀）。
//
// 打开后每条实际执行的 SQL 都会打印语句、参数与耗时，例如：
//
//	[sql] INFO  [SQL] SELECT * FROM site WHERE site_name LIKE ? [%!s(string=%example%)] - took: 1.2ms
//
// 注意：会输出参数原始值（含查询条件、日志内容），生产环境开启时留意日志量与敏感信息。
type sqlLogger struct {
	level   core.LogLevel
	showSQL bool
}

// Debug 输出调试信息（xorm 内部较少使用）。
func (l *sqlLogger) Debug(v ...interface{}) { l.output(core.LOG_DEBUG, "DEBUG", fmt.Sprint(v...)) }

// Debugf 格式化输出调试信息。
func (l *sqlLogger) Debugf(format string, v ...interface{}) {
	l.output(core.LOG_DEBUG, "DEBUG", fmt.Sprintf(format, v...))
}

// Info 输出普通信息（SQL 语句走这里）。
func (l *sqlLogger) Info(v ...interface{}) { l.output(core.LOG_INFO, "INFO", fmt.Sprint(v...)) }

// Infof 格式化输出普通信息（SQL 语句走这里）。
func (l *sqlLogger) Infof(format string, v ...interface{}) {
	l.output(core.LOG_INFO, "INFO", fmt.Sprintf(format, v...))
}

// Warn 输出告警。
func (l *sqlLogger) Warn(v ...interface{}) { l.output(core.LOG_WARNING, "WARN", fmt.Sprint(v...)) }

// Warnf 格式化输出告警。
func (l *sqlLogger) Warnf(format string, v ...interface{}) {
	l.output(core.LOG_WARNING, "WARN", fmt.Sprintf(format, v...))
}

// Error 输出错误（SQL 执行失败走这里）。
func (l *sqlLogger) Error(v ...interface{}) { l.output(core.LOG_ERR, "ERROR", fmt.Sprint(v...)) }

// Errorf 格式化输出错误。
func (l *sqlLogger) Errorf(format string, v ...interface{}) {
	l.output(core.LOG_ERR, "ERROR", fmt.Sprintf(format, v...))
}

// Level 返回当前日志级别。
func (l *sqlLogger) Level() core.LogLevel { return l.level }

// SetLevel 设置日志级别。
func (l *sqlLogger) SetLevel(level core.LogLevel) { l.level = level }

// ShowSQL 设置是否显示 SQL（不传参数表示开启）。
func (l *sqlLogger) ShowSQL(show ...bool) {
	if len(show) == 0 {
		l.showSQL = true
		return
	}
	l.showSQL = show[0]
}

// IsShowSQL 返回是否显示 SQL。
func (l *sqlLogger) IsShowSQL() bool { return l.showSQL }

// output 统一输出入口，按级别与 SQL 开关过滤，补齐前缀并简化参数打印。
// 关闭 SQL 日志时仍保留错误与告警（如连接异常），只是不打印语句。
func (l *sqlLogger) output(level core.LogLevel, tag, message string) {
	if l.level == core.LOG_OFF || level < l.level {
		return
	}
	if !l.showSQL && level < core.LOG_ERR {
		return
	}
	log.Printf("[sql] %-5s %s", tag, cleanArgs(strings.TrimRight(message, "\n")))
}

// cleanArgs 把 xorm 用 %#v 打印参数产生的 "[]interface {}{...}" 简化为 "[...]"，
// 只影响日志可读性，不改变语句本身。
func cleanArgs(message string) string {
	const marker = "[]interface {}{"

	index := strings.Index(message, marker)
	if index < 0 {
		return message
	}

	head := message[:index] + "["
	tail := message[index+len(marker):]

	if end := strings.Index(tail, "} - took:"); end >= 0 {
		return head + tail[:end] + "]" + tail[end+1:]
	}
	if end := strings.Index(tail, "}"); end >= 0 {
		return head + tail[:end] + "]" + tail[end+1:]
	}
	return message
}
