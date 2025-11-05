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
/**
字段含义：
config *models.Config：指向配置，包含 watch_dir、bucket、jenkins 配置等。
s3Client *s3.Client：封装的 S3 客户端，用于上传文件并生成下载 URL。
jenkins *jenkins.Client：Jenkins 客户端，用于触发远端构建。
wechat *wechat.Robot：企业微信机器人实例，用于发送通知消息。
uploadPool *upload.WorkerPool：上传任务的工作池，管理并发上传与队列。
watcher *watcher.FileWatcher：文件系统监控器，监测新文件并提交到上传队列。
设计意图：
以组合方式把各个组件注入到 Service 中，使得 NewFileService 负责初始化和连接它们。
指针字段避免复制并便于在方法中修改或替换实例。
*/
type FileService struct {
	config     *models.Config
	s3Client   *s3.Client
	jenkins    *jenkins.Client
	wechat     *wechat.Robot
	uploadPool *upload.WorkerPool
	watcher    *watcher.FileWatcher
}

// NewFileService 构造并初始化 FileService 的所有依赖
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

	// 创建 FileService 实例并注入已构造的客户端
	service := &FileService{
		config:   config,
		s3Client: s3Client,
		jenkins:  jenkinsClient,
		wechat:   wechatRobot,
	}

	// 初始化上传工作池
	/**
	upload.NewWorkerPool 接受：工作线程数、队列大小、处理函数回调（service.processFile）。含义：
	工作池内部维护一个任务队列和多个 worker goroutine，从队列取任务调用 processFile。
	processFile 被传作回调，因此 worker 在处理每个文件时会调用 service 的方法。注意闭包/方法接收者使用方式：传方法值不会复制 service（是安全的指针接收者使用）。
	*/
	//TODO：没懂
	service.uploadPool = upload.NewWorkerPool(
		config.UploadWorkers,
		config.UploadQueueSize,
		service.processFile,
	)

	// 初始化文件监控器并注入上传池
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
/**
这个方法是关键，worker 对每个任务都会调用它
*/
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
/**
返回上传池的统计信息（队列长度、worker 数量），用于接口或监控展示
*/
func (fs *FileService) GetStats() models.UploadStats {
	if fs.uploadPool != nil {
		return fs.uploadPool.GetStats()
	}
	return models.UploadStats{}
}
