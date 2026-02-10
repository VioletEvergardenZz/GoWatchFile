//go:build darwin
// +build darwin

// 本文件用于 macOS 下 CPU 频率读取
package sysinfo

import "golang.org/x/sys/unix"

// detectCPUMHz 用于读取或返回 CPU 主频信息
func detectCPUMHz() float64 {
	if freq, err := unix.SysctlUint64("hw.cpufrequency"); err == nil && freq > 0 {
		return float64(freq) / 1e6
	}
	return 0
}
