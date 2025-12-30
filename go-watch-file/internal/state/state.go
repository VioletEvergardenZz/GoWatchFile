package state

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"file-watch/internal/models"
)

//“内存中保留数量的上限”常量，用来限制运行态数据列表的长度
const (
	maxTailLines      = 200
	maxTimelineEvents = 120
	maxUploadRecords  = 200
	maxQueuePoints    = 32		//队列趋势图点的最大数量
	maxNotifications  = 200		//通知事件的最大数量
)

type FileStatus int

//枚举类型（整型），表示文件的处理状态
const (
	StatusUnknown FileStatus = iota
	StatusQueued
	StatusUploaded
	StatusFailed
	StatusExisting
)

//记录“被监控文件”的当前状态
type fileState struct {
	Status           FileStatus
	Target           string				//上传目标
	Latency          time.Duration		//上传耗时
	Note             string				//备注信息，如（“自动入队”“上传失败原因”）
	AutoUpload       bool
}

//时间线事件条目
type timelineEntry struct {
	Label  string
	Time   time.Time
	Status string
	Host   string
}

//上传历史记录条目
type uploadHistory struct {
	File    string
	Target  string
	Size    string
	Result  string
	Latency string
	Time    time.Time
	Note    string
}

//通知事件记录结构
type notificationEvent struct {
	Time    time.Time		//通知发送时间
	Channel string			//通知渠道，如“钉钉”“企业微信”等
}

// RuntimeState 保存接口与界面所需的内存运行态数据
type RuntimeState struct {
	//并发控制
	mu sync.RWMutex					//读多写少，读用 RLock，写用 Lock

	//配置/环境
	host      string
	watchDir  string
	fileExt   string

	//文件状态
	fileState map[string]fileState
	autoOn    map[string]bool

	//UI 面板数据（环形缓存式）
	tailLines []string
	timeline  []timelineEntry
	uploads   []uploadHistory
	queue     []ChartPoint			//队列/上传统计的趋势点
	notifications []notificationEvent

	//计数器/指标
	successes int					//累计成功上传数
	failures  int
	workers   int
	queueLen  int
}

// NewRuntimeState 基于配置默认值构建 RuntimeState
func NewRuntimeState(cfg *models.Config) *RuntimeState {
	host, _ := os.Hostname()
	watchDir := normalizePath(cfg.WatchDir)
	auto := map[string]bool{}
	if watchDir != "" {
		auto[normalizeKeyPath(watchDir)] = true
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

// CarryOverFrom 从已有状态迁移计数与历史，保证配置重载后指标连续
func (s *RuntimeState) CarryOverFrom(old *RuntimeState) {
	if old == nil {
		return
	}
	old.mu.RLock()			//加读锁
	defer old.mu.RUnlock()

	s.successes = old.successes
	s.failures = old.failures
	s.queueLen = old.queueLen

	//把旧状态里的几组切片“拷贝一份”到新状态里
	s.timeline = append([]timelineEntry(nil), old.timeline...)
	s.tailLines = append([]string(nil), old.tailLines...)
	s.uploads = append([]uploadHistory(nil), old.uploads...)
	s.notifications = append([]notificationEvent(nil), old.notifications...)
	s.queue = append([]ChartPoint(nil), old.queue...)
}

// BootstrapExisting 预加载监控目录下的已有文件为已存在状态
func (s *RuntimeState) BootstrapExisting() error {
	if s.watchDir == "" {
		return nil
	}

	//扫描监控目录里的所有文件和子目录
	err := filepath.WalkDir(s.watchDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !s.isTargetFile(path) {	//不符合目标后缀,跳过
			return nil
		}
		stat, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		s.mu.Lock()
		s.fileState[normalizePath(path)] = fileState{
			Status:     StatusExisting,
			AutoUpload: s.autoUploadLocked(path),
			Note:       "历史文件",
		}
		s.appendUploadLocked(uploadHistory{
			File:    filepath.Base(path),		//取路径的“最后一段”，也就是文件名
			Target:  "",
			Size:    formatSize(stat.Size()),
			Result:  "success",
			Latency: "--",
			Time:    stat.ModTime(),
			Note:    "历史文件",
		})
		s.mu.Unlock()
		return nil
	})
	return err
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
		for p := range s.autoOn {
			if p != keyPath && isSameOrChildPath(p, keyPath) {
				delete(s.autoOn, p)
			}
		}
	}
	s.autoOn[keyPath] = enabled
	for p, st := range s.fileState {
		if (isDir && isSameOrChildPath(p, normPath)) || (!isDir && normalizeKeyPath(p) == keyPath) {
			st.AutoUpload = enabled
			s.fileState[p] = st
		}
	}
	s.appendTimelineLocked("调整自动上传", "info", s.host, time.Now())
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
	now := time.Now()
	st := s.fetchOrInitStateLocked(norm)
	st.Status = StatusQueued
	st.Note = "自动上传关闭，待手动触发"
	st.AutoUpload = false
	s.fileState[norm] = st
	s.appendTailLocked(fmt.Sprintf("[%s] 跳过 %s: 自动上传关闭", now.Format("15:04:05"), norm))
	s.appendTimelineLocked("自动上传关闭，跳过", "warning", s.host, now)
}

// MarkUploaded 记录上传成功
func (s *RuntimeState) MarkUploaded(path, target string, latency time.Duration) {
	norm := normalizePath(path)
	info := s.statFile(path)
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.fetchOrInitStateLocked(norm)
	st.Status = StatusUploaded
	st.Target = target
	st.Latency = latency
	st.Note = "自动上传"
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

// TailLines 返回近期尾部日志的副本
func (s *RuntimeState) TailLines() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.tailLines...)
}

// TimelineEvents 返回可直接用于控制台展示的时间线事件
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

// UploadRecords 返回按时间倒序的上传历史
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

// FileItems 扫描磁盘并合并运行态，生成文件表数据
func (s *RuntimeState) FileItems() []FileItem {
	files := s.collectFiles()
	items := make([]FileItem, 0, len(files))
	for _, f := range files {
		items = append(items, FileItem{
			Name:             filepath.Base(f.path),
			Path:             normalizePath(f.path),
			Size:             formatSize(f.size),
			Status:           fileStatusLabel(f.state.Status),
			Time:             f.modTime.Format("15:04:05"),
			AutoUpload:       f.state.AutoUpload,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Time > items[j].Time
	})
	return items
}

// DirectoryTree 构建用于界面的目录树
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

	// 文件加入后再把目录挂到其父节点
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

// MonitorSummary 构建摘要卡片
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

// MetricCards 构建概览指标卡片
func (s *RuntimeState) MetricCards() []MetricCard {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
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
		{Label: "今日上传", Value: fmt.Sprintf("%d", successesToday), Trend: "今日成功", Tone: "up"},
		{Label: "通知次数", Value: fmt.Sprintf("%d", notifiesToday), Trend: "钉钉", Tone: "up"},
		{Label: "失败累计", Value: fmt.Sprintf("%d", s.failures), Trend: "重试/隔离", Tone: "muted"},
		{Label: "失败率", Value: fmt.Sprintf("%.1f%%", failRate), Trend: "今日统计", Tone: "warning"},
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
	suffix := fmt.Sprintf("过滤 %s", cfg.FileExt)
	if strings.TrimSpace(cfg.FileExt) == "" {
		suffix = "关闭 · 全量目录"
	}
	return HeroCopy{
		Agent:        s.host,
		WatchDirs:    []string{cfg.WatchDir},
		SuffixFilter: suffix,
		Silence:      cfg.Silence,
		Queue:        fmt.Sprintf("队列 %d", s.queueLen),
		Concurrency:  fmt.Sprintf("上传并发 %d", s.workers),
	}
}

// ConfigSnapshot 构建可编辑的配置快照
func (s *RuntimeState) ConfigSnapshot(cfg *models.Config) ConfigSnapshot {
	return ConfigSnapshot{
		WatchDir:    cfg.WatchDir,
		FileExt:     cfg.FileExt,
		Silence:     cfg.Silence,
		Concurrency: fmt.Sprintf("workers=%d / queue=%d", cfg.UploadWorkers, cfg.UploadQueueSize),
 	}
}

// MonitorNotes 返回配置驱动的说明信息
func (s *RuntimeState) MonitorNotes(cfg *models.Config) []MonitorNote {
	return []MonitorNote{
		{Title: "S3 连接", Detail: fmt.Sprintf("endpoint=%s · region=%s", cfg.Endpoint, cfg.Region)},
		{Title: "上传工作池", Detail: fmt.Sprintf("workers=%d · queue=%d · 当前 backlog=%d", cfg.UploadWorkers, cfg.UploadQueueSize, s.queueLen)},
		{Title: "通知", Detail: "企业微信/钉钉可选 · 失败自动告警"},
	}
}

// Dashboard 聚合所有板块，生成接口返回结构
func (s *RuntimeState) Dashboard(cfg *models.Config) DashboardData {
	return DashboardData{
		HeroCopy:       s.HeroCopy(cfg),
		MetricCards:    s.MetricCards(),
		DirectoryTree:  s.DirectoryTree(),
		Files:          s.FileItems(),
		TailLines:      s.TailLines(),
		TimelineEvents: s.TimelineEvents(),
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
	_ = filepath.WalkDir(s.watchDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
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

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})
	return files
}

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
		Status:     StatusQueued,
		AutoUpload: s.autoUploadLocked(path),
	}
}

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

//追加上传记录
func (s *RuntimeState) appendUploadLocked(record uploadHistory) {
	s.uploads = append(s.uploads, record)
	if len(s.uploads) > maxUploadRecords {  //如果超过 maxUploadRecords，就把前面旧的裁掉，只保留最新的 N 条
		s.uploads = s.uploads[len(s.uploads)-maxUploadRecords:]
	}
}

//判断某个路径是否“自动上传开启”
func (s *RuntimeState) autoUploadLocked(path string) bool {
	norm := normalizeKeyPath(path)
	if norm == "" {
		return true
	}
	if v, ok := s.autoOn[norm]; ok {
		return v
	}
	dir := filepath.Dir(norm)
	for dir != "." && dir != "/" && dir != norm {
		if v, ok := s.autoOn[dir]; ok {		//找到上级目录的设置
			return v
		}
		dir = filepath.Dir(dir)
	}
	return true
}

//是否匹配目标后缀
func (s *RuntimeState) isTargetFile(path string) bool {
	ext := strings.TrimSpace(s.fileExt)
	if ext == "" {
		return true		//所有文件都算目标
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

func normalizeKeyPath(path string) string {
	norm := normalizePath(path)
	if norm == "" {
		return ""
	}
	if runtime.GOOS == "windows" {
		return strings.ToLower(norm)
	}
	return norm
}

func isDirPath(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(filepath.FromSlash(path))
	if err != nil {
		return false
	}
	return info.IsDir()
}

func isSameOrChildPath(path, root string) bool {
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

func autoEnabledFromCopy(auto map[string]bool, path string) bool {
	norm := normalizeKeyPath(path)
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

//把字节数格式化成人类可读字符串的函数
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

//初始化队列趋势图的“默认数据”
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
