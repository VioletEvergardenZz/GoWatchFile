package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"file-watch/internal/logger"
	"file-watch/internal/models"
	"file-watch/internal/service"
)

// Server wraps the HTTP API server.
type Server struct {
	httpServer *http.Server
}

type handler struct {
	cfg *models.Config
	fs  *service.FileService
}

// NewServer builds the HTTP server for console/API consumption.
func NewServer(cfg *models.Config, fs *service.FileService) *Server {
	h := &handler{cfg: cfg, fs: fs}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/dashboard", h.dashboard)
	mux.HandleFunc("/api/auto-upload", h.toggleAutoUpload)
	mux.HandleFunc("/api/manual-upload", h.manualUpload)
	mux.HandleFunc("/api/config", h.updateConfig)
	mux.HandleFunc("/api/health", h.health)

	srv := &http.Server{
		Addr:         cfg.APIBind,
		Handler:      withCORS(mux),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	return &Server{httpServer: srv}
}

// Start boots the API server asynchronously.
func (s *Server) Start() {
	go func() {
		logger.Info("API 服务监听 %s", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("API 服务异常退出: %v", err)
		}
	}()
}

// Shutdown gracefully stops the API server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

func (h *handler) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	cfg := h.fs.Config()
	if cfg == nil {
		cfg = h.cfg
	}
	state := h.fs.State()
	if state == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "runtime state not ready"})
		return
	}
	writeJSON(w, http.StatusOK, state.Dashboard(cfg))
}

func (h *handler) toggleAutoUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req struct {
		Path    string `json:"path"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	state := h.fs.State()
	if state == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "runtime state not ready"})
		return
	}
	state.SetAutoUpload(req.Path, req.Enabled)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"path":   req.Path,
		"status": req.Enabled,
	})
}

func (h *handler) manualUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if err := h.fs.EnqueueManualUpload(req.Path); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"path": req.Path,
	})
}

func (h *handler) updateConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req struct {
		WatchDir        string `json:"watchDir"`
		FileExt         string `json:"fileExt"`
		UploadWorkers   int    `json:"uploadWorkers"`
		UploadQueueSize int    `json:"uploadQueueSize"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	cfg, err := h.fs.UpdateConfig(req.WatchDir, req.FileExt, req.UploadWorkers, req.UploadQueueSize)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	state := h.fs.State()
	if state == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "runtime state not ready"})
		return
	}
	h.cfg = cfg
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"config": state.ConfigSnapshot(cfg),
	})
}

func (h *handler) health(w http.ResponseWriter, r *http.Request) {
	stats := h.fs.GetStats()
	writeJSON(w, http.StatusOK, map[string]any{
		"queue":   stats.QueueLength,
		"workers": stats.Workers,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
