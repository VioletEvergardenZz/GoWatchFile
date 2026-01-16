//go:build !darwin
// +build !darwin

package sysinfo

func detectCPUMHz() float64 {
	return 0
}
