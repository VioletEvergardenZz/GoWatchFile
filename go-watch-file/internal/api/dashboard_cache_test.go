// 本文件用于仪表盘缓存行为测试
package api

import (
	"testing"
	"time"
)

func TestDashboardCache_HitAndExpire(t *testing.T) {
	h := &handler{
		dashboardCacheTTL: 50 * time.Millisecond,
	}
	h.storeDashboardCache(map[string]any{"ok": true})

	payload, hit := h.loadDashboardCache()
	if !hit {
		t.Fatal("expected cache hit")
	}
	if payload == nil {
		t.Fatal("expected non-nil payload")
	}

	time.Sleep(80 * time.Millisecond)
	_, hit = h.loadDashboardCache()
	if hit {
		t.Fatal("expected cache expired")
	}
}

func TestDashboardCache_Invalidate(t *testing.T) {
	h := &handler{
		dashboardCacheTTL: time.Second,
	}
	h.storeDashboardCache("cached")
	h.invalidateDashboardCache()
	_, hit := h.loadDashboardCache()
	if hit {
		t.Fatal("expected cache miss after invalidate")
	}
}
