// 本文件用于告警规则解析的单元测试
package alert

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRules_InvalidYaml(t *testing.T) {
	path := writeRulesFile(t, "::::")
	if _, err := LoadRules(path); err == nil {
		t.Fatal("无效 YAML 应该返回错误")
	}
}

func TestLoadRules_EmptyRules(t *testing.T) {
	path := writeRulesFile(t, "version: 1\nrules: []\n")
	if _, err := LoadRules(path); err == nil || !strings.Contains(err.Error(), "告警规则不能为空") {
		t.Fatalf("期望告警规则不能为空错误，实际: %v", err)
	}
}

func TestLoadRules_InvalidLevel(t *testing.T) {
	content := `version: 1
rules:
  - id: test
    level: unknown
    keywords: ["err"]
`
	path := writeRulesFile(t, content)
	if _, err := LoadRules(path); err == nil || !strings.Contains(err.Error(), "无效的告警级别") {
		t.Fatalf("期望无效告警级别错误，实际: %v", err)
	}
}

func TestLoadRules_MissingKeywords(t *testing.T) {
	content := `version: 1
rules:
  - id: test
    level: system
    keywords: []
`
	path := writeRulesFile(t, content)
	if _, err := LoadRules(path); err == nil || !strings.Contains(err.Error(), "缺少关键字") {
		t.Fatalf("期望缺少关键字错误，实际: %v", err)
	}
}

func writeRulesFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rules.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入规则文件失败: %v", err)
	}
	return path
}
