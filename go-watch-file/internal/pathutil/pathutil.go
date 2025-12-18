package pathutil

import (
	"fmt"
	"path/filepath"
	"strings"
)

// RelativePath 返回从 baseDir 到 fullPath 的相对路径，使用 / 分隔。
// 如果 fullPath 不在 baseDir 下则报错。
func RelativePath(baseDir, fullPath string) (string, error) {
	base := filepath.Clean(baseDir)
	full := filepath.Clean(fullPath)

	rel, err := filepath.Rel(base, full)
	if err != nil {
		return "", fmt.Errorf("计算相对路径失败: %w", err)
	}
	if rel == "." || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("文件路径不在监控目录下: %s", fullPath)
	}

	return toSlashPath(rel), nil
}

// BuildObjectKey 基于 watchDir 与 filePath 构造稳定的 S3 对象 key。
// 优先使用 watchDir 下的相对路径，并去掉前导 /。
func BuildObjectKey(watchDir, filePath string) string {
	rel, err := RelativePath(watchDir, filePath)
	if err != nil {
		return trimLeadingSlash(toSlashPath(filePath))
	}
	if filepath.IsAbs(watchDir) {
		prefix := trimLeadingSlash(toSlashPath(watchDir))
		if prefix != "" && prefix != "." && prefix != "/" {
			return trimLeadingSlash(joinURLPath(prefix, rel))
		}
	}
	return trimLeadingSlash(rel)
}



// ParseAppAndFileName 从 filePath 解析 app 名与文件名(无扩展名)。
// app 名为 watchDir 下的第一层目录。
func ParseAppAndFileName(watchDir, filePath string) (string, string, error) {
	fileName := baseNameNoExt(filePath)
	if fileName == "" {
		return "", "", fmt.Errorf("invalid file path: %s", filePath)
	}

	if appName := appNameFromHomePath(filePath); appName != "" {
		return appName, fileName, nil
	}

	if rel, err := RelativePath(watchDir, filePath); err == nil {
		if appName := appNameFromRelative(rel); appName != "" {
			return appName, fileName, nil
		}
	}

	if appName := parentDirName(filePath); appName != "" {
		return appName, fileName, nil
	}

	return "", "", fmt.Errorf("invalid file path: %s", filePath)
}



// BuildDownloadURL 根据 bucket、endpoint 和对象 key 构造下载 URL。
func BuildDownloadURL(endpoint, bucket, objectKey string, forcePathStyle, disableSSL bool) string {
	scheme := "http"
	if !disableSSL {
		scheme = "https"
	}

	key := trimLeadingSlash(toSlashPath(objectKey))
	if forcePathStyle {
		return fmt.Sprintf("%s://%s/%s", scheme, endpoint, joinURLPath(bucket, key))
	}
	return fmt.Sprintf("%s://%s.%s/%s", scheme, bucket, endpoint, key)
}

func toSlashPath(input string) string {
	cleaned := filepath.Clean(input)
	cleaned = filepath.ToSlash(cleaned)
	return strings.TrimPrefix(cleaned, "./")
}

func trimLeadingSlash(input string) string {
	return strings.TrimPrefix(input, "/")
}

func joinURLPath(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, "/")
		if part != "" {
			cleaned = append(cleaned, part)
		}
	}
	return strings.Join(cleaned, "/")
}

func baseNameNoExt(path string) string {
	base := baseName(path)
	if base == "" {
		return ""
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func baseName(path string) string {
	parts := splitPathParts(path)
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func parentDirName(path string) string {
	parts := splitPathParts(path)
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2]
}

func appNameFromRelative(rel string) string {
	parts := splitPathParts(rel)
	if len(parts) < 2 {
		return ""
	}
	return parts[0]
}

func appNameFromHomePath(path string) string {
	parts := splitPathParts(path)
	if len(parts) < 2 || parts[0] != "home" {
		return ""
	}
	return parts[1]
}

func splitPathParts(path string) []string {
	cleaned := strings.TrimSuffix(toSlashPath(path), "/")
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "" || cleaned == "." {
		return nil
	}
	parts := strings.Split(cleaned, "/")
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" && part != "." {
			filtered = append(filtered, part)
		}
	}
	return filtered
}

