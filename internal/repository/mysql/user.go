package mysql

import (
	"context"
	"fmt"
	"time"

	"github.com/go-xorm/xorm"

	"wafadmin/internal/model"
	"wafadmin/pkg/sqlwatch"
)

// userRepo 是 UserRepository 的 MySQL 实现。
type userRepo struct {
	engine   *xorm.Engine
	observer *sqlwatch.Observer
}

// List 分页查询账号。
func (r *userRepo) List(ctx context.Context, q model.ListQuery) ([]model.User, int64, error) {
	defer r.observer.Trace(ctx, "user.list", "SELECT * FROM user WHERE user_name LIKE ? ORDER BY id LIMIT ? OFFSET ?")()
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}

	// Count 会重置 session 的 WHERE，统计与列表各用一个 session
	countSession := r.engine.NewSession()
	defer countSession.Close()
	if q.Keyword != "" {
		countSession = countSession.Where("user_name LIKE ?", "%"+q.Keyword+"%")
	}
	total, err := countSession.Count(new(model.User))
	if err != nil {
		return nil, 0, err
	}

	listSession := r.engine.NewSession()
	defer listSession.Close()
	if q.Keyword != "" {
		listSession = listSession.Where("user_name LIKE ?", "%"+q.Keyword+"%")
	}
	items := make([]model.User, 0, q.Size)
	if err := listSession.OrderBy("id").Limit(q.Size, q.Offset()).Find(&items); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Get 按主键查询账号。
func (r *userRepo) Get(ctx context.Context, id int64) (*model.User, error) {
	defer r.observer.Trace(ctx, "user.get", "SELECT * FROM user WHERE id = ?")()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	user := new(model.User)
	has, err := r.engine.Id(id).Get(user)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, nil
	}
	return user, nil
}

// GetByName 按用户名查询账号。
func (r *userRepo) GetByName(ctx context.Context, name string) (*model.User, error) {
	defer r.observer.Trace(ctx, "user.get_by_name", "SELECT * FROM user WHERE user_name = ?")()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	user := new(model.User)
	has, err := r.engine.Where("user_name = ?", name).Get(user)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, nil
	}
	return user, nil
}

// Create 新增账号。
func (r *userRepo) Create(ctx context.Context, user *model.User) error {
	defer r.observer.Trace(ctx, "user.create", "INSERT INTO user (user_name, password) VALUES (?, ?)")()
	if err := ctx.Err(); err != nil {
		return err
	}
	affected, err := r.engine.Insert(user)
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("写入账号失败")
	}
	return nil
}

// Update 更新账号。
func (r *userRepo) Update(ctx context.Context, user *model.User) error {
	defer r.observer.Trace(ctx, "user.update", "UPDATE user SET user_name = ?, password = ? WHERE id = ?")()
	if err := ctx.Err(); err != nil {
		return err
	}
	affected, err := r.engine.Id(user.Id).Cols("user_name", "password").Update(user)
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("账号不存在或内容未变化")
	}
	return nil
}

// TouchLogin 记录最近登录时间。
func (r *userRepo) TouchLogin(ctx context.Context, id int64, at time.Time) error {
	defer r.observer.Trace(ctx, "user.touch_login", "UPDATE user SET last_login = ? WHERE id = ?")()
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := r.engine.Exec("UPDATE `user` SET last_login = ? WHERE id = ?", at, id)
	return err
}

// Delete 删除账号。
func (r *userRepo) Delete(ctx context.Context, id int64) error {
	defer r.observer.Trace(ctx, "user.delete", "DELETE FROM user WHERE id = ?")()
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := r.engine.Id(id).Delete(new(model.User))
	return err
}

// Count 返回账号总数。
func (r *userRepo) Count(ctx context.Context) (int64, error) {
	defer r.observer.Trace(ctx, "user.count", "SELECT COUNT(*) FROM user")()
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.engine.Count(new(model.User))
}
