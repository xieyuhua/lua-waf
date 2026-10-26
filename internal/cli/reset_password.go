package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"wafadmin/internal/app"
	"wafadmin/internal/config"
	"wafadmin/internal/repository"
	"wafadmin/internal/service"
	"wafadmin/pkg/crypto"
)

// NewUserService 构造命令行使用的账号服务，与 HTTP 侧共用同一套校验规则。
func NewUserService(store *app.Store, cfg *config.Config) *service.UserService {
	return &service.UserService{
		Users:    store.Repos.User,
		Hasher:   crypto.Hasher{},
		PageSize: cfg.PageSize,
	}
}

// ResetPassword 重置后台账号口令。
type ResetPassword struct {
	// Users 为账号仓储。
	Users repository.UserRepository
	// Service 复用业务层的校验与落库逻辑。
	Service *service.UserService
	// Out 为输出目标。
	Out io.Writer
	// Username 为要重置的账号名。
	Username string
	// Password 为新口令；为空时随机生成。
	Password string
	// Create 表示账号不存在时自动创建。
	Create bool
}

// Run 执行重置。
func (r *ResetPassword) Run(ctx context.Context) error {
	username := strings.TrimSpace(r.Username)
	if username == "" {
		return fmt.Errorf("请用 -user 指定账号名")
	}

	user, err := r.Users.GetByName(ctx, username)
	if err != nil {
		return fmt.Errorf("查询账号失败: %w", err)
	}

	password := strings.TrimSpace(r.Password)
	generated := false
	if password == "" {
		password = crypto.Hasher{}.Token(8)
		generated = true
	}

	if user == nil {
		if !r.Create {
			return fmt.Errorf("账号 %s 不存在（如需创建请加 -create）", username)
		}
		if _, err := r.Service.Create(ctx, service.CreateUserRequest{
			Username: username,
			Password: password,
		}); err != nil {
			return fmt.Errorf("创建账号失败: %w", err)
		}
		r.report(username, password, generated, true)
		return nil
	}

	if _, err := r.Service.Update(ctx, user.Id, service.UpdateUserRequest{
		Username: user.UserName,
		Password: password,
	}); err != nil {
		return fmt.Errorf("重置口令失败: %w", err)
	}

	r.report(username, password, generated, false)
	return nil
}

// report 输出处理结果。
func (r *ResetPassword) report(username, password string, generated, created bool) {
	action := "口令已重置"
	if created {
		action = "账号已创建"
	}
	fmt.Fprintf(r.Out, "[ok] %s：%s\n", username, action)
	if generated {
		fmt.Fprintf(r.Out, "     随机口令: %s\n", password)
		fmt.Fprintln(r.Out, "     请登录后立即在「账号设置」中修改为新口令")
	} else {
		fmt.Fprintln(r.Out, "     已使用 -password 指定的口令")
	}
}
