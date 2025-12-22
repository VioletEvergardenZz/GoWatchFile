package pathutil

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path/filepath"
	"strings"
)

// ErrOutsideBaseDir 表示 fullPath 未落在 baseDir 下的错误。
var ErrOutsideBaseDir = errors.New("文件路径不在监控目录下")

// RelativePath 返回从 baseDir 到 fullPath 的相对路径，使用 / 分隔。
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

// BuildObjectKeyStrict 基于 watchDir 与 filePath 构造稳定的 S3 对象 key。
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

// BuildObjectKeyPermissive 在 RelativePath 失败时会回退到 filePath 的清洗结果。
// 仅在需要兼容遗留逻辑时使用，安全场景请使用 BuildObjectKeyStrict。
func BuildObjectKeyPermissive(watchDir, filePath string) string {
	key, err := BuildObjectKeyStrict(watchDir, filePath)
	if err != nil {
		return trimLeadingSlash(toSlashPath(filePath))
	}
	return key
}

// BuildObjectKey 为兼容旧逻辑的宽松版本，等同于 BuildObjectKeyPermissive。
func BuildObjectKey(watchDir, filePath string) string {
	return BuildObjectKeyPermissive(watchDir, filePath)
}

// ParseAppAndFileName 从 filePath 解析 app 名与文件名(无扩展名)。
// 解析优先级：home 路径 > watchDir 相对路径的首层目录 > 父目录名（尽力而为的兜底，不保证语义正确）。
// app 名为 watchDir 下的第一层目录。
func ParseAppAndFileName(watchDir, filePath string) (string, string, error) {
	fileName := baseNameNoExt(filePath)
	if fileName == "" {
		return "", "", fmt.Errorf("invalid file path: %s", filePath)
	}

	appName := appNameFromHomePath(filePath)
	if appName == "" {
		if rel, err := RelativePath(watchDir, filePath); err == nil {
			appName = appNameFromRelative(rel)
		}
	}
	if appName == "" {
		appName = parentDirName(filePath)
	}
	if appName == "" {
		return "", "", fmt.Errorf("invalid file path: %s", filePath)
	}
	return appName, fileName, nil
}

// BuildDownloadURL 根据 bucket、endpoint 和对象 key 构造下载 URL。
func BuildDownloadURL(endpoint, bucket, objectKey string, forcePathStyle, disableSSL bool) string {
	scheme := "https"
	if disableSSL {
		scheme = "http"
	}

	normalizedScheme, host, basePath := normalizeEndpoint(endpoint)
	if normalizedScheme != "" {
		scheme = normalizedScheme
	}

	rawKey := cleanObjectKey(objectKey)        //未转义的原始 key
	escapedKey := escapeObjectKey(objectKey)   //转义后的 key 

	u := &url.URL{Scheme: scheme}

	rawParts := []string{basePath}
	escapedParts := []string{basePath}

	//forcePathStyle=true → https://host/basePath/bucket/objectKey
	//forcePathStyle=false → https://bucket.host/basePath/objectKey
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

func toSlashPath(input string) string {
	cleaned := filepath.Clean(input)
	cleaned = filepath.ToSlash(cleaned)
	return strings.TrimPrefix(cleaned, "./")
}

func trimLeadingSlash(input string) string {
	return strings.TrimPrefix(input, "/")
}

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
	// 路径的最后一段
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

func escapeObjectKey(objectKey string) string {
	key := cleanObjectKey(objectKey)
	parts := strings.Split(key, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func cleanObjectKey(objectKey string) string {
	return trimLeadingSlash(toSlashPath(objectKey))
}

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
