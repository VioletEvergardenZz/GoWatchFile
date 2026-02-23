package alert

import (
	"context"
	"strings"
	"testing"
)

func TestFormatAIAlertOpsSummary(t *testing.T) {
	input := aiAlertResult{
		Summary:     "应用启动失败，出现依赖注入异常",
		Severity:    "high",
		Causes:      []string{"数据库连接配置缺失"},
		Suggestions: []string{"先核对配置中心数据库地址与账号"},
	}
	got := formatAIAlertOpsSummary(input)
	want := "故障级别=高；根因判断=数据库连接配置缺失；处置建议=先核对配置中心数据库地址与账号"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestBuildAlertAIFallbackReason(t *testing.T) {
	if got := buildAlertAIFallbackReason(context.DeadlineExceeded); !strings.Contains(got, "超时") {
		t.Fatalf("expected timeout fallback, got: %s", got)
	}
	if got := buildAlertAIFallbackReason(context.Canceled); got == "" {
		t.Fatalf("non-timeout fallback should not be empty")
	}
}
