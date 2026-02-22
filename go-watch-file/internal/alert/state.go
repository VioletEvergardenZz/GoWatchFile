// 本文件用于告警运行态与统计
// 文件职责：实现当前模块的核心业务逻辑与数据流转
// 关键路径：入口参数先校验再执行业务处理 最后返回统一结果
// 边界与容错：异常场景显式返回错误 由上层决定重试或降级

package alert

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	maxDecisionRecords = 200
	overviewWindow     = 24 * time.Hour // 告警态势概览统计窗口
)

// Dashboard 表示告警控制台数据
type Dashboard struct {
	Overview  Overview     `json:"overview"`
	Decisions []Decision   `json:"decisions"`
	Stats     Stats        `json:"stats"`
	Rules     RulesSummary `json:"rules"`
	Polling   PollSummary  `json:"polling"`
}

// Overview 表示告警态势概览
type Overview struct {
	Window     string `json:"window"`
	Risk       string `json:"risk"`
	Fatal      int    `json:"fatal"`
	System     int    `json:"system"`
	Business   int    `json:"business"`
	Sent       int    `json:"sent"`
	Suppressed int    `json:"suppressed"`
	Latest     string `json:"latest"`
}

// Decision 表示告警列表项
type Decision struct {
	ID       string           `json:"id"`
	Time     string           `json:"time"`
	Level    string           `json:"level"`
	Rule     string           `json:"rule"`
	Message  string           `json:"message"`
	File     string           `json:"file"`
	Status   string           `json:"status"`
	Reason   string           `json:"reason,omitempty"`
	Explain  *DecisionExplain `json:"explain,omitempty"`
	Analysis string           `json:"analysis,omitempty"`
	// KnowledgeTrace 记录当前告警与知识推荐的关联结果，便于值班回溯“当时推荐了什么”
	KnowledgeTrace *RecommendationTrace `json:"knowledgeTrace,omitempty"`
}

// RecommendationTrace 表示告警与知识推荐之间的一次关联快照
type RecommendationTrace struct {
	AlertID        string                  `json:"alertId"`
	LinkID         string                  `json:"linkId"`
	LinkedAt       string                  `json:"linkedAt"`
	Query          string                  `json:"query"`
	Rule           string                  `json:"rule,omitempty"`
	Message        string                  `json:"message,omitempty"`
	DecisionStatus string                  `json:"decisionStatus,omitempty"`
	DecisionReason string                  `json:"decisionReason,omitempty"`
	HitCount       int                     `json:"hitCount"`
	Articles       []RecommendationArticle `json:"articles,omitempty"`
}

// RecommendationArticle 表示推荐命中的知识条目摘要
type RecommendationArticle struct {
	ArticleID string `json:"articleId"`
	Title     string `json:"title"`
	Version   int    `json:"version"`
	Status    string `json:"status,omitempty"`
	Severity  string `json:"severity,omitempty"`
}

// Stats 表示告警统计
type Stats struct {
	Sent       int `json:"sent"`
	Suppressed int `json:"suppressed"`
	Recorded   int `json:"recorded"`
}

// RuleLevelCount 表示规则数量分布
type RuleLevelCount struct {
	Ignore   int `json:"ignore"`
	Business int `json:"business"`
	System   int `json:"system"`
	Fatal    int `json:"fatal"`
}

// RulesSummary 表示规则摘要
type RulesSummary struct {
	Source          string         `json:"source"`
	LastLoaded      string         `json:"lastLoaded"`
	Total           int            `json:"total"`
	DefaultSuppress string         `json:"defaultSuppress"`
	Escalation      string         `json:"escalation"`
	Levels          RuleLevelCount `json:"levels"`
	Error           string         `json:"error,omitempty"`
}

// PollSummary 表示轮询摘要
type PollSummary struct {
	Interval string   `json:"interval"`
	LogFiles []string `json:"logFiles"`
	LastPoll string   `json:"lastPoll"`
	NextPoll string   `json:"nextPoll"`
	Error    string   `json:"error,omitempty"`
}

type alertRecord struct {
	id       string
	at       time.Time
	level    Level
	rule     string
	message  string
	file     string
	status   DecisionStatus
	reason   string
	explain  *DecisionExplain
	analysis string
	// knowledgeTrace 是可变状态，必须通过 State 的互斥锁读写，避免并发读写竞态
	knowledgeTrace *RecommendationTrace
}

// State 维护告警决策运行态
type State struct {
	mu      sync.RWMutex
	records []alertRecord
	stats   Stats
	rules   RulesSummary
	polling PollSummary
}

// NewState 创建告警运行态
func NewState() *State {
	return &State{
		records: make([]alertRecord, 0, maxDecisionRecords),
	}
}

// Record 记录告警决策
func (s *State) Record(result decisionResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record := alertRecord{
		id:      result.id,
		at:      result.at,
		level:   result.level,
		rule:    result.rule,
		message: result.message,
		file:    result.file,
		status:  result.status,
		reason:  result.reason,
		explain: copyDecisionExplain(result.explain),
	}
	s.records = append(s.records, record)
	if len(s.records) > maxDecisionRecords {
		s.records = append([]alertRecord(nil), s.records[len(s.records)-maxDecisionRecords:]...)
	}

	switch result.status {
	case StatusSent:
		s.stats.Sent++
	case StatusSuppressed:
		s.stats.Suppressed++
	case StatusRecorded:
		s.stats.Recorded++
	}
}

// RecordDecision 用于回放或测试场景下写入告警快照
// 边界：该方法只做轻量校验，不参与规则判定；调用方需确保输入来自可信链路
func (s *State) RecordDecision(decision Decision) bool {
	if s == nil {
		return false
	}
	trimmedID := strings.TrimSpace(decision.ID)
	if trimmedID == "" {
		return false
	}
	level, ok := parseLevel(strings.TrimSpace(decision.Level))
	if !ok {
		level = LevelBusiness
	}
	status := normalizeDecisionStatus(decision.Status)
	at := parseDecisionTime(decision.Time)

	s.mu.Lock()
	defer s.mu.Unlock()

	rec := alertRecord{
		id:             trimmedID,
		at:             at,
		level:          level,
		rule:           strings.TrimSpace(decision.Rule),
		message:        strings.TrimSpace(decision.Message),
		file:           strings.TrimSpace(decision.File),
		status:         status,
		reason:         strings.TrimSpace(decision.Reason),
		explain:        copyDecisionExplainValue(decision.Explain),
		analysis:       strings.TrimSpace(decision.Analysis),
		knowledgeTrace: copyRecommendationTrace(decision.KnowledgeTrace),
	}
	s.records = append(s.records, rec)
	if len(s.records) > maxDecisionRecords {
		s.records = append([]alertRecord(nil), s.records[len(s.records)-maxDecisionRecords:]...)
	}

	switch status {
	case StatusSent:
		s.stats.Sent++
	case StatusSuppressed:
		s.stats.Suppressed++
	default:
		s.stats.Recorded++
	}
	return true
}

func normalizeDecisionStatus(raw string) DecisionStatus {
	switch DecisionStatus(strings.TrimSpace(raw)) {
	case StatusSent:
		return StatusSent
	case StatusSuppressed:
		return StatusSuppressed
	default:
		return StatusRecorded
	}
}

func parseDecisionTime(raw string) time.Time {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Now()
	}
	if ts, err := time.Parse("2006-01-02 15:04:05", trimmed); err == nil {
		return ts
	}
	if ts, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return ts
	}
	return time.Now()
}

// copyDecisionExplain 用于拷贝 explain 避免后续修改影响已落盘记录
func copyDecisionExplain(raw DecisionExplain) *DecisionExplain {
	if raw.DecisionKind == "" &&
		!raw.Notify &&
		!raw.SuppressionEnabled &&
		raw.SuppressWindow == "" &&
		raw.SuppressedBy == "" &&
		raw.EscalationThreshold == 0 &&
		raw.EscalationWindow == "" &&
		raw.EscalationCount == 0 {
		return nil
	}
	val := raw
	return &val
}

func copyRecommendationTrace(raw *RecommendationTrace) *RecommendationTrace {
	if raw == nil {
		return nil
	}
	copied := *raw
	if len(raw.Articles) > 0 {
		copied.Articles = append([]RecommendationArticle(nil), raw.Articles...)
	}
	return &copied
}

// AttachAnalysis 为指定告警记录追加AI分析
func (s *State) AttachAnalysis(id string, analysis string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.records {
		if s.records[i].id == id {
			s.records[i].analysis = analysis
			return
		}
	}
}

// GetDecision 用于按告警 ID 读取当前快照
// 这里返回副本，避免调用方持有内部指针导致并发状态污染
func (s *State) GetDecision(id string) (Decision, bool) {
	if s == nil {
		return Decision{}, false
	}
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return Decision{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.records) - 1; i >= 0; i-- {
		rec := s.records[i]
		if rec.id != trimmedID {
			continue
		}
		file := rec.file
		if file == "" {
			file = "-"
		}
		return Decision{
			ID:             rec.id,
			Time:           formatTime(rec.at),
			Level:          string(rec.level),
			Rule:           rec.rule,
			Message:        rec.message,
			File:           file,
			Status:         string(rec.status),
			Reason:         rec.reason,
			Explain:        copyDecisionExplainValue(rec.explain),
			Analysis:       rec.analysis,
			KnowledgeTrace: copyRecommendationTrace(rec.knowledgeTrace),
		}, true
	}
	return Decision{}, false
}

func copyDecisionExplainValue(raw *DecisionExplain) *DecisionExplain {
	if raw == nil {
		return nil
	}
	val := *raw
	return &val
}

// AttachKnowledgeTrace 把知识推荐结果关联到指定告警
// 告警与知识推荐都属于运行态事件，这里仅覆盖“最近一次”关联，边界是内存保留窗口（maxDecisionRecords）
func (s *State) AttachKnowledgeTrace(id string, trace RecommendationTrace) bool {
	if s == nil {
		return false
	}
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return false
	}
	if strings.TrimSpace(trace.LinkedAt) == "" {
		trace.LinkedAt = formatTime(time.Now())
	}
	if strings.TrimSpace(trace.AlertID) == "" {
		trace.AlertID = trimmedID
	}
	if strings.TrimSpace(trace.LinkID) == "" {
		trace.LinkID = fmt.Sprintf("kb-link-%s-%d", trimmedID, time.Now().UnixNano())
	}
	if trace.HitCount <= 0 {
		trace.HitCount = len(trace.Articles)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.records {
		if s.records[i].id != trimmedID {
			continue
		}
		s.records[i].knowledgeTrace = copyRecommendationTrace(&trace)
		return true
	}
	return false
}

// UpdateRulesSummary 更新规则摘要
func (s *State) UpdateRulesSummary(summary RulesSummary) {
	s.mu.Lock()
	s.rules = summary
	s.mu.Unlock()
}

// UpdatePollSummary 更新轮询摘要
func (s *State) UpdatePollSummary(summary PollSummary) {
	s.mu.Lock()
	s.polling = summary
	s.mu.Unlock()
}

// Dashboard 输出告警面板数据
func (s *State) Dashboard() Dashboard {
	s.mu.RLock()
	records := append([]alertRecord(nil), s.records...)
	stats := s.stats
	rules := s.rules
	polling := s.polling
	s.mu.RUnlock()

	overview := buildOverview(records)
	decisions := make([]Decision, 0, len(records))
	for i := len(records) - 1; i >= 0; i-- {
		rec := records[i]
		file := rec.file
		if file == "" {
			file = "-"
		}
		decisions = append(decisions, Decision{
			ID:             rec.id,
			Time:           formatTime(rec.at),
			Level:          string(rec.level),
			Rule:           rec.rule,
			Message:        rec.message,
			File:           file,
			Status:         string(rec.status),
			Reason:         rec.reason,
			Explain:        copyDecisionExplainValue(rec.explain),
			Analysis:       rec.analysis,
			KnowledgeTrace: copyRecommendationTrace(rec.knowledgeTrace),
		})
	}

	return Dashboard{
		Overview:  overview,
		Decisions: decisions,
		Stats:     stats,
		Rules:     rules,
		Polling:   polling,
	}
}

// buildOverview 用于构建后续流程所需的数据
func buildOverview(records []alertRecord) Overview {
	now := time.Now()
	// 仅统计窗口内的记录用于概览
	windowStart := now.Add(-overviewWindow)

	var fatalCount, systemCount, businessCount int
	var sentCount, suppressedCount int
	var latest string
	for _, record := range records {
		if record.at.Before(windowStart) {
			continue
		}
		switch record.level {
		case LevelFatal:
			fatalCount++
		case LevelSystem:
			systemCount++
		case LevelBusiness:
			businessCount++
		}
		switch record.status {
		case StatusSent:
			sentCount++
		case StatusSuppressed:
			suppressedCount++
		}
		latest = formatTime(record.at)
	}

	risk := "低"
	if fatalCount > 0 {
		risk = "严重"
	} else if systemCount > 0 {
		risk = "高"
	} else if businessCount > 0 {
		risk = "中"
	}

	return Overview{
		Window:     formatWindow(overviewWindow),
		Risk:       risk,
		Fatal:      fatalCount,
		System:     systemCount,
		Business:   businessCount,
		Sent:       sentCount,
		Suppressed: suppressedCount,
		Latest:     defaultTime(latest),
	}
}

// formatWindow 统一概览窗口的展示文案
func formatWindow(window time.Duration) string {
	if window%time.Hour == 0 {
		return fmt.Sprintf("最近%d小时", int(window.Hours()))
	}
	return fmt.Sprintf("最近%d分钟", int(window.Minutes()))
}

// formatTime 用于格式化输出内容
func formatTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.Format("2006-01-02 15:04:05")
}

// defaultTime 用于提供默认值保证行为稳定
func defaultTime(raw string) string {
	if raw == "" {
		return "--"
	}
	return raw
}
