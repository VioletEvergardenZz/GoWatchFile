package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"file-watch/internal/logger"
	"file-watch/internal/models"
	"file-watch/internal/pathutil"
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

//日志读取的限制常量
const (
	maxFileLogBytes = 512 * 1024		//最多读取 512KB 的内容
	maxFileLogLines = 500			    //最多返回 500 行内容	
)

// NewServer builds the HTTP server for console/API consumption.
func NewServer(cfg *models.Config, fs *service.FileService) *Server {
	h := &handler{cfg: cfg, fs: fs}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/dashboard", h.dashboard)
	mux.HandleFunc("/api/auto-upload", h.toggleAutoUpload)
	mux.HandleFunc("/api/manual-upload", h.manualUpload)
	mux.HandleFunc("/api/file-log", h.fileLog)
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

func (h *handler) fileLog(w http.ResponseWriter, r *http.Request) {
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Path) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	cleanedPath := filepath.Clean(filepath.FromSlash(strings.TrimSpace(req.Path)))
	cfg := h.fs.Config()
	if cfg == nil {
		cfg = h.cfg
	}
	if cfg == nil || strings.TrimSpace(cfg.WatchDir) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "watch dir not configured"})
		return
	}
	if _, err := pathutil.RelativePath(cfg.WatchDir, cleanedPath); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	info, err := os.Stat(cleanedPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if info.IsDir() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is a directory"})
		return
	}
	lines, err := readFileLogLines(cleanedPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"lines": lines,
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
		Silence         string `json:"silence"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	cfg, err := h.fs.UpdateConfig(req.WatchDir, req.FileExt, strings.TrimSpace(req.Silence), req.UploadWorkers, req.UploadQueueSize)
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


//读取目标文件内容
func readFileLogLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	size := info.Size()
	var data []byte								//用来存储读取到的文件内容(字节形式)
	if size > maxFileLogBytes {					
		start := size - maxFileLogBytes		    //计算从文件的哪个位置开始读取(只保留最后 512KB)
		buf := make([]byte, maxFileLogBytes)	//创建一个长度为 maxFileLogBytes 的 []byte
		n, err := file.ReadAt(buf, start)		//从文件第 start 个字节位置读取内容到 buf 中,返回实际读到的字节数 n
		if err != nil && err != io.EOF {		//读取过程中出现错误，且不是读到文件末尾的错误
			return nil, err
		}		
		data = buf[:n]							//防止“读不满时包含无效字节”
	} else {
		data, err = io.ReadAll(file)			//文件较小，直接全部读取
		if err != nil {
			return nil, err
		}
	}

	if len(data) == 0 {
		return []string{}, nil
	}
	if !isTextData(data) {
		return nil, fmt.Errorf("仅支持文本文件")
	}

	lines := strings.Split(string(data), "\n")
	if size > maxFileLogBytes && len(lines) > 1 {		        //文件超过 512KB 且大于 1 行
		lines = lines[1:]										
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]					    	//去掉最后一个空行
	}
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, "\r")				//处理 Windows 换行 \r\n，把每行末尾的 \r 去掉
	}
	if maxFileLogLines > 0 && len(lines) > maxFileLogLines {
		lines = lines[len(lines)-maxFileLogLines:]				//只保留最后 maxFileLogLines 行
	}
	return lines, nil
}

//简单判断是否是文本数据
func isTextData(data []byte) bool {
	for _, b := range data {
		if b == 0 {				//如果包含空字节，则认为不是文本数据
			return false
		}
	}
	return true
}
