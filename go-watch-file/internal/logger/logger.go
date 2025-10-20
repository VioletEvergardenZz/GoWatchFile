package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath" //用于文件/路径操作与标准日志

	"file-watch/internal/models" //引入配置结构体类型
)

var (
	//TODO:没明白这个 logger 是干什么的
	//保存一个标准库 log.Logger 的指针，用作包级别的 logger 实例
	logger   *log.Logger
	logLevel string
)

// InitLogger 初始化日志系统
func InitLogger(config *models.Config) error {
	var logOutput *os.File //最终写入日志的文件
	var err error

	// 如果指定了日志文件，则输出到文件
	if config.LogFile != "" {
		// 确保日志目录存在，通过 filepath.Dir(config.LogFile) 得到日志文件所在目录路径  logs/file-monitor.log
		logDir := filepath.Dir(config.LogFile)
		// 递归创建
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return fmt.Errorf("创建日志目录失败: %v", err)
		}
		// os.OpenFile 以创建/打开日志文件（标志：O_CREATE|O_WRONLY|O_APPEND，权限 0666）
		logOutput, err = os.OpenFile(config.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return fmt.Errorf("打开日志文件失败: %v", err)
		}
	} else {
		// 如果 LogFile 为空，就把 logOutput 设为 os.Stdout（写到控制台）。
		logOutput = os.Stdout
	}

	// 设置日志格式：时间 + 日志级别 + 消息
	//log.LstdFlags：包含日期和时间（默认格式）
	//log.Lshortfile：在日志中添加调用文件名和行号（短路径，例如 file.go:23）
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

// 日志封装函数

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
