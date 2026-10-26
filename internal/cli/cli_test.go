package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"wafadmin/internal/cli"
	"wafadmin/internal/model"
	"wafadmin/internal/repository"
	"wafadmin/internal/repository/memory"
	"wafadmin/internal/service"
	"wafadmin/pkg/crypto"
)

// fixture 是命令行测试用的内存仓储环境。
type fixture struct {
	repos  repository.Repositories
	users  *service.UserService
	hasher crypto.Hasher
}

// newFixture 构造内存仓储与账号服务。
func newFixture(t *testing.T) *fixture {
	t.Helper()
	store := (&memory.Store{}).Init(false)
	hasher := crypto.Hasher{}
	return &fixture{
		repos:  store.Repos(),
		users:  &service.UserService{Users: store.Repos().User, Hasher: hasher, PageSize: 10},
		hasher: hasher,
	}
}

// createUser 直接写入一个账号。
func (f *fixture) createUser(t *testing.T, name, password string) *model.User {
	t.Helper()
	user := &model.User{UserName: name, Password: f.hasher.Password(password)}
	if err := f.repos.User.Create(context.Background(), user); err != nil {
		t.Fatalf("写入账号失败: %v", err)
	}
	return user
}

// storedPassword 读取账号当前摘要。
func (f *fixture) storedPassword(t *testing.T, name string) string {
	t.Helper()
	user, err := f.repos.User.GetByName(context.Background(), name)
	if err != nil {
		t.Fatalf("查询账号失败: %v", err)
	}
	if user == nil {
		t.Fatalf("账号 %s 不存在", name)
	}
	return user.Password
}

// TestResetPasswordGenerates 校验未指定口令时随机生成并落库。
func TestResetPasswordGenerates(t *testing.T) {
	fx := newFixture(t)
	fx.createUser(t, "admin", "old-password")

	out := &bytes.Buffer{}
	err := (&cli.ResetPassword{
		Users:    fx.repos.User,
		Service:  fx.users,
		Out:      out,
		Username: "admin",
	}).Run(context.Background())
	if err != nil {
		t.Fatalf("重置失败: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "口令已重置") {
		t.Fatalf("输出缺少结果提示: %s", text)
	}
	if !strings.Contains(text, "随机口令") {
		t.Fatalf("随机生成时应回显口令: %s", text)
	}

	stored := fx.storedPassword(t, "admin")
	if fx.hasher.Verify("old-password", stored) {
		t.Fatal("旧口令仍然可用")
	}
	if stored == fx.hasher.Password("old-password") {
		t.Fatal("摘要未变化")
	}
}

// TestResetPasswordExplicit 校验指定口令时按业务规则落库。
func TestResetPasswordExplicit(t *testing.T) {
	fx := newFixture(t)
	fx.createUser(t, "ops", "old-password")

	out := &bytes.Buffer{}
	err := (&cli.ResetPassword{
		Users:    fx.repos.User,
		Service:  fx.users,
		Out:      out,
		Username: "ops",
		Password: "NewPass2026",
	}).Run(context.Background())
	if err != nil {
		t.Fatalf("重置失败: %v", err)
	}
	if !strings.Contains(out.String(), "指定的口令") {
		t.Fatalf("应说明使用指定口令: %s", out.String())
	}
	if !fx.hasher.Verify("NewPass2026", fx.storedPassword(t, "ops")) {
		t.Fatal("新口令未生效")
	}
}

// TestResetPasswordMissingUser 校验账号不存在且未创建时报错。
func TestResetPasswordMissingUser(t *testing.T) {
	fx := newFixture(t)
	out := &bytes.Buffer{}

	err := (&cli.ResetPassword{
		Users:    fx.repos.User,
		Service:  fx.users,
		Out:      out,
		Username: "ghost",
		Password: "NewPass2026",
	}).Run(context.Background())
	if err == nil {
		t.Fatal("账号不存在时应返回错误")
	}
	if !strings.Contains(err.Error(), "-create") {
		t.Fatalf("错误信息应提示 -create: %v", err)
	}
}

// TestResetPasswordCreate 校验 -create 时自动建号。
func TestResetPasswordCreate(t *testing.T) {
	fx := newFixture(t)
	out := &bytes.Buffer{}

	err := (&cli.ResetPassword{
		Users:    fx.repos.User,
		Service:  fx.users,
		Out:      out,
		Username: "backup",
		Password: "Backup2026",
		Create:   true,
	}).Run(context.Background())
	if err != nil {
		t.Fatalf("创建账号失败: %v", err)
	}
	if !strings.Contains(out.String(), "账号已创建") {
		t.Fatalf("应提示账号已创建: %s", out.String())
	}
	if !fx.hasher.Verify("Backup2026", fx.storedPassword(t, "backup")) {
		t.Fatal("新账号口令不正确")
	}
}

// TestResetPasswordValidates 校验口令与账号名的业务校验生效。
func TestResetPasswordValidates(t *testing.T) {
	fx := newFixture(t)
	fx.createUser(t, "admin", "old-password")

	cases := []struct {
		name     string
		username string
		password string
	}{
		{name: "口令过短", username: "admin", password: "short"},
		{name: "账号名为空", username: "  ", password: "NewPass2026"},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			err := (&cli.ResetPassword{
				Users:    fx.repos.User,
				Service:  fx.users,
				Out:      &bytes.Buffer{},
				Username: item.username,
				Password: item.password,
			}).Run(context.Background())
			if err == nil {
				t.Fatal("应返回校验错误")
			}
		})
	}
}

// TestListUsers 校验账号列表输出。
func TestListUsers(t *testing.T) {
	fx := newFixture(t)
	fx.createUser(t, "admin", "admin-password")

	out := &bytes.Buffer{}
	if err := (&cli.ListUsers{Users: fx.repos.User, Out: out}).Run(context.Background()); err != nil {
		t.Fatalf("列账号失败: %v", err)
	}
	if !strings.Contains(out.String(), "admin") {
		t.Fatalf("输出缺少账号: %s", out.String())
	}
}

// TestDispatch 校验子命令分派：flag 开头交给常驻服务，子命令自行处理。
func TestDispatch(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}

	handled, code := cli.Dispatch([]string{"-config", "conf/app.ini"}, "2.0.0", out, errOut)
	if handled {
		t.Fatal("以 - 开头的参数不应由子命令处理")
	}

	handled, code = cli.Dispatch([]string{"version"}, "2.0.0", out, errOut)
	if !handled || code != 0 {
		t.Fatalf("version 子命令应成功: handled=%v code=%d", handled, code)
	}
	if !strings.Contains(out.String(), "2.0.0") {
		t.Fatalf("版本输出异常: %s", out.String())
	}

	out.Reset()
	handled, code = cli.Dispatch([]string{"help"}, "2.0.0", out, errOut)
	if !handled || code != 0 {
		t.Fatalf("help 子命令应成功: handled=%v code=%d", handled, code)
	}
	if !strings.Contains(out.String(), "reset-password") {
		t.Fatalf("帮助内容缺少子命令: %s", out.String())
	}

	handled, code = cli.Dispatch([]string{"nope"}, "2.0.0", out, errOut)
	if !handled || code != 2 {
		t.Fatalf("未知子命令应返回 2: handled=%v code=%d", handled, code)
	}
}
