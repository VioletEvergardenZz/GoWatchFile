package alert

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v2"
)

const defaultSuppressWindow = "5m"

// Ruleset 表示告警规则集
type Ruleset struct {
	Version    int            `yaml:"version" json:"version"`
	Defaults   RuleDefaults   `yaml:"defaults" json:"defaults"`
	Escalation EscalationRule `yaml:"escalation" json:"escalation"`
	Rules      []Rule         `yaml:"rules" json:"rules"`
}

// RuleDefaults 表示规则默认配置
type RuleDefaults struct {
	SuppressWindow string `yaml:"suppress_window" json:"suppress_window"`
	MatchCase      *bool  `yaml:"match_case" json:"match_case"`
}

// EscalationRule 表示异常升级配置
type EscalationRule struct {
	Enabled        *bool  `yaml:"enabled" json:"enabled"`
	Level          string `yaml:"level" json:"level"`
	Window         string `yaml:"window" json:"window"`
	Threshold      int    `yaml:"threshold" json:"threshold"`
	SuppressWindow string `yaml:"suppress_window" json:"suppress_window"`
	RuleID         string `yaml:"rule_id" json:"rule_id"`
	Title          string `yaml:"title" json:"title"`
	Message        string `yaml:"message" json:"message"`
}

// Rule 表示单条匹配规则
type Rule struct {
	ID             string   `yaml:"id" json:"id"`
	Title          string   `yaml:"title" json:"title"`
	Level          string   `yaml:"level" json:"level"`
	Keywords       []string `yaml:"keywords" json:"keywords"`
	Excludes       []string `yaml:"excludes" json:"excludes"`
	SuppressWindow string   `yaml:"suppress_window" json:"suppress_window"`
	MatchCase      *bool    `yaml:"match_case" json:"match_case"`
	Notify         *bool    `yaml:"notify" json:"notify"`
}

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
