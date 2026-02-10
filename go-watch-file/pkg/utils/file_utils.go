// 本文件用于文件与路径相关的通用工具函数
package utils

import (
	"os"
	"path/filepath"
	"strings"
)

// IsFileExists 检查文件是否存在
func IsFileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !os.IsNotExist(err)
}

// IsDirectory 检查路径是否为目录
func IsDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// GetFileExtension 获取文件扩展名
func GetFileExtension(filePath string) string {
	return filepath.Ext(filePath)
}

// GetFileName 获取文件名（不含扩展名）
func GetFileName(filePath string) string {
	base := filepath.Base(filePath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// GetFileSize 获取文件大小
func GetFileSize(filePath string) (int64, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// EnsureDirectoryExists 确保目录存在，如果不存在则创建
func EnsureDirectoryExists(dirPath string) error {
	return os.MkdirAll(dirPath, 0755)
}

// CleanPath 清理路径
func CleanPath(path string) string {
	return filepath.Clean(path)
}

// JoinPath 连接路径
func JoinPath(elem ...string) string {
	return filepath.Join(elem...)
}
