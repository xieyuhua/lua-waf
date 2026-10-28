package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"wafadmin/internal/model"
	"wafadmin/internal/repository"
)

// RuleFileSyncResult 是规则文件与数据库取并集的统计。
type RuleFileSyncResult struct {
	// Dir 为扫描的规则目录。
	Dir string `json:"dir"`
	// Files 为读取的规则文件数。
	Files int `json:"files"`
	// InFiles 为规则文件中的条目总数。
	InFiles int `json:"in_files"`
	// Added 为数据库中缺失、本次新增的条数。
	Added int `json:"added"`
	// Kept 为数据库已存在、保持不变（不覆盖、不删除）的条数。
	Kept int `json:"kept"`
	// Skipped 为无效条目（缺字段/重复）的条数。
	Skipped int `json:"skipped"`
	// Message 为提示信息（目录不存在等）。
	Message string `json:"message"`
}

// RuleFileSync 负责把 lua 规则文件与数据库规则「取并集」：
// 文件里有、库里没有的补进库；库里已有的保持原样（不覆盖、不删除）。
//
// 之所以只增不删：规则文件可能由运维直接在服务器上维护，
// 而库里可能有人工调整过的规则；取并集既能把新规则带进后台，
// 又不会把人工维护的结果冲掉。
type RuleFileSync struct {
	// Repo 为规则仓储。
	Repo repository.RuleRepository
}

// ruleFileItem 是规则文件中的一条规则。
type ruleFileItem struct {
	Id       int64  `json:"Id"`
	RuleType string `json:"RuleType"`
	RuleItem string `json:"RuleItem"`
}

// SyncFromDir 扫描目录下的 *.rule 文件并与数据库取并集。
func (s *RuleFileSync) SyncFromDir(ctx context.Context, dir string) (RuleFileSyncResult, error) {
	result := RuleFileSyncResult{Dir: dir}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if s.Repo == nil {
		return result, ErrInvalid.New("规则仓储不可用")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		// 目录不存在时不视为错误：生产上规则目录可能挂在别处或尚未部署
		result.Message = "规则目录不可读，已跳过（" + err.Error() + "）"
		return result, nil
	}

	// 现有规则集合（类型 + 规则内容）
	existing, err := s.Repo.List(ctx, "")
	if err != nil {
		return result, ErrInternal.Wrap(err, "读取现有规则失败")
	}
	known := make(map[string]struct{}, len(existing))
	for _, rule := range existing {
		known[ruleKey(rule.RuleType, rule.RuleItem)] = struct{}{}
	}

	pending := make([]struct {
		Type string
		Item string
	}, 0, 16)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".rule") {
			continue
		}
		result.Files++

		fallbackType := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		raw, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			result.Message = "部分规则文件读取失败：" + readErr.Error()
			continue
		}

		var items []ruleFileItem
		if err := json.Unmarshal(raw, &items); err != nil {
			result.Message = "部分规则文件解析失败：" + entry.Name()
			continue
		}

		for _, item := range items {
			ruleType := strings.TrimSpace(item.RuleType)
			if ruleType == "" {
				ruleType = fallbackType
			}
			ruleItem := strings.TrimSpace(item.RuleItem)
			if ruleType == "" || ruleItem == "" {
				result.Skipped++
				continue
			}
			result.InFiles++

			key := ruleKey(ruleType, ruleItem)
			if _, ok := known[key]; ok {
				result.Kept++
				continue
			}
			// 同一批文件内的重复也去重
			known[key] = struct{}{}
			pending = append(pending, struct {
				Type string
				Item string
			}{Type: ruleType, Item: ruleItem})
		}
	}

	if len(pending) == 0 {
		if result.Message == "" {
			result.Message = "规则文件与数据库已一致，无需新增"
		}
		return result, nil
	}

	newRules := make([]model.Rule, 0, len(pending))
	for _, item := range pending {
		newRules = append(newRules, model.Rule{RuleType: item.Type, RuleItem: item.Item})
	}
	if err := s.Repo.BatchCreate(ctx, newRules); err != nil {
		return result, ErrInternal.Wrap(err, "写入新增规则失败")
	}
	result.Added = len(newRules)
	result.Message = "已按并集补齐缺失规则（原有规则保持不动）"
	return result, nil
}

// ruleKey 生成规则的唯一键（类型 + 内容）。
func ruleKey(ruleType, ruleItem string) string {
	return strings.ToLower(strings.TrimSpace(ruleType)) + "\x00" + strings.TrimSpace(ruleItem)
}
