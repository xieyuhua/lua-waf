package cli

import (
	"context"
	"fmt"
	"io"

	"wafadmin/internal/model"
	"wafadmin/internal/repository"
)

// ListUsers 列出后台账号（用于确认可登录的账号名）。
type ListUsers struct {
	// Users 为账号仓储。
	Users repository.UserRepository
	// Out 为输出目标。
	Out io.Writer
}

// Run 执行查询并打印账号列表。
func (l *ListUsers) Run(ctx context.Context) error {
	users, total, err := l.Users.List(ctx, model.ListQuery{Page: 1, Size: 200})
	if err != nil {
		return fmt.Errorf("查询账号失败: %w", err)
	}

	if len(users) == 0 {
		fmt.Fprintln(l.Out, "（没有任何账号，可用 reset-password -create 创建）")
		return nil
	}

	fmt.Fprintf(l.Out, "共 %d 个账号：\n", total)
	fmt.Fprintf(l.Out, "%-6s %-24s %-20s %s\n", "ID", "账号", "最近登录", "创建时间")
	for _, user := range users {
		lastLogin := "-"
		if !user.LastLogin.IsZero() {
			lastLogin = user.LastLogin.Format("2006-01-02 15:04:05")
		}
		created := "-"
		if !user.Created.IsZero() {
			created = user.Created.Format("2006-01-02 15:04:05")
		}
		fmt.Fprintf(l.Out, "%-6d %-24s %-20s %s\n", user.Id, user.UserName, lastLogin, created)
	}
	return nil
}
