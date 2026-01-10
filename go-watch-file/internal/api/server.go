package api

import (
	"bufio"
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

// Server 管理接口服务的启动与关闭
// 统一启动/停止 HTTP 服务
type Server struct {
	httpServer *http.Server //负责监听端口、接收请求、管理超时/连接等
}

// 请求处理器
type handler struct {
	cfg *models.Config
	fs  *service.FileService
}

// 日志读取的限制常量
const (
	maxFileLogBytes        = 512 * 1024  //最多读取 512KB 的内容
	maxFileLogLines        = 500         //最多返回 500 行内容
	maxFileSearchLines     = 2000        //最多返回 2000 行匹配结果
	maxFileSearchLineBytes = 1024 * 1024 //单行最大 1MB，避免超长行撑爆内存
)

// NewServer 创建接口服务并注册路由
func NewServer(cfg *models.Config, fs *service.FileService) *Server {
	h := &handler{cfg: cfg, fs: fs}
	mux := http.NewServeMux() //创建一个路由器（根据 URL 路径分发请求）
	mux.HandleFunc("/api/dashboard", h.dashboard)
	mux.HandleFunc("/api/auto-upload", h.toggleAutoUpload)
	mux.HandleFunc("/api/manual-upload", h.manualUpload)
	mux.HandleFunc("/api/file-log", h.fileLog)
	mux.HandleFunc("/api/config", h.updateConfig)
	mux.HandleFunc("/api/alerts", h.alertDashboard)
	mux.HandleFunc("/api/alert-config", h.alertConfig)
	mux.HandleFunc("/api/health", h.health)

	srv := &http.Server{
		Addr:         cfg.APIBind,
		Handler:      withCORS(mux),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	return &Server{httpServer: srv}
}

// Start 启动接口服务并开始监听
func (s *Server) Start() {
	go func() {
		logger.Info("API 服务监听 %s", s.httpServer.Addr)
		//过滤掉“正常关闭”，只记录真正的异常
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("API 服务异常退出: %v", err)
		}
	}()
}

// Shutdown 优雅关闭接口服务
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
	runtimeState := h.fs.State()
	if runtimeState == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "runtime state not ready"})
		return
	}
	//从 ?mode=... 里取值
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	if mode == "light" || mode == "lite" {
		writeJSON(w, http.StatusOK, runtimeState.DashboardLite(cfg))
		return
	}
	writeJSON(w, http.StatusOK, runtimeState.Dashboard(cfg))
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
		Path          string `json:"path"`
		Query         string `json:"query"`
		Limit         int    `json:"limit"`
		CaseSensitive bool   `json:"caseSensitive"`
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
	watchDirs := pathutil.SplitWatchDirs(cfg.WatchDir)
	if len(watchDirs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "watch dir not configured"})
		return
	}
	if _, _, err := pathutil.RelativePathAny(watchDirs, cleanedPath); err != nil {
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
	// query 为空时走 tail 模式，否则走全文检索
	query := strings.TrimSpace(req.Query)
	if query != "" {
		lines, truncated, err := searchFileLogLines(cleanedPath, query, req.Limit, req.CaseSensitive)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":        true,
			"mode":      "search",
			"query":     query,
			"matched":   len(lines),
			"truncated": truncated,
			"lines":     lines,
		})
		return
	}
	lines, err := readFileLogLines(cleanedPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"mode":  "tail",
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

func (h *handler) alertDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	alertState := h.fs.AlertState()
	if alertState == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    false,
			"error": "告警未启用",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"enabled": h.fs.AlertEnabled(),
		"data":    alertState.Dashboard(),
	})
}

func (h *handler) alertConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	cfg := h.fs.Config()
	if cfg == nil {
		cfg = h.cfg
	}
	if cfg == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "config not ready"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"config": buildAlertConfigSnapshot(cfg, h.fs.AlertEnabled()),
		})
		return
	case http.MethodPost:
		var req struct {
			Enabled      bool   `json:"enabled"`
			RulesFile    string `json:"rulesFile"`
			LogPaths     string `json:"logPaths"`
			PollInterval string `json:"pollInterval"`
			StartFromEnd bool   `json:"startFromEnd"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		updated, err := h.fs.UpdateAlertConfig(req.Enabled, req.RulesFile, req.LogPaths, req.PollInterval, req.StartFromEnd)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		h.cfg = updated
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"config": buildAlertConfigSnapshot(updated, h.fs.AlertEnabled()),
		})
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
}

func buildAlertConfigSnapshot(cfg *models.Config, enabled bool) map[string]any {
	if cfg == nil {
		return map[string]any{
			"enabled":      enabled,
			"rulesFile":    "",
			"logPaths":     "",
			"pollInterval": "",
			"startFromEnd": true,
		}
	}
	startFromEnd := true
	if cfg.AlertStartFromEnd != nil {
		startFromEnd = *cfg.AlertStartFromEnd
	}
	pollInterval := strings.TrimSpace(cfg.AlertPollInterval)
	if pollInterval == "" {
		pollInterval = "2s"
	}
	return map[string]any{
		"enabled":      enabled,
		"rulesFile":    strings.TrimSpace(cfg.AlertRulesFile),
		"logPaths":     strings.TrimSpace(cfg.AlertLogPaths),
		"pollInterval": pollInterval,
		"startFromEnd": startFromEnd,
	}
}

// 统一返回 JSON 响应
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload) //编码成 JSON，直接写进响应体里（这里不处理错误）
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

// 读取目标文件内容
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
	var data []byte //用来存储读取到的文件内容(字节形式)
	if size > maxFileLogBytes {
		start := size - maxFileLogBytes      //计算从文件的哪个位置开始读取(只保留最后 512KB)
		buf := make([]byte, maxFileLogBytes) //创建一个长度为 maxFileLogBytes 的 []byte
		n, err := file.ReadAt(buf, start)    //从文件第 start 个字节位置读取内容到 buf 中,返回实际读到的字节数 n
		if err != nil && err != io.EOF {     //读取过程中出现错误，且不是读到文件末尾的错误
			return nil, err
		}
		data = buf[:n] //防止“读不满时包含无效字节”
	} else {
		data, err = io.ReadAll(file) //文件较小，直接全部读取
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
	if size > maxFileLogBytes && len(lines) > 1 { //文件超过 512KB 且大于 1 行
		lines = lines[1:]
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1] //去掉最后一个空行
	}
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, "\r") //处理 Windows 换行 \r\n，把每行末尾的 \r 去掉
	}
	if maxFileLogLines > 0 && len(lines) > maxFileLogLines {
		lines = lines[len(lines)-maxFileLogLines:] //只保留最后 maxFileLogLines 行
	}
	return lines, nil
}

// searchFileLogLines 搜索文件内容并返回匹配行
func searchFileLogLines(path, query string, limit int, caseSensitive bool) ([]string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()

	// 先做文本判定，避免扫描二进制文件
	if err := ensureTextFile(file); err != nil {
		return nil, false, err
	}
	if limit <= 0 || limit > maxFileSearchLines {
		limit = maxFileSearchLines
	}
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return []string{}, false, nil
	}
	normalizedQuery := trimmedQuery
	if !caseSensitive {
		normalizedQuery = strings.ToLower(trimmedQuery)
	}

	// 使用 Scanner 按行扫描，避免一次性读入大文件
	capHint := limit
	if capHint > 64 {
		capHint = 64
	}
	matches := make([]string, 0, capHint)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxFileSearchLineBytes)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		haystack := line
		if !caseSensitive {
			haystack = strings.ToLower(line)
		}
		// 包含关键字即视为命中
		if strings.Contains(haystack, normalizedQuery) {
			matches = append(matches, line)
			if len(matches) >= limit {
				return matches, true, nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, false, err
	}
	return matches, false, nil
}

// ensureTextFile 用于快速判断是否为文本文件
func ensureTextFile(file *os.File) error {
	buf := make([]byte, 4096)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return err
	}
	if !isTextData(buf[:n]) {
		return fmt.Errorf("仅支持文本文件")
	}
	// 重置到文件开头，避免影响后续扫描
	_, err = file.Seek(0, io.SeekStart)
	return err
}

// 简单判断是否是文本数据
func isTextData(data []byte) bool {
	for _, b := range data {
		if b == 0 { //如果包含空字节，则认为不是文本数据
			return false
		}
	}
	return true
}
