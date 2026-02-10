// 本文件用于提供系统资源格式化与辅助函数
package sysinfo

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
)

// firstIPv4 用于优先提取 IPv4 地址用于展示
func firstIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "--"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil {
				continue
			}
			ip = ip.To4()
			if ip != nil {
				return ip.String()
			}
		}
	}
	return "--"
}

// formatBytes 用于格式化输出内容
func formatBytes(value float64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
		tb = gb * 1024
	)
	switch {
	case value >= tb:
		return fmt.Sprintf("%.1f TB", value/tb)
	case value >= gb:
		return fmt.Sprintf("%.1f GB", value/gb)
	case value >= mb:
		return fmt.Sprintf("%.1f MB", value/mb)
	case value >= kb:
		return fmt.Sprintf("%.1f KB", value/kb)
	case value > 0:
		return fmt.Sprintf("%.0f B", value)
	default:
		return "0 B"
	}
}

// formatRate 用于格式化输出内容
func formatRate(delta uint64, interval time.Duration) string {
	if interval <= 0 {
		return "--"
	}
	perSec := float64(delta) / interval.Seconds()
	return fmt.Sprintf("%s/s", formatBytes(perSec))
}

// formatDurationCN 用于格式化输出内容
func formatDurationCN(d time.Duration) string {
	if d <= 0 {
		return "--"
	}
	totalMinutes := int(d.Minutes())
	if totalMinutes <= 0 {
		return "1分"
	}
	days := totalMinutes / (60 * 24)
	hours := (totalMinutes / 60) % 24
	mins := totalMinutes % 60
	if days > 0 {
		return fmt.Sprintf("%d天 %d小时 %d分", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%d小时 %d分", hours, mins)
	}
	return fmt.Sprintf("%d分", mins)
}

// clampPct 用于限制百分比范围防止异常值扩散
func clampPct(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

// usageTone 用于根据使用率生成状态文案
func usageTone(pct float64) string {
	switch {
	case pct >= 85:
		return "critical"
	case pct >= 70:
		return "warn"
	default:
		return "normal"
	}
}

// normalizeStatus 用于统一数据格式便于比较与存储
func normalizeStatus(raw string) string {
	if raw == "" {
		return "sleeping"
	}
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "running", "r":
		return "running"
	case "sleeping", "s", "idle", "i", "d", "waiting", "w":
		return "sleeping"
	case "stopped", "t", "stop", "stopping", "suspended":
		return "stopped"
	case "zombie", "z":
		return "zombie"
	default:
		if strings.Contains(s, "zombie") {
			return "zombie"
		}
		return "sleeping"
	}
}

// countNonListen 用于统计非监听连接数量
func countNonListen(counts map[string]int) int {
	total := 0
	for status, count := range counts {
		if status == "LISTEN" || status == "LISTENING" {
			continue
		}
		total += count
	}
	return total
}

// formatConnectionBreakdown 用于格式化输出内容
func formatConnectionBreakdown(counts map[string]int) string {
	if len(counts) == 0 {
		return "--"
	}
	order := []string{"ESTABLISHED", "TIME_WAIT", "LISTEN", "CLOSE_WAIT", "SYN_SENT", "SYN_RECV", "FIN_WAIT1", "FIN_WAIT2", "LAST_ACK", "CLOSING"}
	parts := make([]string, 0, 3)
	seen := make(map[string]struct{})
	for _, status := range order {
		if count, ok := counts[status]; ok && count > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", status, count))
			seen[status] = struct{}{}
			if len(parts) >= 3 {
				return strings.Join(parts, " · ")
			}
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, " · ")
	}
	type kv struct {
		key   string
		value int
	}
	rest := make([]kv, 0, len(counts))
	for status, count := range counts {
		if _, ok := seen[status]; ok || count <= 0 {
			continue
		}
		rest = append(rest, kv{key: status, value: count})
	}
	sort.Slice(rest, func(i, j int) bool {
		return rest[i].value > rest[j].value
	})
	for _, item := range rest {
		parts = append(parts, fmt.Sprintf("%s %d", item.key, item.value))
		if len(parts) >= 3 {
			break
		}
	}
	if len(parts) == 0 {
		return "--"
	}
	return strings.Join(parts, " · ")
}

// formatAddr 用于格式化输出内容
func formatAddr(ip string, port uint32) string {
	if port == 0 {
		return ""
	}
	addr := strings.TrimSpace(ip)
	if addr == "" {
		addr = "0.0.0.0"
	}
	if strings.Contains(addr, ":") {
		return fmt.Sprintf("[%s]:%d", addr, port)
	}
	return fmt.Sprintf("%s:%d", addr, port)
}

// uniqueSorted 用于去重并排序字符串列表
func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return values
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, val := range values {
		val = strings.TrimSpace(val)
		if val == "" {
			continue
		}
		if _, ok := seen[val]; ok {
			continue
		}
		seen[val] = struct{}{}
		out = append(out, val)
	}
	sort.Strings(out)
	return out
}
