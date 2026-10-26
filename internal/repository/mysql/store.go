// Package mysql 提供基于 xorm 的 MySQL 仓储实现。
package mysql

import (
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/go-xorm/xorm"

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
	if err := engine.Ping(); err != nil {
		_ = engine.Close()
		return fmt.Errorf("连接 MySQL 失败: %w", err)
	}

	if err := s.ensureTables(engine, database.AutoMigrate); err != nil {
		_ = engine.Close()
		return err
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

// ensureTables 保证必要的表存在。
// 已存在的表**不做任何结构同步**（避免 ALTER 改动线上表结构，也避免
// “Multiple primary key defined” 这类同步错误）；只有表不存在时才创建。
func (s *Store) ensureTables(engine *xorm.Engine, autoMigrate bool) error {
	for _, item := range s.tables() {
		exists, err := engine.IsTableExist(item.bean)
		if err != nil {
			return fmt.Errorf("检查表 %s 是否存在失败: %w", item.name, err)
		}
		if exists {
			log.Printf("[info] 表 %s 已存在，跳过结构同步", item.name)
			continue
		}
		if !autoMigrate {
			return fmt.Errorf("表 %s 不存在，且已关闭自动建表（database.AUTO_MIGRATE = false），请手工建表", item.name)
		}
		if err := engine.Sync2(item.bean); err != nil {
			return fmt.Errorf("创建表 %s 失败: %w", item.name, err)
		}
		log.Printf("[info] 已创建表 %s", item.name)
	}
	return nil
}

// Repos 返回仓储集合。
func (s *Store) Repos() repository.Repositories { return s.Repositories }

// Close 关闭数据库连接。
func (s *Store) Close() error {
	if s.Engine == nil {
		return nil
	}
	return s.Engine.Close()
}
