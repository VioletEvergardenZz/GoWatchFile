package service

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"file-watch/internal/dingtalk"
	"file-watch/internal/logger"
	"file-watch/internal/models"
	"file-watch/internal/pathutil"
	"file-watch/internal/s3"
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
}

const shutdownTimeout = 30 * time.Second

// NewFileService 构造并初始化 FileService 的依赖。
func NewFileService(config *models.Config) (*FileService, error) {
	s3Client, err := newS3Client(config)
	if err != nil {
		return nil, err
	}

	fileService := &FileService{
		config:        config,
		s3Client:      s3Client,
		wechatRobot:   newWeChatRobot(config),
		dingtalkRobot: newDingTalkRobot(config),
	}

	uploadPool, err := newUploadPool(config, fileService.processFile)
	if err != nil {
		return nil, err
	}
	fileService.uploadPool = uploadPool

	fileWatcher, err := newFileWatcher(config, fileService.uploadPool)
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

func newFileWatcher(config *models.Config, uploadPool *upload.WorkerPool) (*watcher.FileWatcher, error) {
	fileWatcher, err := watcher.NewFileWatcher(config, uploadPool)
	if err != nil {
		return nil, fmt.Errorf("初始化文件监控器失败: %w", err)
	}
	return fileWatcher, nil
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
	logger.Info("开始处理文件: %s", filePath)
	downloadURL, err := fs.s3Client.UploadFile(ctx, filePath)
	if err != nil {
		return fmt.Errorf("上传文件到S3失败: %w", err)
	}

	appName, fileName := fs.parseFileInfo(filePath)
	logger.Info("文件信息 - 应用名: %s, 文件名: %s", appName, fileName)

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
