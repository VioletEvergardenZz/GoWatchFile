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
	activeLogger *log.Logger
	logLevel     string
)

// InitLogger 初始化日志系统。
func InitLogger(config *models.Config) error {
	logOutput, err := buildLogWriter(config.LogFile)
	if err != nil {
		return err
	}

	activeLogger = log.New(logOutput, "", log.LstdFlags|log.Lshortfile)
	logLevel = config.LogLevel
	return nil
}

func buildLogWriter(logFile string) (io.Writer, error) {
	if logFile == "" {
		return os.Stdout, nil
	}

	logDir := filepath.Dir(logFile)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}

	logOutput, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("打开日志文件失败: %w", err)
	}

	return io.MultiWriter(os.Stdout, logOutput), nil
}

// Info 记录信息日志。
func Info(format string, v ...interface{}) {
	logWithLevel("INFO", format, v...)
}

// Error 记录错误日志。
func Error(format string, v ...interface{}) {
	logWithLevel("ERROR", format, v...)
}

// Warn 记录警告日志。
func Warn(format string, v ...interface{}) {
	logWithLevel("WARN", format, v...)
}

// Debug 记录调试日志。
func Debug(format string, v ...interface{}) {
	if logLevel == "debug" {
		logWithLevel("DEBUG", format, v...)
	}
}

// SetLogLevel 设置日志级别。
func SetLogLevel(level string) {
	logLevel = level
}

// GetLogger 获取 logger 实例。
func GetLogger() *log.Logger {
	return activeLogger
}

func logWithLevel(level, format string, v ...interface{}) {
	prefix := "[" + level + "] "
	if activeLogger != nil {
		activeLogger.Printf(prefix+format, v...)
		return
	}
	log.Printf(prefix+format, v...)
}
