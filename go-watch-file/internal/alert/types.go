// 本文件用于定义告警相关的数据结构
// 文件职责：实现当前模块的核心业务逻辑与数据流转
// 关键路径：入口参数先校验再执行业务处理 最后返回统一结果
// 边界与容错：异常场景显式返回错误 由上层决定重试或降级

package alert

// Level 表示告警级别
type Level string

const (
	// LevelIgnore 表示忽略
	LevelIgnore Level = "ignore"
	// LevelBusiness 表示业务异常
	LevelBusiness Level = "business"
	// LevelSystem 表示系统异常
	LevelSystem Level = "system"
	// LevelFatal 表示致命异常
	LevelFatal Level = "fatal"
)

// DecisionStatus 表示告警决策状态
type DecisionStatus string

const (
	// StatusSent 表示已发送
	StatusSent DecisionStatus = "sent"
	// StatusSuppressed 表示已抑制
	StatusSuppressed DecisionStatus = "suppressed"
	// StatusRecorded 表示仅记录
	StatusRecorded DecisionStatus = "recorded"
)

// DecisionKind 表示决策来源类型
type DecisionKind string

const (
	// DecisionKindRuleMatch 表示由规则匹配产生的决策
	DecisionKindRuleMatch DecisionKind = "rule_match"
	// DecisionKindEscalation 表示由异常升级策略产生的决策
	DecisionKindEscalation DecisionKind = "escalation"
)

// SuppressedBy 表示抑制来源
type SuppressedBy string

const (
	// SuppressedByRuleWindow 表示命中规则抑制窗口
	SuppressedByRuleWindow SuppressedBy = "rule_window"
	// SuppressedByEscalationWindow 表示命中升级抑制窗口
	SuppressedByEscalationWindow SuppressedBy = "escalation_window"
)

// DecisionExplain 表示告警决策的可解释字段
type DecisionExplain struct {
	DecisionKind        DecisionKind `json:"decisionKind"`
	Notify              bool         `json:"notify"`
	SuppressionEnabled  bool         `json:"suppressionEnabled"`
	SuppressWindow      string       `json:"suppressWindow,omitempty"`
	SuppressedBy        SuppressedBy `json:"suppressedBy,omitempty"`
	EscalationThreshold int          `json:"escalationThreshold,omitempty"`
	EscalationWindow    string       `json:"escalationWindow,omitempty"`
	EscalationCount     int          `json:"escalationCount,omitempty"`
}

// parseLevel 用于解析输入参数或配置
func parseLevel(raw string) (Level, bool) {
	switch Level(raw) {
	case LevelIgnore, LevelBusiness, LevelSystem, LevelFatal:
		return Level(raw), true
	default:
		return "", false
	}
}
