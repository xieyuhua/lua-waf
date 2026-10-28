// Package mysql 提供基于 xorm 的 MySQL 仓储实现。
package mysql

import (
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/go-xorm/xorm"
	"xorm.io/core"

	"wafadmin/internal/config"
	"wafadmin/internal/model"
	"wafadmin/internal/repository"
	"wafadmin/pkg/sqlwatch"
)

// Store 是基于 MySQL 的仓储聚合实现。
type Store struct {
	repository.Repositories

	// Engine 为 xorm 引擎，仅在需要自定义 SQL 时由上层显式使用。
	Engine *xorm.Engine
	// Observer 用于采集后台自身的慢 SQL，可在装配后注入 Recorder。
	Observer *sqlwatch.Observer

	database string
}

// tableSpec 描述一张需要确保存在的表。
type tableSpec struct {
	name string
	bean any
}

// tables 返回后台自建表清单（waf_log 由 openresty 侧维护，不在此列）。
func (s *Store) tables() []tableSpec {
	return []tableSpec{
		{name: "site", bean: new(model.Site)},
		{name: "rules", bean: new(model.Rule)},
		{name: "user", bean: new(model.User)},
		{name: "slow_sql", bean: new(model.SlowSQL)},
		{name: "setting", bean: new(model.Setting)},
	}
}

// Open 建立连接、确保表存在并装配各业务仓储。
func (s *Store) Open(database config.Database) error {
	engine, err := xorm.NewEngine("mysql", database.DSN())
	if err != nil {
		return fmt.Errorf("初始化 MySQL 引擎失败: %w", err)
	}
	engine.SetMaxOpenConns(32)
	engine.SetMaxIdleConns(8)
	engine.SetConnMaxLifetime(time.Hour)
	// SQL 日志：带参数的语句 + 执行耗时，由 database.SHOW_SQL 控制（可热切换）
	engine.SetLogger(&sqlLogger{level: core.LOG_INFO, showSQL: database.ShowSQL})
	engine.ShowSQL(database.ShowSQL)
	engine.ShowExecTime(database.ShowSQL)
	if database.ShowSQL {
		log.Println("[info] 已开启 SQL 日志（database.SHOW_SQL = true），每条 SQL 都会打印语句与耗时")
	}
	if err := engine.Ping(); err != nil {
		_ = engine.Close()
		return fmt.Errorf("连接 MySQL 失败: %w", err)
	}

	if err := s.ensureTables(engine, database); err != nil {
		_ = engine.Close()
		return err
	}
	// 补建查询索引（幂等；避免日志量大时全表扫描导致接口超时）
	if err := s.ensureIndexes(engine); err != nil {
		log.Printf("[warn] 索引检查失败: %v", err)
	}

	s.Engine = engine
	s.database = database.Name
	s.Observer = &sqlwatch.Observer{Database: database.Name}

	s.Site = &siteRepo{engine: engine, observer: s.Observer}
	s.Rule = &ruleRepo{engine: engine, observer: s.Observer}
	s.User = &userRepo{engine: engine, observer: s.Observer}
	s.Log = &logRepo{engine: engine, observer: s.Observer}
	s.SlowSQL = &slowSQLRepo{engine: engine, observer: s.Observer}
	s.Setting = &settingRepo{engine: engine, observer: s.Observer}

	if exists, err := engine.IsTableExist(new(model.WafLog)); err == nil && !exists {
		log.Println("[warn] 未找到 waf_log 表，攻击日志相关页面将为空；该表由 openresty 侧写入")
	}
	return nil
}

// logIndexes 是 waf_log 上需要的索引（缺失会补建）。
//
// 为什么需要：waf_log 在生产上常是百万级，日志列表/概览/地图都按 local_time、attack_type、
// client_ip 过滤或聚合；缺索引时每次查询都是全表扫描（实测 20~45 秒），
// 前端等不及就会超时并报「查询日志失败 / 统计攻击类型失败」。
var logIndexes = []struct {
	Name   string
	Column string
}{
	{Name: "idx_waf_log_local_time", Column: "local_time"},
	{Name: "idx_waf_log_attack_type", Column: "attack_type"},
	{Name: "idx_waf_log_client_ip", Column: "client_ip"},
	{Name: "idx_waf_log_ip_time", Column: "client_ip, local_time"},
	{Name: "idx_waf_log_server_name", Column: "server_name"},
}

// ensureIndexes 为 waf_log 补齐索引（幂等：已存在则跳过）。
// 只在表已存在时执行，不影响首次建表；建索引期间会打印日志，大表可能需要几十秒。
func (s *Store) ensureIndexes(engine *xorm.Engine) error {
	exists, err := engine.IsTableExist(new(model.WafLog))
	if err != nil || !exists {
		return nil
	}

	for _, item := range logIndexes {
		rows, err := engine.QueryString("SHOW INDEX FROM waf_log WHERE Key_name = ?", item.Name)
		if err != nil {
			return fmt.Errorf("检查索引 %s 失败: %w", item.Name, err)
		}
		if len(rows) > 0 {
			continue
		}

		log.Printf("[info] 为 waf_log 补建索引 %s(%s)，大表可能需要一些时间…", item.Name, item.Column)
		if _, err := engine.Exec(fmt.Sprintf("ALTER TABLE waf_log ADD INDEX %s (%s)", item.Name, item.Column)); err != nil {
			log.Printf("[warn] 补建索引 %s 失败（可手工创建）：%v", item.Name, err)
			continue
		}
		log.Printf("[info] 索引 %s 已创建", item.Name)
	}
	return nil
}

// ensureTables 保证必要的表存在。
// 已存在的表默认**不做任何结构同步**（避免 ALTER 改动线上表结构，也避免
// “Multiple primary key defined” 这类同步错误）；只有表不存在时才创建。
// 当 database.ADD_MISSING_COLUMNS = true 时，对已有表只**补充缺失列**（xorm Sync 语义：只增不改删）。
func (s *Store) ensureTables(engine *xorm.Engine, database config.Database) error {
	for _, item := range s.tables() {
		exists, err := engine.IsTableExist(item.bean)
		if err != nil {
			return fmt.Errorf("检查表 %s 是否存在失败: %w", item.name, err)
		}
		if exists {
			log.Printf("[info] 表 %s 已存在，跳过结构同步", item.name)
			if database.AddMissingColumns {
				if syncErr := engine.Sync(item.bean); syncErr != nil {
					log.Printf("[warn] 表 %s 补充缺失列失败: %v", item.name, syncErr)
				} else {
					log.Printf("[info] 表 %s 已补充缺失列（只增不改）", item.name)
				}
			}
			s.warnLegacyColumn(engine, item.name)
			continue
		}
		if !database.AutoMigrate {
			return fmt.Errorf("表 %s 不存在，且已关闭自动建表（database.AUTO_MIGRATE = false），请手工建表", item.name)
		}
		if err := engine.Sync2(item.bean); err != nil {
			return fmt.Errorf("创建表 %s 失败: %w", item.name, err)
		}
		log.Printf("[info] 已创建表 %s", item.name)
	}
	return nil
}

// warnLegacyColumn 检测历史误建列：早期版本把 xorm 标签写成 autoincrement
// （xorm 只认 autoincr），导致该词被当作列名，主键自增失效。
// 这里只告警并给出修复语句，不自动改线上表结构。
func (s *Store) warnLegacyColumn(engine *xorm.Engine, table string) {
	rows, err := engine.QueryString("SHOW COLUMNS FROM `" + table + "` LIKE 'autoincrement'")
	if err != nil || len(rows) == 0 {
		return
	}
	log.Printf("[warn] 表 %s 存在历史误建列 autoincrement（主键没有自增），建议执行：", table)
	log.Printf("[warn]   ALTER TABLE `%s` CHANGE COLUMN `autoincrement` `id` BIGINT NOT NULL AUTO_INCREMENT", table)
}

// Repos 返回仓储集合。
func (s *Store) Repos() repository.Repositories { return s.Repositories }

// ApplyShowSQL 运行时开关 SQL 日志（供后台「运行配置」热切换，无需重启）。
func (s *Store) ApplyShowSQL(enabled bool) {
	if s.Engine == nil {
		return
	}
	s.Engine.ShowSQL(enabled)
	s.Engine.ShowExecTime(enabled)
	s.Engine.Logger().SetLevel(core.LOG_INFO)
	s.Engine.Logger().ShowSQL(enabled)
	log.Printf("[info] SQL 日志已%s", map[bool]string{true: "开启", false: "关闭"}[enabled])
}

// Close 关闭数据库连接。
func (s *Store) Close() error {
	if s.Engine == nil {
		return nil
	}
	return s.Engine.Close()
}
