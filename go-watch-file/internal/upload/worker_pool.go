// Package upload
/**
实现了一个简单的上传工作池（worker pool）
		内部维护一个带缓冲的 channel 作为任务队列（uploadQueue chan string），存放待处理的文件路径。
		启动固定数量的 worker goroutine（workers），每个 worker 从队列取文件并调用 uploadFunc(filePath) 处理（这里上传逻辑由外部传入）。
		支持添加任务（AddFile），优先非阻塞入队，队满或关闭时返回错误。
		支持优雅关闭（Shutdown）：关闭队列，等待 worker 退出，可选超时兜底。
		提供统计（GetStats）供监控/接口查询。
		设计目标：实现一个并发上传框架，避免在主线程阻塞，控制并发量与队列深度，容错处理上游和下游的差异。
*/
package upload

import (
	"context"
	"errors"
	"sync"
	"time"

	"file-watch/internal/logger"
	"file-watch/internal/models"
)

// WorkerPool 上传工作池结构
/**
uploadQueue chan string：带缓冲 channel，用作任务队列。队列里的每个元素是待上传的文件路径（字符串）。
workers int：并发 worker 数量（goroutine 个数）。
wg sync.WaitGroup：用于等待 worker goroutine 退出，Shutdown 会等待 wg.Wait()。
ctx context.Context：用于通知 workers 停止（通过 cancel() 触发）。
cancel context.CancelFunc：取消函数，调用后 ctx.Done() 会关闭，worker 会通过 <-p.ctx.Done() 判断终止。
uploadFunc func(string) error：外部注入的处理函数，Worker 从队列拿到文件路径后调用该函数执行实际工作（例如 FileService.processFile）。
设计思想：
channel 做队列，worker 使用 select 同时监听队列和 ctx.Done，使得 pool 能够优雅退出。
uploadFunc 提供灵活性；这一点便于测试与依赖注入。
*/
type WorkerPool struct {
	uploadQueue chan string
	workers     int
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	uploadFunc  func(context.Context, string) error // 上传函数

	mu     sync.Mutex
	closed bool
}

var (
	ErrQueueFull     = errors.New("upload queue full")
	ErrPoolClosed    = errors.New("worker pool closed")
	ErrShutdownTimed = errors.New("worker pool shutdown timed out")
	ErrUploadFuncNil = errors.New("upload func is nil")
)

// NewWorkerPool 构造函数，创建并启动 worker goroutine
func NewWorkerPool(workers, queueSize int, uploadFunc func(context.Context, string) error) (*WorkerPool, error) {
	if workers <= 0 {
		workers = 3
	}
	if queueSize <= 0 {
		queueSize = 100
	}
	if uploadFunc == nil {
		return nil, ErrUploadFuncNil
	}
	ctx, cancel := context.WithCancel(context.Background()) //context.WithCancel 提供取消上下文以及 cancel() 函数
	pool := &WorkerPool{
		uploadQueue: make(chan string, queueSize),
		workers:     workers,
		ctx:         ctx,
		cancel:      cancel,
		uploadFunc:  uploadFunc,
	}

	// 启动工作协程
	/**
	为每个 worker 增加 WaitGroup，启动 goroutine 执行 pool.worker(i)。
	使用 id（i）做日志标识，便于分辨哪个 worker 在处理。
	*/
	for i := 0; i < workers; i++ {
		pool.wg.Add(1)
		go pool.worker(i)
	}
	logger.Info("上传工作池已启动，工作协程数: %d, 队列大小: %d", workers, queueSize)
	return pool, nil
}

// worker 工作协程函数
func (p *WorkerPool) worker(id int) {
	//保证 worker 退出时 WaitGroup 会减少计数，Shutdown 中的 wg.Wait() 会等待所有 worker 退出
	defer p.wg.Done()
	logger.Info("上传工作协程 %d 已启动", id)
	for {
		select {
		case filePath, ok := <-p.uploadQueue:
			if !ok {
				logger.Info("上传工作协程 %d 已停止", id)
				return
			}
			logger.Info("工作协程 %d 开始处理文件: %s", id, filePath)
			startTime := time.Now()
			// 执行文件上传
			if err := p.uploadFunc(p.ctx, filePath); err != nil {
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

// AddFile 添加文件到上传队列（非阻塞）。队列已满或已关闭时返回错误。
func (p *WorkerPool) AddFile(filePath string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ErrPoolClosed
	}

	select {
	case p.uploadQueue <- filePath:
		logger.Debug("文件已添加到上传队列: %s", filePath)
		return nil
	default:
		logger.Warn("上传队列已满，无法添加文件: %s", filePath)
		return ErrQueueFull
	}
}

// Shutdown 关闭上传工作池，默认采用“先 drain 队列，再超时兜底”语义。
func (p *WorkerPool) Shutdown() error {
	return p.ShutdownGraceful(0)
}

// ShutdownGraceful 关闭上传工作池：先关闭队列并等待 worker 处理完已有任务。
// timeout > 0 时，超过超时时间会触发 cancel 以尽快退出。
func (p *WorkerPool) ShutdownGraceful(timeout time.Duration) error {
	logger.Info("正在关闭上传工作池...")
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	close(p.uploadQueue)
	p.mu.Unlock()

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	var timedOut bool
	if timeout > 0 {
		select {
		case <-done:
		case <-time.After(timeout):
			timedOut = true
			p.cancel()
			<-done
		}
	} else {
		<-done
	}
	p.cancel()
	logger.Info("上传工作池已关闭")
	if timedOut {
		return ErrShutdownTimed
	}
	return nil
}

// ShutdownNow 立即关闭上传工作池：取消上下文并关闭队列，不保证 drain。
func (p *WorkerPool) ShutdownNow() {
	logger.Warn("立即关闭上传工作池（可能丢弃队列中任务）")
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	close(p.uploadQueue)
	p.mu.Unlock()
	p.cancel()
	p.wg.Wait()
	logger.Info("上传工作池已关闭（立即模式）")
}

// GetStats 获取队列状态
func (p *WorkerPool) GetStats() models.UploadStats {
	return models.UploadStats{
		QueueLength: len(p.uploadQueue),
		Workers:     p.workers,
	}
}
