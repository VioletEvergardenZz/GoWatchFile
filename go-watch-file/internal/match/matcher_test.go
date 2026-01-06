package match

import (
	"path/filepath"
	"reflect"
	"testing"
)

// 覆盖后缀解析与匹配逻辑
func TestParseExtList(t *testing.T) {
	exts, err := ParseExtList(".log, .TXT; .gz  .zip")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	want := []string{".log", ".txt", ".gz", ".zip"}
	if !reflect.DeepEqual(exts, want) {
		t.Fatalf("后缀列表不匹配: got=%v want=%v", exts, want)
	}
}

func TestParseExtListInvalid(t *testing.T) {
	if _, err := ParseExtList("log"); err == nil {
		t.Fatalf("期望无效后缀返回错误")
	}
}

func TestMatcherMatchExt(t *testing.T) {
	m := NewMatcher(".log, .txt")
	if !m.IsTargetFile(filepath.Join("/tmp", "a.LOG")) {
		t.Fatalf("期望匹配后缀 .log")
	}
	if m.IsTargetFile(filepath.Join("/tmp", "a.json")) {
		t.Fatalf("期望不匹配后缀 .json")
	}
}
