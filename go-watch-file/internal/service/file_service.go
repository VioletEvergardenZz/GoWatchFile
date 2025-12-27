package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"file-watch/internal/dingtalk"
	"file-watch/internal/logger"
	"file-watch/internal/models"
	"file-watch/internal/pathutil"
	"file-watch/internal/s3"
	"file-watch/internal/state"
	"file-watch/internal/upload"
	"file-watch/internal/watcher"
	"file-watch/internal/wechat"
)

// FileService 负责协调文件监控、上传与通知流程。
type FileService struct {
	config        *models.Config
	s3Client      *s3.Client
	wechatRobot   *wechat.Robot
	dingtalkRobot *dingtalk.Robot
	uploadPool    *upload.WorkerPool
	watcher       *watcher.FileWatcher
	state         *state.RuntimeState
	mu            sync.Mutex
	manualOnce    map[string]bool
}

const shutdownTimeout = 30 * time.Second

// NewFileService 构造并初始化 FileService 的依赖。
func NewFileService(config *models.Config) (*FileService, error) {
	runtimeState := state.NewRuntimeState(config)
	if err := runtimeState.BootstrapExisting(); err != nil {
		logger.Warn("预加载历史文件失败: %v", err)
	}

	s3Client, err := newS3Client(config)
	if err != nil {
		return nil, err
	}

	fileService := &FileService{
		config:        config,
		s3Client:      s3Client,
		wechatRobot:   newWeChatRobot(config),
		dingtalkRobot: newDingTalkRobot(config),
		state:         runtimeState,
		manualOnce:    make(map[string]bool),
	}

	uploadPool, err := newUploadPool(config, fileService.processFile)
	if err != nil {
		return nil, err
	}
	fileService.uploadPool = uploadPool
	runtimeState.SetQueueStats(uploadPool.GetStats())

	fileWatcher, err := newFileWatcher(config, fileService)
	if err != nil {
		return nil, err
	}
	fileService.watcher = fileWatcher

	return fileService, nil
}

func newS3Client(config *models.Config) (*s3.Client, error) {
	client, err := s3.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("初始化S3客户端失败: %w", err)
	}
	return client, nil
}

func newWeChatRobot(config *models.Config) *wechat.Robot {
	if config.RobotKey == "" {
		return nil
	}
	return wechat.NewRobot(config.RobotKey)
}

func newDingTalkRobot(config *models.Config) *dingtalk.Robot {
	if config.DingTalkWebhook == "" {
		return nil
	}
	return dingtalk.NewRobot(config.DingTalkWebhook, config.DingTalkSecret)
}

func newUploadPool(config *models.Config, handler func(context.Context, string) error) (*upload.WorkerPool, error) {
	return upload.NewWorkerPool(config.UploadWorkers, config.UploadQueueSize, handler)
}

func newFileWatcher(config *models.Config, uploadPool watcher.UploadPool) (*watcher.FileWatcher, error) {
	fileWatcher, err := watcher.NewFileWatcher(config, uploadPool)
	if err != nil {
		return nil, fmt.Errorf("初始化文件监控器失败: %w", err)
	}
	return fileWatcher, nil
}

// Config returns a copy of current config.
func (fs *FileService) Config() *models.Config {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.config == nil {
		return nil
	}
	cfgCopy := *fs.config
	return &cfgCopy
}

// UpdateConfig applies config changes at runtime and restarts watcher/pool.
func (fs *FileService) UpdateConfig(watchDir, fileExt string, uploadWorkers, uploadQueueSize int) (*models.Config, error) {
	fs.mu.Lock()
	if fs.config == nil {
		fs.mu.Unlock()
		return nil, fmt.Errorf("配置未初始化")
	}
	current := *fs.config
	fs.mu.Unlock()

	updated := current

	if strings.TrimSpace(watchDir) != "" && strings.TrimSpace(watchDir) != current.WatchDir {
		normalized, err := normalizeWatchDir(strings.TrimSpace(watchDir))
		if err != nil {
			return nil, err
		}
		updated.WatchDir = normalized
	}
	if strings.TrimSpace(fileExt) != current.FileExt {
		if err := validateFileExt(strings.TrimSpace(fileExt)); err != nil {
			return nil, err
		}
		updated.FileExt = strings.TrimSpace(fileExt)
	}
	if uploadWorkers > 0 && uploadWorkers != current.UploadWorkers {
		updated.UploadWorkers = uploadWorkers
	}
	if uploadQueueSize > 0 && uploadQueueSize != current.UploadQueueSize {
		updated.UploadQueueSize = uploadQueueSize
	}

	newState := state.NewRuntimeState(&updated)
	if err := newState.BootstrapExisting(); err != nil {
		logger.Warn("预加载历史文件失败: %v", err)
	}

	newS3, err := newS3Client(&updated)
	if err != nil {
		return nil, err
	}

	newPool, err := newUploadPool(&updated, fs.processFile)
	if err != nil {
		return nil, err
	}
	newWatcher, err := newFileWatcher(&updated, fs)
	if err != nil {
		_ = newPool.ShutdownGraceful(shutdownTimeout)
		return nil, err
	}

	fs.mu.Lock()
	oldCfg := fs.config
	oldState := fs.state
	oldWatcher := fs.watcher
	oldPool := fs.uploadPool
	oldS3 := fs.s3Client

	fs.config = &updated
	fs.state = newState
	fs.uploadPool = newPool
	fs.watcher = newWatcher
	fs.s3Client = newS3
	fs.state.SetQueueStats(fs.uploadPool.GetStats())
	fs.mu.Unlock()

	// Start new watcher with updated state
	if err := newWatcher.Start(); err != nil {
		_ = newPool.ShutdownGraceful(shutdownTimeout)
		_ = newWatcher.Close()
		// 尝试恢复旧配置，避免服务不可用
		if oldCfg != nil {
			if restoredPool, perr := newUploadPool(oldCfg, fs.processFile); perr == nil {
				if restoredWatcher, werr := newFileWatcher(oldCfg, fs); werr == nil {
					if serr := restoredWatcher.Start(); serr == nil {
						fs.mu.Lock()
						fs.config = oldCfg
						fs.state = oldState
						fs.uploadPool = restoredPool
						fs.watcher = restoredWatcher
						if fs.state != nil {
							fs.state.SetQueueStats(fs.uploadPool.GetStats())
						}
						fs.mu.Unlock()
					}
				}
			}
		}
		return nil, err
	}

	// shutdown old components after new watcher is up
	if oldWatcher != nil {
		_ = oldWatcher.Close()
	}
	if oldPool != nil {
		_ = oldPool.ShutdownGraceful(shutdownTimeout)
	}
	if oldS3 != nil {
		// aws sdk 无需显式关闭客户端，保留占位以示释放顺序
	}

	return fs.Config(), nil
}

func normalizeWatchDir(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("监控目录不能为空")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("监控目录无效: %w", err)
	}
	stat, statErr := os.Stat(absPath)
	if statErr != nil {
		return "", fmt.Errorf("监控目录无效: %w", statErr)
	}
	if !stat.IsDir() {
		return "", fmt.Errorf("监控目录不是一个目录")
	}
	return absPath, nil
}

func validateFileExt(ext string) error {
	if strings.TrimSpace(ext) == "" {
		// 允许空字符串，表示不过滤后缀
		return nil
	}
	if !strings.HasPrefix(ext, ".") || ext == "." {
		return fmt.Errorf("文件后缀必须以 '.' 开头，例如 .log")
	}
	return nil
}

// Start 启动文件服务。
func (fs *FileService) Start() error {
	logger.Info("启动文件服务...")
	if err := fs.watcher.Start(); err != nil {
		return fmt.Errorf("启动文件监控失败: %w", err)
	}
	logger.Info("文件服务启动成功")
	return nil
}

// Stop 停止文件服务。
func (fs *FileService) Stop() error {
	logger.Info("停止文件服务...")
	if fs.uploadPool != nil {
		if err := fs.uploadPool.ShutdownGraceful(shutdownTimeout); err != nil {
			logger.Warn("关闭上传工作池超时，已发出取消信号: %v", err)
		}
	}
	if fs.watcher != nil {
		if err := fs.watcher.Close(); err != nil {
			logger.Error("关闭文件监控器失败: %v", err)
		}
	}
	logger.Info("文件服务已停止")
	return nil
}

// processFile 处理单个文件：上传、触发构建、发送通知。
func (fs *FileService) processFile(ctx context.Context, filePath string) error {
	start := time.Now()
	manual := fs.consumeManualOnce(filePath)
	if fs.state != nil && !manual && !fs.state.AutoUploadEnabled(filePath) {
		fs.state.MarkSkipped(filePath)
		return nil
	}

	logger.Info("开始处理文件: %s", filePath)
	downloadURL, err := fs.s3Client.UploadFile(ctx, filePath)
	if err != nil {
		if fs.state != nil {
			fs.state.MarkFailed(filePath, err)
			fs.state.SetQueueStats(fs.uploadPool.GetStats())
		}
		return fmt.Errorf("上传文件到S3失败: %w", err)
	}

	appName, fileName := fs.parseFileInfo(filePath)
	logger.Info("文件信息 - 应用名: %s, 文件名: %s", appName, fileName)

	if fs.state != nil {
		fs.state.MarkUploaded(filePath, fs.config.Bucket, time.Since(start))
		fs.state.SetQueueStats(fs.uploadPool.GetStats())
	}

	fs.sendWeChat(ctx, downloadURL, appName)
	dingAppName := appName
	dingFileName := fileName
	if filePath != "" {
		if parentDir := filepath.Base(filepath.Dir(filePath)); parentDir != "." && parentDir != string(filepath.Separator) {
			dingAppName = parentDir
		}
		if baseName := filepath.Base(filePath); baseName != "." && baseName != string(filepath.Separator) {
			dingFileName = baseName
		}
	}
	fs.sendDingTalk(ctx, downloadURL, dingAppName, dingFileName)

	logger.Info("文件处理完成: %s", filePath)
	return nil
}

func (fs *FileService) parseFileInfo(filePath string) (string, string) {
	appName, fileName, err := pathutil.ParseAppAndFileName(fs.config.WatchDir, filePath)
	if err != nil {
		logger.Error("解析文件信息失败: %v", err)
		// 解析失败时仍继续流程，避免阻塞后续处理。
		return "unknown", "unknown"
	}
	return appName, fileName
}

func (fs *FileService) consumeManualOnce(path string) bool {
	norm := normalizeManualPath(path)
	if norm == "" {
		return false
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.manualOnce[norm] {
		delete(fs.manualOnce, norm)
		return true
	}
	return false
}

func normalizeManualPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

func (fs *FileService) sendWeChat(ctx context.Context, downloadURL, appName string) {
	if fs.wechatRobot == nil {
		return
	}
	if err := fs.wechatRobot.SendMessage(ctx, downloadURL, appName); err != nil {
		logger.Error("发送企业微信消息失败: %v", err)
	}
}

func (fs *FileService) sendDingTalk(ctx context.Context, downloadURL, appName, fileName string) {
	if fs.dingtalkRobot == nil {
		return
	}
	if err := fs.dingtalkRobot.SendMessage(ctx, downloadURL, appName, fileName); err != nil {
		logger.Error("发送钉钉消息失败: %v", err)
	}
}

// GetStats 获取服务统计信息。
func (fs *FileService) GetStats() models.UploadStats {
	if fs.uploadPool != nil {
		return fs.uploadPool.GetStats()
	}
	return models.UploadStats{}
}

// State exposes runtime state for API server.
func (fs *FileService) State() *state.RuntimeState {
	return fs.state
}

// AddFile implements watcher.UploadPool to enqueue a file detected by watcher.
func (fs *FileService) AddFile(filePath string) error {
	return fs.enqueueFile(filePath, false)
}

// EnqueueManualUpload allows API-triggered manual uploads.
func (fs *FileService) EnqueueManualUpload(filePath string) error {
	return fs.enqueueFile(filePath, true)
}

func (fs *FileService) enqueueFile(filePath string, manual bool) error {
	norm := normalizeManualPath(filePath)
	if fs.state != nil {
		if manual {
			fs.state.MarkManualQueued(filePath)
			if norm != "" {
				fs.mu.Lock()
				fs.manualOnce[norm] = true
				fs.mu.Unlock()
			}
		} else if !fs.state.AutoUploadEnabled(filePath) {
			fs.state.MarkSkipped(filePath)
			return nil
		} else {
			fs.state.MarkQueued(filePath)
		}
	}
	if fs.uploadPool == nil {
		return fmt.Errorf("上传工作池未初始化")
	}
	if err := fs.uploadPool.AddFile(filePath); err != nil {
		if fs.state != nil {
			fs.state.MarkFailed(filePath, err)
			fs.state.SetQueueStats(fs.uploadPool.GetStats())
		}
		return err
	}
	if fs.state != nil {
		fs.state.SetQueueStats(fs.uploadPool.GetStats())
	}
	return nil
}
