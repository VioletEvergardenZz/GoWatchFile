package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"file-watch/internal/config"
	"file-watch/internal/logger"
	"file-watch/internal/models"
	"file-watch/internal/service"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("程序退出: %v", err)
	}
}

func run() error {
	configPath := parseFlags()
	log.Printf("程序启动，配置文件: %s", configPath)

	cfg, err := loadAndValidateConfig(configPath)
	if err != nil {
		return err
	}

	if err := logger.InitLogger(cfg); err != nil {
		return err
	}
	defer logger.Close()

	logConfig(cfg)

	fileService, err := service.NewFileService(cfg)
	if err != nil {
		logger.Error("创建文件服务失败: %v", err)
		return err
	}

	if err := fileService.Start(); err != nil {
		logger.Error("启动文件服务失败: %v", err)
		return err
	}

	waitForShutdown(fileService)
	return nil
}

func parseFlags() string {
	var configPath string
	flag.StringVar(&configPath, "config", "config.yaml", "配置文件路径")
	flag.Parse()
	return configPath
}

func loadAndValidateConfig(configPath string) (*models.Config, error) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	if err := config.ValidateConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func logConfig(cfg *models.Config) {
	logger.Info("配置加载成功")
	logger.Info("监控目录: %s", cfg.WatchDir)
	logger.Info("文件后缀: %s", cfg.FileExt)
	logger.Info("企业微信机器人 Key: %s", cfg.RobotKey)
	if cfg.DingTalkWebhook != "" {
		logger.Info("钉钉 Webhook: %s", cfg.DingTalkWebhook)
	}
	if cfg.DingTalkSecret != "" {
		logger.Info("钉钉 Secret: 已配置")
	}
	logger.Info("S3 存储桶: %s", cfg.Bucket)
	// logger.Info("AK: %s", cfg.AK)
	// logger.Info("SK: %s", cfg.SK)
	logger.Info("S3 Endpoint: %s", cfg.Endpoint)
	logger.Info("S3 Region: %s", cfg.Region)
	logger.Info("S3 路径风格: %v", cfg.ForcePathStyle)
	logger.Info("S3 禁用 SSL: %v", cfg.DisableSSL)
	logger.Info("Jenkins 地址: %s", cfg.JenkinsHost)
	logger.Info("Jenkins 任务: %s", cfg.JenkinsJob)
	// logger.Info("Jenkins User: %s", cfg.JenkinsUser)
	// logger.Info("Jenkins Password: %s", cfg.JenkinsPassword)
	logToStd := cfg.LogToStd == nil || *cfg.LogToStd
	logger.Info("日志级别: %s", cfg.LogLevel)
	if cfg.LogFile != "" {
		logger.Info("日志文件: %s", cfg.LogFile)
	}
	logger.Info("日志输出到标准输出: %v", logToStd)
	logger.Info("日志显示调用文件: %v", cfg.LogShowCaller)
	logger.Info("上传工作池大小: %d", cfg.UploadWorkers)
	logger.Info("上传队列大小: %d", cfg.UploadQueueSize)
}

func waitForShutdown(fileService *service.FileService) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	<-signalChan
	logger.Info("收到退出信号，正在关闭服务...")

	if err := fileService.Stop(); err != nil {
		logger.Error("停止文件服务失败: %v", err)
	}

	logger.Info("程序已退出")
	os.Exit(0)
}
