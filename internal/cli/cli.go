package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"wafadmin/internal/app"
	"wafadmin/internal/config"
)

// Dispatch 判断并执行子命令。
// 返回 handled=false 表示第一个参数是 flag（如 `wafadmin -config x`），由调用方按常驻服务启动。
func Dispatch(args []string, version string, stdout, stderr io.Writer) (bool, int) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return false, 0
	}

	runner := &Runner{Version: version, Stdout: stdout, Stderr: stderr}
	return true, runner.run(args[0], args[1:])
}

// Runner 执行命令行子命令（不启动 HTTP 服务）。
type Runner struct {
	Version string
	Stdout  io.Writer
	Stderr  io.Writer
	// OpenStore 打开仓储，测试时可替换。
	OpenStore func(cfg *config.Config) (*app.Store, error)
}

// run 按子命令分发。
func (r *Runner) run(name string, args []string) int {
	switch name {
	case "reset-password":
		return r.resetPassword(args)
	case "list-users":
		return r.listUsers(args)
	case "ruletest":
		return r.ruleTest(args)
	case "version", "--version", "-v":
		fmt.Fprintf(r.Stdout, "wafadmin %s\n", r.Version)
		return 0
	case "help", "--help", "-h":
		r.usage()
		return 0
	default:
		fmt.Fprintf(r.Stderr, "未知子命令: %s\n\n", name)
		r.usage()
		return 2
	}
}

// resetPassword 解析参数并重置账号口令。
func (r *Runner) resetPassword(args []string) int {
	flags := r.newFlagSet("reset-password")
	configPath := flags.String("config", "", "配置文件路径，默认取 WAF_CONFIG 或 conf/app.ini")
	username := flags.String("user", "admin", "要重置的账号名")
	password := flags.String("password", "", "新口令；留空则随机生成 16 位")
	create := flags.Bool("create", false, "账号不存在时自动创建")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	return r.withStore(*configPath, func(store *app.Store, cfg *config.Config) error {
		return (&ResetPassword{
			Users:    store.Repos.User,
			Service:  NewUserService(store, cfg),
			Out:      r.Stdout,
			Username: *username,
			Password: *password,
			Create:   *create,
		}).Run(context.Background())
	})
}

// listUsers 列出后台账号。
func (r *Runner) listUsers(args []string) int {
	flags := r.newFlagSet("list-users")
	configPath := flags.String("config", "", "配置文件路径，默认取 WAF_CONFIG 或 conf/app.ini")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	return r.withStore(*configPath, func(store *app.Store, _ *config.Config) error {
		return (&ListUsers{Users: store.Repos.User, Out: r.Stdout}).Run(context.Background())
	})
}

// newFlagSet 创建带统一参数集的 FlagSet。
func (r *Runner) newFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(r.Stderr)
	return flags
}

// withStore 打开仓储并执行子命令，结束后释放资源。
func (r *Runner) withStore(configPath string, exec func(*app.Store, *config.Config) error) int {
	cfg, err := (&config.Loader{Path: configPath}).Load()
	if err != nil {
		fmt.Fprintf(r.Stderr, "[error] 读取配置失败: %v\n", err)
		return 1
	}

	open := r.OpenStore
	if open == nil {
		open = app.OpenStore
	}
	store, err := open(cfg)
	if err != nil {
		fmt.Fprintf(r.Stderr, "[error] 连接数据源失败: %v\n", err)
		return 1
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			fmt.Fprintf(r.Stderr, "[warn] 释放连接时出错: %v\n", closeErr)
		}
	}()

	if err := exec(store, cfg); err != nil {
		fmt.Fprintf(r.Stderr, "[error] %v\n", err)
		return 1
	}
	return 0
}

// usage 打印子命令帮助。
func (r *Runner) usage() {
	fmt.Fprint(r.Stdout, `wafadmin 命令行工具

用法:
  wafadmin [子命令] [参数]          执行子命令
  wafadmin -config conf/app.ini     不带子命令时启动后台服务

子命令:
  reset-password   重置（或创建）后台账号口令
  list-users       列出后台账号
  ruletest         检查规则能否命中（全量自检 / 载荷验证）
  version          显示版本
  help             显示本帮助

示例:
  # 重置 admin 口令为随机 16 位（推荐，口令不会进入命令行历史）
  wafadmin reset-password -config conf/app.ini

  # 重置指定账号为指定口令
  wafadmin reset-password -config conf/app.ini -user ops -password 'MyPass2026!'

  # 账号不存在时一并创建
  wafadmin reset-password -config conf/app.ini -user ops -password 'MyPass2026!' -create

  # 查看现有账号
  wafadmin list-users -config conf/app.ini
`)
}
