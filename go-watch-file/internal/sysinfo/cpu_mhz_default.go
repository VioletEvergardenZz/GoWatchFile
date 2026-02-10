//go:build !darwin
// +build !darwin

// 本文件用于提供非 macOS 的 CPU 频率默认实现
package sysinfo

// detectCPUMHz 用于读取或返回 CPU 主频信息
func detectCPUMHz() float64 {
	return 0
}
