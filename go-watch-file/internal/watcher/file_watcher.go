// 本文件用于文件监听与入队触发
package watcher

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"

	"file-watch/internal/logger"
	"file-watch/internal/match"
	"file-watch/internal/models"
	"file-watch/internal/pathutil"
)

const (
	// throttleDuration     = 120 * time.Second
	logThrottleDuration  = 5 * time.Second  // 日志节流时间间隔（控制台或者程序日志文件输出频率）
	defaultSilenceWindow = 10 * time.Second // 文件写入完成检测超时时间（默认）
)

var errWatchLimitReached = errors.New("watch limit reached")

// FileWatcher 文件监控器
type FileWatcher struct {
	watcher       *fsnotify.Watcher //实际的文件监听器对象
	config        *models.Config
	uploadPool    UploadPool
	matcher       *match.Matcher
	exclude       *pathutil.ExcludeMatcher
	ctx           context.Context
	cancel        context.CancelFunc
	eventsDone    chan struct{}
	stateMutex    sync.Mutex
	watchMutex    sync.Mutex
	lastLogged    map[string]time.Time
	lastWriteTime map[string]time.Time
	writeTimers   map[string]*time.Timer
	watchedDirs   map[string]struct{}
	silenceWindow time.Duration
}

// UploadPool 上传池接口
type UploadPool interface {
	AddFile(filePath string) error
}

// NewFileWatcher 创建新的文件监控器
func NewFileWatcher(config *models.Config, uploadPool UploadPool) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	// 初始化匹配器用于后缀过滤
	matcher := match.NewMatcher(config.FileExt)
	exclude := pathutil.NewExcludeMatcher(config.WatchExclude)

	return &FileWatcher{
		watcher:       watcher,
		config:        config,
		uploadPool:    uploadPool,
		matcher:       matcher,
		exclude:       exclude,
		lastLogged:    make(map[string]time.Time),
		lastWriteTime: make(map[string]time.Time),
		writeTimers:   make(map[string]*time.Timer),
		watchedDirs:   make(map[string]struct{}),
		silenceWindow: parseSilenceWindow(config.Silence),
	}, nil
}

// Start 启动文件监控
func (fw *FileWatcher) Start() error {
	logger.Info("初始化文件监控器...")
	watchDirs := pathutil.SplitWatchDirs(fw.config.WatchDir)
	logger.Info("开始监控目录: %s", strings.Join(watchDirs, ", "))

	// 多目录场景下逐个加入监听
	err := fw.addWatchRecursivelyForDirs(watchDirs)
	if err != nil {
		if errors.Is(err, errWatchLimitReached) {
			logger.Warn("监控目录过大导致系统句柄不足，已降级为部分监控: %v", err)
		} else {
			logger.Error("添加目录监控失败: %v", err)
			return err
		}
	}

	// 启动事件处理协程
	fw.startEventLoop()

	logger.Info("文件监控服务启动成功，等待文件变化...")
	return nil
}

// Reset 重新加载监控配置并重启监听
func (fw *FileWatcher) Reset(config *models.Config) error {
	if config == nil {
		return errors.New("config is nil")
	}
	if fw.watcher == nil {
		return errors.New("watcher is nil")
	}

	prevConfig := fw.config
	fw.stopEventLoop()
	fw.resetFileState()
	fw.removeAllWatches()

	fw.config = config
	fw.silenceWindow = parseSilenceWindow(config.Silence)
	// 重建匹配器确保配置更新生效
	fw.matcher = match.NewMatcher(config.FileExt)
	fw.exclude = pathutil.NewExcludeMatcher(config.WatchExclude)

	watchDirs := pathutil.SplitWatchDirs(fw.config.WatchDir)
	logger.Info("开始监控目录: %s", strings.Join(watchDirs, ", "))
	err := fw.addWatchRecursivelyForDirs(watchDirs)
	if err != nil && !errors.Is(err, errWatchLimitReached) {
		logger.Error("添加目录监控失败: %v", err)
		if prevConfig != nil {
			fw.removeAllWatches()
			fw.config = prevConfig
			fw.silenceWindow = parseSilenceWindow(prevConfig.Silence)
			restoreErr := fw.addWatchRecursivelyForDirs(pathutil.SplitWatchDirs(prevConfig.WatchDir))
			if restoreErr != nil {
				if errors.Is(restoreErr, errWatchLimitReached) {
					logger.Warn("恢复旧目录监控降级为部分监控: %v", restoreErr)
				} else {
					logger.Error("恢复旧目录监控失败: %v", restoreErr)
				}
			}
		}
		fw.startEventLoop()
		return err
	}
	if errors.Is(err, errWatchLimitReached) {
		logger.Warn("监控目录过大导致系统句柄不足，已降级为部分监控: %v", err)
	}
	fw.startEventLoop()
	logger.Info("文件监控服务重置完成")
	return nil
}

// Close 关闭文件监控器
func (fw *FileWatcher) Close() error {
	fw.stopEventLoop()
	fw.resetFileState()
	fw.resetWatchState()
	return fw.watcher.Close()
}

func (fw *FileWatcher) startEventLoop() {
	ctx, cancel := context.WithCancel(context.Background())
	fw.ctx = ctx
	fw.cancel = cancel
	done := make(chan struct{})
	fw.eventsDone = done
	go fw.handleEvents(ctx, done)
}

func (fw *FileWatcher) stopEventLoop() {
	if fw.cancel != nil {
		fw.cancel()
	}
	if fw.eventsDone == nil {
		return
	}
	select {
	case <-fw.eventsDone:
	case <-time.After(2 * time.Second):
		logger.Warn("停止文件事件循环超时")
	}
}

func (fw *FileWatcher) resetFileState() {
	fw.stateMutex.Lock()
	for _, t := range fw.writeTimers {
		if t != nil {
			t.Stop()
		}
	}
	fw.lastLogged = make(map[string]time.Time)
	fw.lastWriteTime = make(map[string]time.Time)
	fw.writeTimers = make(map[string]*time.Timer)
	fw.stateMutex.Unlock()
}

func (fw *FileWatcher) resetWatchState() {
	fw.watchMutex.Lock()
	fw.watchedDirs = make(map[string]struct{})
	fw.watchMutex.Unlock()
}

// handleEvents 处理文件事件
func (fw *FileWatcher) handleEvents(ctx context.Context, done chan struct{}) {
	defer close(done)
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			fw.handleEvent(event)
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			logger.Error("文件监控错误: %v", err)
		}
	}
}

func (fw *FileWatcher) handleEvent(event fsnotify.Event) {
	logger.Debug("收到文件事件: %s, 操作: %s", event.Name, event.Op.String())

	if fw.isTargetFileEvent(event) {
		fw.handleTargetFileEvent(event)
	}
	// 为了在运行中发现新建的子目录并继续递归监听，需要捕获所有 Create（目录和文件）
	if event.Op&fsnotify.Create == fsnotify.Create {
		fw.handleCreatedPath(event.Name)
	}
	//文件被删除/改名时，清理 lastWriteTime/lastLogged/writeTimers，避免 map 堆积
	if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
		fw.cleanupFileState(event.Name)
		if fw.isWatchedDir(event.Name) {
			fw.removeWatchTree(event.Name)
		}
	}
}

func (fw *FileWatcher) isTargetFileEvent(event fsnotify.Event) bool {
	// 先按后缀过滤事件
	if fw.matcher != nil && !fw.matcher.IsTargetFile(event.Name) {
		return false
	}
	//把 event.Op 当成一个装了很多开关的面板，fsnotify.Write 是其中一个开关。event.Op & fsnotify.Write 就是在检查“Write 这个开关是不是开着”
	return event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create
}

func (fw *FileWatcher) handleTargetFileEvent(event fsnotify.Event) {
	if fw.shouldLogFileEvent(event.Name) {
		logger.Info("检测到目标文件变化: %s, 操作: %s", event.Name, event.Op.String())
	}
	fw.handleFileEvent(event.Name, event.Op)
}

func (fw *FileWatcher) handleCreatedPath(path string) {
	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		return
	}
	if fw.isExcludedPath(path) {
		return
	}
	if err := fw.addWatchRecursively(path); err != nil {
		logger.Warn("递归添加新目录监控失败: %s, 错误: %v", path, err)
		return
	}
	logger.Info("添加目录监控: %s", path)
}

func (fw *FileWatcher) addWatch(path string) (bool, error) {
	fw.watchMutex.Lock()
	defer fw.watchMutex.Unlock()
	if _, exists := fw.watchedDirs[path]; exists {
		return false, nil
	}
	if err := fw.watcher.Add(path); err != nil {
		return false, err
	}
	fw.watchedDirs[path] = struct{}{}
	return true, nil
}

func (fw *FileWatcher) removeAllWatches() {
	fw.watchMutex.Lock()
	for path := range fw.watchedDirs {
		_ = fw.watcher.Remove(path)
	}
	fw.watchedDirs = make(map[string]struct{})
	fw.watchMutex.Unlock()
}

func (fw *FileWatcher) removeWatchTree(root string) {
	root = filepath.Clean(root)
	sep := string(filepath.Separator)

	fw.watchMutex.Lock()
	if root == sep {
		for path := range fw.watchedDirs {
			_ = fw.watcher.Remove(path)
			delete(fw.watchedDirs, path)
		}
		fw.watchMutex.Unlock()
		return
	}
	prefix := root + sep
	for path := range fw.watchedDirs {
		if path == root || strings.HasPrefix(path, prefix) {
			_ = fw.watcher.Remove(path)
			delete(fw.watchedDirs, path)
		}
	}
	fw.watchMutex.Unlock()
}

func (fw *FileWatcher) isWatchedDir(path string) bool {
	fw.watchMutex.Lock()
	_, exists := fw.watchedDirs[path]
	fw.watchMutex.Unlock()
	return exists
}

// addWatchRecursively 递归监控指定目录及子目录的文件变化
func (fw *FileWatcher) addWatchRecursively(dirPath string) error {
	logger.Debug("递归添加目录监控: %s", dirPath)

	return filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path != dirPath {
				if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
					logger.Warn("跳过不存在的路径(可能是断链): %s, 错误: %v", path, err)
					return nil
				}
				if errors.Is(err, fs.ErrPermission) || os.IsPermission(err) {
					logger.Warn("跳过无权限访问的路径: %s, 错误: %v", path, err)
					return nil
				}
			}
			logger.Warn("遍历目录失败: %s, 错误: %v", path, err)
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if fw.isExcludedPath(path) {
			logger.Debug("跳过排除目录: %s", path)
			return fs.SkipDir
		}
		added, err := fw.addWatch(path)
		if err != nil {
			if isTooManyOpenFiles(err) {
				logger.Warn("监控句柄已达上限，停止递归监控: %s, 错误: %v", path, err)
				return errWatchLimitReached
			}
			if path != dirPath {
				if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
					logger.Warn("跳过不存在的目录(可能是断链): %s, 错误: %v", path, err)
					return nil
				}
				if errors.Is(err, fs.ErrPermission) || os.IsPermission(err) {
					logger.Warn("跳过无权限访问的目录: %s, 错误: %v", path, err)
					return nil
				}
			}
			logger.Warn("添加目录监控失败: %s, 错误: %v", path, err)
			return err
		}
		if added {
			logger.Debug("添加目录监控: %s", path)
		}
		return nil
	})
}

// 判断路径是否需要排除
func (fw *FileWatcher) isExcludedPath(path string) bool {
	if fw.exclude == nil {
		return false
	}
	return fw.exclude.IsExcluded(path)
}

func (fw *FileWatcher) addWatchRecursivelyForDirs(dirs []string) error {
	if len(dirs) == 0 {
		return errors.New("watch dir is empty")
	}
	var limitReached bool
	for _, dir := range dirs {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		if err := fw.addWatchRecursively(dir); err != nil {
			if errors.Is(err, errWatchLimitReached) {
				limitReached = true
				break
			}
			return err
		}
	}
	if limitReached {
		return errWatchLimitReached
	}
	return nil
}

// handleFileEvent 处理文件事件
func (fw *FileWatcher) handleFileEvent(filePath string, op fsnotify.Op) {
	logger.Debug("启动文件监听协程: %s, 操作: %s", filePath, op.String())

	// 更新文件写入时间并设置写入完成检测
	fw.updateFileWriteTime(filePath)
}

// updateFileWriteTime 更新文件写入时间并设置写入完成检测，避免边写边传导致传到半截文件
func (fw *FileWatcher) updateFileWriteTime(filePath string) {
	fw.stateMutex.Lock()
	defer fw.stateMutex.Unlock()

	now := time.Now()
	fw.lastWriteTime[filePath] = now

	// 取消之前的定时器（如果存在）
	if timer, exists := fw.writeTimers[filePath]; exists {
		timer.Stop()
	}
	// 如果 silenceWindow 内没有再收到这个文件的写事件，新定时器会触发 handleWriteComplete(filePath)
	fw.writeTimers[filePath] = time.AfterFunc(fw.silenceWindow, func() {
		fw.handleWriteComplete(filePath)
	})
}

func (fw *FileWatcher) handleWriteComplete(filePath string) {
	fw.stateMutex.Lock()
	lastWrite, ok := fw.lastWriteTime[filePath]
	if !ok {
		fw.stateMutex.Unlock()
		return
	}
	if time.Since(lastWrite) < fw.silenceWindow {
		fw.stateMutex.Unlock()
		return
	}

	//防止旧状态占内存或干扰后续同名文件的监控
	delete(fw.lastWriteTime, filePath)
	delete(fw.writeTimers, filePath)
	delete(fw.lastLogged, filePath)
	fw.stateMutex.Unlock()

	logger.Info("文件写入完成: %s (超过 %v 无新写入)", filePath, fw.silenceWindow)
	if err := fw.uploadPool.AddFile(filePath); err != nil {
		logger.Error("无法将文件添加到上传队列: %s, 错误: %v", filePath, err)
	}
}

// 避免删除/改名后的文件还占着内存或触发误操作
func (fw *FileWatcher) cleanupFileState(filePath string) {
	fw.stateMutex.Lock()
	if timer, exists := fw.writeTimers[filePath]; exists {
		timer.Stop()
		delete(fw.writeTimers, filePath)
	}
	delete(fw.lastWriteTime, filePath)
	delete(fw.lastLogged, filePath)
	fw.stateMutex.Unlock()
}

// shouldLogFileEvent 检查是否应该记录文件事件日志
func (fw *FileWatcher) shouldLogFileEvent(filePath string) bool {
	fw.stateMutex.Lock()
	defer fw.stateMutex.Unlock()
	// ok 为 true 表示这个 key 存在，false 表示不存在（第一次见这个文件）
	if lastTime, ok := fw.lastLogged[filePath]; !ok || time.Since(lastTime) > logThrottleDuration {
		fw.lastLogged[filePath] = time.Now()
		return true
	}
	return false
}

func parseSilenceWindow(raw string) time.Duration {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultSilenceWindow
	}
	clean := strings.ToLower(trimmed)
	clean = strings.ReplaceAll(clean, "静默", "")
	clean = strings.ReplaceAll(clean, "秒钟", "秒")
	clean = strings.ReplaceAll(clean, "秒", "s")
	clean = strings.TrimSpace(clean)

	if d, err := time.ParseDuration(clean); err == nil && d > 0 {
		return d
	}

	numRe := regexp.MustCompile(`\\d+`)
	if m := numRe.FindString(clean); m != "" {
		if v, err := strconv.Atoi(m); err == nil && v > 0 {
			return time.Duration(v) * time.Second
		}
	}

	logger.Warn("静默窗口解析失败，使用默认值: %s", trimmed)
	return defaultSilenceWindow
}

func isTooManyOpenFiles(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EMFILE) || errors.Is(err, syscall.ENFILE) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "too many open files")
}
