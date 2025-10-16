package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"file-watch/internal/models"
)

var (
	logger   *log.Logger
	logLevel string
)

// InitLogger 初始化日志系统
func InitLogger(config *models.Config) error {
	var logOutput *os.File
	var err error

	// 如果指定了日志文件，则输出到文件
	if config.LogFile != "" {
		// 确保日志目录存在
		logDir := filepath.Dir(config.LogFile)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return fmt.Errorf("创建日志目录失败: %v", err)
		}

		logOutput, err = os.OpenFile(config.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return fmt.Errorf("打开日志文件失败: %v", err)
		}
	} else {
		// 默认输出到控制台
		logOutput = os.Stdout
	}

	// 设置日志格式：时间 + 日志级别 + 消息
	logger = log.New(logOutput, "", log.LstdFlags|log.Lshortfile)

	// 同时输出到控制台和文件（如果指定了文件）
	if config.LogFile != "" {
		multiWriter := io.MultiWriter(os.Stdout, logOutput)
		logger = log.New(multiWriter, "", log.LstdFlags|log.Lshortfile)
	}

	// 设置日志级别
	logLevel = config.LogLevel

	return nil
}

// Info 记录信息日志
func Info(format string, v ...interface{}) {
	if logger != nil {
		logger.Printf("[INFO] "+format, v...)
	} else {
		log.Printf("[INFO] "+format, v...)
	}
}

// Error 记录错误日志
func Error(format string, v ...interface{}) {
	if logger != nil {
		logger.Printf("[ERROR] "+format, v...)
	} else {
		log.Printf("[ERROR] "+format, v...)
	}
}

// Warn 记录警告日志
func Warn(format string, v ...interface{}) {
	if logger != nil {
		logger.Printf("[WARN] "+format, v...)
	} else {
		log.Printf("[WARN] "+format, v...)
	}
}

// Debug 记录调试日志
func Debug(format string, v ...interface{}) {
	if logLevel == "debug" {
		if logger != nil {
			logger.Printf("[DEBUG] "+format, v...)
		} else {
			log.Printf("[DEBUG] "+format, v...)
		}
	}
}

// SetLogLevel 设置日志级别
func SetLogLevel(level string) {
	// 这里可以实现更复杂的日志级别控制
	// 目前简单实现，通过全局变量控制
}

// GetLogger 获取logger实例
func GetLogger() *log.Logger {
	return logger
}
