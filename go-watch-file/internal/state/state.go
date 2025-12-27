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

	"file-watch/internal/models"
)

const (
	maxTailLines      = 200
	maxTimelineEvents = 120
	maxUploadRecords  = 200
	maxQueuePoints    = 32
)

type fileState struct {
	Status           string
	RequiresApproval bool
	Target           string
	Latency          time.Duration
	Note             string
	Source           string
	AutoUpload       bool
}

type timelineEntry struct {
	Label  string
	Time   time.Time
	Status string
	Host   string
}

type uploadHistory struct {
	File    string
	Target  string
	Size    string
	Result  string
	Latency string
	Time    time.Time
	Note    string
}

// RuntimeState stores in-memory runtime data for the API and UI.
type RuntimeState struct {
	mu sync.RWMutex

	host      string
	watchDir  string
	fileExt   string
	fileState map[string]fileState
	autoOn    map[string]bool

	tailLines []string
	timeline  []timelineEntry
	uploads   []uploadHistory
	queue     []ChartPoint

	successes int
	failures  int
	workers   int
	queueLen  int
}

// NewRuntimeState constructs RuntimeState from config defaults.
func NewRuntimeState(cfg *models.Config) *RuntimeState {
	host, _ := os.Hostname()
	watchDir := normalizePath(cfg.WatchDir)
	auto := map[string]bool{}
	if watchDir != "" {
		auto[watchDir] = true
	}
	return &RuntimeState{
		host:      host,
		watchDir:  watchDir,
		fileExt:   strings.TrimSpace(cfg.FileExt),
		fileState: make(map[string]fileState),
		autoOn:    auto,
		queue:     seedQueuePoints(),
		workers:   cfg.UploadWorkers,
	}
}

// BootstrapExisting preloads existing files under watchDir as uploaded.
func (s *RuntimeState) BootstrapExisting() error {
	if s.watchDir == "" {
		return nil
	}
	err := filepath.WalkDir(s.watchDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			s.setAutoDefault(path)
			return nil
		}
		if !s.isTargetFile(path) {
			return nil
		}
		stat, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		s.mu.Lock()
		s.fileState[normalizePath(path)] = fileState{
			Status:     "uploaded",
			AutoUpload: s.autoUploadLocked(path),
		}
		s.appendUploadLocked(uploadHistory{
			File:    filepath.Base(path),
			Target:  "",
			Size:    formatSize(stat.Size()),
			Result:  "success",
			Latency: "--",
			Time:    stat.ModTime(),
			Note:    "历史文件",
		})
		s.successes++
		s.mu.Unlock()
		return nil
	})
	return err
}

// AutoUploadEnabled returns whether auto upload is enabled for the path.
func (s *RuntimeState) AutoUploadEnabled(path string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.autoUploadLocked(path)
}

// SetAutoUpload toggles auto upload for a path (file or directory) and cascades to existing state.
func (s *RuntimeState) SetAutoUpload(path string, enabled bool) {
	norm := normalizePath(path)
	s.mu.Lock()
	defer s.mu.Unlock()
	if norm == "" {
		return
	}
	s.autoOn[norm] = enabled
	for p, st := range s.fileState {
		if strings.HasPrefix(p, norm) {
			st.AutoUpload = enabled
			st.RequiresApproval = !enabled
			s.fileState[p] = st
		}
	}
	s.appendTimelineLocked("调整自动上传", "info", s.host, time.Now())
}

// MarkQueued records a file entering queue (auto path).
func (s *RuntimeState) MarkQueued(path string) {
	s.recordState(path, "queued", false, "自动入队")
}

// MarkManualQueued records a manual upload request.
func (s *RuntimeState) MarkManualQueued(path string) {
	s.recordState(path, "queued", true, "手动触发")
}

// MarkSkipped records skipped upload because auto-upload off.
func (s *RuntimeState) MarkSkipped(path string) {
	norm := normalizePath(path)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	st := s.fetchOrInitStateLocked(norm)
	st.Status = "queued"
	st.RequiresApproval = true
	st.Note = "自动上传关闭，待审批/手动触发"
	st.AutoUpload = false
	s.fileState[norm] = st
	s.appendTailLocked(fmt.Sprintf("[%s] 跳过 %s: 自动上传关闭", now.Format("15:04:05"), norm))
	s.appendTimelineLocked("自动上传关闭，跳过", "warning", s.host, now)
}

// MarkUploaded records successful upload.
func (s *RuntimeState) MarkUploaded(path, target string, latency time.Duration) {
	norm := normalizePath(path)
	info := s.statFile(path)
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.fetchOrInitStateLocked(norm)
	st.Status = "uploaded"
	st.Target = target
	st.Latency = latency
	st.Note = "自动上传"
	st.RequiresApproval = false
	st.AutoUpload = s.autoUploadLocked(path)
	s.fileState[norm] = st

	s.appendTailLocked(fmt.Sprintf("[%s] 上传成功 %s", now.Format("15:04:05"), norm))
	s.appendTimelineLocked("上传成功", "success", s.host, now)
	s.appendUploadLocked(uploadHistory{
		File:    filepath.Base(norm),
		Target:  target,
		Size:    formatSize(info.size),
		Result:  "success",
		Latency: formatLatency(latency),
		Time:    now,
		Note:    st.Note,
	})
	s.successes++
}

// MarkFailed records a failed upload.
func (s *RuntimeState) MarkFailed(path string, reason error) {
	norm := normalizePath(path)
	info := s.statFile(path)
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.fetchOrInitStateLocked(norm)
	st.Status = "failed"
	st.Note = reason.Error()
	st.AutoUpload = s.autoUploadLocked(path)
	s.fileState[norm] = st

	s.appendTailLocked(fmt.Sprintf("[%s] 上传失败 %s err=%v", now.Format("15:04:05"), norm, reason))
	s.appendTimelineLocked("上传失败", "danger", s.host, now)
	s.appendUploadLocked(uploadHistory{
		File:    filepath.Base(norm),
		Target:  info.target,
		Size:    formatSize(info.size),
		Result:  "failed",
		Latency: "--",
		Time:    now,
		Note:    reason.Error(),
	})
	s.failures++
}

// SetQueueStats updates queue length/workers and appends a chart point.
func (s *RuntimeState) SetQueueStats(stats models.UploadStats) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queueLen = stats.QueueLength
	s.workers = stats.Workers
	label := now.Format("15:04")
	s.queue = append(s.queue, ChartPoint{
		Label:    label,
		Uploads:  s.successes,
		Failures: s.failures,
		Queue:    stats.QueueLength,
	})
	if len(s.queue) > maxQueuePoints {
		s.queue = s.queue[len(s.queue)-maxQueuePoints:]
	}
}

// TailLines returns a copy of recent tail lines.
func (s *RuntimeState) TailLines() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.tailLines...)
}

// TimelineEvents returns console-ready timeline events.
func (s *RuntimeState) TimelineEvents() []TimelineEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	events := make([]TimelineEvent, 0, len(s.timeline))
	for _, ev := range s.timeline {
		events = append(events, TimelineEvent{
			Label:  ev.Label,
			Time:   ev.Time.Format("15:04:05"),
			Status: ev.Status,
			Host:   ev.Host,
		})
	}
	return events
}

// UploadRecords returns upload history ordered by time desc.
func (s *RuntimeState) UploadRecords() []UploadRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]UploadRecord, len(s.uploads))
	for i := range s.uploads {
		item := s.uploads[len(s.uploads)-1-i]
		out[i] = UploadRecord{
			File:    item.File,
			Target:  item.Target,
			Size:    item.Size,
			Result:  item.Result,
			Latency: item.Latency,
			Time:    item.Time.Format("15:04:05"),
			Note:    item.Note,
		}
	}
	return out
}

// FileItems scans disk and merges runtime state for the file table.
func (s *RuntimeState) FileItems() []FileItem {
	files := s.collectFiles()
	items := make([]FileItem, 0, len(files))
	for _, f := range files {
		items = append(items, FileItem{
			Name:             filepath.Base(f.path),
			Path:             normalizePath(f.path),
			Size:             formatSize(f.size),
			Status:           f.state.Status,
			Time:             f.modTime.Format("15:04:05"),
			AutoUpload:       f.state.AutoUpload,
			RequiresApproval: f.state.RequiresApproval,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Time > items[j].Time
	})
	return items
}

// DirectoryTree builds the directory tree for UI.
func (s *RuntimeState) DirectoryTree() []FileNode {
	files := s.collectFiles()
	dirMap := make(map[string]*FileNode)
	rootPath := normalizePath(s.watchDir)
	if rootPath != "" {
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
			Updated:    f.modTime.Format("15:04:05"),
			Content:    f.state.Note,
		})
	}

	// attach directories to their parents after files are added
	dirPaths := make([]string, 0, len(dirMap))
	for p := range dirMap {
		dirPaths = append(dirPaths, p)
	}
	sort.Slice(dirPaths, func(i, j int) bool {
		return strings.Count(dirPaths[i], "/") > strings.Count(dirPaths[j], "/")
	})
	for _, dirPath := range dirPaths {
		if dirPath == rootPath {
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

	if rootPath != "" {
		if root, ok := dirMap[rootPath]; ok {
			return []FileNode{*root}
		}
	}

	roots := []FileNode{}
	for path, node := range dirMap {
		if normalizePath(filepath.Dir(path)) == path || path == "" {
			roots = append(roots, *node)
		}
	}
	return roots
}

func sortChildren(node *FileNode) {
	sort.Slice(node.Children, func(i, j int) bool {
		return node.Children[i].Path < node.Children[j].Path
	})
	for i := range node.Children {
		sortChildren(&node.Children[i])
	}
}

// MonitorSummary builds summary cards.
func (s *RuntimeState) MonitorSummary() []MonitorSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := s.successes + s.failures
	failRate := 0.0
	if total > 0 {
		failRate = (float64(s.failures) / float64(total)) * 100
	}
	return []MonitorSummary{
		{Label: "当前吞吐", Value: fmt.Sprintf("%d/min", s.successes+int(s.queueLen)), Desc: "队列中 + 已上传"},
		{Label: "成功率", Value: fmt.Sprintf("%.1f%%", 100-failRate), Desc: fmt.Sprintf("失败 %d", s.failures)},
		{Label: "队列 backlog", Value: fmt.Sprintf("%d", s.queueLen), Desc: fmt.Sprintf("workers=%d", s.workers)},
		{Label: "失败累计", Value: fmt.Sprintf("%d", s.failures), Desc: "含告警和重试"},
	}
}

// MetricCards builds metric cards for overview.
func (s *RuntimeState) MetricCards() []MetricCard {
	s.mu.RLock()
	defer s.mu.RUnlock()
	total := s.successes + s.failures
	failRate := 0.0
	if total > 0 {
		failRate = (float64(s.failures) / float64(total)) * 100
	}
	return []MetricCard{
		{Label: "运行状态", Value: "Running", Trend: "心跳正常", Tone: "up"},
		{Label: "今日上传", Value: fmt.Sprintf("%d", s.successes), Trend: "累积", Tone: "up"},
		{Label: "失败率", Value: fmt.Sprintf("%.1f%%", failRate), Trend: "动态", Tone: "warning"},
		{Label: "队列深度", Value: fmt.Sprintf("%d", s.queueLen), Trend: "背压监控", Tone: "warning"},
		{Label: "失败累计", Value: fmt.Sprintf("%d", s.failures), Trend: "重试/隔离", Tone: "muted"},
	}
}

// ChartPoints returns queue trend points.
func (s *RuntimeState) ChartPoints() []ChartPoint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	points := append([]ChartPoint(nil), s.queue...)
	if len(points) == 0 {
		return seedQueuePoints()
	}
	return points
}

// HeroCopy builds hero summary info.
func (s *RuntimeState) HeroCopy(cfg *models.Config) HeroCopy {
	s.mu.RLock()
	defer s.mu.RUnlock()
	suffix := fmt.Sprintf("过滤 %s", cfg.FileExt)
	if strings.TrimSpace(cfg.FileExt) == "" {
		suffix = "关闭 · 全量目录"
	}
	return HeroCopy{
		Agent:        s.host,
		WatchDirs:    []string{cfg.WatchDir},
		SuffixFilter: suffix,
		Silence:      "10s 静默",
		Queue:        fmt.Sprintf("队列 %d", s.queueLen),
		Concurrency:  fmt.Sprintf("上传并发 %d", s.workers),
		Bucket:       cfg.Bucket,
	}
}

// ConfigSnapshot builds editable config snapshot.
func (s *RuntimeState) ConfigSnapshot(cfg *models.Config) ConfigSnapshot {
	return ConfigSnapshot{
		WatchDir:    cfg.WatchDir,
		FileExt:     cfg.FileExt,
		Silence:     "10s",
		Concurrency: fmt.Sprintf("workers=%d / queue=%d", cfg.UploadWorkers, cfg.UploadQueueSize),
		Bucket:      cfg.Bucket,
		Action:      "上传 + Webhook",
	}
}

// Routes returns static routing hints.
func (s *RuntimeState) Routes(cfg *models.Config) []RoutePreview {
	return []RoutePreview{
		{Name: "主目录上传", Cond: fmt.Sprintf("path startsWith %s", cfg.WatchDir), Action: fmt.Sprintf("直传 %s", cfg.Bucket)},
		{Name: "大文件审批", Cond: "size > 1GB 或 autoUpload=关闭", Action: "断点续传 + 审批"},
		{Name: "失败重试", Cond: "上传失败", Action: "隔离 + 重试 + 通知"},
	}
}

// MonitorNotes returns config-driven notes.
func (s *RuntimeState) MonitorNotes(cfg *models.Config) []MonitorNote {
	return []MonitorNote{
		{Title: "S3 连接", Detail: fmt.Sprintf("endpoint=%s · region=%s", cfg.Endpoint, cfg.Region)},
		{Title: "上传工作池", Detail: fmt.Sprintf("workers=%d · queue=%d · 当前 backlog=%d", cfg.UploadWorkers, cfg.UploadQueueSize, s.queueLen)},
		{Title: "通知", Detail: "企业微信/钉钉可选 · 失败自动告警"},
	}
}

// HeroHighlights returns headline highlights.
func (s *RuntimeState) HeroHighlights() []string {
	return []string{
		"静默判定防半截",
		"S3/OSS 路径防穿越",
		"上传并发 + 背压",
		"失败重试/隔离",
		"企微/钉钉告警",
	}
}

// Dashboard aggregates all sections for the API payload.
func (s *RuntimeState) Dashboard(cfg *models.Config) DashboardData {
	return DashboardData{
		HeroCopy:       s.HeroCopy(cfg),
		HeroHighlights: s.HeroHighlights(),
		MetricCards:    s.MetricCards(),
		DirectoryTree:  s.DirectoryTree(),
		Files:          s.FileItems(),
		TailLines:      s.TailLines(),
		TimelineEvents: s.TimelineEvents(),
		Routes:         s.Routes(cfg),
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

func (s *RuntimeState) collectFiles() []scannedFile {
	// snapshot state under lock
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
	_ = filepath.WalkDir(s.watchDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if _, ok := autoCopy[normalizePath(path)]; !ok {
				autoCopy[normalizePath(path)] = true
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
		fsState := stateCopy[norm]
		if fsState.Status == "" {
			fsState.Status = "uploaded"
		}
		autoEnabled := autoEnabledFromCopy(autoCopy, norm)
		fsState.AutoUpload = autoEnabled
		fsState.RequiresApproval = fsState.RequiresApproval || !autoEnabled
		files = append(files, scannedFile{
			path:    norm,
			size:    stat.Size(),
			modTime: stat.ModTime(),
			state:   fsState,
		})
		return nil
	})

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})
	return files
}

func (s *RuntimeState) recordState(path, status string, manual bool, note string) {
	norm := normalizePath(path)
	now := time.Now()
	info := s.statFile(path)

	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.fetchOrInitStateLocked(norm)
	st.Status = status
	st.AutoUpload = s.autoUploadLocked(path)
	st.RequiresApproval = !st.AutoUpload
	if manual {
		st.RequiresApproval = false
		st.Note = "手动上传"
	}
	if note != "" {
		st.Note = note
	}
	s.fileState[norm] = st
	s.appendTailLocked(fmt.Sprintf("[%s] 入队 %s", now.Format("15:04:05"), norm))
	s.appendTimelineLocked("检测到目标文件", "info", s.host, now)
	s.appendUploadLocked(uploadHistory{
		File:    filepath.Base(norm),
		Target:  info.target,
		Size:    formatSize(info.size),
		Result:  "pending",
		Latency: "--",
		Time:    now,
		Note:    st.Note,
	})
}

func (s *RuntimeState) fetchOrInitStateLocked(path string) fileState {
	if st, ok := s.fileState[path]; ok {
		return st
	}
	return fileState{
		Status:     "queued",
		AutoUpload: s.autoUploadLocked(path),
	}
}

func (s *RuntimeState) appendTailLocked(line string) {
	s.tailLines = append(s.tailLines, line)
	if len(s.tailLines) > maxTailLines {
		s.tailLines = s.tailLines[len(s.tailLines)-maxTailLines:]
	}
}

func (s *RuntimeState) appendTimelineLocked(label, status, host string, t time.Time) {
	s.timeline = append(s.timeline, timelineEntry{
		Label:  label,
		Status: status,
		Time:   t,
		Host:   host,
	})
	if len(s.timeline) > maxTimelineEvents {
		s.timeline = s.timeline[len(s.timeline)-maxTimelineEvents:]
	}
}

func (s *RuntimeState) appendUploadLocked(record uploadHistory) {
	s.uploads = append(s.uploads, record)
	if len(s.uploads) > maxUploadRecords {
		s.uploads = s.uploads[len(s.uploads)-maxUploadRecords:]
	}
}

func (s *RuntimeState) autoUploadLocked(path string) bool {
	norm := normalizePath(path)
	if norm == "" {
		return true
	}
	if v, ok := s.autoOn[norm]; ok {
		return v
	}
	dir := filepath.Dir(norm)
	for dir != "." && dir != "/" && dir != norm {
		if v, ok := s.autoOn[dir]; ok {
			return v
		}
		dir = filepath.Dir(dir)
	}
	return true
}

func (s *RuntimeState) setAutoDefault(path string) {
	norm := normalizePath(path)
	s.mu.Lock()
	if _, ok := s.autoOn[norm]; !ok {
		s.autoOn[norm] = true
	}
	s.mu.Unlock()
}

func (s *RuntimeState) isTargetFile(path string) bool {
	ext := strings.TrimSpace(s.fileExt)
	if ext == "" {
		return true
	}
	return strings.EqualFold(filepath.Ext(path), ext)
}

type fileInfo struct {
	size   int64
	target string
}

func (s *RuntimeState) statFile(path string) fileInfo {
	info := fileInfo{}
	if st, err := os.Stat(path); err == nil {
		info.size = st.Size()
	}
	return info
}

func normalizePath(path string) string {
	if path == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func autoEnabledFromCopy(auto map[string]bool, path string) bool {
	norm := normalizePath(path)
	if norm == "" {
		return true
	}
	if v, ok := auto[norm]; ok {
		return v
	}
	dir := filepath.Dir(norm)
	for dir != "." && dir != "/" && dir != norm {
		if v, ok := auto[dir]; ok {
			return v
		}
		dir = filepath.Dir(dir)
	}
	return true
}

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

func formatLatency(d time.Duration) string {
	if d <= 0 {
		return "--"
	}
	return fmt.Sprintf("%d ms", d.Milliseconds())
}

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
