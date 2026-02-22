// 本文件用于告警运行态可解释字段输出测试
package alert

import (
	"testing"
	"time"
)

func TestStateDashboard_ShouldExposeDecisionExplain(t *testing.T) {
	alertState := NewState()
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)

	alertState.Record(decisionResult{
		id:      "1",
		ruleID:  "system_db",
		rule:    "数据库连接池耗尽",
		level:   LevelSystem,
		file:    "/var/log/app.log",
		message: "连接池耗尽",
		at:      now,
		status:  StatusSuppressed,
		reason:  "5分钟内已告警",
		explain: DecisionExplain{
			DecisionKind:       DecisionKindRuleMatch,
			Notify:             true,
			SuppressionEnabled: true,
			SuppressWindow:     "5分钟",
			SuppressedBy:       SuppressedByRuleWindow,
		},
	})

	dashboard := alertState.Dashboard()
	if len(dashboard.Decisions) != 1 {
		t.Fatalf("expected 1 decision in dashboard, got %d", len(dashboard.Decisions))
	}
	decision := dashboard.Decisions[0]
	if decision.Explain == nil {
		t.Fatal("expected decision explain fields")
	}
	if decision.Explain.DecisionKind != DecisionKindRuleMatch {
		t.Fatalf("unexpected decision kind: %s", decision.Explain.DecisionKind)
	}
	if decision.Explain.SuppressedBy != SuppressedByRuleWindow {
		t.Fatalf("unexpected suppressedBy: %s", decision.Explain.SuppressedBy)
	}
	if decision.Explain.SuppressWindow != "5分钟" {
		t.Fatalf("unexpected suppress window: %s", decision.Explain.SuppressWindow)
	}
}
