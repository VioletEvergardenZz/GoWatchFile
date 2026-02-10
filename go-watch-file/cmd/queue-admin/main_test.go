package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWithArgs_CheckHealthy(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "queue.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runWithArgs([]string{"-action", "check", "-store", storePath}, &stdout, &stderr)
	if code != exitCodeOK {
		t.Fatalf("check exit code expected %d, got %d", exitCodeOK, code)
	}
	if !strings.Contains(stdout.String(), "status=ok") {
		t.Fatalf("stdout expected status=ok, got: %s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr expected empty, got: %s", stderr.String())
	}
}

func TestRunWithArgs_DoctorOnCorruptedStoreReturnsDegraded(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "queue.json")
	if err := os.WriteFile(storePath, []byte("{bad-json"), 0o644); err != nil {
		t.Fatalf("write corrupted store failed: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runWithArgs([]string{"-action", "doctor", "-store", storePath}, &stdout, &stderr)
	if code != exitCodeDegraded {
		t.Fatalf("doctor exit code expected %d, got %d", exitCodeDegraded, code)
	}
	if !strings.Contains(stdout.String(), "status=degraded") {
		t.Fatalf("stdout expected status=degraded, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "corruptFallbackTotal=1") {
		t.Fatalf("stdout expected corruptFallbackTotal=1, got: %s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr expected empty, got: %s", stderr.String())
	}
}

func TestRunWithArgs_InvalidActionReturnsUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runWithArgs([]string{"-action", "unknown"}, &stdout, &stderr)
	if code != exitCodeUsage {
		t.Fatalf("invalid action exit code expected %d, got %d", exitCodeUsage, code)
	}
	if !strings.Contains(stderr.String(), "不支持的 action") {
		t.Fatalf("stderr expected unsupported action message, got: %s", stderr.String())
	}
}

func TestRunWithArgs_EnqueueWithoutItemReturnsUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runWithArgs([]string{"-action", "enqueue"}, &stdout, &stderr)
	if code != exitCodeUsage {
		t.Fatalf("enqueue without item exit code expected %d, got %d", exitCodeUsage, code)
	}
	if !strings.Contains(stderr.String(), "必须传入 -item") {
		t.Fatalf("stderr expected missing item message, got: %s", stderr.String())
	}
}

func TestRunWithArgs_EnqueueStoreError(t *testing.T) {
	baseDir := t.TempDir()
	parentFile := filepath.Join(baseDir, "store-parent")
	if err := os.WriteFile(parentFile, []byte("not-a-dir"), 0o644); err != nil {
		t.Fatalf("write parent file failed: %v", err)
	}
	storePath := filepath.Join(parentFile, "queue.json")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runWithArgs([]string{"-action", "enqueue", "-item", "a.log", "-store", storePath}, &stdout, &stderr)
	if code != exitCodeStoreErr {
		t.Fatalf("enqueue store error exit code expected %d, got %d", exitCodeStoreErr, code)
	}
	if !strings.Contains(stderr.String(), "queue-admin 执行失败") {
		t.Fatalf("stderr expected run failure message, got: %s", stderr.String())
	}
}
