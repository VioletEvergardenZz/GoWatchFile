// 本文件用于告警规则解析与加载
package alert

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v2"

	"file-watch/internal/models"
)

const defaultSuppressWindow = "5m"

// Ruleset 表示告警规则集
type Ruleset = models.AlertRuleset

// RuleDefaults 表示规则默认配置
type RuleDefaults = models.AlertRuleDefaults

// EscalationRule 表示异常升级配置
type EscalationRule = models.AlertEscalationRule

// Rule 表示单条匹配规则
type Rule = models.AlertRule

// LoadRules 读取并解析规则文件
func LoadRules(path string) (*Ruleset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取告警规则失败: %w", err)
	}

	var ruleset Ruleset
	if err := yaml.Unmarshal(data, &ruleset); err != nil {
		return nil, fmt.Errorf("解析告警规则失败: %w", err)
	}
	if err := normalizeRuleset(&ruleset); err != nil {
		return nil, err
	}
	return &ruleset, nil
}

// NormalizeRuleset 校验并补全规则集默认值
func NormalizeRuleset(ruleset *Ruleset) error {
	return normalizeRuleset(ruleset)
}

func normalizeRuleset(ruleset *Ruleset) error {
	if ruleset == nil {
		return fmt.Errorf("告警规则为空")
	}
	if ruleset.Version == 0 {
		ruleset.Version = 1
	}
	if strings.TrimSpace(ruleset.Defaults.SuppressWindow) == "" {
		ruleset.Defaults.SuppressWindow = defaultSuppressWindow
	}

	if len(ruleset.Rules) == 0 {
		return fmt.Errorf("告警规则不能为空")
	}
	for i := range ruleset.Rules {
		rule := &ruleset.Rules[i]
		rule.ID = strings.TrimSpace(rule.ID)
		if rule.ID == "" {
			rule.ID = fmt.Sprintf("rule-%d", i+1)
		}
		rule.Title = strings.TrimSpace(rule.Title)
		if rule.Title == "" {
			rule.Title = rule.ID
		}
		rule.Level = strings.ToLower(strings.TrimSpace(rule.Level))
		if _, ok := parseLevel(rule.Level); !ok {
			return fmt.Errorf("无效的告警级别: %s", rule.Level)
		}
		rule.Keywords = cleanKeywords(rule.Keywords)
		if len(rule.Keywords) == 0 {
			return fmt.Errorf("告警规则 %s 缺少关键字", rule.ID)
		}
		rule.Excludes = cleanKeywords(rule.Excludes)
	}
	return nil
}

// DefaultRuleset 返回空的默认规则集
func DefaultRuleset() *Ruleset {
	return &Ruleset{
		Version: 1,
		Defaults: RuleDefaults{
			SuppressWindow: defaultSuppressWindow,
		},
		Rules: []Rule{},
	}
}

func cleanKeywords(values []string) []string {
	out := make([]string, 0, len(values))
	for _, val := range values {
		trimmed := strings.TrimSpace(val)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}
