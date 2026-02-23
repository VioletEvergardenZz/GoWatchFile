package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"file-watch/internal/models"
)

func TestResolveUploadAINotifyTimeout(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want time.Duration
	}{
		{name: "empty uses default", raw: "", want: uploadAINotifyDefaultTimeout},
		{name: "duration string", raw: "5s", want: 5 * time.Second},
		{name: "integer seconds", raw: "6", want: 6 * time.Second},
		{name: "invalid uses default", raw: "bad", want: uploadAINotifyDefaultTimeout},
		{name: "cap by max timeout", raw: "90s", want: uploadAINotifyMaxTimeout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveUploadAINotifyTimeout(tc.raw)
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestParseUploadAINotifySummary_JSON(t *testing.T) {
	raw := `{"summary":"检测到连续超时","severity":"high","suggestion":"检查上游连接池"}`
	got := parseUploadAINotifySummary(raw)

	if !strings.Contains(got, "故障级别=高") {
		t.Fatalf("expected severity high in summary, got: %s", got)
	}
	if !strings.Contains(got, "根因判断=检测到连续超时") {
		t.Fatalf("expected root-cause summary in result, got: %s", got)
	}
	if !strings.Contains(got, "处置建议=检查上游连接池") {
		t.Fatalf("expected ops suggestion in result, got: %s", got)
	}
}

func TestParseUploadAINotifySummary_NonJSONFallback(t *testing.T) {
	raw := "  这是直接文本结果  \n"
	got := parseUploadAINotifySummary(raw)
	if !strings.Contains(got, "故障级别=中") {
		t.Fatalf("expected medium severity fallback, got: %s", got)
	}
	if !strings.Contains(got, "根因判断=这是直接文本结果") {
		t.Fatalf("expected fallback root-cause text, got: %s", got)
	}
}

func TestReadUploadFileLinesForAI(t *testing.T) {
	dir := t.TempDir()
	textPath := filepath.Join(dir, "app.log")
	if err := os.WriteFile(textPath, []byte("line-1\nline-2\nline-3\n"), 0o644); err != nil {
		t.Fatalf("write text file failed: %v", err)
	}

	lines, truncated, err := readUploadFileLinesForAI(textPath, 2)
	if err != nil {
		t.Fatalf("read text file failed: %v", err)
	}
	if !truncated {
		t.Fatalf("expected truncated=true when line limit applied")
	}
	if len(lines) != 2 || lines[0] != "line-2" || lines[1] != "line-3" {
		t.Fatalf("unexpected lines: %#v", lines)
	}

	binPath := filepath.Join(dir, "app.bin")
	if err := os.WriteFile(binPath, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatalf("write binary file failed: %v", err)
	}
	if _, _, err := readUploadFileLinesForAI(binPath, 2); err == nil {
		t.Fatalf("expected binary file to be rejected")
	}
}

func TestBuildUploadAIFallbackReason(t *testing.T) {
	if got := buildUploadAIFallbackReason(context.DeadlineExceeded); !strings.Contains(got, "超时") {
		t.Fatalf("expected timeout fallback, got: %s", got)
	}
	if got := buildUploadAIFallbackReason(errors.New("AI_MODEL不能为空")); !strings.Contains(got, "配置不完整") {
		t.Fatalf("expected config fallback, got: %s", got)
	}
	if got := buildUploadAIFallbackReason(errors.New("仅支持文本文件")); !strings.Contains(got, "非文本") {
		t.Fatalf("expected non-text fallback, got: %s", got)
	}
}

func TestBuildUploadAISummaryWhenAIIsDisabled(t *testing.T) {
	fs := &FileService{
		config: &models.Config{AIEnabled: false},
	}
	got := fs.buildUploadAISummary(context.Background(), "/tmp/not-exists.log")
	if got != "" {
		t.Fatalf("expected empty summary when ai disabled, got: %s", got)
	}
}
