// 本文件用于文件持久化队列测试
package persistqueue

import (
	"os"
	"path/filepath"
	"strings"
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

func TestFileQueue_RemoveOne(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "queue.json")
	queue, err := NewFileQueue(storePath)
	if err != nil {
		t.Fatalf("create queue failed: %v", err)
	}
	_ = queue.Enqueue("a")
	_ = queue.Enqueue("b")
	_ = queue.Enqueue("a")

	removed, err := queue.RemoveOne("a")
	if err != nil {
		t.Fatalf("remove one failed: %v", err)
	}
	if !removed {
		t.Fatal("expected removed=true")
	}
	items := queue.Items()
	if len(items) != 2 {
		t.Fatalf("expected len 2, got %d", len(items))
	}
	if items[0] != "b" || items[1] != "a" {
		t.Fatalf("unexpected items after remove: %+v", items)
	}

	removed, err = queue.RemoveOne("not-exist")
	if err != nil {
		t.Fatalf("remove missing failed: %v", err)
	}
	if removed {
		t.Fatal("expected removed=false for missing item")
	}
}

func TestFileQueue_RemoveLastOne(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "queue.json")
	queue, err := NewFileQueue(storePath)
	if err != nil {
		t.Fatalf("create queue failed: %v", err)
	}
	_ = queue.Enqueue("a")
	_ = queue.Enqueue("b")
	_ = queue.Enqueue("a")

	removed, err := queue.RemoveLastOne("a")
	if err != nil {
		t.Fatalf("remove last one failed: %v", err)
	}
	if !removed {
		t.Fatal("expected removed=true")
	}
	items := queue.Items()
	if len(items) != 2 {
		t.Fatalf("expected len 2, got %d", len(items))
	}
	if items[0] != "a" || items[1] != "b" {
		t.Fatalf("unexpected items after remove last: %+v", items)
	}
}

func TestFileQueue_CorruptedStoreFallback(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "queue.json")
	if err := os.WriteFile(storePath, []byte("{bad-json"), 0o644); err != nil {
		t.Fatalf("write corrupted file failed: %v", err)
	}

	queue, err := NewFileQueue(storePath)
	if err != nil {
		t.Fatalf("new queue should fallback on corrupted store, got err: %v", err)
	}
	if got := len(queue.Items()); got != 0 {
		t.Fatalf("expected empty queue after fallback, got %d", got)
	}

	backupMatches, err := filepath.Glob(storePath + ".corrupt-*.bak")
	if err != nil {
		t.Fatalf("glob backup files failed: %v", err)
	}
	if len(backupMatches) != 1 {
		t.Fatalf("expected 1 backup file, got %d", len(backupMatches))
	}

	backupData, err := os.ReadFile(backupMatches[0])
	if err != nil {
		t.Fatalf("read backup file failed: %v", err)
	}
	if string(backupData) != "{bad-json" {
		t.Fatalf("unexpected backup content: %s", string(backupData))
	}

	currentData, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("read current store failed: %v", err)
	}
	if !strings.Contains(string(currentData), `"items"`) {
		t.Fatalf("expected rebuilt queue store json, got: %s", string(currentData))
	}
}
