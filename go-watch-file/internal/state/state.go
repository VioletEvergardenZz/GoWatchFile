// 本文件用于运行时状态聚合与统计
// 文件职责：实现当前模块的核心业务逻辑与数据流转
// 关键路径：入口参数先校验再执行业务处理 最后返回统一结果
// 边界与容错：异常场景显式返回错误 由上层决定重试或降级

package state

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"file-watch/internal/match"
	"file-watch/internal/models"
	"file-watch/internal/pathutil"
)

// “内存中保留数量的上限”常量，用来限制运行态数据列表的长度
const (
	maxUploadRecords = 200
	maxQueuePoints   = 32  //队列趋势图点的最大数量
	maxNotifications = 200 //通知事件的最大数量
)

// FileStatus 表示文件处理状态
type FileStatus int

// 枚举类型（整型），表示文件的处理状态
const (
	// StatusUnknown 表示未知状态
	StatusUnknown FileStatus = iota
	// StatusQueued 表示已入队
	StatusQueued
	// StatusUploaded 表示上传成功
	StatusUploaded
	// StatusFailed 表示上传失败
	StatusFailed
	// StatusExisting 表示历史文件状态
	StatusExisting
)

// 记录“被监控文件”的当前状态
type fileState struct {
	Status     FileStatus
	Note       string //备注信息，如（“自动入队”“上传失败原因”）
	AutoUpload bool
}

// 上传历史记录条目
type uploadHistory struct {
	File    string
	Target  string
	Size    string
	Result  string
	Latency string
	Time    time.Time
	Note    string
}

// 通知事件记录结构
type notificationEvent struct {
	Time    time.Time //通知发送时间
	Channel string    //通知渠道，如“钉钉”“企业微信”等
}

// RuntimeState 保存接口与界面所需的内存运行态数据
type RuntimeState struct {
	//并发控制
	mu sync.RWMutex //读多写少，读用 RLock，写用 Lock

	//配置/环境
	host      string
	watchDirs []string
	matcher   *match.Matcher
	exclude   *pathutil.ExcludeMatcher

	//文件状态
	fileState map[string]fileState
	autoOn    map[string]bool

	//UI 面板数据（环形缓存式）
	uploads       []uploadHistory
	queue         []ChartPoint //队列/上传统计的趋势点
	notifications []notificationEvent

	//计数器/指标
	successes int //累计成功上传数
	failures  int
	workers   int
	queueLen  int
}

// NewRuntimeState 基于配置默认值构建 RuntimeState
func NewRuntimeState(cfg *models.Config) *RuntimeState {
	host, _ := os.Hostname()
	watchDirs := pathutil.SplitWatchDirs(cfg.WatchDir)
	auto := map[string]bool{}
	for _, dir := range watchDirs {
		norm := normalizePath(dir)
		if norm == "" {
			continue
		}
		auto[normalizeKeyPath(norm)] = true
	}
	return &RuntimeState{
		host:      host,
		watchDirs: watchDirs,
		// 统一复用匹配器做后缀过滤
		matcher:   match.NewMatcher(cfg.FileExt),
		exclude:   pathutil.NewExcludeMatcher(cfg.WatchExclude),
		fileState: make(map[string]fileState),
		autoOn:    auto,
		queue:     seedQueuePoints(),
		workers:   cfg.UploadWorkers,
	}
}

// CarryOverFrom 从已有状态迁移计数与历史，保证配置重载后指标连续
func (s *RuntimeState) CarryOverFrom(old *RuntimeState) {
	if old == nil {
		return
	}
	old.mu.RLock() //加读锁
	defer old.mu.RUnlock()

	s.successes = old.successes
	s.failures = old.failures
	s.queueLen = old.queueLen

	//把旧状态里的几组切片“拷贝一份”到新状态里
	s.uploads = append([]uploadHistory(nil), old.uploads...)
	s.notifications = append([]notificationEvent(nil), old.notifications...)
	s.queue = append([]ChartPoint(nil), old.queue...)
}

// BootstrapExisting 预加载监控目录下的已有文件为已存在状态
func (s *RuntimeState) BootstrapExisting() error {
	if len(s.watchDirs) == 0 {
		return nil
	}

	// 扫描多个监控目录并去重
	seen := make(map[string]struct{})
	var walkErr error
	for _, watchDir := range s.watchDirs {
		if strings.TrimSpace(watchDir) == "" {
			continue
		}
		err := filepath.WalkDir(watchDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if s.isExcludedPath(path) {
					return fs.SkipDir
				}
				return nil
			}
			if !s.isTargetFile(path) { //不符合目标后缀,跳过
				return nil
			}
			norm := normalizePath(path)
			if _, ok := seen[norm]; ok {
				return nil
			}
			seen[norm] = struct{}{}
			s.mu.Lock()
			s.fileState[norm] = fileState{
				Status:     StatusExisting,
				AutoUpload: s.autoUploadLocked(path),
				Note:       "历史文件",
			}
			s.mu.Unlock()
			return nil
		})
		if err != nil && walkErr == nil {
			walkErr = err
		}
	}
	return walkErr
}

// AutoUploadEnabled 返回该路径是否启用自动上传
func (s *RuntimeState) AutoUploadEnabled(path string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.autoUploadLocked(path)
}

// SetAutoUpload 为路径（文件或目录）切换自动上传，并联动更新已有状态
func (s *RuntimeState) SetAutoUpload(path string, enabled bool) {
	normPath := normalizePath(path)
	if normPath == "" {
		return
	}
	keyPath := normalizeKeyPath(normPath)
	isDir := isDirPath(normPath)
	s.mu.Lock()
	defer s.mu.Unlock()
	if isDir {
		// 目录开关时清理子级覆盖配置
		for p := range s.autoOn {
			if p != keyPath && isSameOrChildPath(p, keyPath) {
				delete(s.autoOn, p)
			}
		}
	}
	s.autoOn[keyPath] = enabled
	// 同步已有文件状态的开关
	for p, st := range s.fileState {
		if (isDir && isSameOrChildPath(p, normPath)) || (!isDir && normalizeKeyPath(p) == keyPath) {
			st.AutoUpload = enabled
			s.fileState[p] = st
		}
	}
}

// MarkQueued 记录文件自动进入队列
func (s *RuntimeState) MarkQueued(path string) {
	s.recordState(path, StatusQueued, false, "自动入队")
}

// MarkManualQueued 记录手动上传请求
func (s *RuntimeState) MarkManualQueued(path string) {
	s.recordState(path, StatusQueued, true, "手动触发")
}

// MarkSkipped 记录因自动上传关闭而跳过
func (s *RuntimeState) MarkSkipped(path string) {
	norm := normalizePath(path)
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.fetchOrInitStateLocked(norm)
	st.Status = StatusQueued
	st.Note = "自动上传关闭，待手动触发"
	st.AutoUpload = false
	s.fileState[norm] = st
}

// MarkUploaded 记录上传成功
func (s *RuntimeState) MarkUploaded(path, downloadURL string, latency time.Duration, manual bool) {
	norm := normalizePath(path)
	info := s.statFile(path)
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.fetchOrInitStateLocked(norm)
	st.Status = StatusUploaded
	if manual {
		st.Note = "手动上传"
	} else {
		st.Note = "自动上传"
	}
	st.AutoUpload = s.autoUploadLocked(path)
	s.fileState[norm] = st

	s.appendUploadLocked(uploadHistory{
		File:    filepath.Base(norm),
		Target:  downloadURL,
		Size:    formatSize(info.size),
		Result:  "success",
		Latency: formatLatency(latency),
		Time:    now,
		Note:    st.Note,
	})
	s.successes++
}

// MarkFailed 记录上传失败
func (s *RuntimeState) MarkFailed(path string, reason error) {
	norm := normalizePath(path)
	info := s.statFile(path)
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.fetchOrInitStateLocked(norm)
	st.Status = StatusFailed
	st.Note = reason.Error()
	st.AutoUpload = s.autoUploadLocked(path)
	s.fileState[norm] = st

	s.appendUploadLocked(uploadHistory{
		File:    filepath.Base(norm),
		Target:  "",
		Size:    formatSize(info.size),
		Result:  "failed",
		Latency: "--",
		Time:    now,
		Note:    reason.Error(),
	})
	s.failures++
}

// RecordNotification 记录一次通知发送事件
func (s *RuntimeState) RecordNotification(channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ev := notificationEvent{Time: time.Now(), Channel: channel}
	s.notifications = append(s.notifications, ev)
	if len(s.notifications) > maxNotifications {
		s.notifications = s.notifications[len(s.notifications)-maxNotifications:]
	}
}

// SetQueueStats 更新队列长度与工作数，并追加图表点
func (s *RuntimeState) SetQueueStats(stats models.UploadStats) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	total := stats.QueueLength + stats.InFlight
	s.queueLen = total
	s.workers = stats.Workers
	label := now.Format("15:04")
	s.queue = append(s.queue, ChartPoint{
		Label:    label,
		Uploads:  s.successes,
		Failures: s.failures,
		Queue:    total,
	})
	if len(s.queue) > maxQueuePoints {
		s.queue = s.queue[len(s.queue)-maxQueuePoints:]
	}
}

// UploadRecords 返回按时间倒序的上传历史
func (s *RuntimeState) UploadRecords() []UploadRecord {
	s.mu.RLock()
	records := append([]uploadHistory(nil), s.uploads...)
	s.mu.RUnlock()
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].Time.After(records[j].Time)
	})
	out := make([]UploadRecord, len(records))
	for i, item := range records {
		out[i] = UploadRecord{
			File:    item.File,
			Target:  item.Target,
			Size:    item.Size,
			Result:  item.Result,
			Latency: item.Latency,
			Time:    formatDateTime(item.Time),
			Note:    item.Note,
		}
	}
	return out
}

// 构建文件表展示项
func (s *RuntimeState) buildFileItems(files []scannedFile) []FileItem {
	items := make([]FileItem, 0, len(files))
	for _, f := range files {
		items = append(items, FileItem{
			Name:       filepath.Base(f.path),
			Path:       normalizePath(f.path),
			Size:       formatSize(f.size),
			Status:     fileStatusLabel(f.state.Status),
			Time:       formatDateTime(f.modTime),
			AutoUpload: f.state.AutoUpload,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Time > items[j].Time
	})
	return items
}

// FileItems 扫描磁盘并合并运行态，生成文件表数据
func (s *RuntimeState) FileItems() []FileItem {
	return s.buildFileItems(s.collectFiles())
}

// DirectoryTree 构建用于界面的目录树
func (s *RuntimeState) DirectoryTree() []FileNode {
	return s.buildDirectoryTree(s.collectFiles())
}

// 生成目录树结构
func (s *RuntimeState) buildDirectoryTree(files []scannedFile) []FileNode {
	dirMap := make(map[string]*FileNode)
	rootPaths := make([]string, 0, len(s.watchDirs))
	rootSet := make(map[string]struct{})
	for _, watchDir := range s.watchDirs {
		rootPath := normalizePath(watchDir)
		if rootPath == "" {
			continue
		}
		if _, exists := rootSet[rootPath]; exists {
			continue
		}
		rootSet[rootPath] = struct{}{}
		rootPaths = append(rootPaths, rootPath)
		dirMap[rootPath] = &FileNode{
			Name:       rootPath,
			Path:       rootPath,
			Type:       "dir",
			AutoUpload: s.autoUploadLocked(rootPath),
		}
	}

	var ensureDir func(string)
	ensureDir = func(path string) {
		path = normalizePath(path)
		if path == "" {
			return
		}
		if _, ok := dirMap[path]; ok {
			return
		}
		// 递归补齐父目录节点
		dirMap[path] = &FileNode{
			Name:       filepath.Base(path),
			Path:       path,
			Type:       "dir",
			AutoUpload: s.autoUploadLocked(path),
		}
		parent := normalizePath(filepath.Dir(path))
		if parent != "" && parent != path {
			ensureDir(parent)
		}
	}

	for _, f := range files {
		ensureDir(filepath.Dir(f.path))
		parentPath := normalizePath(filepath.Dir(f.path))
		parent, ok := dirMap[parentPath]
		if !ok {
			continue
		}
		parent.Children = append(parent.Children, FileNode{
			Name:       filepath.Base(f.path),
			Path:       normalizePath(f.path),
			Type:       "file",
			AutoUpload: f.state.AutoUpload,
			Size:       formatSize(f.size),
			Updated:    formatDateTime(f.modTime),
		})
	}

	// 文件加入后再把目录挂到其父节点
	dirPaths := make([]string, 0, len(dirMap))
	for p := range dirMap {
		dirPaths = append(dirPaths, p)
	}
	// 先按深度排序再挂载父子关系
	sort.Slice(dirPaths, func(i, j int) bool {
		return strings.Count(dirPaths[i], "/") > strings.Count(dirPaths[j], "/")
	})
	for _, dirPath := range dirPaths {
		if _, isRoot := rootSet[dirPath]; isRoot {
			continue
		}
		parentPath := normalizePath(filepath.Dir(dirPath))
		if parent, ok := dirMap[parentPath]; ok && parentPath != dirPath {
			parent.Children = append(parent.Children, *dirMap[dirPath])
		}
	}

	for _, node := range dirMap {
		sortChildren(node)
	}

	roots := []FileNode{}
	for _, rootPath := range rootPaths {
		if root, ok := dirMap[rootPath]; ok {
			roots = append(roots, *root)
		}
	}
	if len(roots) > 0 {
		return roots
	}

	roots = []FileNode{}
	for path, node := range dirMap {
		if normalizePath(filepath.Dir(path)) == path || path == "" {
			roots = append(roots, *node)
		}
	}
	return roots
}

// 递归排序子节点
func sortChildren(node *FileNode) {
	sort.Slice(node.Children, func(i, j int) bool {
		return node.Children[i].Path < node.Children[j].Path
	})
	for i := range node.Children {
		sortChildren(&node.Children[i])
	}
}

// MonitorSummary 构建摘要卡片
func (s *RuntimeState) MonitorSummary() []MonitorSummary {
	now := time.Now()
	windowStart := now.Add(-1 * time.Minute)
	s.mu.RLock()
	recentProcessed := 0
	for _, up := range s.uploads {
		if up.Time.IsZero() || up.Time.Before(windowStart) {
			continue
		}
		if up.Result == "success" || up.Result == "failed" {
			recentProcessed++
		}
	}
	total := s.successes + s.failures
	failRate := 0.0
	if total > 0 {
		failRate = (float64(s.failures) / float64(total)) * 100
	}
	queueLen := s.queueLen
	workers := s.workers
	failures := s.failures
	s.mu.RUnlock()
	return []MonitorSummary{
		{Label: "当前吞吐", Value: fmt.Sprintf("%d/min", recentProcessed), Desc: "近1分钟处理数"},
		{Label: "成功率", Value: fmt.Sprintf("%.1f%%", 100-failRate), Desc: fmt.Sprintf("失败 %d", failures)},
		{Label: "队列 backlog", Value: fmt.Sprintf("%d", queueLen), Desc: fmt.Sprintf("workers=%d", workers)},
		{Label: "失败累计", Value: fmt.Sprintf("%d", failures), Desc: "含告警和重试"},
	}
}

// MetricCards 构建概览指标卡片
func (s *RuntimeState) MetricCards() []MetricCard {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	todayRange := fmt.Sprintf("%s 00:00-现在", now.Format("01-02"))
	successesToday := 0
	failuresToday := 0
	notifiesToday := 0
	for _, up := range s.uploads {
		if up.Note == "历史文件" {
			continue
		}
		if up.Time.Before(todayStart) {
			continue
		}
		switch up.Result {
		case "success":
			successesToday++
		case "failed":
			failuresToday++
		}
	}
	for _, ev := range s.notifications {
		if ev.Time.Before(todayStart) {
			continue
		}
		notifiesToday++
	}
	totalToday := successesToday + failuresToday
	failRate := 0.0
	if totalToday > 0 {
		failRate = (float64(failuresToday) / float64(totalToday)) * 100
	}
	return []MetricCard{
		{Label: "今日上传", Value: fmt.Sprintf("%d", successesToday), Trend: todayRange, Tone: "up"},
		{Label: "通知次数", Value: fmt.Sprintf("%d", notifiesToday), Trend: todayRange, Tone: "up"},
		{Label: "今日失败", Value: fmt.Sprintf("%d", failuresToday), Trend: todayRange, Tone: "muted"},
		{Label: "失败率", Value: fmt.Sprintf("%.1f%%", failRate), Trend: todayRange, Tone: "warning"},
		{Label: "队列深度", Value: fmt.Sprintf("%d", s.queueLen), Trend: "背压监控", Tone: "warning"},
	}
}

// ChartPoints 返回队列趋势点
func (s *RuntimeState) ChartPoints() []ChartPoint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	points := append([]ChartPoint(nil), s.queue...)
	if len(points) == 0 {
		return seedQueuePoints()
	}
	return points
}

// HeroCopy 构建首页摘要信息
func (s *RuntimeState) HeroCopy(cfg *models.Config) HeroCopy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	suffix := formatSuffixFilter(cfg.FileExt)
	watchDirs := make([]string, 0, len(s.watchDirs))
	for _, dir := range s.watchDirs {
		norm := normalizePath(dir)
		if norm == "" {
			continue
		}
		watchDirs = append(watchDirs, norm)
	}
	return HeroCopy{
		Agent:        s.host,
		WatchDirs:    watchDirs,
		SuffixFilter: suffix,
		Silence:      cfg.Silence,
		Queue:        fmt.Sprintf("队列 %d", s.queueLen),
		Concurrency:  fmt.Sprintf("上传并发 %d", s.workers),
	}
}

// ConfigSnapshot 构建可编辑的配置快照
func (s *RuntimeState) ConfigSnapshot(cfg *models.Config) ConfigSnapshot {
	retryEnabled := true
	if cfg.UploadRetryEnabled != nil {
		retryEnabled = *cfg.UploadRetryEnabled
	}
	return ConfigSnapshot{
		WatchDir:              cfg.WatchDir,
		FileExt:               formatExtList(cfg.FileExt),
		Silence:               cfg.Silence,
		Concurrency:           fmt.Sprintf("workers=%d / queue=%d", cfg.UploadWorkers, cfg.UploadQueueSize),
		UploadRetryDelays:     strings.TrimSpace(cfg.UploadRetryDelays),
		UploadRetryEnabled:    retryEnabled,
		SystemResourceEnabled: cfg.SystemResourceEnabled,
	}
}

// 格式化后缀过滤展示
func formatSuffixFilter(raw string) string {
	extLabel := formatExtList(raw)
	if strings.TrimSpace(extLabel) == "" {
		return "关闭 · 全量目录"
	}
	return fmt.Sprintf("过滤 %s", extLabel)
}

// 解析并格式化后缀列表
func formatExtList(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	// 保持与后端解析一致的显示格式
	if exts, err := match.ParseExtList(trimmed); err == nil && len(exts) > 0 {
		return strings.Join(exts, ", ")
	}
	return trimmed
}

// MonitorNotes 返回配置驱动的说明信息
func (s *RuntimeState) MonitorNotes(cfg *models.Config) []MonitorNote {
	retryDelays := strings.TrimSpace(cfg.UploadRetryDelays)
	if retryDelays == "" {
		retryDelays = "1s,2s,5s"
	}
	retryEnabled := true
	if cfg.UploadRetryEnabled != nil {
		retryEnabled = *cfg.UploadRetryEnabled
	}
	retryStatus := "关闭"
	if retryEnabled {
		retryStatus = "开启"
	}
	return []MonitorNote{
		{Title: "OSS 连接", Detail: fmt.Sprintf("endpoint=%s · region=%s", cfg.Endpoint, cfg.Region)},
		{Title: "上传工作池", Detail: fmt.Sprintf("workers=%d · queue=%d · 当前 backlog=%d", cfg.UploadWorkers, cfg.UploadQueueSize, s.queueLen)},
		{Title: "上传重试", Detail: fmt.Sprintf("%s · 间隔 %s", retryStatus, retryDelays)},
		{Title: "通知", Detail: "钉钉通知 / 邮件通知"},
	}
}

// DashboardLite 返回轻量级的实时数据（不包含目录与文件列表）
func (s *RuntimeState) DashboardLite(cfg *models.Config) DashboardData {
	return DashboardData{
		HeroCopy:       s.HeroCopy(cfg),
		MetricCards:    s.MetricCards(),
		DirectoryTree:  []FileNode{},
		Files:          []FileItem{},
		MonitorNotes:   s.MonitorNotes(cfg),
		UploadRecords:  s.UploadRecords(),
		MonitorSummary: s.MonitorSummary(),
		ConfigSnapshot: s.ConfigSnapshot(cfg),
		ChartPoints:    s.ChartPoints(),
	}
}

// Dashboard 聚合所有板块，生成接口返回结构
func (s *RuntimeState) Dashboard(cfg *models.Config) DashboardData {
	files := s.collectFiles()
	return DashboardData{
		HeroCopy:       s.HeroCopy(cfg),
		MetricCards:    s.MetricCards(),
		DirectoryTree:  s.buildDirectoryTree(files),
		Files:          s.buildFileItems(files),
		MonitorNotes:   s.MonitorNotes(cfg),
		UploadRecords:  s.UploadRecords(),
		MonitorSummary: s.MonitorSummary(),
		ConfigSnapshot: s.ConfigSnapshot(cfg),
		ChartPoints:    s.ChartPoints(),
	}
}

type scannedFile struct {
	path    string
	size    int64
	modTime time.Time
	state   fileState
}

// 遍历磁盘收集文件列表
func (s *RuntimeState) collectFiles() []scannedFile {
	// 在锁内快照状态
	s.mu.RLock()
	stateCopy := make(map[string]fileState, len(s.fileState))
	for p, st := range s.fileState {
		stateCopy[p] = st
	}
	autoCopy := make(map[string]bool, len(s.autoOn))
	for p, v := range s.autoOn {
		autoCopy[p] = v
	}
	s.mu.RUnlock()

	files := []scannedFile{}
	seen := make(map[string]struct{})
	for _, watchDir := range s.watchDirs {
		if strings.TrimSpace(watchDir) == "" {
			continue
		}
		_ = filepath.WalkDir(watchDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if s.isExcludedPath(path) {
					return fs.SkipDir
				}
				return nil
			}
			if !s.isTargetFile(path) {
				return nil
			}
			stat, statErr := d.Info()
			if statErr != nil {
				return nil
			}
			norm := normalizePath(path)
			if _, ok := seen[norm]; ok {
				return nil
			}
			seen[norm] = struct{}{}
			fsState := stateCopy[norm]
			if fsState.Status == StatusUnknown {
				fsState.Status = StatusExisting
			}
			autoEnabled := autoEnabledFromCopy(autoCopy, norm)
			fsState.AutoUpload = autoEnabled
			files = append(files, scannedFile{
				path:    norm,
				size:    stat.Size(),
				modTime: stat.ModTime(),
				state:   fsState,
			})
			return nil
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})
	return files
}

// 记录文件状态变化并补充上传记录
func (s *RuntimeState) recordState(path string, status FileStatus, manual bool, note string) {
	norm := normalizePath(path)
	now := time.Now()
	info := s.statFile(path)

	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.fetchOrInitStateLocked(norm)
	st.Status = status
	st.AutoUpload = s.autoUploadLocked(path)
	if manual {
		st.Note = "手动上传"
	}
	if note != "" {
		st.Note = note
	}
	s.fileState[norm] = st
	s.appendUploadLocked(uploadHistory{
		File:    filepath.Base(norm),
		Target:  "",
		Size:    formatSize(info.size),
		Result:  "pending",
		Latency: "--",
		Time:    now,
		Note:    st.Note,
	})
}

// 获取已存在状态或初始化默认值
func (s *RuntimeState) fetchOrInitStateLocked(path string) fileState {
	if st, ok := s.fileState[path]; ok {
		return st
	}
	return fileState{
		Status:     StatusQueued,
		AutoUpload: s.autoUploadLocked(path),
	}
}

// 状态转为展示文本
func fileStatusLabel(status FileStatus) string {
	switch status {
	case StatusQueued:
		return "queued"
	case StatusUploaded:
		return "uploaded"
	case StatusFailed:
		return "failed"
	case StatusExisting:
		return "existing"
	default:
		return "existing"
	}
}

// 追加上传记录
func (s *RuntimeState) appendUploadLocked(record uploadHistory) {
	s.uploads = append(s.uploads, record)
	if len(s.uploads) > maxUploadRecords { //如果超过 maxUploadRecords，就把前面旧的裁掉，只保留最新的 N 条
		s.uploads = s.uploads[len(s.uploads)-maxUploadRecords:]
	}
}

// 判断某个路径是否“自动上传开启”
func (s *RuntimeState) autoUploadLocked(path string) bool {
	norm := normalizeKeyPath(path)
	if norm == "" {
		return true
	}
	if v, ok := s.autoOn[norm]; ok {
		return v
	}
	dir := normalizeKeyPath(filepath.Dir(norm))
	for dir != "" && dir != "." && dir != "/" && dir != norm {
		if v, ok := s.autoOn[dir]; ok { //找到上级目录的设置
			return v
		}
		next := normalizeKeyPath(filepath.Dir(dir))
		// Windows 盘符根目录可能反复返回自身，避免死循环
		if next == dir {
			break
		}
		dir = next
	}
	return true
}

// 是否匹配目标后缀
func (s *RuntimeState) isTargetFile(path string) bool {
	if s.matcher == nil {
		return true
	}
	// 匹配器内部处理后缀规则
	return s.matcher.IsTargetFile(path)
}

// 判断路径是否需要排除
func (s *RuntimeState) isExcludedPath(path string) bool {
	if s.exclude == nil {
		return false
	}
	return s.exclude.IsExcluded(path)
}

type fileInfo struct {
	size int64
}

// statFile 用于读取文件状态支撑变更判断
func (s *RuntimeState) statFile(path string) fileInfo {
	info := fileInfo{}
	// 读取文件大小用于展示与统计
	if st, err := os.Stat(path); err == nil {
		info.size = st.Size()
	}
	return info
}

// normalizePath 用于统一数据格式便于比较与存储
func normalizePath(path string) string {
	// 统一路径格式便于比较
	if path == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(path))
}

// normalizeKeyPath 用于统一数据格式便于比较与存储
func normalizeKeyPath(path string) string {
	// 保持路径大小写不做系统适配
	norm := normalizePath(path)
	if norm == "" {
		return ""
	}
	return norm
}

// isDirPath 用于判断条件是否成立
func isDirPath(path string) bool {
	// 判断路径是否为目录
	if path == "" {
		return false
	}
	info, err := os.Stat(filepath.FromSlash(path))
	if err != nil {
		return false
	}
	return info.IsDir()
}

// isSameOrChildPath 用于判断条件是否成立
func isSameOrChildPath(path, root string) bool {
	// 判断路径是否为根路径或子路径
	path = normalizeKeyPath(path)
	root = normalizeKeyPath(root)
	if root == "" {
		return false
	}
	if path == root {
		return true
	}
	if strings.HasSuffix(root, "/") {
		return strings.HasPrefix(path, root)
	}
	if strings.HasPrefix(path, root) {
		return strings.HasPrefix(path[len(root):], "/")
	}
	return false
}

// 根据快照计算自动上传开关
func autoEnabledFromCopy(auto map[string]bool, path string) bool {
	norm := normalizeKeyPath(path)
	if norm == "" {
		return true
	}
	if v, ok := auto[norm]; ok {
		return v
	}
	dir := normalizeKeyPath(filepath.Dir(norm))
	for dir != "" && dir != "." && dir != "/" && dir != norm {
		if v, ok := auto[dir]; ok {
			return v
		}
		next := normalizeKeyPath(filepath.Dir(dir))
		if next == dir {
			break
		}
		dir = next
	}
	return true
}

// 把字节数格式化成人类可读字符串的函数
func formatSize(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		if bytes <= 0 {
			return "--"
		}
		return fmt.Sprintf("%d B", bytes)
	}
}

// 格式化耗时展示
func formatLatency(d time.Duration) string {
	if d <= 0 {
		return "--"
	}
	return fmt.Sprintf("%d ms", d.Milliseconds())
}

// 格式化时间展示
func formatDateTime(t time.Time) string {
	if t.IsZero() {
		return "--"
	}
	return t.Format("2006-01-02 15:04:05")
}

// 初始化队列趋势图的“默认数据”
func seedQueuePoints() []ChartPoint {
	return []ChartPoint{
		{Label: "00:00", Uploads: 0, Failures: 0, Queue: 0},
		{Label: "04:00", Uploads: 0, Failures: 0, Queue: 0},
		{Label: "08:00", Uploads: 0, Failures: 0, Queue: 0},
		{Label: "12:00", Uploads: 0, Failures: 0, Queue: 0},
		{Label: "16:00", Uploads: 0, Failures: 0, Queue: 0},
		{Label: "20:00", Uploads: 0, Failures: 0, Queue: 0},
	}
}
