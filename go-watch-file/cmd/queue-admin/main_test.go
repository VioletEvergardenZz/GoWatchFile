// 本文件用于持久化队列管理命令的测试用例
package main

import (
	"bytes"
	"encoding/json"
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

func TestRunWithArgs_CheckJSONHealthy(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "queue.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runWithArgs([]string{"-action", "check", "-format", "json", "-store", storePath}, &stdout, &stderr)
	if code != exitCodeOK {
		t.Fatalf("check json exit code expected %d, got %d", exitCodeOK, code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr expected empty, got: %s", stderr.String())
	}

	var report checkReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("check json unmarshal failed: %v, raw=%s", err, stdout.String())
	}
	if report.Action != "check" {
		t.Fatalf("action expected check, got %s", report.Action)
	}
	if report.Status != "ok" {
		t.Fatalf("status expected ok, got %s", report.Status)
	}
	if report.Store != storePath {
		t.Fatalf("store expected %s, got %s", storePath, report.Store)
	}
}

func TestRunWithArgs_CheckJSONCorruptedReturnsDegraded(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "queue.json")
	if err := os.WriteFile(storePath, []byte("{bad-json"), 0o644); err != nil {
		t.Fatalf("write corrupted store failed: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runWithArgs([]string{"-action", "check", "-format", "json", "-store", storePath}, &stdout, &stderr)
	if code != exitCodeDegraded {
		t.Fatalf("check json degraded exit code expected %d, got %d", exitCodeDegraded, code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr expected empty, got: %s", stderr.String())
	}

	var report checkReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("check json unmarshal failed: %v, raw=%s", err, stdout.String())
	}
	if report.Status != "degraded" {
		t.Fatalf("status expected degraded, got %s", report.Status)
	}
	if report.Reason == "" {
		t.Fatalf("reason expected non-empty, got empty")
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

func TestRunWithArgs_DoctorJSONHealthy(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "queue.json")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runWithArgs([]string{"-action", "doctor", "-format", "json", "-store", storePath}, &stdout, &stderr)
	if code != exitCodeOK {
		t.Fatalf("doctor json exit code expected %d, got %d", exitCodeOK, code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr expected empty, got: %s", stderr.String())
	}

	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor json unmarshal failed: %v, raw=%s", err, stdout.String())
	}
	if report.Action != "doctor" {
		t.Fatalf("action expected doctor, got %s", report.Action)
	}
	if report.Status != "ok" {
		t.Fatalf("status expected ok, got %s", report.Status)
	}
	if report.Store != storePath {
		t.Fatalf("store expected %s, got %s", storePath, report.Store)
	}
}

func TestRunWithArgs_DoctorJSONCorruptedReturnsDegraded(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "queue.json")
	if err := os.WriteFile(storePath, []byte("{bad-json"), 0o644); err != nil {
		t.Fatalf("write corrupted store failed: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runWithArgs([]string{"-action", "doctor", "-format", "json", "-store", storePath}, &stdout, &stderr)
	if code != exitCodeDegraded {
		t.Fatalf("doctor json degraded exit code expected %d, got %d", exitCodeDegraded, code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr expected empty, got: %s", stderr.String())
	}

	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor json unmarshal failed: %v, raw=%s", err, stdout.String())
	}
	if report.Status != "degraded" {
		t.Fatalf("status expected degraded, got %s", report.Status)
	}
	if report.CorruptFallbackTotal != 1 {
		t.Fatalf("corrupt fallback total expected 1, got %d", report.CorruptFallbackTotal)
	}
	if report.Reason == "" {
		t.Fatalf("reason expected non-empty, got empty")
	}
}

func TestRunWithArgs_CheckStrictOnCorruptedStoreReturnsStoreErr(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "queue.json")
	if err := os.WriteFile(storePath, []byte("{bad-json"), 0o644); err != nil {
		t.Fatalf("write corrupted store failed: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runWithArgs([]string{"-action", "check", "-strict", "-store", storePath}, &stdout, &stderr)
	if code != exitCodeStoreErr {
		t.Fatalf("check strict degraded exit code expected %d, got %d", exitCodeStoreErr, code)
	}
	if !strings.Contains(stdout.String(), "status=degraded") {
		t.Fatalf("stdout expected status=degraded, got: %s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr expected empty, got: %s", stderr.String())
	}
}

func TestRunWithArgs_CheckJSONStrictCorruptedReturnsStoreErr(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "queue.json")
	if err := os.WriteFile(storePath, []byte("{bad-json"), 0o644); err != nil {
		t.Fatalf("write corrupted store failed: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runWithArgs([]string{"-action", "check", "-format", "json", "-strict", "-store", storePath}, &stdout, &stderr)
	if code != exitCodeStoreErr {
		t.Fatalf("check json strict degraded exit code expected %d, got %d", exitCodeStoreErr, code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr expected empty, got: %s", stderr.String())
	}

	var report checkReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("check json unmarshal failed: %v, raw=%s", err, stdout.String())
	}
	if report.Status != "degraded" {
		t.Fatalf("status expected degraded, got %s", report.Status)
	}
}

func TestRunWithArgs_DoctorJSONStrictCorruptedReturnsStoreErr(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "queue.json")
	if err := os.WriteFile(storePath, []byte("{bad-json"), 0o644); err != nil {
		t.Fatalf("write corrupted store failed: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runWithArgs([]string{"-action", "doctor", "-format", "json", "-strict", "-store", storePath}, &stdout, &stderr)
	if code != exitCodeStoreErr {
		t.Fatalf("doctor json strict degraded exit code expected %d, got %d", exitCodeStoreErr, code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr expected empty, got: %s", stderr.String())
	}

	var report doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("doctor json unmarshal failed: %v, raw=%s", err, stdout.String())
	}
	if report.Status != "degraded" {
		t.Fatalf("status expected degraded, got %s", report.Status)
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

func TestRunWithArgs_InvalidFormatReturnsUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runWithArgs([]string{"-action", "doctor", "-format", "xml"}, &stdout, &stderr)
	if code != exitCodeUsage {
		t.Fatalf("invalid format exit code expected %d, got %d", exitCodeUsage, code)
	}
	if !strings.Contains(stderr.String(), "不支持的 -format") {
		t.Fatalf("stderr expected invalid format message, got: %s", stderr.String())
	}
}

func TestRunWithArgs_EnqueueJSONFormatReturnsUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runWithArgs([]string{"-action", "enqueue", "-item", "a.log", "-format", "json"}, &stdout, &stderr)
	if code != exitCodeUsage {
		t.Fatalf("enqueue json format exit code expected %d, got %d", exitCodeUsage, code)
	}
	if !strings.Contains(stderr.String(), "仅支持 check/doctor") {
		t.Fatalf("stderr expected format scope message, got: %s", stderr.String())
	}
}

func TestRunWithArgs_StrictWithEnqueueReturnsUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := runWithArgs([]string{"-action", "enqueue", "-item", "a.log", "-strict"}, &stdout, &stderr)
	if code != exitCodeUsage {
		t.Fatalf("strict with enqueue exit code expected %d, got %d", exitCodeUsage, code)
	}
	if !strings.Contains(stderr.String(), "仅支持 check/doctor") {
		t.Fatalf("stderr expected strict scope message, got: %s", stderr.String())
	}
}
