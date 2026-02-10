// 本文件用于健康指标统计测试
package service

import (
	"errors"
	"testing"
)

func TestHealthSnapshot_CountersAndReasons(t *testing.T) {
	fs := &FileService{
		failReasons: make(map[string]uint64),
	}

	fs.recordQueueFull()
	fs.recordQueueFull()
	fs.recordRetryAttempt()
	fs.recordRetryAttempt()
	fs.recordRetryAttempt()
	fs.recordUploadFailure(errors.New("network timeout"))
	fs.recordUploadFailure(errors.New("network timeout"))
	fs.recordUploadFailure(errors.New("permission denied"))

	snapshot := fs.HealthSnapshot()
	if snapshot.QueueFullTotal != 2 {
		t.Fatalf("queue full total expected 2, got %d", snapshot.QueueFullTotal)
	}
	if snapshot.RetryTotal != 3 {
		t.Fatalf("retry total expected 3, got %d", snapshot.RetryTotal)
	}
	if snapshot.UploadFailureTotal != 3 {
		t.Fatalf("upload failure total expected 3, got %d", snapshot.UploadFailureTotal)
	}
	if len(snapshot.FailureReasons) != 2 {
		t.Fatalf("failure reasons expected 2, got %d", len(snapshot.FailureReasons))
	}
	if snapshot.FailureReasons[0].Reason != "network timeout" || snapshot.FailureReasons[0].Count != 2 {
		t.Fatalf("top reason expected network timeout x2, got %s x%d", snapshot.FailureReasons[0].Reason, snapshot.FailureReasons[0].Count)
	}
}

func TestNormalizeFailureReason(t *testing.T) {
	if got := normalizeFailureReason(nil); got != "unknown" {
		t.Fatalf("nil error expected unknown, got %s", got)
	}
	if got := normalizeFailureReason(errors.New("   ")); got != "unknown" {
		t.Fatalf("blank reason expected unknown, got %s", got)
	}

	long := "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"
	got := normalizeFailureReason(errors.New(long))
	if len(got) != 120 {
		t.Fatalf("long reason expected 120 chars, got %d", len(got))
	}
}
