package service

import (
	"context"
	"regexp"
	"strings"

	"wafadmin/internal/model"
	"wafadmin/internal/repository"
	"wafadmin/pkg/crypto"
)

// 账号校验规则。
var (
	userNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,32}$`)
	// 最小口令长度。
	minPasswordLength = 8
)

// CreateUserRequest 是新增账号的入参。
type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// UpdateUserRequest 是修改账号的入参，口令为空表示不修改。
type UpdateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ChangePasswordRequest 是当前登录人修改自己口令的入参。
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// UserService 负责后台账号的增删改查。
type UserService struct {
	Users    repository.UserRepository
	Hasher   crypto.Hasher
	PageSize int
}

// List 分页查询账号，返回脱敏视图。
func (s *UserService) List(ctx context.Context, q model.ListQuery) (model.PageResult, error) {
	q = q.Normalize(s.PageSize)
	items, total, err := s.Users.List(ctx, q)
	if err != nil {
		return model.PageResult{}, ErrInternal.Wrap(err, "查询账号列表失败")
	}

	views := make([]model.UserView, 0, len(items))
	for _, item := range items {
		views = append(views, item.Display())
	}
	return q.Result(views, total), nil
}

// Create 新增账号。
func (s *UserService) Create(ctx context.Context, req CreateUserRequest) (*model.User, error) {
	username, err := s.validateUsername(req.Username)
	if err != nil {
		return nil, err
	}
	if err := s.validatePassword(req.Password); err != nil {
		return nil, err
	}
	if existing, findErr := s.Users.GetByName(ctx, username); findErr != nil {
		return nil, ErrInternal.Wrap(findErr, "查询账号失败")
	} else if existing != nil {
		return nil, ErrConflict.New("账号 %s 已存在", username)
	}

	user := &model.User{UserName: username, Password: s.Hasher.Password(req.Password)}
	if err := s.Users.Create(ctx, user); err != nil {
		return nil, ErrInternal.Wrap(err, "创建账号失败")
	}
	return user, nil
}

// Update 修改账号的用户名与口令（口令留空表示不修改）。
func (s *UserService) Update(ctx context.Context, id int64, req UpdateUserRequest) (*model.User, error) {
	user, err := s.Users.Get(ctx, id)
	if err != nil {
		return nil, ErrInternal.Wrap(err, "查询账号失败")
	}
	if user == nil {
		return nil, ErrNotFound.New("账号不存在")
	}

	username, err := s.validateUsername(req.Username)
	if err != nil {
		return nil, err
	}
	if existing, findErr := s.Users.GetByName(ctx, username); findErr != nil {
		return nil, ErrInternal.Wrap(findErr, "查询账号失败")
	} else if existing != nil && existing.Id != id {
		return nil, ErrConflict.New("账号 %s 已存在", username)
	}

	user.UserName = username
	if strings.TrimSpace(req.Password) != "" {
		if err := s.validatePassword(req.Password); err != nil {
			return nil, err
		}
		user.Password = s.Hasher.Password(req.Password)
	}
	if err := s.Users.Update(ctx, user); err != nil {
		return nil, ErrInternal.Wrap(err, "更新账号失败")
	}
	return user, nil
}

// ChangePassword 修改当前登录人的口令。
func (s *UserService) ChangePassword(ctx context.Context, id int64, req ChangePasswordRequest) error {
	user, err := s.Users.Get(ctx, id)
	if err != nil {
		return ErrInternal.Wrap(err, "查询账号失败")
	}
	if user == nil {
		return ErrNotFound.New("账号不存在")
	}
	if !s.Hasher.Verify(req.OldPassword, user.Password) {
		return ErrInvalid.New("原口令不正确")
	}
	if err := s.validatePassword(req.NewPassword); err != nil {
		return err
	}
	user.Password = s.Hasher.Password(req.NewPassword)
	if err := s.Users.Update(ctx, user); err != nil {
		return ErrInternal.Wrap(err, "更新口令失败")
	}
	return nil
}

// Delete 删除账号，禁止删除自己与最后一个账号。
func (s *UserService) Delete(ctx context.Context, id, operatorId int64) error {
	if id == operatorId {
		return ErrForbidden.New("不能删除当前登录账号")
	}
	total, err := s.Users.Count(ctx)
	if err != nil {
		return ErrInternal.Wrap(err, "统计账号失败")
	}
	if total <= 1 {
		return ErrForbidden.New("至少需要保留一个管理账号")
	}
	if err := s.Users.Delete(ctx, id); err != nil {
		return ErrInternal.Wrap(err, "删除账号失败")
	}
	return nil
}

// SeedDefault 在账号表为空时写入初始账号，返回是否发生了初始化。
func (s *UserService) SeedDefault(ctx context.Context) (bool, error) {
	total, err := s.Users.Count(ctx)
	if err != nil {
		return false, ErrInternal.Wrap(err, "统计账号失败")
	}
	if total > 0 {
		return false, nil
	}
	user := &model.User{UserName: "admin", Password: s.Hasher.Password("admin")}
	if err := s.Users.Create(ctx, user); err != nil {
		return false, ErrInternal.Wrap(err, "初始化管理员账号失败")
	}
	return true, nil
}

// validateUsername 校验并归一化用户名。
func (s *UserService) validateUsername(raw string) (string, error) {
	username := strings.TrimSpace(raw)
	if !userNamePattern.MatchString(username) {
		return "", ErrInvalid.New("用户名需为 3-32 位字母、数字、下划线、点或减号")
	}
	return username, nil
}

// validatePassword 校验口令强度（最小长度）。
func (s *UserService) validatePassword(raw string) error {
	if len(strings.TrimSpace(raw)) < minPasswordLength {
		return ErrInvalid.New("口令长度不能少于 %d 位", minPasswordLength)
	}
	return nil
}
