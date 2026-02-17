// 本文件用于指标接口测试 确保 Prometheus 暴露格式和关键字段稳定

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"file-watch/internal/metrics"
)

func TestPrometheusMetrics(t *testing.T) {
	metrics.Global().ResetForTest()
	metrics.Global().IncFileEvent()

	h := &handler{}
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()

	h.prometheusMetrics(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	contentType := rr.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Fatalf("unexpected content-type: %s", contentType)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "gwf_file_events_total 1") {
		t.Fatalf("unexpected metrics body: %s", body)
	}
}
