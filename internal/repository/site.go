package repository

import (
	"context"

	"wafadmin/internal/model"
)

// SiteRepository 定义站点（反向代理后端）的持久化契约。
type SiteRepository interface {
	// List 按关键词分页查询站点。
	List(ctx context.Context, q model.ListQuery) ([]model.Site, int64, error)
	// All 返回全部站点，用于批量同步。
	All(ctx context.Context) ([]model.Site, error)
	// Get 按主键查询站点。
	Get(ctx context.Context, id int64) (*model.Site, error)
	// ExistsByName 判断域名是否已被占用。
	ExistsByName(ctx context.Context, name string, excludeId int64) (bool, error)
	// Create 新增站点。
	Create(ctx context.Context, site *model.Site) error
	// Update 更新站点。
	Update(ctx context.Context, site *model.Site) error
	// Delete 删除站点。
	Delete(ctx context.Context, id int64) error
	// Count 返回站点总数。
	Count(ctx context.Context) (int64, error)
}
