// 本文件用于文件持久化队列 spike 测试
package persistqueue

import (
	"path/filepath"
	"testing"
)

func TestFileQueue_PersistAcrossRestart(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "queue.json")

	queue, err := NewFileQueue(storePath)
	if err != nil {
		t.Fatalf("create queue failed: %v", err)
	}
	if err := queue.Enqueue("a.log"); err != nil {
		t.Fatalf("enqueue a failed: %v", err)
	}
	if err := queue.Enqueue("b.log"); err != nil {
		t.Fatalf("enqueue b failed: %v", err)
	}

	reopened, err := NewFileQueue(storePath)
	if err != nil {
		t.Fatalf("reopen queue failed: %v", err)
	}
	items := reopened.Items()
	if len(items) != 2 {
		t.Fatalf("items expected 2, got %d", len(items))
	}
	if items[0] != "a.log" || items[1] != "b.log" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestFileQueue_DequeueOrder(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "queue.json")
	queue, err := NewFileQueue(storePath)
	if err != nil {
		t.Fatalf("create queue failed: %v", err)
	}
	_ = queue.Enqueue("1")
	_ = queue.Enqueue("2")

	first, ok, err := queue.Dequeue()
	if err != nil || !ok {
		t.Fatalf("first dequeue expected ok, got ok=%v err=%v", ok, err)
	}
	if first != "1" {
		t.Fatalf("first expected 1, got %s", first)
	}
	second, ok, err := queue.Dequeue()
	if err != nil || !ok {
		t.Fatalf("second dequeue expected ok, got ok=%v err=%v", ok, err)
	}
	if second != "2" {
		t.Fatalf("second expected 2, got %s", second)
	}
	_, ok, err = queue.Dequeue()
	if err != nil {
		t.Fatalf("third dequeue expected nil err, got %v", err)
	}
	if ok {
		t.Fatalf("third dequeue expected empty queue")
	}
}

func TestFileQueue_Reset(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "queue.json")
	queue, err := NewFileQueue(storePath)
	if err != nil {
		t.Fatalf("create queue failed: %v", err)
	}
	_ = queue.Enqueue("x")
	if err := queue.Reset(); err != nil {
		t.Fatalf("reset failed: %v", err)
	}
	if got := len(queue.Items()); got != 0 {
		t.Fatalf("items expected 0 after reset, got %d", got)
	}
}
