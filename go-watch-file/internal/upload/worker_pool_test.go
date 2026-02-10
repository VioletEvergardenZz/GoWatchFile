// 本文件用于上传工作池持久化队列测试
package upload

import (
	"context"
	"sync"
	"testing"
	"time"
)

type mockQueueStore struct {
	mu                 sync.Mutex
	items              []string
	removeOneCalls     int
	removeLastOneCalls int
}

func (m *mockQueueStore) Enqueue(item string) error {
	m.mu.Lock()
	m.items = append(m.items, item)
	m.mu.Unlock()
	return nil
}

func (m *mockQueueStore) RemoveOne(item string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeOneCalls++
	for idx, val := range m.items {
		if val == item {
			next := make([]string, 0, len(m.items)-1)
			next = append(next, m.items[:idx]...)
			next = append(next, m.items[idx+1:]...)
			m.items = next
			return true, nil
		}
	}
	return false, nil
}

func (m *mockQueueStore) RemoveLastOne(item string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeLastOneCalls++
	for idx := len(m.items) - 1; idx >= 0; idx-- {
		if m.items[idx] == item {
			next := make([]string, 0, len(m.items)-1)
			next = append(next, m.items[:idx]...)
			next = append(next, m.items[idx+1:]...)
			m.items = next
			return true, nil
		}
	}
	return false, nil
}

func (m *mockQueueStore) Items() []string {
	m.mu.Lock()
	out := append([]string(nil), m.items...)
	m.mu.Unlock()
	return out
}

func (m *mockQueueStore) RemoveCalls() (int, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.removeOneCalls, m.removeLastOneCalls
}

func TestWorkerPool_PersistQueueSuccessAck(t *testing.T) {
	store := &mockQueueStore{}
	pool, err := NewWorkerPool(1, 10, func(ctx context.Context, filePath string) error {
		return nil
	}, nil, store)
	if err != nil {
		t.Fatalf("new pool failed: %v", err)
	}
	defer pool.ShutdownNow()

	if err := pool.AddFile("a.log"); err != nil {
		t.Fatalf("add file failed: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool {
		return len(store.Items()) == 0
	}, "expected persisted item acked")
}

func TestWorkerPool_PersistQueueFailureKeepsItem(t *testing.T) {
	store := &mockQueueStore{}
	pool, err := NewWorkerPool(1, 10, func(ctx context.Context, filePath string) error {
		return context.DeadlineExceeded
	}, nil, store)
	if err != nil {
		t.Fatalf("new pool failed: %v", err)
	}
	defer pool.ShutdownNow()

	if err := pool.AddFile("a.log"); err != nil {
		t.Fatalf("add file failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if got := len(store.Items()); got != 1 {
		t.Fatalf("expected persisted item kept on failure, got %d", got)
	}
}

func TestWorkerPool_RecoverPersistedItems(t *testing.T) {
	store := &mockQueueStore{items: []string{"a.log", "b.log"}}
	var mu sync.Mutex
	processed := make([]string, 0, 2)

	pool, err := NewWorkerPool(1, 10, func(ctx context.Context, filePath string) error {
		mu.Lock()
		processed = append(processed, filePath)
		mu.Unlock()
		return nil
	}, nil, store)
	if err != nil {
		t.Fatalf("new pool failed: %v", err)
	}
	defer pool.ShutdownNow()

	waitUntil(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(processed) >= 2
	}, "expected recovered items consumed")

	if got := len(store.Items()); got != 0 {
		t.Fatalf("expected recovered items acked, got %d", got)
	}
}

func TestWorkerPool_QueueFullRollbackUsesRemoveLastOne(t *testing.T) {
	store := &mockQueueStore{}
	block := make(chan struct{})

	pool, err := NewWorkerPool(1, 1, func(ctx context.Context, filePath string) error {
		select {
		case <-block:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, nil, store)
	if err != nil {
		t.Fatalf("new pool failed: %v", err)
	}
	defer func() {
		close(block)
		pool.ShutdownNow()
	}()

	if err := pool.AddFile("first.log"); err != nil {
		t.Fatalf("add first file failed: %v", err)
	}
	waitUntil(t, 2*time.Second, func() bool {
		return pool.GetStats().InFlight == 1
	}, "expected first task in flight")

	if err := pool.AddFile("dup.log"); err != nil {
		t.Fatalf("add second file failed: %v", err)
	}
	if err := pool.AddFile("dup.log"); err != ErrQueueFull {
		t.Fatalf("expected ErrQueueFull, got %v", err)
	}
	removeOneCalls, removeLastCalls := store.RemoveCalls()
	if removeLastCalls == 0 {
		t.Fatalf("expected RemoveLastOne called on rollback, got %d", removeLastCalls)
	}
	if removeOneCalls != 0 {
		t.Fatalf("expected RemoveOne not called before worker ack, got %d", removeOneCalls)
	}
}

func waitUntil(t *testing.T, timeout time.Duration, fn func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf(message)
}
