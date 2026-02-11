// 本文件用于 API 服务与路由处理
package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"file-watch/internal/alert"
	"file-watch/internal/logger"
	"file-watch/internal/models"
	"file-watch/internal/pathutil"
	"file-watch/internal/service"
	"file-watch/internal/sysinfo"
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
	sys *sysinfo.Collector

	dashboardCacheMu     sync.Mutex
	dashboardCacheData   any
	dashboardCacheExpire time.Time
	dashboardCacheTTL    time.Duration
}

// 日志读取的限制常量
const (
	maxFileLogBytes        = 512 * 1024  //最多读取 512KB 的内容
	maxFileLogLines        = 500         //最多返回 500 行内容
	maxFileSearchLines     = 2000        //最多返回 2000 行匹配结果
	maxFileSearchLineBytes = 1024 * 1024 //单行最大 1MB，避免超长行撑爆内存
	defaultDashboardTTL    = 2 * time.Second
)

// NewServer 创建接口服务并注册路由
func NewServer(cfg *models.Config, fs *service.FileService) *Server {
	h := &handler{
		cfg:               cfg,
		fs:                fs,
		sys:               sysinfo.NewCollector(sysinfo.Options{}),
		dashboardCacheTTL: defaultDashboardTTL,
	}
	mux := http.NewServeMux() //创建一个路由器（根据 URL 路径分发请求）
	mux.HandleFunc("/api/dashboard", h.dashboard)
	mux.HandleFunc("/api/auto-upload", h.toggleAutoUpload)
	mux.HandleFunc("/api/manual-upload", h.manualUpload)
	mux.HandleFunc("/api/file-log", h.fileLog)
	mux.HandleFunc("/api/ai/log-summary", h.aiLogSummary)
	mux.HandleFunc("/api/config", h.updateConfig)
	mux.HandleFunc("/api/alerts", h.alertDashboard)
	mux.HandleFunc("/api/alert-config", h.alertConfig)
	mux.HandleFunc("/api/alert-rules", h.alertRules)
	mux.HandleFunc("/api/system", h.systemDashboard)
	mux.HandleFunc("/api/system/terminate", h.systemTerminate)
	mux.HandleFunc("/api/health", h.health)

	srv := &http.Server{
		Addr:         cfg.APIBind,
		Handler:      withRecovery(withCORS(cfg, withAPIAuth(cfg, mux))),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: resolveWriteTimeout(cfg),
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

// dashboard 用于返回系统总览数据供控制台展示
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
	forceRefresh := parseBoolQuery(r, "refresh")
	if !forceRefresh {
		if cached, ok := h.loadDashboardCache(); ok {
			w.Header().Set("X-Dashboard-Cache", "hit")
			writeJSON(w, http.StatusOK, cached)
			return
		}
	}
	payload := runtimeState.Dashboard(cfg)
	h.storeDashboardCache(payload)
	w.Header().Set("X-Dashboard-Cache", "miss")
	writeJSON(w, http.StatusOK, payload)
}

// toggleAutoUpload 用于切换自动上传开关并返回最新状态
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
	h.invalidateDashboardCache()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"path":   req.Path,
		"status": req.Enabled,
	})
}

// manualUpload 用于校验并触发手动上传请求
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
	if err := h.fs.EnqueueManualUpload(cleanedPath); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	h.invalidateDashboardCache()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"path": cleanedPath,
	})
}

// fileLog 用于读取或检索文件日志内容
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

// updateConfig 用于更新运行状态或配置
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
		WatchDir              *string `json:"watchDir"`
		FileExt               *string `json:"fileExt"`
		UploadWorkers         *int    `json:"uploadWorkers"`
		UploadQueueSize       *int    `json:"uploadQueueSize"`
		UploadRetryDelays     *string `json:"uploadRetryDelays"`
		UploadRetryEnabled    *bool   `json:"uploadRetryEnabled"`
		SystemResourceEnabled *bool   `json:"systemResourceEnabled"`
		Silence               *string `json:"silence"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	current := h.fs.Config()
	if current == nil {
		current = h.cfg
	}
	if current == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "runtime config not ready"})
		return
	}
	watchDir := current.WatchDir
	if req.WatchDir != nil {
		watchDir = *req.WatchDir
	}
	fileExt := current.FileExt
	if req.FileExt != nil {
		fileExt = *req.FileExt
	}
	silence := current.Silence
	if req.Silence != nil {
		silence = *req.Silence
	}
	uploadWorkers := current.UploadWorkers
	if req.UploadWorkers != nil {
		uploadWorkers = *req.UploadWorkers
	}
	uploadQueueSize := current.UploadQueueSize
	if req.UploadQueueSize != nil {
		uploadQueueSize = *req.UploadQueueSize
	}
	uploadRetryDelays := current.UploadRetryDelays
	if req.UploadRetryDelays != nil {
		uploadRetryDelays = *req.UploadRetryDelays
	}
	uploadRetryEnabled := current.UploadRetryEnabled
	if req.UploadRetryEnabled != nil {
		uploadRetryEnabled = req.UploadRetryEnabled
	}
	cfg, err := h.fs.UpdateConfig(
		watchDir,
		fileExt,
		strings.TrimSpace(silence),
		uploadWorkers,
		uploadQueueSize,
		strings.TrimSpace(uploadRetryDelays),
		uploadRetryEnabled,
		req.SystemResourceEnabled,
	)
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
	h.invalidateDashboardCache()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"config": state.ConfigSnapshot(cfg),
	})
}

// health 用于返回服务健康与队列指标
func (h *handler) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, h.fs.HealthSnapshot())
}

// alertDashboard 用于返回告警模块运行态信息
func (h *handler) alertDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	alertState := h.fs.AlertState()
	if alertState == nil {
		// 告警未启用时返回空结果
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

// alertConfig 用于读取或更新告警配置
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
		// 读取告警配置快照
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"config": buildAlertConfigSnapshot(cfg, h.fs.AlertEnabled()),
		})
		return
	case http.MethodPost:
		// 运行时更新告警配置 仅内存生效
		var req struct {
			Enabled         bool   `json:"enabled"`
			SuppressEnabled *bool  `json:"suppressEnabled"`
			RulesFile       string `json:"rulesFile"`
			LogPaths        string `json:"logPaths"`
			PollInterval    string `json:"pollInterval"`
			StartFromEnd    bool   `json:"startFromEnd"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		suppressEnabled := true
		if cfg.AlertSuppressEnabled != nil {
			suppressEnabled = *cfg.AlertSuppressEnabled
		}
		if req.SuppressEnabled != nil {
			suppressEnabled = *req.SuppressEnabled
		}
		updated, err := h.fs.UpdateAlertConfig(req.Enabled, suppressEnabled, req.RulesFile, req.LogPaths, req.PollInterval, req.StartFromEnd)
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

// alertRules 用于读取或更新告警规则
func (h *handler) alertRules(w http.ResponseWriter, r *http.Request) {
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
		ruleset := cfg.AlertRules
		if ruleset == nil {
			ruleset = alert.DefaultRuleset()
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    true,
			"rules": ruleset,
		})
		return
	case http.MethodPost:
		var req struct {
			Rules *alert.Ruleset `json:"rules"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		if req.Rules == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "rules is required"})
			return
		}
		if err := alert.NormalizeRuleset(req.Rules); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		updated, err := h.fs.UpdateAlertRules(req.Rules)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		h.cfg = updated
		ruleset := req.Rules
		if updated != nil && updated.AlertRules != nil {
			ruleset = updated.AlertRules
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":    true,
			"rules": ruleset,
		})
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
}

// systemDashboard 用于返回系统资源监控数据
func (h *handler) systemDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	cfg := h.fs.Config()
	if cfg == nil {
		cfg = h.cfg
	}
	if cfg == nil || !cfg.SystemResourceEnabled {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "系统资源未启用，请先在控制台配置开启"})
		return
	}
	if h.sys == nil {
		h.sys = sysinfo.NewCollector(sysinfo.Options{})
	}
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	includeProcesses := mode != "lite" && mode != "light"
	includeProcessEnv := parseBoolQuery(r, "includeEnv")
	limit := -1
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		val, err := strconv.Atoi(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid limit"})
			return
		}
		limit = val
	}
	// mode=lite 时跳过进程列表采集，limit 可限制返回的进程数量
	snapshot, err := h.sys.Snapshot(sysinfo.SnapshotOptions{
		IncludeProcesses: includeProcesses,
		ProcessLimit:     limit,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !includeProcessEnv {
		for i := range snapshot.SystemProcesses {
			snapshot.SystemProcesses[i].Env = []string{}
		}
	}
	writeJSON(w, http.StatusOK, snapshot)
}

// systemTerminate 用于终止指定 PID 的进程
func (h *handler) systemTerminate(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	cfg := h.fs.Config()
	if cfg == nil {
		cfg = h.cfg
	}
	if cfg == nil || !cfg.SystemResourceEnabled {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "system resource console disabled"})
		return
	}

	var req struct {
		PID   int32 `json:"pid"`
		Force bool  `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request payload"})
		return
	}
	if req.PID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pid"})
		return
	}
	if req.PID == int32(os.Getpid()) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "refuse to terminate current api process"})
		return
	}

	result, err := sysinfo.TerminateProcess(req.PID, req.Force)
	if err != nil {
		switch {
		case errors.Is(err, sysinfo.ErrInvalidPID):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid pid"})
		case errors.Is(err, sysinfo.ErrProcessNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "process not found"})
		case errors.Is(err, sysinfo.ErrTerminatePermissionDenied):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "permission denied"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"result": result,
	})
}

// buildAlertConfigSnapshot 用于构建后续流程所需的数据
func buildAlertConfigSnapshot(cfg *models.Config, enabled bool) map[string]any {
	if cfg == nil {
		return map[string]any{
			"enabled":         enabled,
			"suppressEnabled": true,
			"rulesFile":       "",
			"logPaths":        "",
			"pollInterval":    "",
			"startFromEnd":    true,
		}
	}
	startFromEnd := true
	if cfg.AlertStartFromEnd != nil {
		startFromEnd = *cfg.AlertStartFromEnd
	}
	suppressEnabled := true
	if cfg.AlertSuppressEnabled != nil {
		suppressEnabled = *cfg.AlertSuppressEnabled
	}
	pollInterval := strings.TrimSpace(cfg.AlertPollInterval)
	if pollInterval == "" {
		pollInterval = "2s"
	}
	return map[string]any{
		"enabled":         enabled,
		"suppressEnabled": suppressEnabled,
		"rulesFile":       "",
		"logPaths":        strings.TrimSpace(cfg.AlertLogPaths),
		"pollInterval":    pollInterval,
		"startFromEnd":    startFromEnd,
	}
}

// 统一返回 JSON 响应
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload) //编码成 JSON，直接写进响应体里（这里不处理错误）
}

// withCORS 用于补充跨域响应头并处理预检请求
func withCORS(cfg *models.Config, next http.Handler) http.Handler {
	allowAll := false
	allowedOrigins := make(map[string]struct{})
	if cfg != nil {
		for _, origin := range strings.FieldsFunc(strings.TrimSpace(cfg.APICORSOrigins), func(r rune) bool {
			return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
		}) {
			normalized := strings.TrimSpace(origin)
			if normalized == "" {
				continue
			}
			if normalized == "*" {
				allowAll = true
				allowedOrigins = map[string]struct{}{}
				break
			}
			allowedOrigins[normalized] = struct{}{}
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestOrigin := strings.TrimSpace(r.Header.Get("Origin"))
		originAllowed := requestOrigin == ""
		if !originAllowed {
			if allowAll {
				originAllowed = true
			} else if _, ok := allowedOrigins[requestOrigin]; ok {
				originAllowed = true
			}
		}

		if originAllowed && requestOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", requestOrigin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization,X-API-Token")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			if !originAllowed {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "origin not allowed"})
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if !originAllowed {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "origin not allowed"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// withAPIAuth 用于统一校验接口访问令牌
func withAPIAuth(cfg *models.Config, next http.Handler) http.Handler {
	token := ""
	if cfg != nil {
		token = strings.TrimSpace(cfg.APIAuthToken)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/api/health" || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if token == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "API_AUTH_TOKEN 未配置，拒绝管理接口访问"})
			return
		}
		requestToken := extractAuthToken(r)
		if requestToken == "" || requestToken != token {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// extractAuthToken 用于提取有效片段供后续处理
func extractAuthToken(r *http.Request) string {
	if r == nil {
		return ""
	}
	if bearer := strings.TrimSpace(r.Header.Get("Authorization")); bearer != "" {
		if strings.HasPrefix(strings.ToLower(bearer), "bearer ") {
			return strings.TrimSpace(bearer[len("Bearer "):])
		}
	}
	return strings.TrimSpace(r.Header.Get("X-API-Token"))
}

// withRecovery 用于兜底捕获 panic 防止服务崩溃
func withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("API发生异常: %v\n%s", recovered, string(debug.Stack()))
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "服务器内部错误"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// resolveWriteTimeout 用于解析依赖并返回可用结果
func resolveWriteTimeout(cfg *models.Config) time.Duration {
	base := 90 * time.Second
	if cfg == nil {
		return base
	}
	aiTimeout := parseAITimeout(cfg.AITimeout)
	if aiTimeout > 0 {
		candidate := aiTimeout + 5*time.Second
		if candidate > base {
			base = candidate
		}
	}
	return base
}

// parseBoolQuery 用于解析输入参数或配置
func parseBoolQuery(r *http.Request, key string) bool {
	if r == nil {
		return false
	}
	raw := strings.ToLower(strings.TrimSpace(r.URL.Query().Get(key)))
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

// loadDashboardCache 用于加载运行数据
func (h *handler) loadDashboardCache() (any, bool) {
	if h == nil {
		return nil, false
	}
	h.dashboardCacheMu.Lock()
	defer h.dashboardCacheMu.Unlock()
	if h.dashboardCacheData == nil {
		return nil, false
	}
	if h.dashboardCacheExpire.IsZero() || time.Now().After(h.dashboardCacheExpire) {
		h.dashboardCacheData = nil
		h.dashboardCacheExpire = time.Time{}
		return nil, false
	}
	return h.dashboardCacheData, true
}

// storeDashboardCache 用于写入仪表盘缓存减少重复采集开销
func (h *handler) storeDashboardCache(payload any) {
	if h == nil {
		return
	}
	ttl := h.dashboardCacheTTL
	if ttl <= 0 {
		ttl = defaultDashboardTTL
	}
	h.dashboardCacheMu.Lock()
	h.dashboardCacheData = payload
	h.dashboardCacheExpire = time.Now().Add(ttl)
	h.dashboardCacheMu.Unlock()
}

// invalidateDashboardCache 用于在配置变更后失效仪表盘缓存
func (h *handler) invalidateDashboardCache() {
	if h == nil {
		return
	}
	h.dashboardCacheMu.Lock()
	h.dashboardCacheData = nil
	h.dashboardCacheExpire = time.Time{}
	h.dashboardCacheMu.Unlock()
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
