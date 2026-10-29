package mysql

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/go-xorm/xorm"

	"wafadmin/internal/model"
	"wafadmin/internal/repository"
	"wafadmin/pkg/sqlwatch"
)

// logRepo 是 LogRepository 的 MySQL 实现。
type logRepo struct {
	engine   *xorm.Engine
	observer *sqlwatch.Observer
}

// Get 按主键查询单条日志。
func (r *logRepo) Get(ctx context.Context, id int64) (*model.WafLog, error) {
	defer r.observer.Trace(ctx, "log.get", "SELECT * FROM waf_log WHERE id = ?")()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	item := new(model.WafLog)
	has, err := r.engine.Id(id).Get(item)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, nil
	}
	return item, nil
}

// BatchCreate 批量写入攻击日志（一次 INSERT 多行）。
func (r *logRepo) BatchCreate(ctx context.Context, items []model.WafLog) (int64, error) {
	defer r.observer.Trace(ctx, "log.batch_create", "INSERT INTO waf_log (...) VALUES (...),(...)")()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(items) == 0 {
		return 0, nil
	}

	// xorm 支持传切片指针做批量插入
	batch := make([]model.WafLog, len(items))
	copy(batch, items)
	affected, err := r.engine.Insert(&batch)
	if err != nil {
		return 0, err
	}
	return affected, nil
}

// List 按筛选条件分页查询日志，按 ID 倒序返回。
func (r *logRepo) List(ctx context.Context, q model.LogQuery) ([]model.WafLog, int64, error) {
	defer r.observer.Trace(ctx, "log.list", "SELECT * FROM waf_log WHERE ... ORDER BY id DESC LIMIT ? OFFSET ?")()
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}

	where, args := r.buildFilter(q)

	// 注意：xorm 的 Count 执行后会重置 session 上的 WHERE 条件（queryRows 里 defer resetStatement），
	// 因此统计与列表必须使用两个独立 session，否则列表会退化为全量查询。
	countSession := r.engine.NewSession()
	defer countSession.Close()
	if where != "" {
		countSession = countSession.Where(where, args...)
	}
	total, err := countSession.Count(new(model.WafLog))
	if err != nil {
		return nil, 0, err
	}

	listSession := r.engine.NewSession()
	defer listSession.Close()
	if where != "" {
		listSession = listSession.Where(where, args...)
	}
	items := make([]model.WafLog, 0, q.Size)
	if err := listSession.Desc("id").Limit(q.Size, q.Offset()).Find(&items); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// TagSamples 返回某条命中规则最近的日志样例。
func (r *logRepo) TagSamples(ctx context.Context, tag string, days int, limit int) ([]model.WafLog, error) {
	if limit <= 0 || limit > 50 {
		limit = 5
	}

	session := r.engine.NewSession().Where("rule_tag = ?", tag)
	if days > 0 {
		session = session.And("local_time >= ?", time.Now().AddDate(0, 0, -days))
	}
	session = session.Desc("id").Limit(limit)
	defer session.Close()

	defer r.observer.Trace(ctx, "log.tag_samples", "SELECT * FROM waf_log WHERE rule_tag = ? ORDER BY id DESC LIMIT ?")()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	items := make([]model.WafLog, 0, limit)
	if err := session.Find(&items); err != nil {
		return nil, err
	}
	return items, nil
}

// TagStats 按命中规则聚合最近 days 天的日志。
func (r *logRepo) TagStats(ctx context.Context, days int, limit int) ([]model.TagStat, error) {
	if limit <= 0 || limit > 200 {
		limit = 30
	}
	sqlText := "SELECT rule_tag, COUNT(*) AS total FROM `waf_log` WHERE rule_tag <> ''"
	args := make([]any, 0, 1)
	if days > 0 {
		sqlText += " AND local_time >= ?"
		args = append(args, time.Now().AddDate(0, 0, -days))
	}
	sqlText += " GROUP BY rule_tag ORDER BY total DESC LIMIT ?"
	args = append(args, limit)

	defer r.observer.Trace(ctx, "log.tag_stats", sqlText)()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	rows, err := r.engine.SQL(sqlText, args...).QueryString()
	if err != nil {
		return nil, err
	}

	stats := make([]model.TagStat, 0, len(rows))
	for _, row := range rows {
		count, _ := strconv.ParseInt(row["total"], 10, 64)
		stats = append(stats, model.TagStat{RuleTag: row["rule_tag"], Count: count})
	}
	return stats, nil
}

// Count 返回日志总数。
func (r *logRepo) Count(ctx context.Context) (int64, error) {
	defer r.observer.Trace(ctx, "log.count", "SELECT COUNT(*) FROM waf_log")()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.engine.Count(new(model.WafLog))
}

// CountOnDay 返回指定日期的日志条数。
func (r *logRepo) CountOnDay(ctx context.Context, day string) (int64, error) {
	defer r.observer.Trace(ctx, "log.count_day", "SELECT COUNT(*) FROM waf_log WHERE local_time LIKE ?")()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.engine.Where("local_time LIKE ?", day+"%").Count(new(model.WafLog))
}

// DistinctAttackTypes 返回出现过的攻击类型（供筛选项使用，走 attack_type 索引）。
func (r *logRepo) DistinctAttackTypes(ctx context.Context) ([]string, error) {
	defer r.observer.Trace(ctx, "log.distinct_attack_types", "SELECT DISTINCT attack_type FROM waf_log")()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	rows, err := r.engine.QueryString("SELECT DISTINCT attack_type FROM waf_log WHERE attack_type <> '' ORDER BY attack_type ASC")
	if err != nil {
		return nil, err
	}

	types := make([]string, 0, len(rows))
	for _, row := range rows {
		if value := strings.TrimSpace(row["attack_type"]); value != "" {
			types = append(types, value)
		}
	}
	return types, nil
}

// IPStats 按来源 IP 聚合攻击次数。
func (r *logRepo) IPStats(ctx context.Context, filter repository.StatsFilter) ([]model.IPStat, error) {
	defer r.observer.Trace(ctx, "log.ip_stats", "SELECT client_ip, area, COUNT(*) AS num FROM waf_log GROUP BY client_ip")()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 两段式查询：先只按 client_ip 聚合取 Top N（可用 client_ip 索引），
	// 再对这一小批 IP 回表取地区与攻击类型——避免在百万行上做 GROUP_CONCAT。
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}

	// 未指定日期时默认只统计近 30 天：源站 IP 排行榜看“最近谁在打”即可，
	// 且能用上 local_time 索引，避免百万行全表 GROUP BY。
	since := ""
	if filter.Day != "" {
		since = "local_time LIKE '" + filter.Day + "%'"
	} else {
		since = "local_time >= '" + time.Now().AddDate(0, 0, -30).Format("2006-01-02") + " 00:00:00'"
	}

	inner := "SELECT client_ip, COUNT(*) AS num FROM waf_log WHERE " + since + " "
	innerArgs := make([]any, 0, 2)
	inner += "GROUP BY client_ip ORDER BY num DESC LIMIT ?"
	innerArgs = append(innerArgs, limit)

	query := "SELECT t.client_ip, MAX(l.area) AS area, MAX(t.num) AS num, " +
		"GROUP_CONCAT(DISTINCT l.attack_type) AS attack_type " +
		"FROM (" + inner + ") t JOIN waf_log l ON l.client_ip = t.client_ip "
	jargs := make([]any, 0, len(innerArgs)+2)
	jargs = append(jargs, innerArgs...)
	query += "WHERE l." + since + " GROUP BY t.client_ip ORDER BY num DESC"
	args := jargs

	stats := make([]model.IPStat, 0, 32)
	if err := r.engine.SQL(query, args...).Find(&stats); err != nil {
		return nil, err
	}
	return stats, nil
}

// TypeStats 按攻击类型聚合。
func (r *logRepo) TypeStats(ctx context.Context, filter repository.StatsFilter) ([]model.TypeStat, error) {
	defer r.observer.Trace(ctx, "log.type_stats", "SELECT attack_type, COUNT(*) AS num FROM waf_log GROUP BY attack_type")()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 注意：不要在这里 GROUP_CONCAT(area)——area 取值上千种，在百万行日志上会退化成
	// 全表扫描 + 临时表去重（实测 20s+）。类型统计只需要类型与次数，地区由地图/概览单独统计。
	query := "SELECT attack_type, COUNT(*) AS num FROM waf_log "
	args := make([]any, 0, 2)
	if filter.Day != "" {
		query += "WHERE local_time LIKE ? "
		args = append(args, filter.Day+"%")
	}
	query += "GROUP BY attack_type ORDER BY num DESC"
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}

	stats := make([]model.TypeStat, 0, 16)
	if err := r.engine.SQL(query, args...).Find(&stats); err != nil {
		return nil, err
	}
	return stats, nil
}

// DateTrend 返回最近 days 天的每日攻击量，按日期升序并补齐空缺日期。
func (r *logRepo) DateTrend(ctx context.Context, days int) ([]model.DateStat, error) {
	defer r.observer.Trace(ctx, "log.date_trend", "SELECT substring_index(local_time, ' ', 1) AS date, COUNT(*) AS num FROM waf_log GROUP BY date")()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if days <= 0 {
		days = 15
	}

	today := time.Now()
	from := today.AddDate(0, 0, -(days-1)).Format("2006-01-02") + " 00:00:00"
	rows := make([]model.DateStat, 0, days)
	query := "SELECT substring_index(local_time, ' ', 1) AS date, COUNT(*) AS num FROM waf_log " +
		"WHERE local_time >= ? GROUP BY date ORDER BY date ASC"
	if err := r.engine.SQL(query, from).Find(&rows); err != nil {
		return nil, err
	}

	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.Date] = row.Num
	}

	stats := make([]model.DateStat, 0, days)
	for offset := days - 1; offset >= 0; offset-- {
		day := today.AddDate(0, 0, -offset).Format("2006-01-02")
		stats = append(stats, model.DateStat{Date: day, Num: counts[day]})
	}
	return stats, nil
}

// buildFilter 把查询条件转换为 SQL 片段与参数。
func (r *logRepo) buildFilter(q model.LogQuery) (string, []any) {
	conditions := make([]string, 0, 6)
	args := make([]any, 0, 12)

	if q.AttackType != "" {
		conditions = append(conditions, "attack_type = ?")
		args = append(args, q.AttackType)
	}
	if q.ClientIp != "" {
		conditions = append(conditions, "client_ip LIKE ?")
		args = append(args, "%"+q.ClientIp+"%")
	}
	if q.ServerName != "" {
		conditions = append(conditions, "server_name = ?")
		args = append(args, q.ServerName)
	}
	if q.Day != "" {
		conditions = append(conditions, "local_time LIKE ?")
		args = append(args, q.Day+"%")
	}
	if q.From != "" {
		conditions = append(conditions, "local_time >= ?")
		args = append(args, q.From)
	}
	if q.To != "" {
		conditions = append(conditions, "local_time <= ?")
		args = append(args, q.To)
	}
	if q.Keyword != "" {
		conditions = append(conditions, "(client_ip LIKE ? OR req_url LIKE ? OR user_agent LIKE ? OR attack_type LIKE ? OR rule_tag LIKE ? OR server_name LIKE ?)")
		like := "%" + q.Keyword + "%"
		for i := 0; i < 6; i++ {
			args = append(args, like)
		}
	}
	return strings.Join(conditions, " AND "), args
}

// PurgeBefore 删除 local_time 早于 before 的日志（local_time 是字符串，按字典序比较即可）。
//
// 用途：数据保留策略。建议先用 mysqldump 归档再删除，避免历史数据不可追溯。
func (r *logRepo) PurgeBefore(ctx context.Context, before string) (int64, error) {
	defer r.observer.Trace(ctx, "log.purge_before", "DELETE FROM waf_log WHERE local_time < ?")()
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	result, err := r.engine.Exec("DELETE FROM waf_log WHERE local_time < ? LIMIT 50000", before)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return affected, nil
}
