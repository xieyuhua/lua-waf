// Package ratelimit 提供轻量的内存限流器，用于保护登录等敏感入口。
package ratelimit

import (
	"sync"
	"time"
)

// Limiter 是滑动窗口计数器限流器。
type Limiter struct {
	// Limit 为窗口内允许的调用次数。
	Limit int
	// Window 为窗口长度。
	Window time.Duration

	mu   sync.Mutex
	hits map[string][]time.Time
}

// Allow 判断某个键是否仍在配额内，并记录本次调用。
func (l *Limiter) Allow(key string) bool {
	limit, window := l.policy()
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.hits == nil {
		l.hits = make(map[string][]time.Time, 16)
	}

	recent := l.hits[key][:0]
	for _, at := range l.hits[key] {
		if now.Sub(at) < window {
			recent = append(recent, at)
		}
	}
	if len(recent) >= limit {
		l.hits[key] = recent
		return false
	}
	l.hits[key] = append(recent, now)
	return true
}

// Reset 清空某个键的计数，例如登录成功后解除限制。
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, key)
}

// policy 返回生效的限流参数，默认 10 次 / 5 分钟。
func (l *Limiter) policy() (int, time.Duration) {
	limit, window := l.Limit, l.Window
	if limit <= 0 {
		limit = 10
	}
	if window <= 0 {
		window = 5 * time.Minute
	}
	return limit, window
}
