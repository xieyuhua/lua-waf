package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"wafadmin/internal/model"
)

// maxAnalyzeSlowSQL 是单次可提交的慢 SQL 条数上限。
const maxAnalyzeSlowSQL = 20

// defaultSlowSQLQuestion 是未填写追问时的默认问题。
const defaultSlowSQLQuestion = "请分析这些慢 SQL 的原因，并给出可落地的优化方案。"

// AnalyzeSlowSQLRequest 是慢 SQL 分析的请求体。
type AnalyzeSlowSQLRequest struct {
	// SlowSQLIds 为待分析的慢 SQL 记录 ID。
	SlowSQLIds []int64 `json:"slow_sql_ids"`
	// Question 为可选的追加提问。
	Question string `json:"question"`
	// Refresh 为 true 时忽略缓存，强制重新请求模型。
	Refresh bool `json:"refresh"`
}

// AnalyzeSlowSQL 让大模型分析慢 SQL 并给出优化方案（复用日志分析的缓存与结果结构）。
func (s *AIService) AnalyzeSlowSQL(ctx context.Context, req AnalyzeSlowSQLRequest) (*AnalyzeLogsResult, error) {
	cfg, err := s.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Configured() {
		return nil, ErrInvalid.New("AI 未启用或配置不完整，请先在「系统设置 → AI 设置」中完成配置")
	}
	if s.SlowSQLs == nil {
		return nil, ErrInvalid.New("当前模式不支持慢 SQL 分析")
	}
	if len(req.SlowSQLIds) == 0 {
		return nil, ErrInvalid.New("请至少选择一条慢 SQL 记录")
	}
	if len(req.SlowSQLIds) > maxAnalyzeSlowSQL {
		return nil, ErrInvalid.New("单次最多分析 %d 条慢 SQL", maxAnalyzeSlowSQL)
	}

	// 分析请求提示词与输出都很长，超时至少按 minAnalyzeTimeoutSeconds 执行（不改动用户配置）
	if cfg.TimeoutSeconds < minAnalyzeTimeoutSeconds {
		cfg.TimeoutSeconds = minAnalyzeTimeoutSeconds
	}

	question := strings.TrimSpace(req.Question)
	if question == "" {
		question = defaultSlowSQLQuestion
	}

	cacheKey := ""
	if s.Cache != nil && !req.Refresh {
		cacheKey = s.Cache.Key(cfg, SceneSlowSQL, req.SlowSQLIds, question)
		if cached, storedAt, ok := s.Cache.Get(cacheKey); ok {
			cached.Cached = true
			cached.CachedAt = storedAt.Format("2006-01-02 15:04:05")
			return &cached, nil
		}
	}

	items := make([]model.SlowSQL, 0, len(req.SlowSQLIds))
	for _, id := range req.SlowSQLIds {
		item, findErr := s.SlowSQLs.Get(ctx, id)
		if findErr != nil {
			return nil, ErrInternal.Wrap(findErr, "查询慢 SQL 记录失败")
		}
		if item != nil {
			items = append(items, *item)
		}
	}
	if len(items) == 0 {
		return nil, ErrNotFound.New("未找到可分析的慢 SQL 记录")
	}

	start := time.Now()
	outcome, err := s.chat(ctx, cfg, []chatMessage{
		{Role: "system", Content: cfg.SystemPrompt},
		{Role: "user", Content: buildSlowSQLPrompt(items, question)},
	})
	if err != nil {
		return nil, err
	}

	result := &AnalyzeLogsResult{
		Content:      outcome.Content,
		Model:        outcome.Model,
		ElapsedMs:    time.Since(start).Milliseconds(),
		LogCount:     len(items),
		Usage:        outcome.Usage,
		Truncated:    outcome.Truncated,
		FinishReason: outcome.FinishReason,
	}
	if s.Cache != nil && strings.TrimSpace(result.Content) != "" {
		s.Cache.Set(cacheKey, SceneSlowSQL, *result, req.SlowSQLIds, time.Duration(cfg.CacheMinutes)*time.Minute)
	}
	return result, nil
}

// CachedSlowSQLResult 按单条慢 SQL 查询服务端缓存的研判结果（多端查看）。
func (s *AIService) CachedSlowSQLResult(ctx context.Context, id int64) (*AnalyzeLogsResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.Cache == nil {
		return nil, ErrNotFound.New("暂无分析结果")
	}

	result, storedAt, ok := s.Cache.FindByObject(SceneSlowSQL, id)
	if !ok {
		return nil, ErrNotFound.New("该慢 SQL 暂无分析结果（可能已超过缓存有效期，请重新分析）")
	}

	result.Cached = true
	result.CachedAt = storedAt.Format("2006-01-02 15:04:05")
	return &result, nil
}

// buildSlowSQLPrompt 把慢 SQL 记录拼装为分析提示词。
func buildSlowSQLPrompt(items []model.SlowSQL, question string) string {
	var builder strings.Builder
	builder.WriteString("以下是数据库慢查询记录：\n\n")

	for index, item := range items {
		builder.WriteString(fmt.Sprintf("### 慢 SQL %d\n", index+1))
		builder.WriteString(fmt.Sprintf("- 执行时间: %s\n", item.OccurredAt.Format("2006-01-02 15:04:05")))
		builder.WriteString(fmt.Sprintf("- 耗时: %d ms\n", item.DurationMs))
		builder.WriteString(fmt.Sprintf("- 数据库: %s\n", fallback(item.Database, "未知")))
		builder.WriteString(fmt.Sprintf("- 操作类型: %s\n", fallback(item.Operation, "未知")))
		builder.WriteString(fmt.Sprintf("- 来源: %s\n", slowSQLSourceLabel(item.Source)))
		if item.Rows > 0 {
			builder.WriteString(fmt.Sprintf("- 影响行数: %d\n", item.Rows))
		}
		if item.ClientIp != "" {
			builder.WriteString(fmt.Sprintf("- 客户端 IP: %s\n", item.ClientIp))
		}
		builder.WriteString("- SQL:\n```sql\n")
		builder.WriteString(truncateText(item.Statement, 8000))
		builder.WriteString("\n```\n\n")
	}

	builder.WriteString("请按以下结构输出 Markdown：\n")
	builder.WriteString("1. **结论**：这条 SQL 慢在哪（一句话判断，例如缺索引 / 全表扫描 / 隐式类型转换 / 排序临时表 / 大事务）\n")
	builder.WriteString("2. **原因分析**：结合 WHERE / JOIN / ORDER BY / LIMIT 说明为什么慢，指出可能被扫的行数与选择性问题\n")
	builder.WriteString("3. **优化方案**：给出可直接执行的建议，包括\n")
	builder.WriteString("   - 索引：给出具体的 `CREATE INDEX` 语句（列顺序要说明理由）\n")
	builder.WriteString("   - SQL 改写：改写后的语句（避免 SELECT *、去掉函数包裹字段、拆分大 IN、改为覆盖索引 / 延迟关联等）\n")
	builder.WriteString("   - 业务侧：分页/缓存/异步/归档等\n")
	builder.WriteString("4. **风险提示**：加索引或改写的副作用（写放大、锁、DDL 影响、回滚方案）\n")
	builder.WriteString("5. **验证方式**：用 EXPLAIN / 压测 / 监控指标确认优化效果\n\n")
	builder.WriteString("如果信息不足（例如看不出表结构），请明确指出还缺什么信息，不要编造字段名。\n\n")
	builder.WriteString("附加问题：")
	builder.WriteString(strings.TrimSpace(question))
	return builder.String()
}

// slowSQLSourceLabel 返回来源的中文说明。
func slowSQLSourceLabel(source string) string {
	if source == model.SlowSQLSourceAgent {
		return "外部上报"
	}
	return "后台自身"
}
