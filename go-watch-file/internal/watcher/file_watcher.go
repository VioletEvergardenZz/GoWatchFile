package watcher

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"file-watch/internal/logger"
	"file-watch/internal/models"
)

const (
	// throttleDuration     = 120 * time.Second
	logThrottleDuration  = 5 * time.Second  // 日志节流时间间隔
	writeCompleteTimeout = 10 * time.Second // 文件写入完成检测超时时间
)

// FileWatcher 文件监控器
type FileWatcher struct {
	watcher        *fsnotify.Watcher //实际的文件监听器对象
	config         *models.Config
	uploadPool     UploadPool
	stateMutex     sync.Mutex
	lastLogged     map[string]time.Time
	lastWriteTime  map[string]time.Time
	writeTimers    map[string]*time.Timer
	activeMonitors map[string]bool // 防止重复的 monitorFileSize 协程
}

// UploadPool 上传池接口
type UploadPool interface {
	AddFile(filePath string) bool
}

// NewFileWatcher 创建新的文件监控器
func NewFileWatcher(config *models.Config, uploadPool UploadPool) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &FileWatcher{
		watcher:        watcher,
		config:         config,
		uploadPool:     uploadPool,
		lastLogged:     make(map[string]time.Time),
		lastWriteTime:  make(map[string]time.Time),
		writeTimers:    make(map[string]*time.Timer),
		activeMonitors: make(map[string]bool),
	}, nil
}

// Start 启动文件监控
func (fw *FileWatcher) Start() error {
	logger.Info("初始化文件监控器...")
	logger.Info("开始监控目录: %s", fw.config.WatchDir)

	//递归把目录都加入监听
	err := fw.addWatchRecursively(fw.config.WatchDir)
	if err != nil {
		logger.Error("添加目录监控失败: %v", err)
		return err
	}

	// 启动事件处理协程
	go fw.handleEvents()

	logger.Info("文件监控服务启动成功，等待文件变化...")
	return nil
}

// Close 关闭文件监控器
func (fw *FileWatcher) Close() error {
	// 停止并清理所有写入完成检测定时器
	fw.stateMutex.Lock()
	for _, t := range fw.writeTimers {
		if t != nil {
			t.Stop()
		}
	}
	fw.writeTimers = make(map[string]*time.Timer)
	fw.stateMutex.Unlock()

	return fw.watcher.Close()
}

// handleEvents 处理文件事件
func (fw *FileWatcher) handleEvents() {
	for {
		select {
		case event := <-fw.watcher.Events:
			fw.handleEvent(event)
		case err := <-fw.watcher.Errors:
			logger.Error("文件监控错误: %v", err)
		}
	}
}

func (fw *FileWatcher) handleEvent(event fsnotify.Event) {
	logger.Debug("收到文件事件: %s, 操作: %s", event.Name, event.Op.String())

	if fw.isTargetFileEvent(event) {
		fw.handleTargetFileEvent(event)
	}

	if event.Op&fsnotify.Create == fsnotify.Create {
		fw.handleCreatedPath(event.Name)
	}
}

func (fw *FileWatcher) isTargetFileEvent(event fsnotify.Event) bool {
	if filepath.Ext(event.Name) != fw.config.FileExt {
		return false
	}
	return event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create
}

func (fw *FileWatcher) handleTargetFileEvent(event fsnotify.Event) {
	if fw.shouldLogFileEvent(event.Name) {
		logger.Info("检测到目标文件变化: %s, 操作: %s", event.Name, event.Op.String())
	}
	go fw.handleFileEvent(event.Name, event.Op)
}

func (fw *FileWatcher) handleCreatedPath(path string) {
	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		return
	}

	if err := fw.watcher.Add(path); err != nil {
		logger.Warn("添加目录监控失败: %s, 错误: %v", path, err)
	} else {
		logger.Info("添加目录监控: %s", path)
	}
	if err := fw.addWatchRecursively(path); err != nil {
		logger.Warn("递归添加新目录监控失败: %s, 错误: %v", path, err)
	}
}

// addWatchRecursively 递归监控指定目录及子目录的文件变化
func (fw *FileWatcher) addWatchRecursively(dirPath string) error {
	logger.Debug("递归添加目录监控: %s", dirPath)

	return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Warn("遍历目录失败: %s, 错误: %v", path, err)
			return err
		}
		if !info.IsDir() {
			return nil
		}
		if err := fw.watcher.Add(path); err != nil {
			logger.Warn("添加目录监控失败: %s, 错误: %v", path, err)
			return err
		}
		logger.Debug("添加目录监控: %s", path)
		return fw.addSubDirectories(path)
	})
}

func (fw *FileWatcher) addSubDirectories(dirPath string) error {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		logger.Warn("读取目录内容失败: %s, 错误: %v", dirPath, err)
		return err
	}
	for _, file := range files {
		if file.IsDir() {
			_ = fw.addWatchRecursively(filepath.Join(dirPath, file.Name()))
		}
	}
	return nil
}

// handleFileEvent 处理文件事件
func (fw *FileWatcher) handleFileEvent(filePath string, op fsnotify.Op) {
	logger.Debug("启动文件监听协程: %s, 操作: %s", filePath, op.String())

	// 更新文件写入时间并设置写入完成检测
	fw.updateFileWriteTime(filePath)

	// 如果是创建事件，启动文件大小监控协程（避免重复启动）
	if op&fsnotify.Create == fsnotify.Create {
		fw.stateMutex.Lock()
		if fw.activeMonitors[filePath] {
			fw.stateMutex.Unlock()
			return
		}
		fw.activeMonitors[filePath] = true
		fw.stateMutex.Unlock()
		go fw.monitorFileSize(filePath)
	}
}

// updateFileWriteTime 更新文件写入时间并设置写入完成检测
func (fw *FileWatcher) updateFileWriteTime(filePath string) {
	fw.stateMutex.Lock()
	defer fw.stateMutex.Unlock()

	now := time.Now()
	fw.lastWriteTime[filePath] = now

	// 取消之前的定时器（如果存在）
	if timer, exists := fw.writeTimers[filePath]; exists {
		timer.Stop()
	}

	fw.writeTimers[filePath] = time.AfterFunc(writeCompleteTimeout, func() {
		fw.handleWriteComplete(filePath)
	})
}

func (fw *FileWatcher) handleWriteComplete(filePath string) {
	fw.stateMutex.Lock()
	defer fw.stateMutex.Unlock()

	lastWrite, ok := fw.lastWriteTime[filePath]
	if !ok {
		return
	}
	if time.Since(lastWrite) < writeCompleteTimeout {
		return
	}

	logger.Info("文件写入完成: %s (超过 %v 无新写入)", filePath, writeCompleteTimeout)
	delete(fw.lastWriteTime, filePath)
	delete(fw.writeTimers, filePath)
	delete(fw.lastLogged, filePath)
	if !fw.uploadPool.AddFile(filePath) {
		logger.Error("无法将文件添加到上传队列: %s", filePath)
	}
}

// shouldLogFileEvent 检查是否应该记录文件事件日志
func (fw *FileWatcher) shouldLogFileEvent(filePath string) bool {
	fw.stateMutex.Lock()
	defer fw.stateMutex.Unlock()

	if lastTime, ok := fw.lastLogged[filePath]; !ok || time.Since(lastTime) > logThrottleDuration {
		fw.lastLogged[filePath] = time.Now()
		return true
	}
	return false
}

// monitorFileSize 监控文件大小变化
func (fw *FileWatcher) monitorFileSize(filePath string) {
	logger.Debug("开始监控文件大小: %s", filePath)

	// 结束时清理 activeMonitors 标志
	defer func() {
		fw.stateMutex.Lock()
		delete(fw.activeMonitors, filePath)
		fw.stateMutex.Unlock()
	}()

	// 创建一个定时器，定期检查文件大小
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var lastSize int64 = 0
	stableCount := 0
	maxStableCount := 5 // 连续5次大小不变认为文件稳定

	for {
		select {
		case <-ticker.C:
			// 检查文件是否存在
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				logger.Debug("文件不存在，停止监控: %s", filePath)
				return
			}

			// 获取文件大小
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				logger.Warn("获取文件信息失败: %s, 错误: %v", filePath, err)
				continue
			}

			currentSize := fileInfo.Size()

			// 如果文件大小没有变化
			if currentSize == lastSize {
				stableCount++
				if stableCount >= maxStableCount {
					logger.Info("文件大小稳定，停止监控: %s, 最终大小: %d bytes", filePath, currentSize)
					return
				}
			} else {
				// 文件大小有变化，重置计数器
				stableCount = 0
				lastSize = currentSize
				logger.Debug("文件大小变化: %s, 当前大小: %d bytes", filePath, currentSize)
			}
		}
	}
}
