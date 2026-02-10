// 本文件用于上传队列持久化的文件队列实现
package persistqueue

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"file-watch/internal/logger"
)

type fileQueueStore struct {
	Items []string `json:"items"`
}

// HealthStats 表示持久化队列健康指标
type HealthStats struct {
	StoreFile                string
	RecoveredTotal           uint64
	CorruptFallbackTotal     uint64
	PersistWriteFailureTotal uint64
}

// FileQueue 表示文件持久化队列
type FileQueue struct {
	path                     string
	mu                       sync.Mutex
	items                    []string
	recoveredTotal           uint64
	corruptFallbackTotal     uint64
	persistWriteFailureTotal uint64
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
	defer q.mu.Unlock()

	prevItems := append([]string(nil), q.items...)
	q.items = append(q.items, trimmed)
	if err := q.saveLocked(); err != nil {
		q.persistWriteFailureTotal++
		q.items = prevItems
		return err
	}
	return nil
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
	prevItems := append([]string(nil), q.items...)
	item := q.items[0]
	q.items = append([]string(nil), q.items[1:]...)
	if err := q.saveLocked(); err != nil {
		q.persistWriteFailureTotal++
		q.items = prevItems
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
	prevItems := append([]string(nil), q.items...)
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
		q.persistWriteFailureTotal++
		q.items = prevItems
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
	prevItems := append([]string(nil), q.items...)
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
		q.persistWriteFailureTotal++
		q.items = prevItems
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
	prevItems := append([]string(nil), q.items...)
	q.items = []string{}
	err := q.saveLocked()
	if err != nil {
		q.persistWriteFailureTotal++
		q.items = prevItems
	}
	q.mu.Unlock()
	return err
}

// RecordRecovered 记录启动恢复到内存队列的任务数
func (q *FileQueue) RecordRecovered(count int) {
	if q == nil || count <= 0 {
		return
	}
	q.mu.Lock()
	q.recoveredTotal += uint64(count)
	q.mu.Unlock()
}

// HealthStats 返回持久化队列健康指标快照
func (q *FileQueue) HealthStats() HealthStats {
	if q == nil {
		return HealthStats{}
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return HealthStats{
		StoreFile:                q.path,
		RecoveredTotal:           q.recoveredTotal,
		CorruptFallbackTotal:     q.corruptFallbackTotal,
		PersistWriteFailureTotal: q.persistWriteFailureTotal,
	}
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
		return q.fallbackFromCorruptedStoreLocked(data, err)
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
	tmp, err := os.CreateTemp(dir, "persist-queue-*.tmp")
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

func (q *FileQueue) fallbackFromCorruptedStoreLocked(rawData []byte, parseErr error) error {
	q.corruptFallbackTotal++
	backupPath := buildCorruptBackupPath(q.path)
	if err := writeFileAtomic(backupPath, rawData, 0o644); err != nil {
		return fmt.Errorf("解析队列文件失败且备份损坏文件失败: %w", err)
	}

	q.items = []string{}
	if err := q.saveLocked(); err != nil {
		q.persistWriteFailureTotal++
		return fmt.Errorf("队列文件损坏后重建空队列失败: %w", err)
	}

	logger.Error("上传持久化队列文件损坏，已降级为空队列并完成备份: 源文件=%s 备份文件=%s 错误=%v", q.path, backupPath, parseErr)
	return nil
}

func buildCorruptBackupPath(path string) string {
	timestamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	return fmt.Sprintf("%s.corrupt-%s.bak", path, timestamp)
}
