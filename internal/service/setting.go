package service

import (
	"context"
	"sync"

	"wafadmin/internal/repository"
)

// SettingService 提供键值型配置的读写，并带进程内缓存。
type SettingService struct {
	Repo repository.SettingRepository

	mu     sync.RWMutex
	cache  map[string]string
	loaded bool
}

// All 返回全部配置项。
func (s *SettingService) All(ctx context.Context) (map[string]string, error) {
	values, err := s.Repo.All(ctx)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "读取系统配置失败")
	}

	s.mu.Lock()
	s.cache = values
	s.loaded = true
	s.mu.Unlock()
	return values, nil
}

// Get 读取单个配置项，未配置时返回空串。
func (s *SettingService) Get(ctx context.Context, key string) (string, error) {
	s.mu.RLock()
	if s.loaded {
		value := s.cache[key]
		s.mu.RUnlock()
		return value, nil
	}
	s.mu.RUnlock()

	value, _, err := s.Repo.Get(ctx, key)
	if err != nil {
		return "", ErrInternal.Wrap(err, "读取系统配置失败")
	}
	return value, nil
}

// Set 写入配置并同步刷新缓存。
func (s *SettingService) Set(ctx context.Context, key, value string) error {
	if err := s.Repo.Set(ctx, key, value); err != nil {
		return ErrInternal.Wrap(err, "保存系统配置失败")
	}

	s.mu.Lock()
	if s.cache == nil {
		s.cache = make(map[string]string, 8)
	}
	s.cache[key] = value
	s.mu.Unlock()
	return nil
}

// Delete 删除配置项。
func (s *SettingService) Delete(ctx context.Context, key string) error {
	if err := s.Repo.Delete(ctx, key); err != nil {
		return ErrInternal.Wrap(err, "删除系统配置失败")
	}

	s.mu.Lock()
	delete(s.cache, key)
	s.mu.Unlock()
	return nil
}

// Invalidate 清空缓存，下次读取时回源。
func (s *SettingService) Invalidate() {
	s.mu.Lock()
	s.cache = nil
	s.loaded = false
	s.mu.Unlock()
}
