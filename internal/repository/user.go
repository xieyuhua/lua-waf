package repository

import (
	"context"
	"time"

	"wafadmin/internal/model"
)

// UserRepository 定义后台账号的持久化契约。
type UserRepository interface {
	// List 分页查询账号。
	List(ctx context.Context, q model.ListQuery) ([]model.User, int64, error)
	// Get 按主键查询账号。
	Get(ctx context.Context, id int64) (*model.User, error)
	// GetByName 按用户名查询账号。
	GetByName(ctx context.Context, name string) (*model.User, error)
	// Create 新增账号。
	Create(ctx context.Context, user *model.User) error
	// Update 更新账号（用户名与口令）。
	Update(ctx context.Context, user *model.User) error
	// TouchLogin 记录最近登录时间。
	TouchLogin(ctx context.Context, id int64, at time.Time) error
	// Delete 删除账号。
	Delete(ctx context.Context, id int64) error
	// Count 返回账号总数。
	Count(ctx context.Context) (int64, error)
}
