package app

import (
	"log"

	"wafadmin/internal/config"
	"wafadmin/internal/repository"
	"wafadmin/internal/repository/memory"
	"wafadmin/internal/repository/mysql"
	"wafadmin/pkg/sqlwatch"
)

// Store 是按运行模式打开的仓储集合，供服务与命令行子命令共用。
type Store struct {
	// Repos 为各业务仓储。
	Repos repository.Repositories
	// MySQL 为 MySQL 仓储实例，演示模式为 nil。
	MySQL *mysql.Store
	// Observer 为慢 SQL 观察器，演示模式为 nil。
	Observer *sqlwatch.Observer

	closers []func() error
}

// OpenStore 按运行模式打开仓储：demo 使用内存仓储并载入演示数据，其余连接 MySQL。
func OpenStore(cfg *config.Config) (*Store, error) {
	return openStore(cfg, false)
}

// OpenStoreQuiet 与 OpenStore 相同，但屏蔽「表已存在/补充缺失列」这类结构同步日志。
//
// 命令行子命令用它，避免迁移日志淹没真正的输出（如 ruletest 报告）。
func OpenStoreQuiet(cfg *config.Config) (*Store, error) {
	return openStore(cfg, true)
}

// openStore 是 OpenStore / OpenStoreQuiet 的共同实现。
func openStore(cfg *config.Config, silent bool) (*Store, error) {
	if cfg.IsDemo() {
		store := (&memory.Store{}).Init(true)
		if !silent {
			log.Println("[info] 已启用内存仓储并载入演示数据")
		}
		return &Store{Repos: store.Repos()}, nil
	}

	instance := &mysql.Store{Silent: silent}
	if err := instance.Open(cfg.Database); err != nil {
		return nil, err
	}
	return &Store{
		Repos:    instance.Repos(),
		MySQL:    instance,
		Observer: instance.Observer,
		closers:  []func() error{instance.Close},
	}, nil
}

// Close 释放仓储持有的外部资源。
func (s *Store) Close() error {
	var firstErr error
	for _, closer := range s.closers {
		if err := closer(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
