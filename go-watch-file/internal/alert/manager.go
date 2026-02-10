// 本文件用于告警管理与调度
package alert

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"file-watch/internal/dingtalk"
	"file-watch/internal/email"
	"file-watch/internal/logger"
	"file-watch/internal/models"
)

// Notifier 表示告警通知发送器
type Notifier interface {
	Notify(ctx context.Context, payload NotifyPayload) error
}

// NotifyPayload 表示告警通知负载
type NotifyPayload struct {
	Level   Level
	Rule    string
	File    string
	Message string
	Time    time.Time
}

// NotifierSet 组合钉钉与邮件通知
type NotifierSet struct {
	DingTalk *dingtalk.Robot
	Email    *email.Sender
}

// Notify 发送告警通知
func (n *NotifierSet) Notify(ctx context.Context, payload NotifyPayload) error {
	if n == nil {
		return nil
	}
	var firstErr error
	if n.DingTalk != nil {
		if err := n.DingTalk.SendMarkdown(ctx, buildTitle(payload), buildMarkdown(payload)); err != nil {
			firstErr = err
		}
	}
	if n.Email != nil {
		if err := n.Email.SendMessage(ctx, buildSubject(payload), buildEmailBody(payload)); err != nil {
			if email.IsQuitError(err) {
				return firstErr
			}
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// Manager 管理告警规则加载与日志轮询
type Manager struct {
	mu              sync.Mutex
	cfg             *models.Config
	state           *State
	engine          *Engine
	tailer          *Tailer
	notifier        Notifier
	ruleset         *Ruleset
	logPaths        []string
	pollInterval    time.Duration
	startFromEnd    bool
	suppressEnabled bool
	enabled         bool
	running         bool
	ctx             context.Context
	cancel          context.CancelFunc
	aiEnabled       bool
	aiWindow        time.Duration
	aiLimiter       chan struct{}
	aiHistory       map[string]time.Time
	lineBuffers     map[string]*lineBuffer
	aiMu            sync.Mutex
}

// ConfigUpdate 表示告警配置的运行时更新
type ConfigUpdate struct {
	Enabled         bool
	SuppressEnabled bool
	LogPaths        string
	PollInterval    string
	StartFromEnd    bool
}

// NewManager 创建告警管理器
func NewManager(cfg *models.Config, notifier Notifier) (*Manager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("告警配置为空")
	}
	enabled := cfg.AlertEnabled
	if !enabled {
		return nil, nil
	}
	logPaths := parseLogPaths(cfg.AlertLogPaths)
	if len(logPaths) == 0 {
		return nil, fmt.Errorf("告警日志路径不能为空")
	}
	ruleset := cfg.AlertRules
	if ruleset == nil {
		return nil, fmt.Errorf("告警规则不能为空")
	}
	if err := NormalizeRuleset(ruleset); err != nil {
		return nil, err
	}
	engine, err := NewEngine(ruleset)
	if err != nil {
		return nil, err
	}
	pollInterval, err := parseDuration(cfg.AlertPollInterval, 2*time.Second)
	if err != nil || pollInterval <= 0 {
		return nil, fmt.Errorf("告警轮询间隔无效: %w", err)
	}
	startFromEnd := true
	if cfg.AlertStartFromEnd != nil {
		startFromEnd = *cfg.AlertStartFromEnd
	}
	suppressEnabled := true
	if cfg.AlertSuppressEnabled != nil {
		suppressEnabled = *cfg.AlertSuppressEnabled
	}

	state := NewState()
	manager := &Manager{
		cfg:             cfg,
		state:           state,
		engine:          engine,
		notifier:        notifier,
		ruleset:         ruleset,
		logPaths:        logPaths,
		pollInterval:    pollInterval,
		startFromEnd:    startFromEnd,
		suppressEnabled: suppressEnabled,
		enabled:         enabled,
		aiEnabled:       cfg.AIEnabled,
		aiWindow:        alertAIDedupeWindow,
		aiLimiter:       make(chan struct{}, alertAIWorkerLimit),
		aiHistory:       make(map[string]time.Time),
		lineBuffers:     make(map[string]*lineBuffer),
	}
	if manager.engine != nil {
		manager.engine.SetSuppressionEnabled(suppressEnabled)
	}
	manager.updateRulesSummary(ruleset, "")
	manager.updatePollSummary(time.Time{}, nil)
	manager.tailer = NewTailer(logPaths, pollInterval, startFromEnd, manager.handleLine, manager.handlePoll)
	return manager, nil
}

// Start 启动告警处理
func (m *Manager) Start() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startLocked()
}

// Stop 停止告警处理
func (m *Manager) Stop() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
}

// Enabled 返回告警是否启用
func (m *Manager) Enabled() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	enabled := m.enabled
	m.mu.Unlock()
	return enabled
}

// State 返回告警运行态
func (m *Manager) State() *State {
	if m == nil {
		return nil
	}
	return m.state
}

// UpdateConfig 运行时更新告警配置
func (m *Manager) UpdateConfig(update ConfigUpdate, shouldRun bool) error {
	if m == nil {
		return fmt.Errorf("告警未初始化")
	}
	logPathsRaw := strings.TrimSpace(update.LogPaths)
	pollRaw := strings.TrimSpace(update.PollInterval)

	pollInterval, err := parseDuration(pollRaw, 2*time.Second)
	if err != nil || pollInterval <= 0 {
		return fmt.Errorf("告警轮询间隔无效: %w", err)
	}

	parsedLogPaths := parseLogPaths(logPathsRaw)
	if update.Enabled {
		if m.ruleset == nil {
			return fmt.Errorf("告警规则不能为空")
		}
		if len(parsedLogPaths) == 0 {
			return fmt.Errorf("告警日志路径不能为空")
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	logPathsChanged := !sameStringSlice(parsedLogPaths, m.logPaths)
	pollChanged := pollInterval != m.pollInterval
	startChanged := update.StartFromEnd != m.startFromEnd
	enabledChanged := update.Enabled != m.enabled
	suppressChanged := update.SuppressEnabled != m.suppressEnabled

	m.enabled = update.Enabled
	if suppressChanged {
		m.suppressEnabled = update.SuppressEnabled
		if m.engine != nil {
			m.engine.SetSuppressionEnabled(update.SuppressEnabled)
		}
	}
	m.logPaths = parsedLogPaths
	m.pollInterval = pollInterval
	m.startFromEnd = update.StartFromEnd

	if logPathsChanged || pollChanged || startChanged || enabledChanged {
		// 影响轮询参数时刷新轮询摘要
		m.updatePollSummary(time.Time{}, nil)
	}

	if !update.Enabled {
		if m.running {
			m.stopLocked()
		}
		m.tailer = nil
		return nil
	}

	if m.tailer == nil || logPathsChanged || pollChanged || startChanged {
		if m.running {
			m.stopLocked()
		}
		m.tailer = NewTailer(parsedLogPaths, pollInterval, update.StartFromEnd, m.handleLine, m.handlePoll)
	}

	if shouldRun && !m.running {
		m.startLocked()
	} else if !shouldRun && m.running {
		m.stopLocked()
	}

	return nil
}

// UpdateRules 运行时更新告警规则
func (m *Manager) UpdateRules(ruleset *Ruleset) error {
	if m == nil {
		return fmt.Errorf("告警未初始化")
	}
	if ruleset == nil {
		return fmt.Errorf("告警规则不能为空")
	}
	if err := NormalizeRuleset(ruleset); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.engine == nil {
		engine, err := NewEngine(ruleset)
		if err != nil {
			return err
		}
		engine.SetSuppressionEnabled(m.suppressEnabled)
		m.engine = engine
	} else if err := m.engine.Reset(ruleset); err != nil {
		return err
	}

	m.ruleset = ruleset
	m.updateRulesSummary(ruleset, "")
	return nil
}

// startLocked 用于启动流程或服务
func (m *Manager) startLocked() {
	if m.tailer == nil || m.running || !m.enabled {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancel = cancel
	m.running = true
	go m.tailer.Run(ctx)
	logger.Info("告警轮询已启动: logs=%d interval=%s", len(m.logPaths), formatDuration(m.pollInterval))
}

// stopLocked 用于停止流程并释放资源
func (m *Manager) stopLocked() {
	if !m.running {
		return
	}
	if m.cancel != nil {
		m.cancel()
	}
	m.running = false
}

// handleLine 用于处理核心流程
func (m *Manager) handleLine(path, line string) {
	if m == nil || m.engine == nil || m.state == nil {
		return
	}
	now := time.Now()
	beforeLines, afterLines := m.captureLineContext(path, line)
	// 规则匹配可能产生多条决策 包含升级告警
	results := m.engine.Evaluate(line, path, now)
	if len(results) == 0 {
		return
	}
	contextLines := buildAlertContextLines(beforeLines, line, afterLines)
	for _, result := range results {
		// 决策写入运行态并按需触发通知
		m.state.Record(result)
		if result.status == StatusSent {
			m.sendNotification(result)
		}
		if m.shouldRunAlertAI(result, line) {
			signature := buildAlertAISignature(result, line)
			if !m.allowAlertAI(signature, now) {
				continue
			}
			m.enqueueAlertAI(result, line, contextLines)
		}
	}
}

// sendNotification 用于发送通知并处理异常回退
func (m *Manager) sendNotification(result decisionResult) {
	if m.notifier == nil {
		return
	}
	payload := NotifyPayload{
		Level:   result.level,
		Rule:    result.rule,
		File:    result.file,
		Message: result.message,
		Time:    result.at,
	}
	if err := m.notifier.Notify(context.Background(), payload); err != nil {
		logger.Error("发送告警通知失败: %v", err)
	}
}

// handlePoll 用于处理核心流程
func (m *Manager) handlePoll(at time.Time, pollErr error) {
	if m == nil || m.state == nil {
		return
	}
	m.updatePollSummary(at, pollErr)
}

// updatePollSummary 用于更新运行状态或配置
func (m *Manager) updatePollSummary(at time.Time, pollErr error) {
	summary := PollSummary{
		Interval: formatDuration(m.pollInterval),
		LogFiles: append([]string(nil), m.logPaths...),
		LastPoll: formatTime(at),
		NextPoll: formatTime(at.Add(m.pollInterval)),
	}
	if at.IsZero() {
		summary.LastPoll = "--"
		summary.NextPoll = "--"
	}
	if pollErr != nil {
		summary.Error = pollErr.Error()
	}
	m.state.UpdatePollSummary(summary)
}

// updateRulesSummary 用于更新运行状态或配置
func (m *Manager) updateRulesSummary(ruleset *Ruleset, errMsg string) {
	summary := RulesSummary{
		Source:     "控制台",
		LastLoaded: formatTime(time.Now()),
		Error:      errMsg,
	}
	if ruleset == nil {
		m.state.UpdateRulesSummary(summary)
		return
	}
	levelCount := RuleLevelCount{}
	for _, rule := range ruleset.Rules {
		switch Level(rule.Level) {
		case LevelIgnore:
			levelCount.Ignore++
		case LevelBusiness:
			levelCount.Business++
		case LevelSystem:
			levelCount.System++
		case LevelFatal:
			levelCount.Fatal++
		}
	}
	summary.Total = len(ruleset.Rules)
	summary.Levels = levelCount
	summary.DefaultSuppress = ruleset.Defaults.SuppressWindow
	if d, err := parseDuration(ruleset.Defaults.SuppressWindow, 0); err == nil && d > 0 {
		summary.DefaultSuppress = formatDuration(d)
	}
	summary.Escalation = buildEscalationSummary(ruleset.Escalation)
	m.state.UpdateRulesSummary(summary)
}

// buildEscalationSummary 用于构建后续流程所需的数据
func buildEscalationSummary(raw EscalationRule) string {
	enabled := false
	if raw.Enabled != nil {
		enabled = *raw.Enabled
	} else if raw.Threshold > 0 {
		enabled = true
	}
	if !enabled {
		return "未启用"
	}
	window := raw.Window
	if d, err := parseDuration(raw.Window, 0); err == nil && d > 0 {
		window = formatDuration(d)
	}
	level := raw.Level
	if strings.TrimSpace(level) == "" {
		level = string(LevelFatal)
	}
	return fmt.Sprintf("阈值%d次 / %s -> %s", raw.Threshold, window, strings.ToLower(level))
}

// parseLogPaths 用于解析输入参数或配置
func parseLogPaths(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' ' || r == '，' || r == '；'
	})
	seen := make(map[string]struct{})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		cleaned := filepath.Clean(trimmed)
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		out = append(out, cleaned)
	}
	return out
}

// sameStringSlice 用于判断两个集合是否等价
func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// buildTitle 用于构建后续流程所需的数据
func buildTitle(payload NotifyPayload) string {
	return fmt.Sprintf("告警 %s", strings.ToUpper(string(payload.Level)))
}

// buildSubject 用于构建后续流程所需的数据
func buildSubject(payload NotifyPayload) string {
	return fmt.Sprintf("告警通知 [%s]", strings.ToUpper(string(payload.Level)))
}

// buildMarkdown 用于构建后续流程所需的数据
func buildMarkdown(payload NotifyPayload) string {
	file := payload.File
	if strings.TrimSpace(file) == "" {
		file = "无"
	}
	return fmt.Sprintf("### 告警详情\n\n- 级别: %s\n- 规则: %s\n- 文件: `%s`\n- 时间: %s\n- 内容: %s",
		strings.ToLower(string(payload.Level)),
		payload.Rule,
		file,
		formatTime(payload.Time),
		payload.Message,
	)
}

// buildEmailBody 用于构建后续流程所需的数据
func buildEmailBody(payload NotifyPayload) string {
	file := payload.File
	if strings.TrimSpace(file) == "" {
		file = "无"
	}
	return fmt.Sprintf("级别: %s\n规则: %s\n文件: %s\n时间: %s\n内容: %s\n",
		strings.ToLower(string(payload.Level)),
		payload.Rule,
		file,
		formatTime(payload.Time),
		payload.Message,
	)
}
