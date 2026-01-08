// Package upload
package upload

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"file-watch/internal/logger"
	"file-watch/internal/models"
)

// WorkerPool 上传工作池结构
/**
管理带缓冲的上传任务队列 + 固定数量 worker goroutine 并发消费队列
*/
type WorkerPool struct {
	uploadQueue chan string	     					    //任务队列，每个元素是待处理文件路径 
	workers     int
	inFlight    int64
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	uploadFunc  func(context.Context, string) error		// 外部注入的实际上传逻辑，Worker 从队列取路径后调用
	onStats     func(models.UploadStats)

	mu     sync.Mutex									//保护关闭状态，避免重复关闭或在关闭后入队
	closed bool
}

var (
	// ErrQueueFull 表示上传队列已满
	ErrQueueFull     = errors.New("upload queue full")
	// ErrPoolClosed 表示上传池已经关闭
	ErrPoolClosed    = errors.New("worker pool closed")
	// ErrShutdownTimed 表示上传池关闭超时
	ErrShutdownTimed = errors.New("worker pool shutdown timed out")
	// ErrUploadFuncNil 表示上传处理函数为空
	ErrUploadFuncNil = errors.New("upload func is nil")
)

// NewWorkerPool 构造函数，创建并启动 worker goroutine
func NewWorkerPool(workers, queueSize int, uploadFunc func(context.Context, string) error, onStats func(models.UploadStats)) (*WorkerPool, error) {
	if workers <= 0 {
		workers = 3
	}
	if queueSize <= 0 {
		queueSize = 100
	}
	if uploadFunc == nil {
		return nil, ErrUploadFuncNil
	}
	//基于一个根上下文 context.Background() 创建一个可取消的子上下文 ctx，并拿到对应的取消函数 cancel
	ctx, cancel := context.WithCancel(context.Background()) 
	pool := &WorkerPool{
		uploadQueue: make(chan string, queueSize),
		workers:     workers,
		ctx:         ctx,
		cancel:      cancel,
		uploadFunc:  uploadFunc,
		onStats:     onStats,
	}

	// 启动工作协程
	for i := 0; i < workers; i++ {
		// Add(1) 标记“我有一个新工人上班”；Done() 标记“工人下班了”；Wait() 等到所有工人下班，保证收尾完整
		pool.wg.Add(1)		
		go pool.worker(i)
	}
	logger.Info("上传工作池已启动，工作协程数: %d, 队列大小: %d", workers, queueSize)
	return pool, nil
}

// worker 工作协程函数
func (p *WorkerPool) worker(id int) {
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
			atomic.AddInt64(&p.inFlight, 1)
			startTime := time.Now()
			// 执行文件上传
			if err := p.uploadFunc(p.ctx, filePath); err != nil {
				logger.Error("工作协程 %d 处理文件失败: %s, 错误: %v", id, filePath, err)
			} else {
				elapsed := time.Since(startTime)
				logger.Info("工作协程 %d 处理文件完成: %s, 耗时: %v", id, filePath, elapsed)
			}
			atomic.AddInt64(&p.inFlight, -1)
			if p.onStats != nil {
				p.onStats(p.GetStats())
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

	//等待 worker 全部退出
	//新建一个通知通道 done
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)						//等到 done 关闭（表示所有 worker 退出）
	}()

	var timedOut bool
	if timeout > 0 {
		select {
		case <-done:					//worker 全部退出
		case <-time.After(timeout):		//超时时间先到，说明还没等到 worker 全部退出
			timedOut = true
			p.cancel()					//取消上下文，强制退出
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
		InFlight:    int(atomic.LoadInt64(&p.inFlight)),
	}
}
