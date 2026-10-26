package memory

import (
	"context"
	"sort"
	"strings"

	"wafadmin/internal/model"
)

// siteRepo 是 SiteRepository 的内存实现。
type siteRepo struct {
	store *Store
}

// List 按关键词分页查询站点。
func (r *siteRepo) List(ctx context.Context, q model.ListQuery) ([]model.Site, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	keyword := strings.ToLower(q.Keyword)
	items := make([]model.Site, 0, len(r.store.sites))
	for _, item := range r.store.sites {
		if keyword != "" && !strings.Contains(strings.ToLower(item.SiteName), keyword) {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Id < items[j].Id })
	return pager[model.Site]{items: items}.Slice(q), int64(len(items)), nil
}

// All 返回全部站点。
func (r *siteRepo) All(ctx context.Context) ([]model.Site, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	items := make([]model.Site, len(r.store.sites))
	copy(items, r.store.sites)
	sort.Slice(items, func(i, j int) bool { return items[i].Id < items[j].Id })
	return items, nil
}

// Get 按主键查询站点。
func (r *siteRepo) Get(ctx context.Context, id int64) (*model.Site, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	for _, item := range r.store.sites {
		if item.Id == id {
			found := item
			return &found, nil
		}
	}
	return nil, nil
}

// ExistsByName 判断域名是否已被占用。
func (r *siteRepo) ExistsByName(ctx context.Context, name string, excludeId int64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	for _, item := range r.store.sites {
		if item.Id != excludeId && strings.EqualFold(item.SiteName, name) {
			return true, nil
		}
	}
	return false, nil
}

// Create 新增站点。
func (r *siteRepo) Create(ctx context.Context, site *model.Site) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	r.store.siteSeq++
	site.Id = r.store.siteSeq
	site.LastChange = r.store.clock()
	site.Version = 1
	r.store.sites = append(r.store.sites, *site)
	return nil
}

// Update 更新站点。
func (r *siteRepo) Update(ctx context.Context, site *model.Site) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	for i := range r.store.sites {
		if r.store.sites[i].Id != site.Id {
			continue
		}
		site.LastChange = r.store.clock()
		site.Version = r.store.sites[i].Version + 1
		r.store.sites[i] = *site
		return nil
	}
	return nil
}

// Delete 删除站点。
func (r *siteRepo) Delete(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	kept := make([]model.Site, 0, len(r.store.sites))
	for _, item := range r.store.sites {
		if item.Id != id {
			kept = append(kept, item)
		}
	}
	r.store.sites = kept
	return nil
}

// Count 返回站点总数。
func (r *siteRepo) Count(ctx context.Context) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	return int64(len(r.store.sites)), nil
}
