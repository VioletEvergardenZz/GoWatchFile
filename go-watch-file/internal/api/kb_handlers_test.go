// 本文件用于知识库处理器测试 通过接口级用例保障知识库闭环行为

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"file-watch/internal/alert"
	"file-watch/internal/kb"
	"file-watch/internal/models"
)

func TestKBHandlers_ArticleLifecycle(t *testing.T) {
	h, cleanup := newKBTestHandler(t)
	defer cleanup()

	createBody := map[string]any{
		"title":    "上传队列拥塞排障",
		"summary":  "用于排查队列堆积",
		"category": "runbook",
		"severity": "high",
		"content":  "step1\nstep2",
		"tags":     []string{"queue", "upload"},
	}
	createResp := doJSONRequest(t, h.kbArticles, http.MethodPost, "/api/kb/articles", createBody)
	if createResp.Code != http.StatusOK {
		t.Fatalf("create article failed: status=%d body=%s", createResp.Code, createResp.Body.String())
	}
	var created struct {
		OK      bool       `json:"ok"`
		Article kb.Article `json:"article"`
	}
	mustDecodeJSON(t, createResp.Body.Bytes(), &created)
	if !created.OK || created.Article.ID == "" {
		t.Fatalf("unexpected create response: %+v", created)
	}
	articleID := created.Article.ID

	listResp := doJSONRequest(t, h.kbArticles, http.MethodGet, "/api/kb/articles?status=draft", nil)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list articles failed: status=%d body=%s", listResp.Code, listResp.Body.String())
	}
	var listed struct {
		OK    bool         `json:"ok"`
		Items []kb.Article `json:"items"`
		Total int          `json:"total"`
	}
	mustDecodeJSON(t, listResp.Body.Bytes(), &listed)
	if !listed.OK || listed.Total < 1 {
		t.Fatalf("unexpected list response: %+v", listed)
	}

	updateBody := map[string]any{
		"title":      "上传队列拥塞排障-v2",
		"summary":    "更新后的说明",
		"category":   "runbook",
		"severity":   "medium",
		"content":    "step1-updated\nstep2-updated",
		"tags":       []string{"queue", "retry"},
		"updatedBy":  "tester",
		"changeNote": "update in test",
	}
	updateResp := doJSONRequest(t, h.kbArticleByID, http.MethodPut, "/api/kb/articles/"+articleID, updateBody)
	if updateResp.Code != http.StatusOK {
		t.Fatalf("update article failed: status=%d body=%s", updateResp.Code, updateResp.Body.String())
	}
	var updated struct {
		OK      bool       `json:"ok"`
		Article kb.Article `json:"article"`
	}
	mustDecodeJSON(t, updateResp.Body.Bytes(), &updated)
	if !updated.OK || updated.Article.CurrentVersion < 2 {
		t.Fatalf("unexpected update response: %+v", updated)
	}

	submitResp := doJSONRequest(t, h.kbArticleByID, http.MethodPost, "/api/kb/articles/"+articleID+"/submit", map[string]any{
		"operator": "tester",
		"comment":  "submit test",
	})
	if submitResp.Code != http.StatusOK {
		t.Fatalf("submit article failed: status=%d body=%s", submitResp.Code, submitResp.Body.String())
	}
	var submitted struct {
		OK      bool       `json:"ok"`
		Article kb.Article `json:"article"`
	}
	mustDecodeJSON(t, submitResp.Body.Bytes(), &submitted)
	if !submitted.OK || submitted.Article.Status != kb.StatusReviewing {
		t.Fatalf("unexpected submit response: %+v", submitted)
	}

	approveResp := doJSONRequest(t, h.kbArticleByID, http.MethodPost, "/api/kb/articles/"+articleID+"/approve", map[string]any{
		"operator": "tester",
		"comment":  "approve test",
	})
	if approveResp.Code != http.StatusOK {
		t.Fatalf("approve article failed: status=%d body=%s", approveResp.Code, approveResp.Body.String())
	}
	var approved struct {
		OK      bool       `json:"ok"`
		Article kb.Article `json:"article"`
	}
	mustDecodeJSON(t, approveResp.Body.Bytes(), &approved)
	if !approved.OK || approved.Article.Status != kb.StatusPublished {
		t.Fatalf("unexpected approve response: %+v", approved)
	}

	rollbackResp := doJSONRequest(t, h.kbArticleByID, http.MethodPost, "/api/kb/articles/"+articleID+"/rollback", map[string]any{
		"targetVersion": 1,
		"operator":      "tester",
		"comment":       "rollback test",
	})
	if rollbackResp.Code != http.StatusOK {
		t.Fatalf("rollback article failed: status=%d body=%s", rollbackResp.Code, rollbackResp.Body.String())
	}
}

func TestKBHandlers_AskRecommendationsAndPendingReviews(t *testing.T) {
	h, cleanup := newKBTestHandler(t)
	defer cleanup()

	for _, title := range []string{"上传队列拥塞排障", "AI 日志超时处理"} {
		resp := doJSONRequest(t, h.kbArticles, http.MethodPost, "/api/kb/articles", map[string]any{
			"title":    title,
			"summary":  title + "摘要",
			"category": "runbook",
			"severity": "medium",
			"content":  title + " content",
			"tags":     []string{"ops"},
		})
		if resp.Code != http.StatusOK {
			t.Fatalf("create article failed: status=%d body=%s", resp.Code, resp.Body.String())
		}
		var created struct {
			OK      bool       `json:"ok"`
			Article kb.Article `json:"article"`
		}
		mustDecodeJSON(t, resp.Body.Bytes(), &created)
		if created.Article.ID == "" {
			t.Fatalf("invalid created response: %+v", created)
		}
		submitResp := doJSONRequest(t, h.kbArticleByID, http.MethodPost, "/api/kb/articles/"+created.Article.ID+"/submit", map[string]any{
			"operator": "tester",
			"comment":  "submit for ask",
		})
		if submitResp.Code != http.StatusOK {
			t.Fatalf("submit for ask failed: status=%d body=%s", submitResp.Code, submitResp.Body.String())
		}
		approveResp := doJSONRequest(t, h.kbArticleByID, http.MethodPost, "/api/kb/articles/"+created.Article.ID+"/approve", map[string]any{
			"operator": "tester",
			"comment":  "publish for ask",
		})
		if approveResp.Code != http.StatusOK {
			t.Fatalf("approve for ask failed: status=%d body=%s", approveResp.Code, approveResp.Body.String())
		}
	}
	// 保留一个待审核条目用于待复审队列测试
	draftResp := doJSONRequest(t, h.kbArticles, http.MethodPost, "/api/kb/articles", map[string]any{
		"title":    "待审核草稿",
		"summary":  "draft",
		"category": "runbook",
		"severity": "low",
		"content":  "draft content",
		"tags":     []string{"draft"},
	})
	if draftResp.Code != http.StatusOK {
		t.Fatalf("create draft failed: status=%d body=%s", draftResp.Code, draftResp.Body.String())
	}
	var draftCreated struct {
		OK      bool       `json:"ok"`
		Article kb.Article `json:"article"`
	}
	mustDecodeJSON(t, draftResp.Body.Bytes(), &draftCreated)
	if !draftCreated.OK || draftCreated.Article.ID == "" {
		t.Fatalf("unexpected draft create response: %+v", draftCreated)
	}
	submitDraftResp := doJSONRequest(t, h.kbArticleByID, http.MethodPost, "/api/kb/articles/"+draftCreated.Article.ID+"/submit", map[string]any{
		"operator": "tester",
		"comment":  "submit draft for pending reviews",
	})
	if submitDraftResp.Code != http.StatusOK {
		t.Fatalf("submit draft failed: status=%d body=%s", submitDraftResp.Code, submitDraftResp.Body.String())
	}

	askResp := doJSONRequest(t, h.kbAsk, http.MethodPost, "/api/kb/ask", map[string]any{
		"question": "上传队列堆积怎么处理",
		"limit":    3,
	})
	if askResp.Code != http.StatusOK {
		t.Fatalf("ask failed: status=%d body=%s", askResp.Code, askResp.Body.String())
	}
	var asked struct {
		OK        bool          `json:"ok"`
		Answer    string        `json:"answer"`
		Citations []kb.Citation `json:"citations"`
		Meta      struct {
			Degraded       bool   `json:"degraded"`
			ErrorClass     string `json:"errorClass"`
			FallbackReason string `json:"fallbackReason"`
		} `json:"meta"`
	}
	mustDecodeJSON(t, askResp.Body.Bytes(), &asked)
	if !asked.OK || asked.Answer == "" || len(asked.Citations) == 0 {
		t.Fatalf("unexpected ask response: %+v", asked)
	}
	if !asked.Meta.Degraded {
		t.Fatalf("ask meta expected degraded=true when AI is disabled: %+v", asked.Meta)
	}
	if asked.Meta.ErrorClass != "ai_disabled" {
		t.Fatalf("ask meta errorClass expected ai_disabled, got %s", asked.Meta.ErrorClass)
	}
	if asked.Meta.FallbackReason == "" {
		t.Fatalf("ask meta fallbackReason expected non-empty")
	}

	recoResp := doJSONRequest(t, h.kbRecommendations, http.MethodGet, "/api/kb/recommendations?query=上传队列&limit=2", nil)
	if recoResp.Code != http.StatusOK {
		t.Fatalf("recommendations failed: status=%d body=%s", recoResp.Code, recoResp.Body.String())
	}
	var recommendations struct {
		OK    bool         `json:"ok"`
		Items []kb.Article `json:"items"`
	}
	mustDecodeJSON(t, recoResp.Body.Bytes(), &recommendations)
	if !recommendations.OK || len(recommendations.Items) == 0 {
		t.Fatalf("unexpected recommendations response: %+v", recommendations)
	}

	pendingResp := doJSONRequest(t, h.kbPendingReviews, http.MethodGet, "/api/kb/reviews/pending?limit=10", nil)
	if pendingResp.Code != http.StatusOK {
		t.Fatalf("pending reviews failed: status=%d body=%s", pendingResp.Code, pendingResp.Body.String())
	}
	var pending struct {
		OK    bool         `json:"ok"`
		Items []kb.Article `json:"items"`
	}
	mustDecodeJSON(t, pendingResp.Body.Bytes(), &pending)
	if !pending.OK || len(pending.Items) == 0 {
		t.Fatalf("unexpected pending response: %+v", pending)
	}
}

func TestKBHandlers_Gates(t *testing.T) {
	h := &handler{}

	getResp := doJSONRequest(t, h.kbGates, http.MethodGet, "/api/kb/gates", nil)
	if getResp.Code != http.StatusOK {
		t.Fatalf("gates get failed: status=%d body=%s", getResp.Code, getResp.Body.String())
	}
	var payload struct {
		OK    bool            `json:"ok"`
		Gates kb.QualityGates `json:"gates"`
	}
	mustDecodeJSON(t, getResp.Body.Bytes(), &payload)
	want := kb.DefaultQualityGates()
	if !payload.OK {
		t.Fatalf("expected ok=true, got false")
	}
	if payload.Gates != want {
		t.Fatalf("unexpected gates: got=%+v want=%+v", payload.Gates, want)
	}

	postResp := doJSONRequest(t, h.kbGates, http.MethodPost, "/api/kb/gates", map[string]any{})
	if postResp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusMethodNotAllowed, postResp.Code, postResp.Body.String())
	}
}

func TestKBHandlers_RecommendationsShouldLinkAlertDecision(t *testing.T) {
	h, cleanup := newKBTestHandler(t)
	defer cleanup()

	createResp := doJSONRequest(t, h.kbArticles, http.MethodPost, "/api/kb/articles", map[string]any{
		"title":    "数据库连接池耗尽排障",
		"summary":  "连接池异常的处置步骤",
		"category": "runbook",
		"severity": "high",
		"content":  "步骤1：排查连接数\n步骤2：扩容连接池",
		"tags":     []string{"db", "pool"},
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create article failed: status=%d body=%s", createResp.Code, createResp.Body.String())
	}
	var created struct {
		OK      bool       `json:"ok"`
		Article kb.Article `json:"article"`
	}
	mustDecodeJSON(t, createResp.Body.Bytes(), &created)
	if !created.OK || created.Article.ID == "" {
		t.Fatalf("unexpected create response: %+v", created)
	}

	submitResp := doJSONRequest(t, h.kbArticleByID, http.MethodPost, "/api/kb/articles/"+created.Article.ID+"/submit", map[string]any{
		"operator": "tester",
		"comment":  "submit for recommendation trace",
	})
	if submitResp.Code != http.StatusOK {
		t.Fatalf("submit article failed: status=%d body=%s", submitResp.Code, submitResp.Body.String())
	}

	approveResp := doJSONRequest(t, h.kbArticleByID, http.MethodPost, "/api/kb/articles/"+created.Article.ID+"/approve", map[string]any{
		"operator": "tester",
		"comment":  "publish for recommendation trace",
	})
	if approveResp.Code != http.StatusOK {
		t.Fatalf("approve article failed: status=%d body=%s", approveResp.Code, approveResp.Body.String())
	}

	if h.alertStateOverride == nil {
		t.Fatal("expected alert state override initialized")
	}
	ok := h.alertStateOverride.RecordDecision(alert.Decision{
		ID:      "alert-100",
		Time:    "2026-02-22 12:00:00",
		Level:   string(alert.LevelSystem),
		Rule:    "数据库连接池耗尽",
		Message: "连接池耗尽导致接口超时",
		File:    "/var/log/app.log",
		Status:  string(alert.StatusSent),
	})
	if !ok {
		t.Fatal("seed alert decision failed")
	}

	recoResp := doJSONRequest(t, h.kbRecommendations, http.MethodGet, "/api/kb/recommendations?alertId=alert-100&limit=2", nil)
	if recoResp.Code != http.StatusOK {
		t.Fatalf("recommendations failed: status=%d body=%s", recoResp.Code, recoResp.Body.String())
	}
	var recommendations struct {
		OK    bool                       `json:"ok"`
		Items []kb.Article               `json:"items"`
		Trace *alert.RecommendationTrace `json:"trace"`
	}
	mustDecodeJSON(t, recoResp.Body.Bytes(), &recommendations)
	if !recommendations.OK {
		t.Fatalf("unexpected recommendations response: %+v", recommendations)
	}
	if len(recommendations.Items) == 0 {
		t.Fatalf("expected recommendations hit, got 0")
	}
	if recommendations.Trace == nil {
		t.Fatalf("expected trace in recommendations response")
	}
	if recommendations.Trace.AlertID != "alert-100" {
		t.Fatalf("unexpected trace alert id: %s", recommendations.Trace.AlertID)
	}
	if recommendations.Trace.DecisionStatus != string(alert.StatusSent) {
		t.Fatalf("unexpected decision status in trace: %s", recommendations.Trace.DecisionStatus)
	}
	if recommendations.Trace.HitCount <= 0 {
		t.Fatalf("unexpected trace hit count: %d", recommendations.Trace.HitCount)
	}

	decision, found := h.alertStateOverride.GetDecision("alert-100")
	if !found {
		t.Fatal("expected linked alert decision found")
	}
	if decision.KnowledgeTrace == nil {
		t.Fatal("expected knowledge trace attached to alert decision")
	}
	if decision.KnowledgeTrace.LinkID != recommendations.Trace.LinkID {
		t.Fatalf("trace link mismatch, got=%s want=%s", decision.KnowledgeTrace.LinkID, recommendations.Trace.LinkID)
	}
}

func TestKBHandlers_ImportDocs(t *testing.T) {
	h, cleanup := newKBTestHandler(t)
	defer cleanup()

	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs failed: %v", err)
	}
	docPath := filepath.Join(docsDir, "runbook.md")
	docContent := "# 队列堆积\n\n排障步骤A\n排障步骤B\n"
	if err := os.WriteFile(docPath, []byte(docContent), 0o644); err != nil {
		t.Fatalf("write doc failed: %v", err)
	}

	importResp := doJSONRequest(t, h.kbImportDocs, http.MethodPost, "/api/kb/import/docs", map[string]any{
		"path":     docsDir,
		"operator": "tester",
	})
	if importResp.Code != http.StatusOK {
		t.Fatalf("import docs failed: status=%d body=%s", importResp.Code, importResp.Body.String())
	}
	var imported struct {
		OK     bool            `json:"ok"`
		Result kb.ImportResult `json:"result"`
	}
	mustDecodeJSON(t, importResp.Body.Bytes(), &imported)
	if !imported.OK || imported.Result.Imported < 1 {
		t.Fatalf("unexpected import response: %+v", imported)
	}
}

func newKBTestHandler(t *testing.T) (*handler, func()) {
	t.Helper()
	svc, err := kb.NewService(filepath.Join(t.TempDir(), "kb-data"))
	if err != nil {
		t.Fatalf("create kb service failed: %v", err)
	}
	h := &handler{
		cfg: &models.Config{
			AIEnabled: false,
		},
		kb:                 svc,
		alertStateOverride: alert.NewState(),
	}
	cleanup := func() {
		_ = svc.Close()
	}
	return h, cleanup
}

func doJSONRequest(t *testing.T, fn http.HandlerFunc, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload failed: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	fn(rec, req)
	return rec
}

func mustDecodeJSON(t *testing.T, data []byte, out any) {
	t.Helper()
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decode json failed: %v; body=%s", err, string(data))
	}
}
