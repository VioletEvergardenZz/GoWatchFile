// 本文件用于定义告警规则结构体
package models

// AlertRuleset 表示告警规则集
type AlertRuleset struct {
	Version    int                 `yaml:"version" json:"version"`
	Defaults   AlertRuleDefaults   `yaml:"defaults" json:"defaults"`
	Escalation AlertEscalationRule `yaml:"escalation" json:"escalation"`
	Rules      []AlertRule         `yaml:"rules" json:"rules"`
}

// AlertRuleDefaults 表示规则默认配置
type AlertRuleDefaults struct {
	SuppressWindow string `yaml:"suppress_window" json:"suppress_window"`
	MatchCase      *bool  `yaml:"match_case" json:"match_case"`
}

// AlertEscalationRule 表示异常升级配置
type AlertEscalationRule struct {
	Enabled        *bool  `yaml:"enabled" json:"enabled"`
	Level          string `yaml:"level" json:"level"`
	Window         string `yaml:"window" json:"window"`
	Threshold      int    `yaml:"threshold" json:"threshold"`
	SuppressWindow string `yaml:"suppress_window" json:"suppress_window"`
	RuleID         string `yaml:"rule_id" json:"rule_id"`
	Title          string `yaml:"title" json:"title"`
	Message        string `yaml:"message" json:"message"`
}

// AlertRule 表示单条匹配规则
type AlertRule struct {
	ID             string   `yaml:"id" json:"id"`
	Title          string   `yaml:"title" json:"title"`
	Level          string   `yaml:"level" json:"level"`
	Keywords       []string `yaml:"keywords" json:"keywords"`
	Excludes       []string `yaml:"excludes" json:"excludes"`
	SuppressWindow string   `yaml:"suppress_window" json:"suppress_window"`
	MatchCase      *bool    `yaml:"match_case" json:"match_case"`
	Notify         *bool    `yaml:"notify" json:"notify"`
}
