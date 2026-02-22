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

func TestState_ShouldAttachKnowledgeTrace(t *testing.T) {
	alertState := NewState()
	now := time.Date(2026, 2, 22, 13, 0, 0, 0, time.UTC)

	alertState.Record(decisionResult{
		id:      "42",
		ruleID:  "system_db",
		rule:    "数据库连接池耗尽",
		level:   LevelSystem,
		file:    "/var/log/app.log",
		message: "连接池耗尽",
		at:      now,
		status:  StatusSent,
	})

	trace := RecommendationTrace{
		LinkID:   "kb-link-42",
		LinkedAt: "2026-02-22 13:01:02",
		Query:    "数据库连接池耗尽",
		Rule:     "数据库连接池耗尽",
		Message:  "连接池耗尽",
		HitCount: 1,
		Articles: []RecommendationArticle{
			{
				ArticleID: "kb_1",
				Title:     "连接池耗尽排障手册",
				Version:   3,
				Status:    "published",
				Severity:  "high",
			},
		},
	}
	if ok := alertState.AttachKnowledgeTrace("42", trace); !ok {
		t.Fatal("expected attach knowledge trace success")
	}

	// 修改原始输入，确保状态中持有的是拷贝而不是共享引用
	trace.Articles[0].Title = "mutated"
	trace.Query = "mutated-query"

	decision, found := alertState.GetDecision("42")
	if !found {
		t.Fatal("expected decision found")
	}
	if decision.KnowledgeTrace == nil {
		t.Fatal("expected knowledge trace on decision")
	}
	if decision.KnowledgeTrace.LinkID != "kb-link-42" {
		t.Fatalf("unexpected link id: %s", decision.KnowledgeTrace.LinkID)
	}
	if decision.KnowledgeTrace.Query != "数据库连接池耗尽" {
		t.Fatalf("unexpected trace query: %s", decision.KnowledgeTrace.Query)
	}
	if len(decision.KnowledgeTrace.Articles) != 1 {
		t.Fatalf("unexpected trace articles length: %d", len(decision.KnowledgeTrace.Articles))
	}
	if decision.KnowledgeTrace.Articles[0].Title != "连接池耗尽排障手册" {
		t.Fatalf("unexpected trace article title: %s", decision.KnowledgeTrace.Articles[0].Title)
	}

	dashboard := alertState.Dashboard()
	if len(dashboard.Decisions) != 1 {
		t.Fatalf("expected one dashboard decision, got %d", len(dashboard.Decisions))
	}
	if dashboard.Decisions[0].KnowledgeTrace == nil {
		t.Fatal("expected dashboard expose knowledge trace")
	}
}
