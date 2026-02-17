// 本文件用于 Prometheus 指标聚合与导出 将运行时指标统一收口便于监控接入

package metrics

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"file-watch/internal/models"
)

// Collector 聚合运行期指标，并以 Prometheus 文本格式输出。
type Collector struct {
	queueLength atomic.Int64
	workers     atomic.Int64
	inFlight    atomic.Int64

	fileEventsTotal    atomic.Uint64
	queueFullTotal     atomic.Uint64
	queueShedTotal     atomic.Uint64
	uploadRetryTotal   atomic.Uint64
	uploadSuccessTotal atomic.Uint64
	uploadFailureTotal atomic.Uint64

	aiRetryTotal atomic.Uint64

	kbSearchTotal      atomic.Uint64
	kbSearchHitTotal   atomic.Uint64
	kbAskTotal         atomic.Uint64
	kbAskCitationTotal atomic.Uint64

	mu                     sync.RWMutex
	uploadFailuresByReason map[string]uint64
	aiByOutcome            map[string]uint64
	uploadDurationSec      *histogram
	aiDurationSec          *histogram
	kbReviewLatencyMs      *histogram
}

type histogram struct {
	buckets []float64
	counts  []uint64 // 累计桶计数
	count   uint64
	sum     float64
}

var (
	globalCollector = NewCollector()
)

// Global 返回进程级全局指标收集器。
func Global() *Collector {
	return globalCollector
}

// NewCollector 创建指标收集器。
func NewCollector() *Collector {
	return &Collector{
		uploadFailuresByReason: make(map[string]uint64),
		aiByOutcome:            make(map[string]uint64),
		uploadDurationSec:      newHistogram([]float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30}),
		aiDurationSec:          newHistogram([]float64{0.2, 0.5, 1, 2, 3, 5, 8, 13, 20, 30}),
		kbReviewLatencyMs:      newHistogram([]float64{50, 100, 200, 300, 500, 800, 1200, 2000, 5000}),
	}
}

func newHistogram(buckets []float64) *histogram {
	clean := make([]float64, 0, len(buckets))
	for _, bucket := range buckets {
		if bucket <= 0 {
			continue
		}
		clean = append(clean, bucket)
	}
	sort.Float64s(clean)
	return &histogram{
		buckets: clean,
		counts:  make([]uint64, len(clean)),
	}
}

func (h *histogram) observe(v float64) {
	if h == nil {
		return
	}
	for idx, bound := range h.buckets {
		if v <= bound {
			h.counts[idx]++
		}
	}
	h.count++
	h.sum += v
}

func (h *histogram) writePrometheus(builder *strings.Builder, metric string, labels map[string]string) {
	if h == nil {
		return
	}
	for idx, bound := range h.buckets {
		bucketLabels := mergeLabels(labels, map[string]string{
			"le": trimFloat(bound),
		})
		builder.WriteString(metric)
		builder.WriteString("_bucket")
		writeLabels(builder, bucketLabels)
		builder.WriteByte(' ')
		builder.WriteString(strconv.FormatUint(h.counts[idx], 10))
		builder.WriteByte('\n')
	}
	infLabels := mergeLabels(labels, map[string]string{
		"le": "+Inf",
	})
	builder.WriteString(metric)
	builder.WriteString("_bucket")
	writeLabels(builder, infLabels)
	builder.WriteByte(' ')
	builder.WriteString(strconv.FormatUint(h.count, 10))
	builder.WriteByte('\n')

	builder.WriteString(metric)
	builder.WriteString("_sum")
	writeLabels(builder, labels)
	builder.WriteByte(' ')
	builder.WriteString(trimFloat(h.sum))
	builder.WriteByte('\n')

	builder.WriteString(metric)
	builder.WriteString("_count")
	writeLabels(builder, labels)
	builder.WriteByte(' ')
	builder.WriteString(strconv.FormatUint(h.count, 10))
	builder.WriteByte('\n')
}

// SetQueueStats 刷新队列长度和并发工作数。
func (c *Collector) SetQueueStats(stats models.UploadStats) {
	if c == nil {
		return
	}
	c.queueLength.Store(int64(stats.QueueLength))
	c.workers.Store(int64(stats.Workers))
	c.inFlight.Store(int64(stats.InFlight))
}

// IncFileEvent 记录一次文件入队事件。
func (c *Collector) IncFileEvent() {
	if c == nil {
		return
	}
	c.fileEventsTotal.Add(1)
}

// IncQueueFull 记录队列已满次数。
func (c *Collector) IncQueueFull() {
	if c == nil {
		return
	}
	c.queueFullTotal.Add(1)
}

// IncQueueShed 记录队列饱和时的主动限流次数。
func (c *Collector) IncQueueShed() {
	if c == nil {
		return
	}
	c.queueShedTotal.Add(1)
}

// IncUploadRetry 记录上传重试次数。
func (c *Collector) IncUploadRetry() {
	if c == nil {
		return
	}
	c.uploadRetryTotal.Add(1)
}

// ObserveUploadSuccess 记录上传成功与耗时。
func (c *Collector) ObserveUploadSuccess(latency time.Duration) {
	if c == nil {
		return
	}
	c.uploadSuccessTotal.Add(1)
	c.mu.Lock()
	c.uploadDurationSec.observe(latency.Seconds())
	c.mu.Unlock()
}

// ObserveUploadFailure 记录上传失败原因。
func (c *Collector) ObserveUploadFailure(reason string) {
	if c == nil {
		return
	}
	c.uploadFailureTotal.Add(1)
	key := normalizeMetricLabel(reason)
	c.mu.Lock()
	c.uploadFailuresByReason[key]++
	c.mu.Unlock()
}

// ObserveAILogSummary 记录 AI 日志分析结果与耗时。
func (c *Collector) ObserveAILogSummary(outcome string, latency time.Duration, retries int) {
	if c == nil {
		return
	}
	label := normalizeMetricLabel(outcome)
	c.mu.Lock()
	c.aiByOutcome[label]++
	c.aiDurationSec.observe(latency.Seconds())
	c.mu.Unlock()
	if retries > 0 {
		c.aiRetryTotal.Add(uint64(retries))
	}
}

// ObserveKBSearch 记录知识检索调用与命中。
func (c *Collector) ObserveKBSearch(hitCount int) {
	if c == nil {
		return
	}
	c.kbSearchTotal.Add(1)
	if hitCount > 0 {
		c.kbSearchHitTotal.Add(1)
	}
}

// ObserveKBAsk 记录知识问答调用与引用约束命中。
func (c *Collector) ObserveKBAsk(citationCount int) {
	if c == nil {
		return
	}
	c.kbAskTotal.Add(1)
	if citationCount > 0 {
		c.kbAskCitationTotal.Add(1)
	}
}

// ObserveKBReviewLatency 记录知识评审动作耗时（毫秒）。
func (c *Collector) ObserveKBReviewLatency(latency time.Duration) {
	if c == nil {
		return
	}
	ms := float64(latency.Milliseconds())
	c.mu.Lock()
	c.kbReviewLatencyMs.observe(ms)
	c.mu.Unlock()
}

// RenderPrometheus 以 text exposition 格式导出指标。
func (c *Collector) RenderPrometheus() string {
	if c == nil {
		return ""
	}
	builder := strings.Builder{}
	builder.Grow(4096)

	writeMetricHeader(&builder, "gwf_file_events_total", "counter", "Total file enqueue events observed by the watcher.")
	writeCounter(&builder, "gwf_file_events_total", c.fileEventsTotal.Load(), nil)

	writeMetricHeader(&builder, "gwf_upload_queue_length", "gauge", "Current upload queue length.")
	writeGaugeInt(&builder, "gwf_upload_queue_length", c.queueLength.Load(), nil)

	writeMetricHeader(&builder, "gwf_upload_inflight", "gauge", "Current in-flight upload tasks.")
	writeGaugeInt(&builder, "gwf_upload_inflight", c.inFlight.Load(), nil)

	writeMetricHeader(&builder, "gwf_upload_workers", "gauge", "Current upload workers.")
	writeGaugeInt(&builder, "gwf_upload_workers", c.workers.Load(), nil)

	writeMetricHeader(&builder, "gwf_upload_queue_full_total", "counter", "Total enqueue failures due to full queue.")
	writeCounter(&builder, "gwf_upload_queue_full_total", c.queueFullTotal.Load(), nil)

	writeMetricHeader(&builder, "gwf_upload_queue_shed_total", "counter", "Total files rejected by queue saturation circuit breaker.")
	writeCounter(&builder, "gwf_upload_queue_shed_total", c.queueShedTotal.Load(), nil)

	writeMetricHeader(&builder, "gwf_upload_retry_total", "counter", "Total upload retry attempts.")
	writeCounter(&builder, "gwf_upload_retry_total", c.uploadRetryTotal.Load(), nil)

	writeMetricHeader(&builder, "gwf_upload_success_total", "counter", "Total successful uploads.")
	writeCounter(&builder, "gwf_upload_success_total", c.uploadSuccessTotal.Load(), nil)

	writeMetricHeader(&builder, "gwf_upload_failure_total", "counter", "Total failed uploads.")
	writeCounter(&builder, "gwf_upload_failure_total", c.uploadFailureTotal.Load(), nil)

	uploadFailureByReason := make(map[string]uint64)
	aiByOutcome := make(map[string]uint64)
	var uploadDurationCopy histogram
	var aiDurationCopy histogram
	var kbReviewCopy histogram
	c.mu.RLock()
	for reason, count := range c.uploadFailuresByReason {
		uploadFailureByReason[reason] = count
	}
	for outcome, count := range c.aiByOutcome {
		aiByOutcome[outcome] = count
	}
	uploadDurationCopy = cloneHistogram(c.uploadDurationSec)
	aiDurationCopy = cloneHistogram(c.aiDurationSec)
	kbReviewCopy = cloneHistogram(c.kbReviewLatencyMs)
	c.mu.RUnlock()

	writeMetricHeader(&builder, "gwf_upload_failure_reason_total", "counter", "Upload failures grouped by reason.")
	reasons := sortedStringKeysFromUintMap(uploadFailureByReason)
	for _, reason := range reasons {
		writeCounter(&builder, "gwf_upload_failure_reason_total", uploadFailureByReason[reason], map[string]string{
			"reason": reason,
		})
	}

	writeMetricHeader(&builder, "gwf_upload_duration_seconds", "histogram", "Upload latency distribution in seconds.")
	uploadDurationCopy.writePrometheus(&builder, "gwf_upload_duration_seconds", nil)

	writeMetricHeader(&builder, "gwf_ai_log_summary_total", "counter", "Total AI log summary requests grouped by outcome.")
	// 始终输出 success/degraded 两个 outcome，避免零流量时缺失时序导致巡检误报
	if _, ok := aiByOutcome["success"]; !ok {
		aiByOutcome["success"] = 0
	}
	if _, ok := aiByOutcome["degraded"]; !ok {
		aiByOutcome["degraded"] = 0
	}
	outcomes := sortedStringKeysFromUintMap(aiByOutcome)
	for _, outcome := range outcomes {
		writeCounter(&builder, "gwf_ai_log_summary_total", aiByOutcome[outcome], map[string]string{
			"outcome": outcome,
		})
	}

	writeMetricHeader(&builder, "gwf_ai_log_summary_retry_total", "counter", "Total retry attempts for AI log summary.")
	writeCounter(&builder, "gwf_ai_log_summary_retry_total", c.aiRetryTotal.Load(), nil)

	writeMetricHeader(&builder, "gwf_ai_log_summary_duration_seconds", "histogram", "AI log summary latency distribution in seconds.")
	aiDurationCopy.writePrometheus(&builder, "gwf_ai_log_summary_duration_seconds", nil)

	kbSearchTotal := c.kbSearchTotal.Load()
	kbSearchHitTotal := c.kbSearchHitTotal.Load()
	kbAskTotal := c.kbAskTotal.Load()
	kbAskCitationTotal := c.kbAskCitationTotal.Load()

	writeMetricHeader(&builder, "gwf_kb_search_total", "counter", "Total knowledge base search requests.")
	writeCounter(&builder, "gwf_kb_search_total", kbSearchTotal, nil)

	writeMetricHeader(&builder, "gwf_kb_search_hit_total", "counter", "Total knowledge base searches with at least one hit.")
	writeCounter(&builder, "gwf_kb_search_hit_total", kbSearchHitTotal, nil)

	writeMetricHeader(&builder, "gwf_kb_search_hit_ratio", "gauge", "Knowledge base search hit ratio.")
	writeGaugeFloat(&builder, "gwf_kb_search_hit_ratio", safeRatio(kbSearchHitTotal, kbSearchTotal), nil)

	writeMetricHeader(&builder, "gwf_kb_ask_total", "counter", "Total knowledge base ask requests.")
	writeCounter(&builder, "gwf_kb_ask_total", kbAskTotal, nil)

	writeMetricHeader(&builder, "gwf_kb_ask_citation_total", "counter", "Total knowledge base ask requests with citations.")
	writeCounter(&builder, "gwf_kb_ask_citation_total", kbAskCitationTotal, nil)

	writeMetricHeader(&builder, "gwf_kb_ask_citation_ratio", "gauge", "Knowledge base ask citation ratio.")
	writeGaugeFloat(&builder, "gwf_kb_ask_citation_ratio", safeRatio(kbAskCitationTotal, kbAskTotal), nil)

	writeMetricHeader(&builder, "gwf_kb_review_latency_ms", "histogram", "Knowledge base review action latency distribution in milliseconds.")
	kbReviewCopy.writePrometheus(&builder, "gwf_kb_review_latency_ms", nil)

	return builder.String()
}

func cloneHistogram(h *histogram) histogram {
	if h == nil {
		return histogram{}
	}
	copyHist := histogram{
		buckets: append([]float64(nil), h.buckets...),
		counts:  append([]uint64(nil), h.counts...),
		count:   h.count,
		sum:     h.sum,
	}
	return copyHist
}

func writeMetricHeader(builder *strings.Builder, metric, metricType, help string) {
	builder.WriteString("# HELP ")
	builder.WriteString(metric)
	builder.WriteByte(' ')
	builder.WriteString(help)
	builder.WriteByte('\n')
	builder.WriteString("# TYPE ")
	builder.WriteString(metric)
	builder.WriteByte(' ')
	builder.WriteString(metricType)
	builder.WriteByte('\n')
}

func writeCounter(builder *strings.Builder, metric string, value uint64, labels map[string]string) {
	builder.WriteString(metric)
	writeLabels(builder, labels)
	builder.WriteByte(' ')
	builder.WriteString(strconv.FormatUint(value, 10))
	builder.WriteByte('\n')
}

func writeGaugeInt(builder *strings.Builder, metric string, value int64, labels map[string]string) {
	builder.WriteString(metric)
	writeLabels(builder, labels)
	builder.WriteByte(' ')
	builder.WriteString(strconv.FormatInt(value, 10))
	builder.WriteByte('\n')
}

func writeGaugeFloat(builder *strings.Builder, metric string, value float64, labels map[string]string) {
	builder.WriteString(metric)
	writeLabels(builder, labels)
	builder.WriteByte(' ')
	builder.WriteString(trimFloat(value))
	builder.WriteByte('\n')
}

func writeLabels(builder *strings.Builder, labels map[string]string) {
	if len(labels) == 0 {
		return
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	builder.WriteByte('{')
	for idx, key := range keys {
		if idx > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(key)
		builder.WriteString("=\"")
		builder.WriteString(escapeLabelValue(labels[key]))
		builder.WriteByte('"')
	}
	builder.WriteByte('}')
}

func mergeLabels(base, ext map[string]string) map[string]string {
	if len(base) == 0 && len(ext) == 0 {
		return nil
	}
	merged := make(map[string]string, len(base)+len(ext))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range ext {
		merged[key] = value
	}
	return merged
}

func normalizeMetricLabel(value string) string {
	clean := strings.TrimSpace(strings.ToLower(value))
	if clean == "" {
		return "unknown"
	}
	clean = strings.ReplaceAll(clean, "\n", " ")
	clean = strings.ReplaceAll(clean, "\r", " ")
	clean = strings.ReplaceAll(clean, "\t", " ")
	clean = strings.Join(strings.Fields(clean), " ")
	if len(clean) > 120 {
		clean = clean[:120]
	}
	return clean
}

func escapeLabelValue(value string) string {
	replacer := strings.NewReplacer(
		`\\`, `\\\\`,
		`"`, `\"`,
		"\n", `\n`,
	)
	return replacer.Replace(value)
}

func sortedStringKeysFromUintMap(items map[string]uint64) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func trimFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func safeRatio(hit, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(hit) / float64(total)
}

// ResetForTest 仅用于测试，避免跨用例污染。
func (c *Collector) ResetForTest() {
	if c == nil {
		return
	}
	c.queueLength.Store(0)
	c.workers.Store(0)
	c.inFlight.Store(0)
	c.fileEventsTotal.Store(0)
	c.queueFullTotal.Store(0)
	c.queueShedTotal.Store(0)
	c.uploadRetryTotal.Store(0)
	c.uploadSuccessTotal.Store(0)
	c.uploadFailureTotal.Store(0)
	c.aiRetryTotal.Store(0)
	c.kbSearchTotal.Store(0)
	c.kbSearchHitTotal.Store(0)
	c.kbAskTotal.Store(0)
	c.kbAskCitationTotal.Store(0)

	c.mu.Lock()
	c.uploadFailuresByReason = make(map[string]uint64)
	c.aiByOutcome = make(map[string]uint64)
	c.uploadDurationSec = newHistogram([]float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30})
	c.aiDurationSec = newHistogram([]float64{0.2, 0.5, 1, 2, 3, 5, 8, 13, 20, 30})
	c.kbReviewLatencyMs = newHistogram([]float64{50, 100, 200, 300, 500, 800, 1200, 2000, 5000})
	c.mu.Unlock()
}

// MustGlobalPrometheus 返回全局指标文本，便于在 handler 中直接输出。
func MustGlobalPrometheus() string {
	return Global().RenderPrometheus()
}

// EnsureCollectorForTest 仅用于测试替换全局实例。
func EnsureCollectorForTest(collector *Collector) {
	if collector == nil {
		return
	}
	globalCollector = collector
}

// NewTestCollector 提供带默认配置的测试 Collector。
func NewTestCollector() *Collector {
	collector := NewCollector()
	collector.ResetForTest()
	return collector
}

// SnapshotString 仅用于本地调试。
func (c *Collector) SnapshotString() string {
	if c == nil {
		return ""
	}
	return fmt.Sprintf(
		"queue=%d workers=%d inflight=%d events=%d success=%d failure=%d",
		c.queueLength.Load(),
		c.workers.Load(),
		c.inFlight.Load(),
		c.fileEventsTotal.Load(),
		c.uploadSuccessTotal.Load(),
		c.uploadFailureTotal.Load(),
	)
}
