package kb

import "testing"

func publishTestArticle(t *testing.T, s *Service, title, summary, content string, tags []string) *Article {
	t.Helper()
	article, err := s.CreateArticle(CreateArticleInput{
		Title:     title,
		Summary:   summary,
		Category:  "ops",
		Severity:  SeverityMedium,
		Content:   content,
		Tags:      tags,
		CreatedBy: "tester",
	})
	if err != nil {
		t.Fatalf("create article failed: %v", err)
	}
	if _, err := s.ApplyAction(article.ID, "submit", "tester", "submit"); err != nil {
		t.Fatalf("submit article failed: %v", err)
	}
	published, err := s.ApplyAction(article.ID, "approve", "reviewer", "approve")
	if err != nil {
		t.Fatalf("approve article failed: %v", err)
	}
	return published
}

func TestSearchFallbackTokenScoring(t *testing.T) {
	s, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("new kb service failed: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	target := publishTestArticle(
		t,
		s,
		"Upload Queue Saturation Runbook",
		"Handle upload queue full and retry behaviors",
		"When upload queue full occurs, tune upload workers and queue size first.",
		[]string{"upload", "queue", "runbook"},
	)
	_ = publishTestArticle(
		t,
		s,
		"AI Degraded Troubleshooting",
		"Fallback behavior",
		"Check AI endpoint timeout and network errors.",
		[]string{"ai"},
	)

	items, err := s.Search("upload queue full 的处理步骤", 3, false)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(items) == 0 {
		t.Fatalf("expected at least one search result")
	}
	if items[0].ID != target.ID {
		t.Fatalf("expected first result %s, got %s", target.ID, items[0].ID)
	}
}

func TestAskReturnsCitationWithLongQuestion(t *testing.T) {
	s, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("new kb service failed: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	_ = publishTestArticle(
		t,
		s,
		"上传队列堆积排查",
		"上传队列满时的处理步骤",
		"当上传队列堆积时，先检查 queue_size 与 workers，再查看 retry 和失败原因。",
		[]string{"上传", "队列"},
	)

	result, err := s.Ask("上传队列积压如何排查", 3)
	if err != nil {
		t.Fatalf("ask failed: %v", err)
	}
	if len(result.Citations) == 0 {
		t.Fatalf("expected ask to return citations")
	}
}

func TestTokenizeSearchQueryKeepsUsefulTerms(t *testing.T) {
	tokens := tokenizeSearchQuery("CORS origin not allowed 怎么处理")
	if len(tokens) == 0 {
		t.Fatalf("expected tokens, got empty")
	}
	want := map[string]bool{
		"cors":    false,
		"origin":  false,
		"allowed": false,
	}
	for _, token := range tokens {
		if _, ok := want[token]; ok {
			want[token] = true
		}
	}
	for key, ok := range want {
		if !ok {
			t.Fatalf("expected token %q in %v", key, tokens)
		}
	}
}
