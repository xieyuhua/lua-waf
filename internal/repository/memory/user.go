package memory

import (
	"context"
	"sort"
	"strings"
	"time"

	"wafadmin/internal/model"
	"wafadmin/pkg/crypto"
)

// userRepo 是 UserRepository 的内存实现。
type userRepo struct {
	store *Store
	// hasher 用于初始化演示账号的口令摘要。
	hasher crypto.Hasher
}

// List 分页查询账号。
func (r *userRepo) List(ctx context.Context, q model.ListQuery) ([]model.User, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	keyword := strings.ToLower(q.Keyword)
	items := make([]model.User, 0, len(r.store.users))
	for _, item := range r.store.users {
		if keyword != "" && !strings.Contains(strings.ToLower(item.UserName), keyword) {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Id < items[j].Id })
	return pager[model.User]{items: items}.Slice(q), int64(len(items)), nil
}

// Get 按主键查询账号。
func (r *userRepo) Get(ctx context.Context, id int64) (*model.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	for _, item := range r.store.users {
		if item.Id == id {
			found := item
			return &found, nil
		}
	}
	return nil, nil
}

// GetByName 按用户名查询账号。
func (r *userRepo) GetByName(ctx context.Context, name string) (*model.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()

	for _, item := range r.store.users {
		if strings.EqualFold(item.UserName, name) {
			found := item
			return &found, nil
		}
	}
	return nil, nil
}

// Create 新增账号。
func (r *userRepo) Create(ctx context.Context, user *model.User) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	r.store.userSeq++
	user.Id = r.store.userSeq
	now := r.store.clock()
	user.Created = now
	user.Updated = now
	r.store.users = append(r.store.users, *user)
	return nil
}

// Update 更新账号。
func (r *userRepo) Update(ctx context.Context, user *model.User) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	for i := range r.store.users {
		if r.store.users[i].Id != user.Id {
			continue
		}
		r.store.users[i].UserName = user.UserName
		r.store.users[i].Password = user.Password
		r.store.users[i].Updated = r.store.clock()
		return nil
	}
	return nil
}

// TouchLogin 记录最近登录时间。
func (r *userRepo) TouchLogin(ctx context.Context, id int64, at time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	for i := range r.store.users {
		if r.store.users[i].Id == id {
			r.store.users[i].LastLogin = at
			return nil
		}
	}
	return nil
}

// Delete 删除账号。
func (r *userRepo) Delete(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.store.mu.Lock()
	defer r.store.mu.Unlock()

	kept := make([]model.User, 0, len(r.store.users))
	for _, item := range r.store.users {
		if item.Id != id {
			kept = append(kept, item)
		}
	}
	r.store.users = kept
	return nil
}

// Count 返回账号总数。
func (r *userRepo) Count(ctx context.Context) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	r.store.mu.RLock()
	defer r.store.mu.RUnlock()
	return int64(len(r.store.users)), nil
}
