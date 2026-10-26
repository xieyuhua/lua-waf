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

// Open 建立连接、同步表结构并装配各业务仓储。
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

	// 表结构以代码为准自动同步；waf_log 由 openresty 侧维护，不在此同步。
	if err := engine.Sync2(new(model.Site), new(model.Rule), new(model.User), new(model.SlowSQL), new(model.Setting)); err != nil {
		_ = engine.Close()
		return fmt.Errorf("同步表结构失败: %w", err)
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

// Repos 返回仓储集合。
func (s *Store) Repos() repository.Repositories { return s.Repositories }

// Close 关闭数据库连接。
func (s *Store) Close() error {
	if s.Engine == nil {
		return nil
	}
	return s.Engine.Close()
}
