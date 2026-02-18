// 本文件用于文件监控服务的核心协作流程
// 文件职责：实现当前模块的核心业务逻辑与数据流转
// 关键路径：入口参数先校验再执行业务处理 最后返回统一结果
// 边界与容错：异常场景显式返回错误 由上层决定重试或降级

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"file-watch/internal/alert"
	"file-watch/internal/config"
	"file-watch/internal/dingtalk"
	"file-watch/internal/email"
	"file-watch/internal/logger"
	"file-watch/internal/match"
	"file-watch/internal/metrics"
	"file-watch/internal/models"
	"file-watch/internal/oss"
	"file-watch/internal/pathutil"
	"file-watch/internal/persistqueue"
	"file-watch/internal/state"
	"file-watch/internal/upload"
	"file-watch/internal/watcher"
)

// FileService 负责协调文件监控、上传与通知流程
type FileService struct {
	config        *models.Config
	configPath    string
	ossClient     *oss.Client
	dingtalkRobot *dingtalk.Robot
	emailSender   *email.Sender
	uploadPool    *upload.WorkerPool
	persistQueue  *persistqueue.FileQueue
	watcher       *watcher.FileWatcher
	alertManager  *alert.Manager
	state         *state.RuntimeState
	running       bool
	mu            sync.Mutex      //互斥锁，用来保护 FileService 内部共享数据的并发读写
	manualOnce    map[string]bool //标记“某个路径的下一次处理是手动上传”
	metricsMu     sync.Mutex
	queueFull     uint64
	queueShed     uint64
	retryTotal    uint64
	uploadFailure uint64
	failReasons   map[string]uint64
}

const shutdownTimeout = 30 * time.Second
const defaultUploadQueuePersistFile = "logs/upload-queue.json"
const defaultQueueSaturationThreshold = 0.9

var defaultUploadRetryDelays = []time.Duration{
	1 * time.Second,
	2 * time.Second,
	5 * time.Second,
}

const (
	defaultUploadRetryMaxAttempts = 4
	maxUploadRetryDelay           = 60 * time.Second
)

// NewFileService 构造并初始化 FileService 的依赖
// 初始化顺序固定为 状态 -> 存储客户端 -> 告警管理 -> 上传池 -> 监听器
// 这样任一环节失败都能在启动前暴露 不把半初始化实例暴露给外层
func NewFileService(config *models.Config, configPath string) (*FileService, error) {
	runtimeState := state.NewRuntimeState(config)
	if err := runtimeState.BootstrapExisting(); err != nil {
		logger.Warn("预加载历史文件失败: %v", err)
	}

	ossClient, err := newOSSClient(config)
	if err != nil {
		return nil, err
	}

	fileService := &FileService{
		config:        config,
		configPath:    strings.TrimSpace(configPath),
		ossClient:     ossClient,
		dingtalkRobot: newDingTalkRobot(config),
		emailSender:   newEmailSender(config),
		state:         runtimeState,
		manualOnce:    make(map[string]bool),
		failReasons:   make(map[string]uint64),
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

	uploadPool, persistStore, err := newUploadPool(config, fileService.processFile, fileService.handlePoolStats)
	if err != nil {
		return nil, err
	}
	fileService.uploadPool = uploadPool
	fileService.persistQueue = persistStore
	runtimeState.SetQueueStats(uploadPool.GetStats())

	fileWatcher, err := newFileWatcher(config, fileService)
	if err != nil {
		return nil, err
	}
	fileService.watcher = fileWatcher

	return fileService, nil
}

// newOSSClient 初始化 OSS 客户端
func newOSSClient(config *models.Config) (*oss.Client, error) {
	client, err := oss.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("初始化OSS客户端失败: %w", err)
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
func newUploadPool(config *models.Config, handler func(context.Context, string) error, onStats func(models.UploadStats)) (*upload.WorkerPool, *persistqueue.FileQueue, error) {
	var queueStore upload.QueueStore
	var persistStore *persistqueue.FileQueue
	if config.UploadQueuePersistEnabled {
		storePath := strings.TrimSpace(config.UploadQueuePersistFile)
		if storePath == "" {
			storePath = defaultUploadQueuePersistFile
		}
		store, err := persistqueue.NewFileQueue(storePath)
		if err != nil {
			return nil, nil, fmt.Errorf("初始化上传持久化队列失败: %w", err)
		}
		persistStore = store
		queueStore = store
		logger.Info("上传持久化队列已启用: %s", storePath)
	}
	pool, err := upload.NewWorkerPool(config.UploadWorkers, config.UploadQueueSize, handler, onStats, queueStore)
	if err != nil {
		return nil, nil, err
	}
	return pool, persistStore, nil
}

// handlePoolStats 将队列统计写入运行态
func (fs *FileService) handlePoolStats(stats models.UploadStats) {
	if fs.state != nil {
		fs.state.SetQueueStats(stats)
	}
	metrics.Global().SetQueueStats(stats)
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

// persistRuntimeConfig 将控制台更新的配置写入运行时配置文件
func (fs *FileService) persistRuntimeConfig(cfg *models.Config) {
	if cfg == nil {
		return
	}
	if strings.TrimSpace(fs.configPath) == "" {
		return
	}
	if err := config.SaveRuntimeConfig(fs.configPath, cfg); err != nil {
		logger.Warn("write runtime config failed: %v", err)
	}
}

// UpdateConfig 更新运行时配置并重建监听器与上传池
// UpdateConfig 负责运行态配置热更新
// 仅允许更新白名单字段 其余静态策略保持重启生效原则
func (fs *FileService) UpdateConfig(watchDir, fileExt, silence string, uploadWorkers, uploadQueueSize int, uploadRetryDelays string, uploadRetryEnabled *bool, systemResourceEnabled *bool) (*models.Config, error) {
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
	oldOSS := fs.ossClient
	oldPersistQueue := fs.persistQueue
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
	if strings.TrimSpace(uploadRetryDelays) != "" && strings.TrimSpace(uploadRetryDelays) != current.UploadRetryDelays {
		updated.UploadRetryDelays = strings.TrimSpace(uploadRetryDelays)
	}
	currentRetryEnabled := true
	if current.UploadRetryEnabled != nil {
		currentRetryEnabled = *current.UploadRetryEnabled
	}
	if uploadRetryEnabled != nil && *uploadRetryEnabled != currentRetryEnabled {
		enabled := *uploadRetryEnabled
		updated.UploadRetryEnabled = &enabled
	}
	if systemResourceEnabled != nil && *systemResourceEnabled != current.SystemResourceEnabled {
		updated.SystemResourceEnabled = *systemResourceEnabled
	}

	// 基于新配置初始化运行态
	newState := state.NewRuntimeState(&updated)
	if err := newState.BootstrapExisting(); err != nil {
		logger.Warn("预加载历史文件失败: %v", err)
	}
	// 迁移旧的计数与历史数据
	newState.CarryOverFrom(oldState)

	// 初始化新的 OSS 客户端
	newOSS, err := newOSSClient(&updated)
	if err != nil {
		return nil, err
	}

	// 初始化新的上传工作池
	newPool, newPersistQueue, err := newUploadPool(&updated, fs.processFile, fs.handlePoolStats)
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
	// 先原子替换再启动或重置 watcher，避免运行中读到半更新状态
	fs.mu.Lock()
	fs.config = &updated
	fs.state = newState
	fs.uploadPool = newPool
	fs.persistQueue = newPersistQueue
	fs.watcher = activeWatcher
	fs.ossClient = newOSS
	fs.state.SetQueueStats(fs.uploadPool.GetStats())
	fs.mu.Unlock()

	if oldWatcher == nil {
		// 使用新配置启动 watcher
		if err := activeWatcher.Start(); err != nil {
			// watcher 启动失败则回滚
			// 回滚顺序保持和替换顺序相反，确保资源引用一致
			_ = newPool.ShutdownGraceful(shutdownTimeout)
			_ = activeWatcher.Close()
			// 回滚到旧配置，避免留下已关闭的工作池
			fs.mu.Lock()
			fs.config = oldCfg
			fs.state = oldState
			fs.uploadPool = oldPool
			fs.persistQueue = oldPersistQueue
			fs.watcher = oldWatcher
			fs.ossClient = oldOSS
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
			// 旧 watcher 仍可继续工作，因此只需丢弃新建组件
			_ = newPool.ShutdownGraceful(shutdownTimeout)
			fs.mu.Lock()
			fs.config = oldCfg
			fs.state = oldState
			fs.uploadPool = oldPool
			fs.persistQueue = oldPersistQueue
			fs.watcher = oldWatcher
			fs.ossClient = oldOSS
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
	if oldOSS != nil {
		// oss sdk 无需显式关闭客户端，保留占位以示释放顺序
	}

	logger.Info("运行时配置已更新: watchDir=%s, fileExt=%s, silence=%s, workers=%d, queue=%d",
		updated.WatchDir,
		updated.FileExt,
		updated.Silence,
		updated.UploadWorkers,
		updated.UploadQueueSize,
	)

	fs.persistRuntimeConfig(&updated)
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

// normalizeWatchDirKey 生成目录去重键
// Windows 下统一转小写，避免盘符大小写导致重复监听
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
// Start 启动监听 告警和上传执行链路
// 启动失败时直接返回错误 避免服务处于“部分可用”状态
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
// Stop 按 watcher -> alert -> upload 的顺序关闭
// 顺序关闭可以减少“新任务继续入队但消费已停止”的竞态窗口
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
// processFile 是上传执行主路径
// 它统一负责上传 重试 通知和运行态更新 避免分散状态更新导致口径不一致
func (fs *FileService) processFile(ctx context.Context, filePath string) error {
	start := time.Now()
	manual := fs.consumeManualOnce(filePath)
	// 手动上传不受自动开关限制
	if fs.state != nil && !manual && !fs.state.AutoUploadEnabled(filePath) {
		fs.state.MarkSkipped(filePath)
		return nil
	}

	logger.Info("开始处理文件: %s", filePath)
	downloadURL, err := fs.uploadFileWithRetry(ctx, filePath)
	if err != nil {
		fs.recordUploadFailure(err)
		if fs.state != nil {
			fs.state.MarkFailed(filePath, err)
		}
		return fmt.Errorf("上传文件到OSS失败: %w", err)
	}

	fileName := filepath.Base(filePath)
	logger.Info("文件信息 - 文件名: %s", fileName)

	if fs.state != nil {
		fs.state.MarkUploaded(filePath, downloadURL, time.Since(start), manual)
	}
	metrics.Global().ObserveUploadSuccess(time.Since(start))

	fullPath := filepath.Clean(filePath)
	fs.sendDingTalk(ctx, downloadURL, fullPath)
	fs.sendEmailNotification(ctx, downloadURL, fullPath)

	logger.Info("文件处理完成: %s", filePath)
	return nil
}

// uploadFileWithRetry 负责上传重试，避免短暂失败导致任务丢失
// uploadFileWithRetry 把重试策略与上传调用绑定
// 每次失败都记录可观测指标 便于区分瞬时波动与持续性故障
func (fs *FileService) uploadFileWithRetry(ctx context.Context, filePath string) (string, error) {
	if fs.ossClient == nil {
		return "", fmt.Errorf("OSS客户端未初始化")
	}
	if !isUploadRetryEnabled(fs.config) {
		return fs.ossClient.UploadFile(ctx, filePath)
	}
	delays := buildUploadRetryPlan(fs.config)
	tries := len(delays) + 1
	var lastErr error
	for attempt := 1; attempt <= tries; attempt++ {
		if ctx != nil && ctx.Err() != nil {
			return "", ctx.Err()
		}
		downloadURL, err := fs.ossClient.UploadFile(ctx, filePath)
		if err == nil {
			return downloadURL, nil
		}
		lastErr = err
		if attempt == tries {
			break
		}
		// 失败后按配置间隔退避，降低瞬时抖动导致的连续失败
		delay := delays[attempt-1]
		fs.recordRetryAttempt()
		logger.Warn("上传失败，准备重试: %s, 第 %d/%d 次, 等待 %v, 错误: %v", filePath, attempt, tries, delay, err)
		if err := sleepWithContext(ctx, delay); err != nil {
			return "", err
		}
	}
	return "", lastErr
}

// isUploadRetryEnabled 判断是否启用上传重试
func isUploadRetryEnabled(cfg *models.Config) bool {
	if cfg == nil {
		return true
	}
	if cfg.UploadRetryEnabled == nil {
		return true
	}
	return *cfg.UploadRetryEnabled
}

// isQueueCircuitBreakerEnabled 判断是否启用队列饱和熔断。
func isQueueCircuitBreakerEnabled(cfg *models.Config) bool {
	if cfg == nil {
		return true
	}
	if cfg.UploadQueueCircuitBreakerEnabled == nil {
		return true
	}
	return *cfg.UploadQueueCircuitBreakerEnabled
}

// resolveQueueSaturationThreshold 解析队列饱和阈值，范围 (0,1]。
func resolveQueueSaturationThreshold(cfg *models.Config) float64 {
	if cfg == nil {
		return defaultQueueSaturationThreshold
	}
	threshold := cfg.UploadQueueSaturationThreshold
	if threshold <= 0 || threshold > 1 {
		return defaultQueueSaturationThreshold
	}
	return threshold
}

func resolveUploadQueueSize(cfg *models.Config) int {
	if cfg == nil {
		return 100
	}
	if cfg.UploadQueueSize <= 0 {
		return 100
	}
	return cfg.UploadQueueSize
}

// parseUploadRetryDelays 解析上传重试间隔配置
func parseUploadRetryDelays(cfg *models.Config) []time.Duration {
	if cfg == nil {
		return defaultUploadRetryDelays
	}
	raw := strings.TrimSpace(cfg.UploadRetryDelays)
	if raw == "" {
		return defaultUploadRetryDelays
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', ';', ' ', '\n', '\r', '\t':
			return true
		default:
			return false
		}
	})
	delays := make([]time.Duration, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		d, err := time.ParseDuration(trimmed)
		if err != nil || d <= 0 {
			logger.Warn("上传重试间隔解析失败，已忽略: %s", trimmed)
			continue
		}
		delays = append(delays, d)
	}
	if len(delays) == 0 {
		return defaultUploadRetryDelays
	}
	return delays
}

// resolveUploadRetryMaxAttempts 解析重试上限，包含首次尝试。
func resolveUploadRetryMaxAttempts(cfg *models.Config) int {
	if cfg == nil {
		return defaultUploadRetryMaxAttempts
	}
	if cfg.UploadRetryMaxAttempts <= 0 {
		return defaultUploadRetryMaxAttempts
	}
	if cfg.UploadRetryMaxAttempts > 20 {
		return 20
	}
	return cfg.UploadRetryMaxAttempts
}

// buildUploadRetryPlan 构建退避计划，返回每次重试前等待时长。
func buildUploadRetryPlan(cfg *models.Config) []time.Duration {
	maxAttempts := resolveUploadRetryMaxAttempts(cfg)
	if maxAttempts <= 1 {
		return nil
	}
	need := maxAttempts - 1
	base := parseUploadRetryDelays(cfg)
	plan := make([]time.Duration, 0, need)
	for _, delay := range base {
		if len(plan) >= need {
			break
		}
		if delay <= 0 {
			continue
		}
		plan = append(plan, delay)
	}
	if len(plan) == 0 {
		plan = append(plan, defaultUploadRetryDelays[0])
	}
	for len(plan) < need {
		next := plan[len(plan)-1] * 2
		if next > maxUploadRetryDelay {
			next = maxUploadRetryDelay
		}
		plan = append(plan, next)
	}
	return plan
}

// sleepWithContext 支持在等待期间响应停止信号
func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	if ctx == nil {
		time.Sleep(delay)
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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

// formatHostPath 用于格式化输出内容
func formatHostPath(filePath string) string {
	host, err := os.Hostname()
	if err != nil {
		host = ""
	}
	host = strings.TrimSpace(host)
	if host == "" {
		host = "unknown-host"
	}
	cleaned := filepath.ToSlash(filepath.Clean(filePath))
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "" {
		return host
	}
	return host + "/" + cleaned
}

// sendDingTalk 发送钉钉通知
func (fs *FileService) sendDingTalk(ctx context.Context, downloadURL, fileName string) {
	if fs.dingtalkRobot == nil {
		return
	}
	displayName := formatHostPath(fileName)
	if err := fs.dingtalkRobot.SendMessage(ctx, downloadURL, displayName); err != nil {
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
		formatHostPath(filePath),
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

// HealthSnapshot 返回运行健康指标
func (fs *FileService) HealthSnapshot() models.HealthSnapshot {
	queueStats := fs.GetStats()
	persistHealth := models.PersistQueueHealth{}

	fs.mu.Lock()
	cfg := fs.config
	persistStore := fs.persistQueue
	fs.mu.Unlock()

	if cfg != nil && cfg.UploadQueuePersistEnabled {
		persistHealth.Enabled = true
		persistHealth.StoreFile = strings.TrimSpace(cfg.UploadQueuePersistFile)
		if persistHealth.StoreFile == "" {
			persistHealth.StoreFile = defaultUploadQueuePersistFile
		}
	}
	if persistStore != nil {
		stats := persistStore.HealthStats()
		if strings.TrimSpace(stats.StoreFile) != "" {
			persistHealth.StoreFile = stats.StoreFile
		}
		persistHealth.RecoveredTotal = stats.RecoveredTotal
		persistHealth.CorruptFallbackTotal = stats.CorruptFallbackTotal
		persistHealth.PersistWriteFailureTotal = stats.PersistWriteFailureTotal
	}

	fs.metricsMu.Lock()
	snapshot := models.HealthSnapshot{
		QueueLength:        queueStats.QueueLength,
		Workers:            queueStats.Workers,
		InFlight:           queueStats.InFlight,
		QueueFullTotal:     fs.queueFull,
		QueueShedTotal:     fs.queueShed,
		RetryTotal:         fs.retryTotal,
		UploadFailureTotal: fs.uploadFailure,
		FailureReasons:     make([]models.FailureReasonCount, 0, len(fs.failReasons)),
		PersistQueue:       persistHealth,
	}
	for reason, count := range fs.failReasons {
		snapshot.FailureReasons = append(snapshot.FailureReasons, models.FailureReasonCount{
			Reason: reason,
			Count:  count,
		})
	}
	fs.metricsMu.Unlock()

	sort.Slice(snapshot.FailureReasons, func(i, j int) bool {
		if snapshot.FailureReasons[i].Count == snapshot.FailureReasons[j].Count {
			return snapshot.FailureReasons[i].Reason < snapshot.FailureReasons[j].Reason
		}
		return snapshot.FailureReasons[i].Count > snapshot.FailureReasons[j].Count
	})
	if len(snapshot.FailureReasons) > 10 {
		snapshot.FailureReasons = snapshot.FailureReasons[:10]
	}
	return snapshot
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
	updated.AlertRulesFile = ""
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
			Enabled:         enabled,
			SuppressEnabled: suppressEnabled,
			LogPaths:        updated.AlertLogPaths,
			PollInterval:    updated.AlertPollInterval,
			StartFromEnd:    startFromEnd,
		}, running); err != nil {
			return nil, err
		}
	}

	fs.mu.Lock()
	fs.config = &updated
	fs.alertManager = manager
	fs.mu.Unlock()

	fs.persistRuntimeConfig(&updated)
	return fs.Config(), nil
}

// UpdateAlertRules 运行时更新告警规则并持久化
func (fs *FileService) UpdateAlertRules(ruleset *alert.Ruleset) (*models.Config, error) {
	fs.mu.Lock()
	if fs.config == nil {
		fs.mu.Unlock()
		return nil, fmt.Errorf("配置未初始化")
	}
	current := *fs.config
	manager := fs.alertManager
	running := fs.running
	fs.mu.Unlock()

	if ruleset == nil {
		return nil, fmt.Errorf("告警规则不能为空")
	}
	if err := alert.NormalizeRuleset(ruleset); err != nil {
		return nil, err
	}
	current.AlertRules = ruleset

	if manager == nil {
		if current.AlertEnabled {
			newManager, err := alert.NewManager(&current, &alert.NotifierSet{
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
		if err := manager.UpdateRules(ruleset); err != nil {
			return nil, err
		}
	}

	fs.mu.Lock()
	fs.config = &current
	fs.alertManager = manager
	fs.mu.Unlock()

	fs.persistRuntimeConfig(&current)
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
// enqueueFile 统一处理自动与手动入队
// 入队前会执行队列饱和判断与熔断策略 防止雪崩式堆积
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
	if !manual && fs.shouldShedByQueueSaturation() {
		err := fmt.Errorf("upload queue saturated")
		fs.recordQueueShed()
		if fs.state != nil {
			fs.state.MarkFailed(filePath, err)
			fs.state.SetQueueStats(fs.uploadPool.GetStats())
		}
		return err
	}
	if err := fs.uploadPool.AddFile(filePath); err != nil {
		if errors.Is(err, upload.ErrQueueFull) {
			// 队列满单独记指标，便于区分是容量问题还是上传失败
			fs.recordQueueFull()
		}
		if fs.state != nil {
			fs.state.MarkFailed(filePath, err)
			fs.state.SetQueueStats(fs.uploadPool.GetStats())
		}
		return err
	}
	metrics.Global().IncFileEvent()
	if fs.state != nil {
		fs.state.SetQueueStats(fs.uploadPool.GetStats())
	}
	return nil
}

// shouldShedByQueueSaturation 在队列接近满载时触发限流，避免持续堆积。
// shouldShedByQueueSaturation 在入队前做背压判定
// 判定逻辑只依赖当前队列快照 保持快速且无阻塞
func (fs *FileService) shouldShedByQueueSaturation() bool {
	if fs == nil || fs.uploadPool == nil {
		return false
	}
	fs.mu.Lock()
	cfg := fs.config
	fs.mu.Unlock()
	if !isQueueCircuitBreakerEnabled(cfg) {
		return false
	}
	queueCap := resolveUploadQueueSize(cfg)
	if queueCap <= 0 {
		return false
	}
	stats := fs.uploadPool.GetStats()
	ratio := float64(stats.QueueLength) / float64(queueCap)
	if ratio < resolveQueueSaturationThreshold(cfg) {
		return false
	}
	logger.Warn("上传队列触发限流: queue=%d cap=%d ratio=%.2f", stats.QueueLength, queueCap, ratio)
	return true
}

// recordQueueFull 记录上传队列满次数
func (fs *FileService) recordQueueFull() {
	fs.metricsMu.Lock()
	fs.queueFull++
	fs.metricsMu.Unlock()
	metrics.Global().IncQueueFull()
}

// recordQueueShed 记录饱和阈值触发的限流次数
func (fs *FileService) recordQueueShed() {
	fs.metricsMu.Lock()
	fs.queueShed++
	fs.metricsMu.Unlock()
	metrics.Global().IncQueueShed()
}

// recordRetryAttempt 记录上传重试次数
func (fs *FileService) recordRetryAttempt() {
	fs.metricsMu.Lock()
	fs.retryTotal++
	fs.metricsMu.Unlock()
	metrics.Global().IncUploadRetry()
}

// recordUploadFailure 记录上传失败次数与失败原因分布
func (fs *FileService) recordUploadFailure(err error) {
	fs.metricsMu.Lock()
	fs.uploadFailure++
	if fs.failReasons == nil {
		fs.failReasons = make(map[string]uint64)
	}
	reason := normalizeFailureReason(err)
	fs.failReasons[reason]++
	fs.metricsMu.Unlock()
	metrics.Global().ObserveUploadFailure(reason)
}

// normalizeFailureReason 将错误信息规整为可聚合的统计键
func normalizeFailureReason(err error) string {
	if err == nil {
		return "unknown"
	}
	reason := strings.TrimSpace(err.Error())
	if reason == "" {
		return "unknown"
	}
	if len(reason) > 120 {
		return reason[:120]
	}
	return reason
}
