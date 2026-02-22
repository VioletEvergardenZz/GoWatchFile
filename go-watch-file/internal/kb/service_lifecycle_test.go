package kb

import "testing"

func TestApplyAction_ShouldUseReviewingLifecycle(t *testing.T) {
	s, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("new kb service failed: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	article, err := s.CreateArticle(CreateArticleInput{
		Title:     "连接池排障",
		Summary:   "连接池耗尽",
		Category:  "runbook",
		Severity:  SeverityHigh,
		Content:   "step1\nstep2",
		CreatedBy: "tester",
	})
	if err != nil {
		t.Fatalf("create article failed: %v", err)
	}
	if article.Status != StatusDraft {
		t.Fatalf("unexpected initial status: %s", article.Status)
	}

	if _, err := s.ApplyAction(article.ID, "approve", "reviewer", "approve without submit"); err == nil {
		t.Fatal("approve should fail when article is not reviewing")
	}

	submitted, err := s.ApplyAction(article.ID, "submit", "reviewer", "submit")
	if err != nil {
		t.Fatalf("submit article failed: %v", err)
	}
	if submitted.Status != StatusReviewing {
		t.Fatalf("unexpected status after submit: %s", submitted.Status)
	}

	rejected, err := s.ApplyAction(article.ID, "reject", "reviewer", "reject")
	if err != nil {
		t.Fatalf("reject article failed: %v", err)
	}
	if rejected.Status != StatusDraft {
		t.Fatalf("unexpected status after reject: %s", rejected.Status)
	}

	submittedAgain, err := s.ApplyAction(article.ID, "submit", "reviewer", "submit again")
	if err != nil {
		t.Fatalf("submit article second time failed: %v", err)
	}
	if submittedAgain.Status != StatusReviewing {
		t.Fatalf("unexpected status after second submit: %s", submittedAgain.Status)
	}

	approved, err := s.ApplyAction(article.ID, "approve", "reviewer", "approve")
	if err != nil {
		t.Fatalf("approve article failed: %v", err)
	}
	if approved.Status != StatusPublished {
		t.Fatalf("unexpected status after approve: %s", approved.Status)
	}

	if _, err := s.ApplyAction(article.ID, "submit", "reviewer", "submit published"); err == nil {
		t.Fatal("submit should fail when article is published")
	}

	archived, err := s.ApplyAction(article.ID, "archive", "reviewer", "archive")
	if err != nil {
		t.Fatalf("archive article failed: %v", err)
	}
	if archived.Status != StatusArchived {
		t.Fatalf("unexpected status after archive: %s", archived.Status)
	}
}

func TestPendingReviews_ShouldOnlyContainReviewingOrNeedsReview(t *testing.T) {
	s, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("new kb service failed: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	draft, err := s.CreateArticle(CreateArticleInput{
		Title:     "纯草稿条目",
		Summary:   "尚未提交",
		Category:  "runbook",
		Severity:  SeverityLow,
		Content:   "draft",
		CreatedBy: "tester",
	})
	if err != nil {
		t.Fatalf("create draft article failed: %v", err)
	}

	reviewing, err := s.CreateArticle(CreateArticleInput{
		Title:     "待审核条目",
		Summary:   "已提交审核",
		Category:  "runbook",
		Severity:  SeverityMedium,
		Content:   "reviewing",
		CreatedBy: "tester",
	})
	if err != nil {
		t.Fatalf("create reviewing article failed: %v", err)
	}
	reviewing, err = s.ApplyAction(reviewing.ID, "submit", "tester", "submit")
	if err != nil {
		t.Fatalf("submit reviewing article failed: %v", err)
	}
	if reviewing.Status != StatusReviewing {
		t.Fatalf("unexpected reviewing status: %s", reviewing.Status)
	}

	items, err := s.PendingReviews(10)
	if err != nil {
		t.Fatalf("pending reviews failed: %v", err)
	}
	if !containsArticleID(items, reviewing.ID) {
		t.Fatalf("pending reviews should contain reviewing article %s", reviewing.ID)
	}
	if containsArticleID(items, draft.ID) {
		t.Fatalf("pending reviews should not contain plain draft article %s", draft.ID)
	}
}

func containsArticleID(items []Article, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}
