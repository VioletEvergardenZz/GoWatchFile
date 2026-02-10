// 本文件用于队列持久化 spike 的文件队列实现
package persistqueue

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type fileQueueStore struct {
	Items []string `json:"items"`
}

// FileQueue 表示文件持久化队列（PoC）
type FileQueue struct {
	path  string
	mu    sync.Mutex
	items []string
}

// NewFileQueue 创建并加载持久化队列
func NewFileQueue(path string) (*FileQueue, error) {
	cleaned := strings.TrimSpace(path)
	if cleaned == "" {
		return nil, fmt.Errorf("队列文件路径不能为空")
	}
	queue := &FileQueue{
		path:  cleaned,
		items: []string{},
	}
	if err := queue.load(); err != nil {
		return nil, err
	}
	return queue, nil
}

// Enqueue 入队并持久化
func (q *FileQueue) Enqueue(item string) error {
	if q == nil {
		return fmt.Errorf("队列未初始化")
	}
	trimmed := strings.TrimSpace(item)
	if trimmed == "" {
		return fmt.Errorf("入队元素不能为空")
	}
	q.mu.Lock()
	q.items = append(q.items, trimmed)
	err := q.saveLocked()
	q.mu.Unlock()
	return err
}

// Dequeue 出队并持久化
func (q *FileQueue) Dequeue() (string, bool, error) {
	if q == nil {
		return "", false, fmt.Errorf("队列未初始化")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return "", false, nil
	}
	item := q.items[0]
	q.items = append([]string(nil), q.items[1:]...)
	if err := q.saveLocked(); err != nil {
		return "", false, err
	}
	return item, true, nil
}

// RemoveOne 删除一条匹配元素并持久化
func (q *FileQueue) RemoveOne(item string) (bool, error) {
	if q == nil {
		return false, fmt.Errorf("队列未初始化")
	}
	trimmed := strings.TrimSpace(item)
	if trimmed == "" {
		return false, fmt.Errorf("删除元素不能为空")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	idx := -1
	for i, val := range q.items {
		if val == trimmed {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false, nil
	}
	next := make([]string, 0, len(q.items)-1)
	next = append(next, q.items[:idx]...)
	next = append(next, q.items[idx+1:]...)
	q.items = next
	if err := q.saveLocked(); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveLastOne 删除一条最后匹配元素并持久化
func (q *FileQueue) RemoveLastOne(item string) (bool, error) {
	if q == nil {
		return false, fmt.Errorf("队列未初始化")
	}
	trimmed := strings.TrimSpace(item)
	if trimmed == "" {
		return false, fmt.Errorf("删除元素不能为空")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	idx := -1
	for i := len(q.items) - 1; i >= 0; i-- {
		if q.items[i] == trimmed {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false, nil
	}
	next := make([]string, 0, len(q.items)-1)
	next = append(next, q.items[:idx]...)
	next = append(next, q.items[idx+1:]...)
	q.items = next
	if err := q.saveLocked(); err != nil {
		return false, err
	}
	return true, nil
}

// Items 返回当前队列快照
func (q *FileQueue) Items() []string {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	out := append([]string(nil), q.items...)
	q.mu.Unlock()
	return out
}

// Reset 清空队列并持久化
func (q *FileQueue) Reset() error {
	if q == nil {
		return fmt.Errorf("队列未初始化")
	}
	q.mu.Lock()
	q.items = []string{}
	err := q.saveLocked()
	q.mu.Unlock()
	return err
}

func (q *FileQueue) load() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	data, err := os.ReadFile(q.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取队列文件失败: %w", err)
	}
	if len(data) == 0 {
		q.items = []string{}
		return nil
	}
	var store fileQueueStore
	if err := json.Unmarshal(data, &store); err != nil {
		return fmt.Errorf("解析队列文件失败: %w", err)
	}
	q.items = append([]string(nil), store.Items...)
	return nil
}

func (q *FileQueue) saveLocked() error {
	store := fileQueueStore{
		Items: append([]string(nil), q.items...),
	}
	data, err := json.Marshal(store)
	if err != nil {
		return fmt.Errorf("序列化队列失败: %w", err)
	}
	return writeFileAtomic(q.path, data, 0o644)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(dir, "queue-spike-*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(tmp.Name())
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
