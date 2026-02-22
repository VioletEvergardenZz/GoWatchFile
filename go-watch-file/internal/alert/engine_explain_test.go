// 本文件用于告警可解释字段的单元测试
package alert

import (
	"strings"
	"testing"
	"time"
)

func TestEngineEvaluate_RuleSuppressionExplain(t *testing.T) {
	ruleset := &Ruleset{
		Version: 1,
		Defaults: RuleDefaults{
			SuppressWindow: "5m",
		},
		Rules: []Rule{
			{
				ID:       "system_db",
				Title:    "数据库连接池耗尽",
				Level:    "system",
				Keywords: []string{"连接池耗尽"},
			},
		},
	}
	if err := NormalizeRuleset(ruleset); err != nil {
		t.Fatalf("normalize ruleset failed: %v", err)
	}
	engine, err := NewEngine(ruleset)
	if err != nil {
		t.Fatalf("new engine failed: %v", err)
	}

	now := time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC)

	first := engine.Evaluate("连接池耗尽", "/var/log/app.log", now)
	if len(first) != 1 {
		t.Fatalf("first evaluate should return 1 decision, got %d", len(first))
	}
	if first[0].status != StatusSent {
		t.Fatalf("first decision should be sent, got %s", first[0].status)
	}
	if first[0].explain.DecisionKind != DecisionKindRuleMatch {
		t.Fatalf("unexpected decision kind: %s", first[0].explain.DecisionKind)
	}
	if !first[0].explain.Notify {
		t.Fatal("rule decision should notify by default for system level")
	}
	if !first[0].explain.SuppressionEnabled {
		t.Fatal("suppression should be enabled by default")
	}
	if first[0].explain.SuppressWindow != "5分钟" {
		t.Fatalf("unexpected suppress window: %s", first[0].explain.SuppressWindow)
	}
	if first[0].explain.SuppressedBy != "" {
		t.Fatalf("first decision should not be suppressed, got %s", first[0].explain.SuppressedBy)
	}

	second := engine.Evaluate("连接池耗尽", "/var/log/app.log", now.Add(time.Minute))
	if len(second) != 1 {
		t.Fatalf("second evaluate should return 1 decision, got %d", len(second))
	}
	if second[0].status != StatusSuppressed {
		t.Fatalf("second decision should be suppressed, got %s", second[0].status)
	}
	if second[0].explain.SuppressedBy != SuppressedByRuleWindow {
		t.Fatalf("unexpected suppressedBy: %s", second[0].explain.SuppressedBy)
	}
	if !strings.Contains(second[0].reason, "5分钟内已告警") {
		t.Fatalf("unexpected reason: %s", second[0].reason)
	}
}

func TestEngineEvaluate_EscalationExplainAndSuppression(t *testing.T) {
	enabled := true
	ruleset := &Ruleset{
		Version: 1,
		Defaults: RuleDefaults{
			SuppressWindow: "1s",
		},
		Escalation: EscalationRule{
			Enabled:        &enabled,
			Level:          "fatal",
			Window:         "30s",
			Threshold:      2,
			SuppressWindow: "5m",
			RuleID:         "system_spike",
			Title:          "系统异常激增",
		},
		Rules: []Rule{
			{
				ID:       "system_error",
				Title:    "系统异常",
				Level:    "system",
				Keywords: []string{"panic"},
			},
		},
	}
	if err := NormalizeRuleset(ruleset); err != nil {
		t.Fatalf("normalize ruleset failed: %v", err)
	}
	engine, err := NewEngine(ruleset)
	if err != nil {
		t.Fatalf("new engine failed: %v", err)
	}

	now := time.Date(2026, 2, 22, 11, 0, 0, 0, time.UTC)

	first := engine.Evaluate("panic: first", "/var/log/app.log", now)
	if len(first) != 1 {
		t.Fatalf("first evaluate should return 1 decision, got %d", len(first))
	}
	second := engine.Evaluate("panic: second", "/var/log/app.log", now.Add(time.Second))
	if len(second) != 2 {
		t.Fatalf("second evaluate should return 2 decisions (rule + escalation), got %d", len(second))
	}

	escalationSent := second[1]
	if escalationSent.status != StatusSent {
		t.Fatalf("escalation should be sent on first trigger, got %s", escalationSent.status)
	}
	if escalationSent.explain.DecisionKind != DecisionKindEscalation {
		t.Fatalf("unexpected escalation decision kind: %s", escalationSent.explain.DecisionKind)
	}
	if escalationSent.explain.EscalationThreshold != 2 {
		t.Fatalf("unexpected escalation threshold: %d", escalationSent.explain.EscalationThreshold)
	}
	if escalationSent.explain.EscalationWindow != "30秒" {
		t.Fatalf("unexpected escalation window: %s", escalationSent.explain.EscalationWindow)
	}
	if escalationSent.explain.EscalationCount != 2 {
		t.Fatalf("unexpected escalation count: %d", escalationSent.explain.EscalationCount)
	}
	if escalationSent.explain.SuppressedBy != "" {
		t.Fatalf("unexpected suppressedBy for first escalation: %s", escalationSent.explain.SuppressedBy)
	}

	_ = engine.Evaluate("panic: third", "/var/log/app.log", now.Add(40*time.Second))
	fourth := engine.Evaluate("panic: fourth", "/var/log/app.log", now.Add(41*time.Second))
	if len(fourth) != 2 {
		t.Fatalf("fourth evaluate should return 2 decisions (rule + escalation), got %d", len(fourth))
	}

	escalationSuppressed := fourth[1]
	if escalationSuppressed.status != StatusSuppressed {
		t.Fatalf("escalation should be suppressed in suppress window, got %s", escalationSuppressed.status)
	}
	if escalationSuppressed.explain.SuppressedBy != SuppressedByEscalationWindow {
		t.Fatalf("unexpected suppressedBy: %s", escalationSuppressed.explain.SuppressedBy)
	}
	if !strings.Contains(escalationSuppressed.reason, "5分钟内已升级") {
		t.Fatalf("unexpected reason: %s", escalationSuppressed.reason)
	}
}
