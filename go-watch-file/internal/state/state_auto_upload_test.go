package state

import (
	"runtime"
	"testing"
	"time"

	"file-watch/internal/models"
)

func runWithTimeout(t *testing.T, fn func() bool) bool {
	t.Helper()
	done := make(chan bool, 1)
	go func() {
		done <- fn()
	}()
	select {
	case v := <-done:
		return v
	case <-time.After(300 * time.Millisecond):
		t.Fatal("auto upload check did not return in time")
		return false
	}
}

func TestRuntimeStateAutoUploadEnabledWindowsPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific path semantics")
	}
	cfg := &models.Config{
		WatchDir: "D:/work/go/GWF/data/gwf-stress",
	}
	s := NewRuntimeState(cfg)

	got := runWithTimeout(t, func() bool {
		return s.AutoUploadEnabled(`D:\work\go\GWF\data\gwf-stress\probe.log`)
	})
	if !got {
		t.Fatalf("expected auto upload enabled for watched child path")
	}
}

func TestAutoEnabledFromCopyWindowsPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific path semantics")
	}
	auto := map[string]bool{
		"D:/work/go/GWF/data/gwf-stress": false,
	}
	got := runWithTimeout(t, func() bool {
		return autoEnabledFromCopy(auto, `D:\work\go\GWF\data\gwf-stress\probe.log`)
	})
	if got {
		t.Fatalf("expected child path to inherit parent auto-upload=false")
	}
}
