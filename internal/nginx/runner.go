package nginx

import (
	"fmt"
	"os/exec"
	"strings"
)

// Test 执行 nginx -t 检查配置语法，返回命令输出。
func (m *Manager) Test() (string, error) {
	if m.DryRun {
		return "dry-run: 跳过 nginx -t 检查", nil
	}
	return m.run("-t")
}

// Reload 平滑重载 nginx。
func (m *Manager) Reload() (string, error) {
	if m.DryRun {
		return "dry-run: 跳过 nginx -s reload", nil
	}
	if output, err := m.Test(); err != nil {
		return output, fmt.Errorf("配置检查未通过，已取消重载: %w", err)
	}
	return m.run("-s", "reload")
}

// ReloadIfNeeded 在 DryRun 或未配置可执行文件时跳过重载。
func (m *Manager) ReloadIfNeeded() (string, error) {
	if strings.TrimSpace(m.Bin) == "" && !m.DryRun {
		return "未配置 NGINX_BIN，跳过重载", nil
	}
	return m.Reload()
}

// run 执行 nginx 命令并合并输出。
func (m *Manager) run(args ...string) (string, error) {
	if strings.TrimSpace(m.Bin) == "" {
		return "", fmt.Errorf("未配置 nginx 可执行文件路径")
	}
	output, err := exec.Command(m.Bin, args...).CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		return text, fmt.Errorf("执行 %s %s 失败: %w", m.Bin, strings.Join(args, " "), err)
	}
	return text, nil
}
