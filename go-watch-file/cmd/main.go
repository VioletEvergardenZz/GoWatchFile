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

var configFile string

func main() {
	// 定义命令行参数
	flag.StringVar(&configFile, "config", "config.yaml", "配置文件路径")
	flag.Parse()

	log.Printf("程序启动，配置文件: %s", configFile)

	// 加载配置文件
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 验证配置
	if err := config.ValidateConfig(cfg); err != nil {
		log.Fatalf("配置验证失败: %v", err)
	}

	// 初始化日志系统
	if err := logger.InitLogger(cfg); err != nil {
		log.Fatalf("初始化日志系统失败: %v", err)
	}

	// 打印配置信息
	printConfig(cfg)

	// 创建文件服务
	fileService, err := service.NewFileService(cfg)
	if err != nil {
		logger.Error("创建文件服务失败: %v", err)
		log.Fatalf("创建文件服务失败: %v", err)
	}

	// 启动文件服务
	if err := fileService.Start(); err != nil {
		logger.Error("启动文件服务失败: %v", err)
		log.Fatalf("启动文件服务失败: %v", err)
	}

	// 设置信号处理
	setupSignalHandler(fileService)

	// 保持程序运行
	select {}
}

// printConfig 打印配置信息
func printConfig(cfg *models.Config) {
	logger.Info("配置加载成功:")
	logger.Info("监控目录: %s", cfg.WatchDir)
	logger.Info("文件后缀: %s", cfg.FileExt)
	logger.Info("机器人Key: %s", cfg.RobotKey)
	logger.Info("Bucket: %s", cfg.Bucket)
	// logger.Info("AK: %s", cfg.AK)
	// logger.Info("SK: %s", cfg.SK)
	logger.Info("Endpoint: %s", cfg.Endpoint)
	logger.Info("Region: %s", cfg.Region)
	logger.Info("Force Path Style: %v", cfg.ForcePathStyle)
	logger.Info("Disable SSL: %v", cfg.DisableSSL)
	logger.Info("Jenkins Host: %s", cfg.JenkinsHost)
	logger.Info("Jenkins Job: %s", cfg.JenkinsJob)
	// logger.Info("Jenkins User: %s", cfg.JenkinsUser)
	// logger.Info("Jenkins Password: %s", cfg.JenkinsPassword)
	logger.Info("日志级别: %s", cfg.LogLevel)
	if cfg.LogFile != "" {
		logger.Info("日志文件: %s", cfg.LogFile)
	}
	logger.Info("上传工作池大小: %d", cfg.UploadWorkers)
	logger.Info("上传队列大小: %d", cfg.UploadQueueSize)
}

// setupSignalHandler 设置信号处理器
func setupSignalHandler(fileService *service.FileService) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		logger.Info("收到退出信号，正在关闭服务...")

		if err := fileService.Stop(); err != nil {
			logger.Error("停止文件服务失败: %v", err)
		}

		logger.Info("程序已退出")
		os.Exit(0)
	}()
}
