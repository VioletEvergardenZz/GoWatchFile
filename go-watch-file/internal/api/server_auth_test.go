package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"file-watch/internal/models"
)

func TestWithAPIAuth_TokenEmpty_AllowsAccess(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := withAPIAuth(&models.Config{APIAuthToken: ""}, next)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestWithAPIAuth_PlaceholderToken_AllowsAccess(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := withAPIAuth(&models.Config{APIAuthToken: "${API_AUTH_TOKEN}"}, next)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestWithAPIAuth_ExplicitDisabledEnv_AllowsAccess(t *testing.T) {
	t.Setenv("API_AUTH_DISABLED", "true")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := withAPIAuth(&models.Config{APIAuthToken: "enabled-token"}, next)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestWithAPIAuth_EnabledToken_RequiresHeader(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := withAPIAuth(&models.Config{APIAuthToken: "enabled-token"}, next)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestWithCORS_EmptyConfig_AllowsLoopbackOrigin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := withCORS(&models.Config{APICORSOrigins: "", APIAuthToken: "enabled-token"}, next)

	req := httptest.NewRequest(http.MethodPost, "/api/config", nil)
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected allow origin header set, got %q", got)
	}
}

func TestWithCORS_EmptyConfig_DeniesUnknownOrigin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := withCORS(&models.Config{APICORSOrigins: "", APIAuthToken: "enabled-token"}, next)

	req := httptest.NewRequest(http.MethodPost, "/api/config", nil)
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Origin", "http://evil.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestWithCORS_EmptyConfig_AllowsSameHostOrigin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := withCORS(&models.Config{APICORSOrigins: "", APIAuthToken: "enabled-token"}, next)

	req := httptest.NewRequest(http.MethodPost, "/api/config", nil)
	req.Host = "10.10.1.8:8080"
	req.Header.Set("Origin", "http://10.10.1.8:5173")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestWithCORS_ExplicitAllowList_StillEnforced(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := withCORS(&models.Config{APICORSOrigins: "http://localhost:5173"}, next)

	req := httptest.NewRequest(http.MethodPost, "/api/config", nil)
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Origin", "http://localhost:5174")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestWithCORS_AuthDisabledAndEmptyOrigins_AllowsUnknownOrigin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := withCORS(&models.Config{APICORSOrigins: "", APIAuthToken: ""}, next)

	req := httptest.NewRequest(http.MethodPost, "/api/config", nil)
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Origin", "http://master:8081")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://master:8081" {
		t.Fatalf("expected allow origin header set, got %q", got)
	}
}
