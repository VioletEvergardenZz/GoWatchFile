package upload

import (
	"context"
	"sync"
	"time"

	"file-watch/internal/logger"
	"file-watch/internal/models"
)

// WorkerPool 上传工作池结构
type WorkerPool struct {
	uploadQueue chan string
	workers     int
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	uploadFunc  func(string) error // 上传函数
}

// NewWorkerPool 创建上传工作池
func NewWorkerPool(workers, queueSize int, uploadFunc func(string) error) *WorkerPool {
	if workers <= 0 {
		workers = 3 // 默认3个工作协程
	}
	if queueSize <= 0 {
		queueSize = 100 // 默认队列大小100
	}

	ctx, cancel := context.WithCancel(context.Background())

	pool := &WorkerPool{
		uploadQueue: make(chan string, queueSize),
		workers:     workers,
		ctx:         ctx,
		cancel:      cancel,
		uploadFunc:  uploadFunc,
	}

	// 启动工作协程
	for i := 0; i < workers; i++ {
		pool.wg.Add(1)
		go pool.worker(i)
	}

	logger.Info("上传工作池已启动，工作协程数: %d, 队列大小: %d", workers, queueSize)
	return pool
}

// worker 工作协程函数
func (p *WorkerPool) worker(id int) {
	defer p.wg.Done()
	logger.Info("上传工作协程 %d 已启动", id)

	for {
		select {
		case filePath := <-p.uploadQueue:
			logger.Info("工作协程 %d 开始处理文件: %s", id, filePath)
			startTime := time.Now()

			// 执行文件上传
			if err := p.uploadFunc(filePath); err != nil {
				logger.Error("工作协程 %d 处理文件失败: %s, 错误: %v", id, filePath, err)
			} else {
				elapsed := time.Since(startTime)
				logger.Info("工作协程 %d 处理文件完成: %s, 耗时: %v", id, filePath, elapsed)
			}

		case <-p.ctx.Done():
			logger.Info("上传工作协程 %d 已停止", id)
			return
		}
	}
}

// AddFile 添加文件到上传队列
func (p *WorkerPool) AddFile(filePath string) bool {
	select {
	case p.uploadQueue <- filePath:
		logger.Debug("文件已添加到上传队列: %s", filePath)
		return true
	default:
		logger.Warn("上传队列已满，无法添加文件: %s", filePath)
		return false
	}
}

// Shutdown 关闭上传工作池
func (p *WorkerPool) Shutdown() {
	logger.Info("正在关闭上传工作池...")
	p.cancel()
	close(p.uploadQueue)
	p.wg.Wait()
	logger.Info("上传工作池已关闭")
}

// GetStats 获取队列状态
func (p *WorkerPool) GetStats() models.UploadStats {
	return models.UploadStats{
		QueueLength: len(p.uploadQueue),
		Workers:     p.workers,
	}
}
