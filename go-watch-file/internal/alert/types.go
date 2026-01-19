// 本文件用于定义告警相关的数据结构
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

func parseLevel(raw string) (Level, bool) {
	switch Level(raw) {
	case LevelIgnore, LevelBusiness, LevelSystem, LevelFatal:
		return Level(raw), true
	default:
		return "", false
	}
}
