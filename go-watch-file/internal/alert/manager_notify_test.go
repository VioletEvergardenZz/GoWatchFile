package alert

import (
	"strings"
	"testing"
	"time"
)

func TestBuildMarkdown_UsesAnalysisAsContent(t *testing.T) {
	payload := NotifyPayload{
		Level:    LevelFatal,
		Rule:     "应用启动失败",
		File:     "/var/log/app.log",
		Message:  "Error starting ApplicationContext",
		Analysis: "故障级别=高；根因判断=启动阶段依赖注入失败；处置建议=检查 Bean 初始化日志",
		Time:     time.Date(2026, 2, 23, 20, 14, 4, 0, time.Local),
	}
	got := buildMarkdown(payload)
	if !strings.Contains(got, "- 内容: 故障级别=高；根因判断=启动阶段依赖注入失败；处置建议=检查 Bean 初始化日志") {
		t.Fatalf("analysis should be shown as content, got: %s", got)
	}
	if !strings.Contains(got, "- 原文: Error starting ApplicationContext") {
		t.Fatalf("raw message should be kept for comparison, got: %s", got)
	}
}

func TestBuildMarkdown_NoAnalysisKeepsRawMessage(t *testing.T) {
	payload := NotifyPayload{
		Level:   LevelSystem,
		Rule:    "系统异常",
		File:    "/var/log/app.log",
		Message: "connection reset by peer",
		Time:    time.Date(2026, 2, 23, 20, 14, 4, 0, time.Local),
	}
	got := buildMarkdown(payload)
	if !strings.Contains(got, "- 内容: connection reset by peer") {
		t.Fatalf("raw message should stay as content when analysis empty, got: %s", got)
	}
	if strings.Contains(got, "- 原文:") {
		t.Fatalf("raw line should not be duplicated when analysis empty, got: %s", got)
	}
}

func TestBuildEmailBody_UsesAnalysisAsContent(t *testing.T) {
	payload := NotifyPayload{
		Level:    LevelFatal,
		Rule:     "应用启动失败",
		File:     "/var/log/app.log",
		Message:  "Error starting ApplicationContext",
		Analysis: "故障级别=高；根因判断=启动阶段依赖注入失败；处置建议=检查 Bean 初始化日志",
		Time:     time.Date(2026, 2, 23, 20, 14, 4, 0, time.Local),
	}
	got := buildEmailBody(payload)
	if !strings.Contains(got, "内容: 故障级别=高；根因判断=启动阶段依赖注入失败；处置建议=检查 Bean 初始化日志") {
		t.Fatalf("analysis should be shown as content in email, got: %s", got)
	}
	if !strings.Contains(got, "原文: Error starting ApplicationContext") {
		t.Fatalf("raw message should be shown in email when analysis exists, got: %s", got)
	}
}
