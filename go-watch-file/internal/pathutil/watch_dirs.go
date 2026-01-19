// 本文件用于监控目录拆分与清理
package pathutil

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

// SplitWatchDirs 拆分多目录配置
func SplitWatchDirs(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', ';', '\n', '\r':
			return true
		default:
			return false
		}
	})
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{})
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		cleaned := filepath.Clean(trimmed)
		key := normalizeWatchDirKey(cleaned)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, cleaned)
	}
	return out
}

// RelativePathAny 返回命中的监控目录与相对路径
func RelativePathAny(baseDirs []string, fullPath string) (string, string, error) {
	if len(baseDirs) == 0 {
		return "", "", fmt.Errorf("%w: %s", ErrOutsideBaseDir, fullPath)
	}
	fullResolved, err := resolvePath(fullPath)
	if err != nil {
		return "", "", fmt.Errorf("解析文件路径失败: %w", err)
	}

	bestBase := ""
	bestRel := ""
	bestLen := -1
	for _, baseDir := range baseDirs {
		baseResolved, err := resolvePath(baseDir)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(baseResolved, fullResolved)
		if err != nil {
			continue
		}
		if rel == "." || strings.HasPrefix(rel, "..") {
			continue
		}
		// 选择匹配深度最高的目录
		if len(baseResolved) > bestLen {
			bestLen = len(baseResolved)
			bestBase = baseDir
			bestRel = toSlashPath(rel)
		}
	}
	if bestBase == "" {
		return "", "", fmt.Errorf("%w: %s", ErrOutsideBaseDir, fullPath)
	}
	return bestBase, bestRel, nil
}

// BuildObjectKeyStrictForDirs 基于多目录构建对象 key
func BuildObjectKeyStrictForDirs(watchDirs []string, filePath string) (string, error) {
	baseDir, rel, err := RelativePathAny(watchDirs, filePath)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(baseDir) {
		prefix := trimLeadingSlash(toSlashPath(baseDir))
		if prefix != "" && prefix != "." && prefix != "/" {
			return trimLeadingSlash(joinURLPath(prefix, rel)), nil
		}
	}
	return trimLeadingSlash(rel), nil
}

func normalizeWatchDirKey(path string) string {
	key := filepath.ToSlash(path)
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	return key
}
