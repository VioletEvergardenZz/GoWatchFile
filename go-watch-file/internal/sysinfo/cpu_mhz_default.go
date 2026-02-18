//go:build !darwin
// +build !darwin

// 本文件用于提供非 macOS 的 CPU 频率默认实现
// 文件职责：实现当前模块的核心业务逻辑与数据流转
// 关键路径：入口参数先校验再执行业务处理 最后返回统一结果
// 边界与容错：异常场景显式返回错误 由上层决定重试或降级

package sysinfo

// detectCPUMHz 用于读取或返回 CPU 主频信息
func detectCPUMHz() float64 {
	return 0
}
