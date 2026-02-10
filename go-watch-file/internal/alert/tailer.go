// 本文件用于日志轮询读取与回调
package alert

import (
	"context"
	"io"
	"os"
	"strings"
	"time"
)

type fileCursor struct {
	offset    int64
	remainder string
	inited    bool
}

// Tailer 负责轮询读取日志文件新增内容
type Tailer struct {
	paths        []string
	interval     time.Duration
	startFromEnd bool
	onLine       func(path, line string)
	onPoll       func(at time.Time, err error)
	cursors      map[string]*fileCursor
}

// NewTailer 创建日志轮询器
func NewTailer(paths []string, interval time.Duration, startFromEnd bool, onLine func(path, line string), onPoll func(at time.Time, err error)) *Tailer {
	return &Tailer{
		paths:        paths,
		interval:     interval,
		startFromEnd: startFromEnd,
		onLine:       onLine,
		onPoll:       onPoll,
		cursors:      make(map[string]*fileCursor),
	}
}

// Run 启动轮询循环
func (t *Tailer) Run(ctx context.Context) {
	if t == nil {
		return
	}
	if t.interval <= 0 {
		t.interval = 2 * time.Second
	}
	t.pollOnce()
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.pollOnce()
		}
	}
}

// pollOnce 用于执行单次轮询并推进状态
func (t *Tailer) pollOnce() {
	now := time.Now()
	var pollErr error
	for _, path := range t.paths {
		if err := t.readFile(path); err != nil && pollErr == nil {
			pollErr = err
		}
	}
	if t.onPoll != nil {
		t.onPoll(now, pollErr)
	}
}

// readFile 用于读取数据
func (t *Tailer) readFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	size := info.Size()
	cursor := t.cursorFor(path)
	if !cursor.inited {
		cursor.inited = true
		if t.startFromEnd {
			// 首次启动可从末尾开始 忽略历史内容
			cursor.offset = size
			cursor.remainder = ""
			return nil
		}
		// 从文件头开始读取历史内容
		cursor.offset = 0
		cursor.remainder = ""
	}
	if size < cursor.offset {
		// 文件截断或轮转时重置游标
		cursor.offset = 0
		cursor.remainder = ""
	}
	if size == cursor.offset {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Seek(cursor.offset, io.SeekStart); err != nil {
		return err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	cursor.offset += int64(len(data))

	content := cursor.remainder + string(data)
	lines := strings.Split(content, "\n")
	if strings.HasSuffix(content, "\n") {
		cursor.remainder = ""
	} else {
		cursor.remainder = lines[len(lines)-1]
		lines = lines[:len(lines)-1]
	}
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if t.onLine != nil {
			t.onLine(path, line)
		}
	}
	return nil
}

// cursorFor 用于生成轮询游标保持增量读取
func (t *Tailer) cursorFor(path string) *fileCursor {
	if cursor, ok := t.cursors[path]; ok {
		return cursor
	}
	cursor := &fileCursor{}
	t.cursors[path] = cursor
	return cursor
}
