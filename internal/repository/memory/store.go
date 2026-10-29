package memory

import (
	"sync"
	"time"

	"wafadmin/internal/model"
	"wafadmin/internal/repository"
)

// Store 是基于内存的仓储实现，用于演示模式（RUN_MODE=demo）与单元测试。
// 它不依赖 MySQL，便于快速体验后台功能。
type Store struct {
	repository.Repositories

	mu sync.RWMutex

	siteSeq int64
	ruleSeq int64
	userSeq int64
	logSeq  int64
	slowSeq int64
	rateSeq int64

	sites      []model.Site
	rules      []model.Rule
	users      []model.User
	logs       []model.WafLog
	slows      []model.SlowSQL
	rateLimits []model.RateLimitRule

	settings map[string]string

	// now 便于测试注入固定时间。
	now func() time.Time
}

// Init 装配各业务仓储，seed 为 true 时写入演示数据。
func (s *Store) Init(seed bool) *Store {
	s.now = time.Now
	s.Site = &siteRepo{store: s}
	s.Rule = &ruleRepo{store: s}
	s.User = &userRepo{store: s}
	s.Log = &logRepo{store: s}
	s.SlowSQL = &slowSQLRepo{store: s}
	s.Setting = &settingRepo{store: s}
	s.RateLimit = &rateLimitRepo{store: s}
	s.settings = make(map[string]string, 8)
	if seed {
		s.seedDemo()
	}
	return s
}

// Repos 返回可注入服务层的仓储集合。
func (s *Store) Repos() repository.Repositories { return s.Repositories }

// clock 返回当前时间。
func (s *Store) clock() time.Time { return s.now() }

// pager 为内存切片提供与 MySQL 一致的分页语义。
type pager[T any] struct {
	items []T
}

// Slice 返回当前页数据。
func (p pager[T]) Slice(q model.ListQuery) []T {
	start := q.Offset()
	if start < 0 {
		start = 0
	}
	if start >= len(p.items) {
		return []T{}
	}
	end := start + q.Size
	if end > len(p.items) {
		end = len(p.items)
	}
	out := make([]T, end-start)
	copy(out, p.items[start:end])
	return out
}
