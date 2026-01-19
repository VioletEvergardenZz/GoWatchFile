//go:build darwin
// +build darwin

// 本文件用于 macOS 下 CPU 频率读取
package sysinfo

import "golang.org/x/sys/unix"

func detectCPUMHz() float64 {
	if freq, err := unix.SysctlUint64("hw.cpufrequency"); err == nil && freq > 0 {
		return float64(freq) / 1e6
	}
	return 0
}
