package service

import (
	"fmt"

	"file-watch/internal/jenkins"
	"file-watch/internal/logger"
	"file-watch/internal/models"
	"file-watch/internal/s3"
	"file-watch/internal/upload"
	"file-watch/internal/watcher"
	"file-watch/internal/wechat"
)

// FileService 文件服务
type FileService struct {
	config     *models.Config
	s3Client   *s3.Client
	jenkins    *jenkins.Client
	wechat     *wechat.Robot
	uploadPool *upload.WorkerPool
	watcher    *watcher.FileWatcher
}

// NewFileService 创建新的文件服务
func NewFileService(config *models.Config) (*FileService, error) {
	// 初始化S3客户端
	s3Client, err := s3.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("初始化S3客户端失败: %v", err)
	}

	// 初始化Jenkins客户端
	jenkinsClient, err := jenkins.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("初始化Jenkins客户端失败: %v", err)
	}

	// 初始化企业微信机器人
	wechatRobot := wechat.NewRobot(config.RobotKey)

	// 创建文件服务
	service := &FileService{
		config:   config,
		s3Client: s3Client,
		jenkins:  jenkinsClient,
		wechat:   wechatRobot,
	}

	// 初始化上传工作池
	service.uploadPool = upload.NewWorkerPool(
		config.UploadWorkers,
		config.UploadQueueSize,
		service.processFile,
	)

	// 初始化文件监控器
	watcher, err := watcher.NewFileWatcher(config, service.uploadPool)
	if err != nil {
		return nil, fmt.Errorf("初始化文件监控器失败: %v", err)
	}
	service.watcher = watcher

	return service, nil
}

// Start 启动文件服务
func (fs *FileService) Start() error {
	logger.Info("启动文件服务...")

	// 启动文件监控
	if err := fs.watcher.Start(); err != nil {
		return fmt.Errorf("启动文件监控失败: %v", err)
	}

	logger.Info("文件服务启动成功")
	return nil
}

// Stop 停止文件服务
func (fs *FileService) Stop() error {
	logger.Info("停止文件服务...")

	// 关闭上传工作池
	if fs.uploadPool != nil {
		fs.uploadPool.Shutdown()
	}

	// 关闭文件监控器
	if fs.watcher != nil {
		if err := fs.watcher.Close(); err != nil {
			logger.Error("关闭文件监控器失败: %v", err)
		}
	}

	logger.Info("文件服务已停止")
	return nil
}

// processFile 处理文件（上传到S3、触发Jenkins、发送微信消息）
func (fs *FileService) processFile(filePath string) error {
	logger.Info("开始处理文件: %s", filePath)

	// 1. 上传文件到S3
	downloadUrl, err := fs.s3Client.UploadFile(filePath)
	if err != nil {
		return fmt.Errorf("上传文件到S3失败: %v", err)
	}

	// 2. 解析文件信息
	appName, fileName, err := jenkins.ParseFileInfo(filePath)
	if err != nil {
		logger.Error("解析文件信息失败: %v", err)
		// 继续处理，不因为解析失败而中断
		appName = "unknown"
		fileName = "unknown"
	}

	logger.Info("文件信息 - 应用名: %s, 文件名: %s", appName, fileName)

	// 3. 触发Jenkins构建
	if err = fs.jenkins.BuildJob(downloadUrl, appName, fileName); err != nil {
		logger.Error("触发Jenkins构建失败: %v", err)
		// 继续处理，不因为Jenkins失败而中断
	}

	// 4. 发送企业微信消息
	if err = fs.wechat.SendMessage(downloadUrl, filePath); err != nil {
		logger.Error("发送企业微信消息失败: %v", err)
		// 继续处理，不因为微信消息失败而中断
	}

	logger.Info("文件处理完成: %s", filePath)
	return nil
}

// GetStats 获取服务统计信息
func (fs *FileService) GetStats() models.UploadStats {
	if fs.uploadPool != nil {
		return fs.uploadPool.GetStats()
	}
	return models.UploadStats{}
}
