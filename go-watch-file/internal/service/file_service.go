// 本文件用于文件监控服务的核心协作流程
package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"file-watch/internal/alert"
	"file-watch/internal/dingtalk"
	"file-watch/internal/email"
	"file-watch/internal/logger"
	"file-watch/internal/match"
	"file-watch/internal/models"
	"file-watch/internal/pathutil"
	"file-watch/internal/s3"
	"file-watch/internal/state"
	"file-watch/internal/upload"
	"file-watch/internal/watcher"
)

// FileService 负责协调文件监控、上传与通知流程
type FileService struct {
	config        *models.Config
	s3Client      *s3.Client
	dingtalkRobot *dingtalk.Robot
	emailSender   *email.Sender
	uploadPool    *upload.WorkerPool
	watcher       *watcher.FileWatcher
	alertManager  *alert.Manager
	state         *state.RuntimeState
	running       bool
	mu            sync.Mutex      //互斥锁，用来保护 FileService 内部共享数据的并发读写
	manualOnce    map[string]bool //标记“某个路径的下一次处理是手动上传”
}

const shutdownTimeout = 30 * time.Second

// NewFileService 构造并初始化 FileService 的依赖
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
		dingtalkRobot: newDingTalkRobot(config),
		emailSender:   newEmailSender(config),
		state:         runtimeState,
		manualOnce:    make(map[string]bool),
	}
	// 初始化告警管理器并复用通知器
	alertManager, err := alert.NewManager(config, &alert.NotifierSet{
		DingTalk: fileService.dingtalkRobot,
		Email:    fileService.emailSender,
	})
	if err != nil {
		return nil, err
	}
	fileService.alertManager = alertManager

	uploadPool, err := newUploadPool(config, fileService.processFile, fileService.handlePoolStats)
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

// newS3Client 初始化 S3 客户端
func newS3Client(config *models.Config) (*s3.Client, error) {
	client, err := s3.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("初始化S3客户端失败: %w", err)
	}
	return client, nil
}

// newDingTalkRobot 根据配置创建钉钉机器人
func newDingTalkRobot(config *models.Config) *dingtalk.Robot {
	if config.DingTalkWebhook == "" {
		return nil
	}
	return dingtalk.NewRobot(config.DingTalkWebhook, config.DingTalkSecret)
}

// newEmailSender 根据配置创建邮件发送器
func newEmailSender(config *models.Config) *email.Sender {
	// 读取 SMTP 主机配置
	host := strings.TrimSpace(config.EmailHost)
	if host == "" {
		// 未配置则不启用邮件通知
		return nil
	}

	// 解析收件人列表
	recipients := parseEmailRecipients(config.EmailTo)
	if len(recipients) == 0 {
		// 无收件人则禁用
		logger.Warn("邮件通知未启用: email_to 为空")
		return nil
	}

	// 优先使用配置的 From
	from := strings.TrimSpace(config.EmailFrom)
	if from == "" && strings.Contains(config.EmailUser, "@") {
		// 若未设置 From 则退回到账号作为发件人
		from = strings.TrimSpace(config.EmailUser)
	}
	if from == "" {
		// 仍为空则不启用
		logger.Warn("邮件通知未启用: email_from 为空")
		return nil
	}

	// 读取端口与 TLS 配置
	port := config.EmailPort
	useTLS := config.EmailUseTLS
	if port <= 0 {
		// 未指定端口时根据 TLS 选择默认值
		if useTLS {
			port = 587
		} else {
			port = 25
		}
	}
	if port == 465 {
		// 465 端口强制启用 TLS
		useTLS = true
	}
	if port <= 0 || port > 65535 {
		// 端口非法则不启用
		logger.Warn("邮件通知未启用: email_port 无效")
		return nil
	}

	// 生成 SMTP 发送器
	return email.NewSender(host, port, config.EmailUser, config.EmailPass, from, recipients, useTLS)
}

// parseEmailRecipients 解析收件人列表
func parseEmailRecipients(raw string) []string {
	// 支持逗号分号空白等分隔符
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	// 过滤空项并保留顺序
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		// 逐项清理空格
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

// newUploadPool 创建上传工作池
func newUploadPool(config *models.Config, handler func(context.Context, string) error, onStats func(models.UploadStats)) (*upload.WorkerPool, error) {
	return upload.NewWorkerPool(config.UploadWorkers, config.UploadQueueSize, handler, onStats)
}

// handlePoolStats 将队列统计写入运行态
func (fs *FileService) handlePoolStats(stats models.UploadStats) {
	if fs.state != nil {
		fs.state.SetQueueStats(stats)
	}
}

// newFileWatcher 创建文件监听器
func newFileWatcher(config *models.Config, uploadPool watcher.UploadPool) (*watcher.FileWatcher, error) {
	fileWatcher, err := watcher.NewFileWatcher(config, uploadPool)
	if err != nil {
		return nil, fmt.Errorf("初始化文件监控器失败: %w", err)
	}
	return fileWatcher, nil
}

// Config 返回当前配置的副本
func (fs *FileService) Config() *models.Config {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.config == nil {
		return nil
	}
	cfgCopy := *fs.config
	return &cfgCopy
}

// UpdateConfig 运行时更新配置并重建 watcher 和上传池
func (fs *FileService) UpdateConfig(watchDir, fileExt, silence string, uploadWorkers, uploadQueueSize int) (*models.Config, error) {
	// 先加锁读取当前配置与组件引用
	fs.mu.Lock()
	if fs.config == nil {
		// 配置不存在时直接返回
		fs.mu.Unlock()
		return nil, fmt.Errorf("配置未初始化")
	}
	// 保存旧组件用于失败回滚
	oldCfg := fs.config
	oldState := fs.state
	oldWatcher := fs.watcher
	oldPool := fs.uploadPool
	oldS3 := fs.s3Client
	// 复制旧配置作为更新基线
	current := *oldCfg
	fs.mu.Unlock()

	// 基于当前配置构造更新版
	updated := current

	// 处理 watchDir 更新并做校验
	if strings.TrimSpace(watchDir) != "" && strings.TrimSpace(watchDir) != current.WatchDir {
		normalized, err := normalizeWatchDirs(strings.TrimSpace(watchDir))
		if err != nil {
			return nil, err
		}
		updated.WatchDir = normalized
	}
	// 处理 fileExt 更新并做校验
	if strings.TrimSpace(fileExt) != current.FileExt {
		if err := validateFileExt(strings.TrimSpace(fileExt)); err != nil {
			return nil, err
		}
		updated.FileExt = strings.TrimSpace(fileExt)
	}
	// 处理静默窗口更新
	if strings.TrimSpace(silence) != "" && strings.TrimSpace(silence) != current.Silence {
		updated.Silence = strings.TrimSpace(silence)
	}
	// 处理上传 worker 数量更新
	if uploadWorkers > 0 && uploadWorkers != current.UploadWorkers {
		updated.UploadWorkers = uploadWorkers
	}
	// 处理上传队列长度更新
	if uploadQueueSize > 0 && uploadQueueSize != current.UploadQueueSize {
		updated.UploadQueueSize = uploadQueueSize
	}

	// 基于新配置初始化运行态
	newState := state.NewRuntimeState(&updated)
	if err := newState.BootstrapExisting(); err != nil {
		logger.Warn("预加载历史文件失败: %v", err)
	}
	// 迁移旧的计数与历史数据
	newState.CarryOverFrom(oldState)

	// 初始化新的 S3 客户端
	newS3, err := newS3Client(&updated)
	if err != nil {
		return nil, err
	}

	// 初始化新的上传工作池
	newPool, err := newUploadPool(&updated, fs.processFile, fs.handlePoolStats)
	if err != nil {
		return nil, err
	}

	// watcher 为空时新建实例
	activeWatcher := oldWatcher
	if activeWatcher == nil {
		created, createErr := newFileWatcher(&updated, fs)
		if createErr != nil {
			_ = newPool.ShutdownGraceful(shutdownTimeout)
			return nil, createErr
		}
		activeWatcher = created
	}

	// 切换到新配置与新组件
	fs.mu.Lock()
	fs.config = &updated
	fs.state = newState
	fs.uploadPool = newPool
	fs.watcher = activeWatcher
	fs.s3Client = newS3
	fs.state.SetQueueStats(fs.uploadPool.GetStats())
	fs.mu.Unlock()

	if oldWatcher == nil {
		// 使用新配置启动 watcher
		if err := activeWatcher.Start(); err != nil {
			// watcher 启动失败则回滚
			_ = newPool.ShutdownGraceful(shutdownTimeout)
			_ = activeWatcher.Close()
			// 回滚到旧配置，避免留下已关闭的工作池
			fs.mu.Lock()
			fs.config = oldCfg
			fs.state = oldState
			fs.uploadPool = oldPool
			fs.watcher = oldWatcher
			fs.s3Client = oldS3
			if fs.state != nil && fs.uploadPool != nil {
				fs.state.SetQueueStats(fs.uploadPool.GetStats())
			}
			fs.mu.Unlock()
			return nil, err
		}
	} else {
		// 已存在 watcher 则按新配置重置
		if err := activeWatcher.Reset(&updated); err != nil {
			// reset 失败则回滚
			_ = newPool.ShutdownGraceful(shutdownTimeout)
			fs.mu.Lock()
			fs.config = oldCfg
			fs.state = oldState
			fs.uploadPool = oldPool
			fs.watcher = oldWatcher
			fs.s3Client = oldS3
			if fs.state != nil && fs.uploadPool != nil {
				fs.state.SetQueueStats(fs.uploadPool.GetStats())
			}
			fs.mu.Unlock()
			return nil, err
		}
	}

	// 新 watcher 启动后关闭旧组件
	if oldPool != nil {
		_ = oldPool.ShutdownGraceful(shutdownTimeout)
	}
	if oldS3 != nil {
		// aws sdk 无需显式关闭客户端，保留占位以示释放顺序
	}

	logger.Info("运行时配置已更新: watchDir=%s, fileExt=%s, silence=%s, workers=%d, queue=%d",
		updated.WatchDir,
		updated.FileExt,
		updated.Silence,
		updated.UploadWorkers,
		updated.UploadQueueSize,
	)

	return fs.Config(), nil
}

// normalizeWatchDirs 校验并规范化监控目录列表
func normalizeWatchDirs(raw string) (string, error) {
	dirs := pathutil.SplitWatchDirs(raw)
	if len(dirs) == 0 {
		return "", fmt.Errorf("监控目录不能为空")
	}
	normalized := make([]string, 0, len(dirs))
	seen := make(map[string]struct{})
	for _, dir := range dirs {
		absPath, err := filepath.Abs(dir)
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
		key := normalizeWatchDirKey(absPath)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, absPath)
	}
	return strings.Join(normalized, ","), nil
}

func normalizeWatchDirKey(path string) string {
	key := filepath.ToSlash(path)
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	return key
}

// validateFileExt 校验文件后缀格式
func validateFileExt(ext string) error {
	if strings.TrimSpace(ext) == "" {
		// 允许空字符串，表示不过滤后缀
		return nil
	}
	// 复用多后缀解析进行格式校验
	if _, err := match.ParseExtList(strings.TrimSpace(ext)); err != nil {
		return err
	}
	return nil
}

// Start 启动文件服务
func (fs *FileService) Start() error {
	logger.Info("启动文件服务...")
	// 先启动文件监听 再启动告警轮询
	if err := fs.watcher.Start(); err != nil {
		return fmt.Errorf("启动文件监控失败: %w", err)
	}
	if fs.alertManager != nil {
		fs.alertManager.Start()
	}
	fs.mu.Lock()
	fs.running = true
	fs.mu.Unlock()
	logger.Info("文件服务启动成功")
	return nil
}

// Stop 停止文件服务
func (fs *FileService) Stop() error {
	logger.Info("停止文件服务...")
	fs.mu.Lock()
	fs.running = false
	fs.mu.Unlock()
	if fs.alertManager != nil {
		fs.alertManager.Stop()
	}
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

// processFile 处理单个文件：上传、触发构建、发送通知
func (fs *FileService) processFile(ctx context.Context, filePath string) error {
	start := time.Now()
	manual := fs.consumeManualOnce(filePath)
	// 手动上传不受自动开关限制
	if fs.state != nil && !manual && !fs.state.AutoUploadEnabled(filePath) {
		fs.state.MarkSkipped(filePath)
		return nil
	}

	logger.Info("开始处理文件: %s", filePath)
	downloadURL, err := fs.s3Client.UploadFile(ctx, filePath)
	if err != nil {
		if fs.state != nil {
			fs.state.MarkFailed(filePath, err)
		}
		return fmt.Errorf("上传文件到S3失败: %w", err)
	}

	fileName := filepath.Base(filePath)
	logger.Info("文件信息 - 文件名: %s", fileName)

	if fs.state != nil {
		fs.state.MarkUploaded(filePath, downloadURL, time.Since(start), manual)
	}

	fullPath := filepath.Clean(filePath)
	fs.sendDingTalk(ctx, downloadURL, fullPath)
	fs.sendEmailNotification(ctx, downloadURL, fullPath)

	logger.Info("文件处理完成: %s", filePath)
	return nil
}

// consumeManualOnce 消费单次手动上传标记
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

// normalizeManualPath 归一化手动上传路径
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

// sendDingTalk 发送钉钉通知
func (fs *FileService) sendDingTalk(ctx context.Context, downloadURL, fileName string) {
	if fs.dingtalkRobot == nil {
		return
	}
	if err := fs.dingtalkRobot.SendMessage(ctx, downloadURL, fileName); err != nil {
		logger.Error("发送钉钉消息失败: %v", err)
		return
	}
	if fs.state != nil {
		fs.state.RecordNotification("dingtalk")
	}
}

// sendEmailNotification 发送邮件通知
func (fs *FileService) sendEmailNotification(ctx context.Context, downloadURL, filePath string) {
	// 未配置邮件发送器则跳过
	if fs.emailSender == nil {
		return
	}
	// 读取主机名用于邮件内容
	host, _ := os.Hostname()
	// 邮件主题与内容与钉钉保持一致
	subject := "File uploaded"
	body := fmt.Sprintf(
		"Time: %s\nHost: %s\nFile: %s\nDownload: %s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		host,
		filepath.Clean(filePath),
		downloadURL,
	)
	// 发送邮件通知
	if err := fs.emailSender.SendMessage(ctx, subject, body); err != nil {
		// QUIT 异常视为已发送但连接结束异常
		if email.IsQuitError(err) {
			logger.Warn("邮件通知已发送但连接退出异常: %v", err)
			if fs.state != nil {
				// 仍记录通知次数
				fs.state.RecordNotification("email")
			}
			return
		}
		// 非 QUIT 异常记为发送失败
		logger.Error("发送邮件通知失败: %v", err)
		return
	}
	if fs.state != nil {
		// 发送成功记录通知次数
		fs.state.RecordNotification("email")
	}
}

// GetStats 获取服务统计信息
func (fs *FileService) GetStats() models.UploadStats {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.uploadPool != nil {
		return fs.uploadPool.GetStats()
	}
	return models.UploadStats{}
}

// State 暴露运行态给 API 服务
func (fs *FileService) State() *state.RuntimeState {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.state
}

// AlertState 暴露告警运行态给 API 服务
func (fs *FileService) AlertState() *alert.State {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.alertManager == nil {
		return nil
	}
	return fs.alertManager.State()
}

// AlertEnabled 返回告警是否启用
func (fs *FileService) AlertEnabled() bool {
	fs.mu.Lock()
	manager := fs.alertManager
	cfg := fs.config
	fs.mu.Unlock()
	if manager != nil {
		return manager.Enabled()
	}
	if cfg == nil {
		return false
	}
	return cfg.AlertEnabled
}

// UpdateAlertConfig 运行时更新告警配置（仅内存）
func (fs *FileService) UpdateAlertConfig(enabled bool, suppressEnabled bool, rulesFile, logPaths, pollInterval string, startFromEnd bool) (*models.Config, error) {
	fs.mu.Lock()
	if fs.config == nil {
		fs.mu.Unlock()
		return nil, fmt.Errorf("配置未初始化")
	}
	current := *fs.config
	manager := fs.alertManager
	running := fs.running
	fs.mu.Unlock()

	updated := current
	updated.AlertEnabled = enabled
	updated.AlertSuppressEnabled = &suppressEnabled
	updated.AlertRulesFile = strings.TrimSpace(rulesFile)
	updated.AlertLogPaths = strings.TrimSpace(logPaths)
	updated.AlertPollInterval = strings.TrimSpace(pollInterval)
	updated.AlertStartFromEnd = &startFromEnd
	if strings.TrimSpace(updated.AlertPollInterval) == "" {
		updated.AlertPollInterval = "2s"
	}

	// 告警管理器按需创建或热更新
	if manager == nil {
		if enabled {
			newManager, err := alert.NewManager(&updated, &alert.NotifierSet{
				DingTalk: fs.dingtalkRobot,
				Email:    fs.emailSender,
			})
			if err != nil {
				return nil, err
			}
			manager = newManager
			if running && manager != nil {
				manager.Start()
			}
		}
	} else {
		if err := manager.UpdateConfig(alert.ConfigUpdate{
			Enabled:      enabled,
			SuppressEnabled: suppressEnabled,
			RulesFile:    updated.AlertRulesFile,
			LogPaths:     updated.AlertLogPaths,
			PollInterval: updated.AlertPollInterval,
			StartFromEnd: startFromEnd,
		}, running); err != nil {
			return nil, err
		}
	}

	fs.mu.Lock()
	fs.config = &updated
	fs.alertManager = manager
	fs.mu.Unlock()

	return fs.Config(), nil
}

// AddFile 实现 watcher.UploadPool 用于入队监控到的文件
func (fs *FileService) AddFile(filePath string) error {
	return fs.enqueueFile(filePath, false)
}

// EnqueueManualUpload 允许 API 触发手动上传
func (fs *FileService) EnqueueManualUpload(filePath string) error {
	return fs.enqueueFile(filePath, true)
}

// enqueueFile 将文件加入上传队列并更新状态
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
