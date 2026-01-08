package match

import (
	"path/filepath"
	"strings"
)

// Matcher 负责后缀匹配
// 负责后缀匹配
type Matcher struct {
	extSet          map[string]struct{}
	caseInsensitive bool
}

// NewMatcher 创建后缀匹配器
// 根据后缀构建匹配器
func NewMatcher(fileExt string) *Matcher {
	exts, _ := ParseExtList(fileExt)
	return &Matcher{
		extSet:          buildExtSet(exts),
		caseInsensitive: false,
	}
}

// IsTargetFile 判断路径是否符合后缀规则
// 判断路径是否符合后缀规则
func (m *Matcher) IsTargetFile(filePath string) bool {
	if m == nil || len(m.extSet) == 0 {
		// 未配置后缀表示全量匹配
		return true
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	if m.caseInsensitive {
		// 大小写不敏感系统统一转小写便于比较
		ext = strings.ToLower(ext)
	}
	_, ok := m.extSet[ext]
	return ok
}

// ParseExtList 解析并归一化后缀列表
// 将多后缀字符串拆分并归一化
func ParseExtList(raw string) ([]string, error) {
	parts := splitList(raw)
	// 去重并归一化后缀列表
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{})
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, ".") || trimmed == "." {
			return nil, ErrInvalidExt{Value: trimmed}
		}
		normalized := strings.ToLower(trimmed)
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out, nil
}

// ErrInvalidExt 表示无效后缀
// 表示无效后缀
type ErrInvalidExt struct {
	Value string
}

func (e ErrInvalidExt) Error() string {
	if e.Value == "" {
		return "文件后缀不能为空"
	}
	return "文件后缀必须以 '.' 开头"
}

func buildExtSet(exts []string) map[string]struct{} {
	if len(exts) == 0 {
		return map[string]struct{}{}
	}
	out := make(map[string]struct{}, len(exts))
	for _, ext := range exts {
		out[ext] = struct{}{}
	}
	return out
}

func splitList(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', ';', ' ', '\n', '\r', '\t':
			return true
		default:
			return false
		}
	})
}
