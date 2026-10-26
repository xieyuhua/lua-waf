package memory

import (
	"context"
	"sort"
)

// settingRepo 是 SettingRepository 的内存实现。
type settingRepo struct {
	store *Store
}

// All 返回全部配置项。
func (r *settingRepo) All(ctx context.Context) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	out := make(map[string]string, len(r.store.settings))
	for key, value := range r.store.settings {
		out[key] = value
	}
	return out, nil
}

// Get 读取单个配置项。
func (r *settingRepo) Get(ctx context.Context, key string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	value, ok := r.store.settings[key]
	return value, ok, nil
}

// Set 写入或覆盖配置项。
func (r *settingRepo) Set(ctx context.Context, key, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	if r.store.settings == nil {
		r.store.settings = make(map[string]string, 8)
	}
	r.store.settings[key] = value
	return nil
}

// Delete 删除配置项。
func (r *settingRepo) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()
	delete(r.store.settings, key)
	return nil
}

// Keys 返回已配置的键名（用于演示数据初始化与测试）。
func (r *settingRepo) Keys(ctx context.Context) ([]string, error) {
	all, err := r.All(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(all))
	for key := range all {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}
