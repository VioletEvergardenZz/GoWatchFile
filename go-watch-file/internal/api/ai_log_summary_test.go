// 本文件用于 AI 日志分析接口测试 通过固定样例确保路径校验和降级行为稳定

package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"file-watch/internal/models"
)

func TestValidateLogPath_AllowsWatchDirPath(t *testing.T) {
	watchDir := t.TempDir()
	logFile := filepath.Join(watchDir, "app.log")
	if err := os.WriteFile(logFile, []byte("line1\nline2"), 0o644); err != nil {
		t.Fatalf("write log file failed: %v", err)
	}

	cfg := &models.Config{
		WatchDir: watchDir,
	}
	got, err := validateLogPath(cfg, logFile)
	if err != nil {
		t.Fatalf("validateLogPath should pass for watch dir file: %v", err)
	}
	if got != logFile {
		t.Fatalf("unexpected validated path: got=%s want=%s", got, logFile)
	}
}

func TestValidateLogPath_AllowsAlertLogPathOutsideWatchDir(t *testing.T) {
	watchDir := t.TempDir()
	alertDir := t.TempDir()
	alertFile := filepath.Join(alertDir, "error.log")
	if err := os.WriteFile(alertFile, []byte("error"), 0o644); err != nil {
		t.Fatalf("write alert log file failed: %v", err)
	}

	cfg := &models.Config{
		WatchDir:      watchDir,
		AlertLogPaths: alertFile,
	}
	got, err := validateLogPath(cfg, alertFile)
	if err != nil {
		t.Fatalf("validateLogPath should allow alert log path: %v", err)
	}
	if got != alertFile {
		t.Fatalf("unexpected validated path: got=%s want=%s", got, alertFile)
	}
}

func TestValidateLogPath_AllowsAlertLogWhenWatchDirEmpty(t *testing.T) {
	alertDir := t.TempDir()
	alertFile := filepath.Join(alertDir, "backend.log")
	if err := os.WriteFile(alertFile, []byte("panic"), 0o644); err != nil {
		t.Fatalf("write alert log file failed: %v", err)
	}

	cfg := &models.Config{
		WatchDir:      "",
		AlertLogPaths: alertFile,
	}
	got, err := validateLogPath(cfg, alertFile)
	if err != nil {
		t.Fatalf("validateLogPath should allow alert log even when watch_dir is empty: %v", err)
	}
	if got != alertFile {
		t.Fatalf("unexpected validated path: got=%s want=%s", got, alertFile)
	}
}

func TestValidateLogPath_RejectsOutsideAllowedPaths(t *testing.T) {
	watchDir := t.TempDir()
	alertDir := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "other.log")
	if err := os.WriteFile(outsideFile, []byte("warn"), 0o644); err != nil {
		t.Fatalf("write outside file failed: %v", err)
	}

	cfg := &models.Config{
		WatchDir:      watchDir,
		AlertLogPaths: filepath.Join(alertDir, "error.log"),
	}
	if _, err := validateLogPath(cfg, outsideFile); err == nil {
		t.Fatal("validateLogPath should reject path outside watch_dir and alert_log_paths")
	}
}

func TestValidateLogPath_RejectsDirectory(t *testing.T) {
	watchDir := t.TempDir()
	cfg := &models.Config{
		WatchDir: watchDir,
	}
	if _, err := validateLogPath(cfg, watchDir); err == nil {
		t.Fatal("validateLogPath should reject directory path")
	}
}

func TestClassifyAIError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "timeout", err: context.DeadlineExceeded, want: "timeout"},
		{name: "rate limit", err: errText("ai response status 429: too many requests"), want: "rate_limit"},
		{name: "auth", err: errText("ai response status 401: unauthorized"), want: "auth"},
		{name: "upstream 5xx", err: errText("ai response status 503: unavailable"), want: "upstream_5xx"},
		{name: "network", err: errText("dial tcp: connection refused"), want: "network"},
		{name: "unknown", err: errText("random error"), want: "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyAIError(tc.err)
			if got != tc.want {
				t.Fatalf("expected %s, got %s", tc.want, got)
			}
		})
	}
}

func TestBuildFallbackAIResult(t *testing.T) {
	lines := []string{
		"2026-02-17 10:00:01 INFO service started",
		"2026-02-17 10:00:03 ERROR connection refused to db",
		"2026-02-17 10:00:05 panic: out of memory",
	}
	result := buildFallbackAIResult(lines, "timeout")
	if result.Summary == "" {
		t.Fatal("fallback summary should not be empty")
	}
	if len(result.KeyErrors) == 0 {
		t.Fatal("fallback key errors should not be empty")
	}
	if result.Severity != "high" {
		t.Fatalf("fallback severity expected high, got %s", result.Severity)
	}
	if result.Confidence == nil {
		t.Fatal("fallback confidence should be set")
	}
}

type errText string

func (e errText) Error() string {
	return string(e)
}
