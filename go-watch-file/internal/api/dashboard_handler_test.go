package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"file-watch/internal/state"
)

func decodeDashboardPayload(t *testing.T, body []byte) state.DashboardData {
	t.Helper()
	var payload state.DashboardData
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	return payload
}

func TestDashboard_NilHandler_ReturnsFallback200(t *testing.T) {
	var h *handler

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	rec := httptest.NewRecorder()
	h.dashboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, rec.Code)
	}
	payload := decodeDashboardPayload(t, rec.Body.Bytes())
	if len(payload.DirectoryTree) != 0 {
		t.Fatalf("expected empty directory tree, got %d", len(payload.DirectoryTree))
	}
	if len(payload.Files) != 0 {
		t.Fatalf("expected empty files list, got %d", len(payload.Files))
	}
}

func TestDashboard_NilFileService_ModeLight_ReturnsFallback200(t *testing.T) {
	h := &handler{}

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard?mode=light", nil)
	rec := httptest.NewRecorder()
	h.dashboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, rec.Code)
	}
	payload := decodeDashboardPayload(t, rec.Body.Bytes())
	if len(payload.DirectoryTree) != 0 {
		t.Fatalf("expected empty directory tree, got %d", len(payload.DirectoryTree))
	}
	if len(payload.Files) != 0 {
		t.Fatalf("expected empty files list, got %d", len(payload.Files))
	}
}

func TestDashboard_MethodNotAllowed(t *testing.T) {
	h := &handler{}

	req := httptest.NewRequest(http.MethodPost, "/api/dashboard", nil)
	rec := httptest.NewRecorder()
	h.dashboard(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}
