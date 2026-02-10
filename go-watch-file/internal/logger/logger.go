// 本文件用于日志初始化与输出封装
package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"file-watch/internal/models"
)

const (
	levelDebug int32 = iota //从 0 开始递增
	levelInfo
	levelWarn
	levelError
	defaultLogToStd = true
)

type managedLogger struct {
	logger *log.Logger
	file   *os.File // 日志文件句柄
}

var (
	activeLogger atomic.Pointer[managedLogger] //原子指针，指向当前使用的 managedLogger，用原子操作读写，保证并发下安全切换/读取
	logLevel     atomic.Int32                  //原子整数，表示当前日志级别
)

// init 用于初始化模块级默认配置
func init() {
	logLevel.Store(levelInfo)
}

// InitLogger 初始化日志系统
func InitLogger(config *models.Config) error {
	logToStd := defaultLogToStd
	if config.LogToStd != nil {
		logToStd = *config.LogToStd
	}

	logOutput, logFile, err := buildLogWriter(config.LogFile, logToStd)
	if err != nil {
		return err
	}

	closeActiveLogger()

	flags := log.LstdFlags
	if config.LogShowCaller {
		flags |= log.Lshortfile
	}

	newLogger := &managedLogger{
		logger: log.New(logOutput, "", flags),
		file:   logFile,
	}
	activeLogger.Store(newLogger)
	logLevel.Store(parseLogLevel(config.LogLevel))
	return nil
}

// buildLogWriter 用于构建后续流程所需的数据
func buildLogWriter(logFile string, logToStd bool) (io.Writer, *os.File, error) {
	// 如果同时禁用了文件和 stdout
	if logFile == "" && !logToStd {
		// 为避免日志丢失，强制回落到 stdout（兜底）
		logToStd = true
	}

	if logFile == "" {
		return os.Stdout, nil, nil
	}

	logDir := filepath.Dir(logFile)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, nil, fmt.Errorf("创建日志目录失败: %w", err)
	}

	logOutput, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, nil, fmt.Errorf("打开日志文件失败: %w", err)
	}

	switch {
	case logToStd:
		return io.MultiWriter(os.Stdout, logOutput), logOutput, nil
	default:
		return logOutput, logOutput, nil
	}
}

// Info 记录信息日志
func Info(format string, v ...interface{}) {
	logWithLevel(levelInfo, "INFO", format, v...)
}

// Error 记录错误日志
func Error(format string, v ...interface{}) {
	logWithLevel(levelError, "ERROR", format, v...)
}

// Warn 记录警告日志
func Warn(format string, v ...interface{}) {
	logWithLevel(levelWarn, "WARN", format, v...)
}

// Debug 记录调试日志
func Debug(format string, v ...interface{}) {
	logWithLevel(levelDebug, "DEBUG", format, v...)
}

// SetLogLevel 设置日志级别
func SetLogLevel(level string) {
	logLevel.Store(parseLogLevel(level))
}

// Close 释放底层日志文件句柄
func Close() error {
	logger := activeLogger.Swap(nil)
	if logger == nil || logger.file == nil {
		return nil
	}
	return logger.file.Close()
}

// logWithLevel 用于按级别统一输出日志
func logWithLevel(level int32, levelLabel, format string, v ...interface{}) {
	// 通过阈值过滤低级别日志
	if level < logLevel.Load() {
		return
	}

	msg := fmt.Sprintf(format, v...)
	entry := fmt.Sprintf("level=%s msg=%q", levelLabel, msg)

	if logger := activeLogger.Load(); logger != nil && logger.logger != nil {
		logger.logger.Print(entry)
		return
	}
	log.Print(entry)
}

// parseLogLevel 用于解析输入参数或配置
func parseLogLevel(level string) int32 {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return levelDebug
	case "warn", "warning":
		return levelWarn
	case "error":
		return levelError
	default:
		return levelInfo
	}
}

// 重新初始化或关闭时清理旧的日志资源
func closeActiveLogger() {
	// 原子地把全局指针设为 nil，并返回“设前”的旧指针
	logger := activeLogger.Swap(nil)
	if logger == nil || logger.file == nil {
		return
	}
	_ = logger.file.Close() // 关闭文件句柄
}
