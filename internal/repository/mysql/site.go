package mysql

import (
	"context"
	"fmt"

	"github.com/go-xorm/xorm"

	"wafadmin/internal/model"
	"wafadmin/pkg/sqlwatch"
)

// siteRepo 是 SiteRepository 的 MySQL 实现。
type siteRepo struct {
	engine   *xorm.Engine
	observer *sqlwatch.Observer
}

// List 按关键词分页查询站点。
func (r *siteRepo) List(ctx context.Context, q model.ListQuery) ([]model.Site, int64, error) {
	defer r.observer.Trace(ctx, "site.list", "SELECT * FROM site WHERE site_name LIKE ? ORDER BY id LIMIT ? OFFSET ?")()
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}

	// Count 会重置 session 的 WHERE，统计与列表各用一个 session
	countSession := r.engine.NewSession()
	defer countSession.Close()
	if q.Keyword != "" {
		countSession = countSession.Where("site_name LIKE ?", "%"+q.Keyword+"%")
	}
	total, err := countSession.Count(new(model.Site))
	if err != nil {
		return nil, 0, err
	}

	listSession := r.engine.NewSession()
	defer listSession.Close()
	if q.Keyword != "" {
		listSession = listSession.Where("site_name LIKE ?", "%"+q.Keyword+"%")
	}
	items := make([]model.Site, 0, q.Size)
	if err := listSession.OrderBy("id").Limit(q.Size, q.Offset()).Find(&items); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// All 返回全部站点。
func (r *siteRepo) All(ctx context.Context) ([]model.Site, error) {
	defer r.observer.Trace(ctx, "site.all", "SELECT * FROM site")()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	items := make([]model.Site, 0, 16)
	if err := r.engine.OrderBy("id").Find(&items); err != nil {
		return nil, err
	}
	return items, nil
}

// Get 按主键查询站点。
func (r *siteRepo) Get(ctx context.Context, id int64) (*model.Site, error) {
	defer r.observer.Trace(ctx, "site.get", "SELECT * FROM site WHERE id = ?")()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	site := new(model.Site)
	has, err := r.engine.Id(id).Get(site)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, nil
	}
	return site, nil
}

// ExistsByName 判断域名是否已被占用。
func (r *siteRepo) ExistsByName(ctx context.Context, name string, excludeId int64) (bool, error) {
	defer r.observer.Trace(ctx, "site.exists", "SELECT COUNT(*) FROM site WHERE site_name = ? AND id <> ?")()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	session := r.engine.Where("site_name = ?", name)
	if excludeId > 0 {
		session = session.And("id <> ?", excludeId)
	}
	total, err := session.Count(new(model.Site))
	return total > 0, err
}

// Create 新增站点。
func (r *siteRepo) Create(ctx context.Context, site *model.Site) error {
	defer r.observer.Trace(ctx, "site.create", "INSERT INTO site (...) VALUES (...)")()
	if err := ctx.Err(); err != nil {
		return err
	}
	affected, err := r.engine.Insert(site)
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("写入站点失败")
	}
	return nil
}

// Update 更新站点，version 冲突时返回错误。
func (r *siteRepo) Update(ctx context.Context, site *model.Site) error {
	defer r.observer.Trace(ctx, "site.update", "UPDATE site SET ... WHERE id = ?")()
	if err := ctx.Err(); err != nil {
		return err
	}
	affected, err := r.engine.Id(site.Id).Update(site)
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("站点已被其他操作修改，请刷新后重试")
	}
	return nil
}

// Delete 删除站点。
func (r *siteRepo) Delete(ctx context.Context, id int64) error {
	defer r.observer.Trace(ctx, "site.delete", "DELETE FROM site WHERE id = ?")()
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := r.engine.Id(id).Delete(new(model.Site)); err != nil {
		return err
	}
	return nil
}

// Count 返回站点总数。
func (r *siteRepo) Count(ctx context.Context) (int64, error) {
	defer r.observer.Trace(ctx, "site.count", "SELECT COUNT(*) FROM site")()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.engine.Count(new(model.Site))
}
