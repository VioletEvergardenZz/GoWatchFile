// 本文件用于路径规范化与对象 key 生成
package pathutil

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path/filepath"
	"strings"
)

// ErrOutsideBaseDir 表示 fullPath 未落在 baseDir 下的错误
var ErrOutsideBaseDir = errors.New("文件路径不在监控目录下")

// RelativePath 返回从 baseDir 到 fullPath 的相对路径，使用 / 分隔
func RelativePath(baseDir, fullPath string) (string, error) {
	base, err := resolvePath(baseDir)
	if err != nil {
		return "", fmt.Errorf("解析监控目录失败: %w", err)
	}
	full, err := resolvePath(fullPath)
	if err != nil {
		return "", fmt.Errorf("解析文件路径失败: %w", err)
	}

	rel, err := filepath.Rel(base, full)
	if err != nil {
		return "", fmt.Errorf("计算相对路径失败: %w", err)
	}
	// 如果 fullPath 不在 baseDir 下则报错
	if rel == "." || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("%w: %s", ErrOutsideBaseDir, fullPath)
	}

	return toSlashPath(rel), nil
}

// BuildObjectKeyStrict 基于 watchDir 与 filePath 构造稳定的 S3 对象 key
func BuildObjectKeyStrict(watchDir, filePath string) (string, error) {
	rel, err := RelativePath(watchDir, filePath)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(watchDir) {
		prefix := trimLeadingSlash(toSlashPath(watchDir))
		if prefix != "" && prefix != "." && prefix != "/" {
			return trimLeadingSlash(joinURLPath(prefix, rel)), nil
		}
	}
	return trimLeadingSlash(rel), nil
}

// BuildObjectKeyPermissive 在 RelativePath 失败时会回退到 filePath 的清洗结果
// 仅在需要兼容遗留逻辑时使用，安全场景请使用 BuildObjectKeyStrict
func BuildObjectKeyPermissive(watchDir, filePath string) string {
	key, err := BuildObjectKeyStrict(watchDir, filePath)
	if err != nil {
		return trimLeadingSlash(toSlashPath(filePath))
	}
	return key
}

// BuildObjectKey 为兼容旧逻辑的宽松版本，等同于 BuildObjectKeyPermissive
func BuildObjectKey(watchDir, filePath string) string {
	return BuildObjectKeyPermissive(watchDir, filePath)
}

// BuildDownloadURL 根据 bucket、endpoint 和对象 key 构造下载 URL
func BuildDownloadURL(endpoint, bucket, objectKey string, forcePathStyle, disableSSL bool) string {
	scheme := "https"
	if disableSSL {
		scheme = "http"
	}

	normalizedScheme, host, basePath := normalizeEndpoint(endpoint)
	if normalizedScheme != "" {
		scheme = normalizedScheme
	}

	rawKey := cleanObjectKey(objectKey)      //未转义的原始 key
	escapedKey := escapeObjectKey(objectKey) //转义后的 key

	u := &url.URL{Scheme: scheme}

	rawParts := []string{basePath}
	escapedParts := []string{basePath}

	// 强制路径风格时使用 host/basePath/bucket/objectKey 形式
	// 非强制路径风格时使用 bucket.host/basePath/objectKey 形式
	if forcePathStyle {
		rawParts = append(rawParts, bucket, rawKey)
		escapedParts = append(escapedParts, bucket, escapedKey)
		u.Host = host
	} else {
		u.Host = host
		if bucket != "" {
			if host != "" {
				u.Host = fmt.Sprintf("%s.%s", bucket, host)
			} else {
				u.Host = bucket
			}
		}
		rawParts = append(rawParts, rawKey)
		escapedParts = append(escapedParts, escapedKey)
	}

	u.Path = "/" + joinURLPath(rawParts...)
	u.RawPath = "/" + joinURLPath(escapedParts...)
	return u.String()
}

// toSlashPath 用于将路径统一为斜杠格式
func toSlashPath(input string) string {
	cleaned := filepath.Clean(input)
	cleaned = filepath.ToSlash(cleaned)
	return strings.TrimPrefix(cleaned, "./")
}

// trimLeadingSlash 用于移除或清理数据
func trimLeadingSlash(input string) string {
	return strings.TrimPrefix(input, "/")
}

// joinURLPath 用于拼接 URL 路径并避免重复分隔符
func joinURLPath(parts ...string) string {
	//字符串切片
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, "/")
		if part != "" {
			cleaned = append(cleaned, part)
		}
	}
	return strings.Join(cleaned, "/")
}

// resolvePath 返回绝对路径，并解析路径中的符号链接
func resolvePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("路径为空")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// 如果最后一级不存在，允许在解析其父目录后继续拼接最后一级，避免新增文件场景失败
		if errors.Is(err, fs.ErrNotExist) {
			parent := filepath.Dir(abs)
			parentResolved, dirErr := filepath.EvalSymlinks(parent)
			if dirErr != nil {
				return "", err
			}
			return filepath.Join(parentResolved, filepath.Base(abs)), nil
		}
		return "", err
	}
	return resolved, nil
}

// escapeObjectKey 用于转义对象 key 保障 URL 安全
func escapeObjectKey(objectKey string) string {
	key := cleanObjectKey(objectKey)
	parts := strings.Split(key, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

// cleanObjectKey 用于清理对象 key 避免异常路径
func cleanObjectKey(objectKey string) string {
	return trimLeadingSlash(toSlashPath(objectKey))
}

// normalizeEndpoint 用于统一数据格式便于比较与存储
func normalizeEndpoint(endpoint string) (scheme, host, basePath string) {
	cleaned := strings.TrimSpace(endpoint)
	parsed, err := url.Parse(cleaned)
	if err != nil || parsed.Host == "" {
		// 让不带协议的 endpoint 也能被正确当成主机名解析
		parsed, err = url.Parse("//" + cleaned)
		if err != nil {
			return "", cleaned, ""
		}
		return "", parsed.Host, strings.TrimSuffix(parsed.Path, "/")
	}
	return parsed.Scheme, parsed.Host, strings.TrimSuffix(parsed.Path, "/")
}
