// 本文件用于告警规则匹配与抑制逻辑
package alert

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const maxDecisionMessage = 240

type compiledRule struct {
	id             string
	title          string
	level          Level
	keywords       []string
	excludes       []string
	matchCase      bool
	suppressWindow time.Duration
	notify         bool
}

type compiledEscalation struct {
	enabled        bool
	level          Level
	threshold      int
	window         time.Duration
	suppressWindow time.Duration
	ruleID         string
	title          string
	message        string
}

type decisionResult struct {
	id      string
	ruleID  string
	rule    string
	level   Level
	file    string
	message string
	at      time.Time
	status  DecisionStatus
	reason  string
}

// Engine 负责规则匹配与抑制升级判定
type Engine struct {
	mu                 sync.Mutex
	rules              []compiledRule
	escalation         compiledEscalation
	lastAlert          map[string]time.Time
	systemEvents       []time.Time
	escalationActive   bool
	lastEscalationAt   time.Time
	suppressionEnabled bool
	seq                atomic.Uint64
}

// NewEngine 构建告警规则引擎
func NewEngine(ruleset *Ruleset) (*Engine, error) {
	rules, escalation, err := compileRules(ruleset)
	if err != nil {
		return nil, err
	}
	return &Engine{
		rules:              rules,
		escalation:         escalation,
		lastAlert:          make(map[string]time.Time),
		systemEvents:       make([]time.Time, 0, 32),
		suppressionEnabled: true,
	}, nil
}

// Reset 用于重载规则并重置内部状态
func (e *Engine) Reset(ruleset *Ruleset) error {
	rules, escalation, err := compileRules(ruleset)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.rules = rules
	e.escalation = escalation
	e.lastAlert = make(map[string]time.Time)
	e.systemEvents = e.systemEvents[:0]
	e.escalationActive = false
	e.lastEscalationAt = time.Time{}
	e.mu.Unlock()
	return nil
}

// SetSuppressionEnabled 控制是否启用抑制窗口
func (e *Engine) SetSuppressionEnabled(enabled bool) {
	if e == nil {
		return
	}
	e.mu.Lock()
	if e.suppressionEnabled == enabled {
		e.mu.Unlock()
		return
	}
	e.suppressionEnabled = enabled
	e.lastAlert = make(map[string]time.Time)
	e.lastEscalationAt = time.Time{}
	e.mu.Unlock()
}

// Evaluate 根据日志内容生成告警决策
func (e *Engine) Evaluate(line, filePath string, now time.Time) []decisionResult {
	cleaned := strings.TrimSpace(line)
	if cleaned == "" {
		return nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// 按顺序匹配第一条命中的规则
	rule := e.matchRuleLocked(cleaned)
	if rule == nil || rule.level == LevelIgnore {
		return nil
	}

	result := decisionResult{
		id:      e.nextID(),
		ruleID:  rule.id,
		rule:    rule.title,
		level:   rule.level,
		file:    filePath,
		message: truncateMessage(cleaned, maxDecisionMessage),
		at:      now,
	}
	// 应用抑制窗口与通知策略
	result.status, result.reason = e.applySuppressionLocked(rule, now)

	if rule.level == LevelSystem {
		// system 级别用于异常升级统计
		e.appendSystemEventLocked(now)
	}

	results := []decisionResult{result}
	if escalation := e.maybeEscalateLocked(now); escalation != nil {
		results = append(results, *escalation)
	}
	return results
}

// matchRuleLocked 用于在持锁状态下匹配告警规则
func (e *Engine) matchRuleLocked(line string) *compiledRule {
	for i := range e.rules {
		rule := &e.rules[i]
		haystack := line
		if !rule.matchCase {
			haystack = strings.ToLower(line)
		}
		if !containsAny(haystack, rule.keywords) {
			continue
		}
		if len(rule.excludes) > 0 && containsAny(haystack, rule.excludes) {
			continue
		}
		return rule
	}
	return nil
}

// applySuppressionLocked 用于应用配置并保持运行态一致
func (e *Engine) applySuppressionLocked(rule *compiledRule, now time.Time) (DecisionStatus, string) {
	if !rule.notify {
		return StatusRecorded, ""
	}
	if !e.suppressionEnabled {
		return StatusSent, ""
	}
	if rule.suppressWindow <= 0 {
		e.lastAlert[rule.id] = now
		return StatusSent, ""
	}
	last, ok := e.lastAlert[rule.id]
	if ok && now.Sub(last) < rule.suppressWindow {
		return StatusSuppressed, fmt.Sprintf("%s内已告警", formatDuration(rule.suppressWindow))
	}
	e.lastAlert[rule.id] = now
	return StatusSent, ""
}

// appendSystemEventLocked 用于添加数据到目标集合
func (e *Engine) appendSystemEventLocked(now time.Time) {
	if e.escalation.window <= 0 {
		return
	}
	e.systemEvents = append(e.systemEvents, now)
	e.pruneSystemEventsLocked(now)
}

// pruneSystemEventsLocked 用于在持锁状态下清理过期系统事件
func (e *Engine) pruneSystemEventsLocked(now time.Time) {
	if e.escalation.window <= 0 || len(e.systemEvents) == 0 {
		return
	}
	cut := 0
	for _, ts := range e.systemEvents {
		if now.Sub(ts) <= e.escalation.window {
			break
		}
		cut++
	}
	if cut > 0 {
		e.systemEvents = append([]time.Time(nil), e.systemEvents[cut:]...)
	}
}

// maybeEscalateLocked 用于按条件决定是否触发附加流程
func (e *Engine) maybeEscalateLocked(now time.Time) *decisionResult {
	if !e.escalation.enabled || e.escalation.threshold <= 0 {
		return nil
	}
	e.pruneSystemEventsLocked(now)
	if len(e.systemEvents) < e.escalation.threshold {
		e.escalationActive = false
		return nil
	}
	if e.escalationActive {
		return nil
	}
	e.escalationActive = true

	status := StatusSent
	reason := ""
	if e.suppressionEnabled && e.escalation.suppressWindow > 0 && !e.lastEscalationAt.IsZero() && now.Sub(e.lastEscalationAt) < e.escalation.suppressWindow {
		status = StatusSuppressed
		reason = fmt.Sprintf("%s内已升级", formatDuration(e.escalation.suppressWindow))
	} else if e.suppressionEnabled {
		e.lastEscalationAt = now
	}

	message := e.escalation.message
	if message == "" {
		message = fmt.Sprintf("系统异常在%s内达到%d次", formatDuration(e.escalation.window), e.escalation.threshold)
	}
	return &decisionResult{
		id:      e.nextID(),
		ruleID:  e.escalation.ruleID,
		rule:    e.escalation.title,
		level:   e.escalation.level,
		file:    "",
		message: message,
		at:      now,
		status:  status,
		reason:  reason,
	}
}

// nextID 用于生成递增标识保证事件可追踪
func (e *Engine) nextID() string {
	return fmt.Sprintf("%d", e.seq.Add(1))
}

// compileRules 用于编译规则为高效匹配结构
func compileRules(ruleset *Ruleset) ([]compiledRule, compiledEscalation, error) {
	if ruleset == nil {
		return nil, compiledEscalation{}, fmt.Errorf("告警规则为空")
	}

	defaultMatchCase := false
	if ruleset.Defaults.MatchCase != nil {
		defaultMatchCase = *ruleset.Defaults.MatchCase
	}
	defaultSuppress, err := parseDuration(ruleset.Defaults.SuppressWindow, 0)
	if err != nil {
		return nil, compiledEscalation{}, fmt.Errorf("默认抑制窗口无效: %w", err)
	}
	if defaultSuppress <= 0 {
		defaultSuppress = 5 * time.Minute
	}

	compiled := make([]compiledRule, 0, len(ruleset.Rules))
	for _, rule := range ruleset.Rules {
		level, ok := parseLevel(rule.Level)
		if !ok {
			return nil, compiledEscalation{}, fmt.Errorf("无效的告警级别: %s", rule.Level)
		}
		matchCase := defaultMatchCase
		if rule.MatchCase != nil {
			matchCase = *rule.MatchCase
		}
		suppress := defaultSuppress
		if strings.TrimSpace(rule.SuppressWindow) != "" {
			val, err := parseDuration(rule.SuppressWindow, defaultSuppress)
			if err != nil {
				return nil, compiledEscalation{}, fmt.Errorf("告警规则 %s 抑制窗口无效: %w", rule.ID, err)
			}
			suppress = val
		}
		notify := false
		if rule.Notify != nil {
			notify = *rule.Notify
		} else {
			notify = level == LevelSystem || level == LevelFatal
		}
		keywords := rule.Keywords
		excludes := rule.Excludes
		if !matchCase {
			keywords = lowerSlice(keywords)
			excludes = lowerSlice(excludes)
		}
		compiled = append(compiled, compiledRule{
			id:             rule.ID,
			title:          rule.Title,
			level:          level,
			keywords:       keywords,
			excludes:       excludes,
			matchCase:      matchCase,
			suppressWindow: suppress,
			notify:         notify,
		})
	}
	escalation, err := compileEscalation(ruleset.Escalation)
	if err != nil {
		return nil, compiledEscalation{}, err
	}
	return compiled, escalation, nil
}

// compileEscalation 用于编译升级策略并校验阈值配置
func compileEscalation(raw EscalationRule) (compiledEscalation, error) {
	level := LevelFatal
	if strings.TrimSpace(raw.Level) != "" {
		parsed, ok := parseLevel(strings.ToLower(strings.TrimSpace(raw.Level)))
		if !ok {
			return compiledEscalation{}, fmt.Errorf("异常升级级别无效: %s", raw.Level)
		}
		level = parsed
	}
	enabled := false
	if raw.Enabled != nil {
		enabled = *raw.Enabled
	} else if raw.Threshold > 0 {
		enabled = true
	}
	window, err := parseDuration(raw.Window, 0)
	if err != nil {
		return compiledEscalation{}, fmt.Errorf("异常升级窗口无效: %w", err)
	}
	suppress, err := parseDuration(raw.SuppressWindow, 0)
	if err != nil {
		return compiledEscalation{}, fmt.Errorf("异常升级抑制窗口无效: %w", err)
	}
	if suppress <= 0 && window > 0 {
		suppress = window
	}
	if enabled && raw.Threshold <= 0 {
		return compiledEscalation{}, fmt.Errorf("异常升级阈值必须大于零")
	}
	if enabled && window <= 0 {
		return compiledEscalation{}, fmt.Errorf("异常升级窗口必须大于零")
	}
	ruleID := strings.TrimSpace(raw.RuleID)
	if ruleID == "" {
		ruleID = "system_spike"
	}
	title := strings.TrimSpace(raw.Title)
	if title == "" {
		title = "系统异常激增"
	}
	message := strings.TrimSpace(raw.Message)

	return compiledEscalation{
		enabled:        enabled,
		level:          level,
		threshold:      raw.Threshold,
		window:         window,
		suppressWindow: suppress,
		ruleID:         ruleID,
		title:          title,
		message:        message,
	}, nil
}

// containsAny 用于判断集合中是否包含目标项
func containsAny(haystack string, keywords []string) bool {
	for _, keyword := range keywords {
		if keyword == "" {
			continue
		}
		if strings.Contains(haystack, keyword) {
			return true
		}
	}
	return false
}

// lowerSlice 用于统一大小写便于比较
func lowerSlice(values []string) []string {
	if len(values) == 0 {
		return values
	}
	out := make([]string, 0, len(values))
	for _, val := range values {
		out = append(out, strings.ToLower(val))
	}
	return out
}

// truncateMessage 用于截断内容以控制大小
func truncateMessage(message string, limit int) string {
	if limit <= 0 || len(message) <= limit {
		return message
	}
	if limit <= 3 {
		return message[:limit]
	}
	return message[:limit-3] + "..."
}

// formatDuration 用于格式化输出内容
func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "0秒"
	}
	if d >= time.Hour {
		hours := int(d.Round(time.Minute).Hours())
		if hours <= 0 {
			hours = 1
		}
		return fmt.Sprintf("%d小时", hours)
	}
	if d >= time.Minute {
		minutes := int(d.Round(time.Second).Minutes())
		if minutes <= 0 {
			minutes = 1
		}
		return fmt.Sprintf("%d分钟", minutes)
	}
	seconds := int(d.Round(time.Second).Seconds())
	if seconds <= 0 {
		seconds = 1
	}
	return fmt.Sprintf("%d秒", seconds)
}

// parseDuration 用于解析输入参数或配置
func parseDuration(raw string, fallback time.Duration) (time.Duration, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback, nil
	}
	clean := strings.ToLower(trimmed)
	clean = strings.ReplaceAll(clean, "秒钟", "秒")
	clean = strings.ReplaceAll(clean, "秒", "s")
	clean = strings.ReplaceAll(clean, "分钟", "m")
	clean = strings.ReplaceAll(clean, "分", "m")
	clean = strings.ReplaceAll(clean, "小时", "h")
	clean = strings.TrimSpace(clean)
	if d, err := time.ParseDuration(clean); err == nil && d > 0 {
		return d, nil
	}
	numRe := regexp.MustCompile(`\d+`)
	if m := numRe.FindString(clean); m != "" {
		if v, err := strconv.Atoi(m); err == nil && v > 0 {
			return time.Duration(v) * time.Second, nil
		}
	}
	return 0, fmt.Errorf("无效时间: %s", raw)
}
