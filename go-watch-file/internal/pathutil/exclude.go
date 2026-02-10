// 本文件用于监控排除规则匹配
package pathutil

import (
	"path/filepath"
	"strings"
)

// ExcludeMatcher 用于判断目录是否应被跳过
type ExcludeMatcher struct {
	absPrefixes     []string
	segmentPatterns []string
	names           map[string]struct{}
}

// NewExcludeMatcher 解析排除目录配置，支持绝对路径、相对路径片段与目录名
func NewExcludeMatcher(raw string) *ExcludeMatcher {
	parts := splitExcludeList(raw)
	if len(parts) == 0 {
		return nil
	}

	absSet := make(map[string]struct{})
	segmentSet := make(map[string]struct{})
	nameSet := make(map[string]struct{})

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		cleaned := filepath.Clean(trimmed)
		if cleaned == "." {
			continue
		}
		normalized := normalizeWatchDirKey(filepath.ToSlash(cleaned))
		normalized = strings.TrimSuffix(normalized, "/")
		if normalized == "" || normalized == "." {
			continue
		}

		if filepath.IsAbs(cleaned) {
			absSet[normalized] = struct{}{}
			continue
		}
		if strings.Contains(normalized, "/") {
			segmentSet[strings.Trim(normalized, "/")] = struct{}{}
			continue
		}
		nameSet[normalized] = struct{}{}
	}

	if len(absSet) == 0 && len(segmentSet) == 0 && len(nameSet) == 0 {
		return nil
	}

	return &ExcludeMatcher{
		absPrefixes:     setToSlice(absSet),
		segmentPatterns: setToSlice(segmentSet),
		names:           nameSet,
	}
}

// IsExcluded 判断路径是否命中排除规则
func (m *ExcludeMatcher) IsExcluded(path string) bool {
	if m == nil || path == "" {
		return false
	}
	norm := normalizeWatchDirKey(filepath.ToSlash(filepath.Clean(path)))
	if norm == "" || norm == "." {
		return false
	}

	for _, prefix := range m.absPrefixes {
		if norm == prefix || strings.HasPrefix(norm, prefix+"/") {
			return true
		}
	}

	for _, pattern := range m.segmentPatterns {
		if pattern == "" {
			continue
		}
		if matchSegmentPattern(norm, pattern) {
			return true
		}
	}

	if len(m.names) == 0 {
		return false
	}
	for _, seg := range strings.Split(norm, "/") {
		if seg == "" {
			continue
		}
		if _, ok := m.names[seg]; ok {
			return true
		}
	}
	return false
}

// splitExcludeList 用于拆分配置字符串并清理空项
func splitExcludeList(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', ';', '\n', '\r':
			return true
		default:
			return false
		}
	})
}

// 判断路径是否命中目录片段
func matchSegmentPattern(path, pattern string) bool {
	pattern = strings.Trim(pattern, "/")
	if pattern == "" {
		return false
	}
	if path == pattern {
		return true
	}
	if strings.HasPrefix(path, pattern+"/") {
		return true
	}
	if strings.Contains(path, "/"+pattern+"/") {
		return true
	}
	if strings.HasSuffix(path, "/"+pattern) {
		return true
	}
	return false
}

// setToSlice 用于将集合转换为稳定切片输出
func setToSlice(input map[string]struct{}) []string {
	out := make([]string, 0, len(input))
	for key := range input {
		out = append(out, key)
	}
	return out
}
