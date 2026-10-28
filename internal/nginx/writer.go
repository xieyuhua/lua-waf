package nginx

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"wafadmin/internal/model"
)

// WriteVhost 渲染并写入站点 vhost 文件，返回写入路径。
func (m *Manager) WriteVhost(site model.Site) (string, error) {
	content, err := m.Render(site)
	if err != nil {
		return "", err
	}

	target := path.Join(m.vhostsDir(), site.SiteName+".conf")
	if m.DryRun {
		return target, nil
	}
	if err := os.MkdirAll(m.vhostsDir(), 0o755); err != nil {
		return "", fmt.Errorf("创建 vhost 目录失败: %w", err)
	}
	// 统一使用 LF 换行，避免在 Windows 上生成不可被 nginx 解析的文件。
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("写入 vhost 文件失败: %w", err)
	}
	return target, nil
}

// RemoveVhost 删除站点 vhost 文件。
func (m *Manager) RemoveVhost(siteName string) error {
	if m.DryRun {
		return nil
	}
	target := path.Join(m.vhostsDir(), siteName+".conf")
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除 vhost 文件失败: %w", err)
	}
	return nil
}

// WriteRules 把按类型分组的规则写入 <type>.rule 文件，返回写入的路径列表。
func (m *Manager) WriteRules(grouped map[string][]model.Rule) ([]string, error) {
	written := make([]string, 0, len(grouped))
	if m.DryRun {
		for ruleType := range grouped {
			written = append(written, path.Join(m.ruleDir(), ruleType+".rule"))
		}
		return written, nil
	}
	if err := os.MkdirAll(m.ruleDir(), 0o755); err != nil {
		return nil, fmt.Errorf("创建规则目录失败: %w", err)
	}

	for ruleType, rules := range grouped {
		// 只下发启用中的规则；停用的规则保留在库中但不再生效
		items := make([]model.RuleFileItem, 0, len(rules))
		for _, rule := range rules {
			if !rule.Enabled {
				continue
			}
			items = append(items, rule.ToFileItem())
		}
		payload, err := json.Marshal(items)
		if err != nil {
			return written, fmt.Errorf("序列化规则 %s 失败: %w", ruleType, err)
		}

		target := path.Join(m.ruleDir(), ruleType+".rule")
		if err := os.WriteFile(target, payload, 0o644); err != nil {
			return written, fmt.Errorf("写入规则文件 %s 失败: %w", ruleType, err)
		}
		written = append(written, target)
	}
	return written, nil
}

// vhostsDir 返回生效的 vhost 目录。
func (m *Manager) vhostsDir() string {
	if m.VhostsDir == "" {
		return "/usr/local/openresty/nginx/conf/vhosts"
	}
	return m.VhostsDir
}

// ruleDir 返回生效的规则目录。
func (m *Manager) ruleDir() string {
	if m.RuleDir == "" {
		return "/www/server/nginx/x-waf/rules"
	}
	return m.RuleDir
}
