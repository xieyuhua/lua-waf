package service

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 缓存条目上限，超过时清理过期项、仍超限则丢弃最早写入的条目。
const analyzeCacheLimit = 256

// AnalyzeCache 是 AI 分析结果的进程内缓存。
//
// 相同参数（模型 + 系统提示词 + 日志集合 + 追加提问）在有效期内直接复用结果，
// 避免重复消耗 token；默认有效期 10 分钟（可在「AI 设置」中调整）。
type AnalyzeCache struct {
	// Now 便于测试注入时间，为空时使用 time.Now。
	Now func() time.Time

	mu      sync.Mutex
	entries map[string]analyzeCacheEntry
}

// analyzeCacheEntry 是一条缓存记录。
type analyzeCacheEntry struct {
	result    AnalyzeLogsResult
	storedAt  time.Time
	expiresAt time.Time
}

// now 返回当前时间。
func (c *AnalyzeCache) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Key 生成缓存键：模型、系统提示词、日志 ID 集合、追问任一变化都视为不同请求。
func (c *AnalyzeCache) Key(cfg AIConfig, logIDs []int64, question string) string {
	ids := make([]int64, len(logIDs))
	copy(ids, logIDs)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	parts := make([]string, 0, len(ids)+3)
	parts = append(parts, cfg.BaseURL, cfg.Model, cfg.SystemPrompt)
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	parts = append(parts, strings.TrimSpace(question))

	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:16])
}

// Get 读取缓存，返回结果、写入时间与是否命中。
func (c *AnalyzeCache) Get(key string) (AnalyzeLogsResult, time.Time, bool) {
	if key == "" {
		return AnalyzeLogsResult{}, time.Time{}, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return AnalyzeLogsResult{}, time.Time{}, false
	}
	if !c.now().Before(entry.expiresAt) {
		delete(c.entries, key)
		return AnalyzeLogsResult{}, time.Time{}, false
	}
	return entry.result, entry.storedAt, true
}

// Set 写入缓存；ttl <= 0 表示不缓存。
func (c *AnalyzeCache) Set(key string, result AnalyzeLogsResult, ttl time.Duration) {
	if key == "" || ttl <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.entries == nil {
		c.entries = make(map[string]analyzeCacheEntry, 16)
	}

	// 缓存内容随结果一起返回，避免把 "来自缓存" 的标记存进缓存
	result.Cached = false
	result.CachedAt = ""

	now := c.now()
	c.entries[key] = analyzeCacheEntry{result: result, storedAt: now, expiresAt: now.Add(ttl)}
	c.evictLocked(now)
}

// Purge 清空缓存（配置变更后调用）。
func (c *AnalyzeCache) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil
}

// Size 返回当前未过期条目数。
func (c *AnalyzeCache) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	total := 0
	for _, entry := range c.entries {
		if now.Before(entry.expiresAt) {
			total++
		}
	}
	return total
}

// evictLocked 清理过期条目并在超限时丢弃最早写入的条目（调用方已持锁）。
func (c *AnalyzeCache) evictLocked(now time.Time) {
	for key, entry := range c.entries {
		if !now.Before(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
	if len(c.entries) <= analyzeCacheLimit {
		return
	}

	oldestKey, oldestAt := "", now
	for key, entry := range c.entries {
		if oldestKey == "" || entry.storedAt.Before(oldestAt) {
			oldestKey, oldestAt = key, entry.storedAt
		}
	}
	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}
