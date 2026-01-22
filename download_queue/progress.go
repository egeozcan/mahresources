package download_queue

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// ProgressReader wraps an io.Reader and tracks bytes read
// Implements interfaces.File (io.Reader + io.Closer)
type ProgressReader struct {
	reader     io.Reader
	downloaded int64
	onProgress func(downloaded int64)
}

// NewProgressReader creates a new progress-tracking reader
func NewProgressReader(r io.Reader, onProgress func(downloaded int64)) *ProgressReader {
	return &ProgressReader{
		reader:     r,
		onProgress: onProgress,
	}
}

// Read implements io.Reader and tracks progress
func (pr *ProgressReader) Read(p []byte) (n int, err error) {
	n, err = pr.reader.Read(p)
	if n > 0 {
		downloaded := atomic.AddInt64(&pr.downloaded, int64(n))
		if pr.onProgress != nil {
			pr.onProgress(downloaded)
		}
	}
	return n, err
}

// Close implements io.Closer
func (pr *ProgressReader) Close() error {
	if closer, ok := pr.reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// Downloaded returns the total bytes downloaded so far
func (pr *ProgressReader) Downloaded() int64 {
	return atomic.LoadInt64(&pr.downloaded)
}

// TimeoutReaderWithContext wraps an io.Reader with both idle timeout detection
// and context-based cancellation support
type TimeoutReaderWithContext struct {
	reader      io.Reader
	idleTimeout time.Duration
	ctx         context.Context
	done        chan struct{}
	mu          sync.Mutex
	lastRead    time.Time
	err         error
}

// NewTimeoutReaderWithContext creates a new timeout reader with context cancellation
func NewTimeoutReaderWithContext(r io.Reader, idleTimeout time.Duration, ctx context.Context) *TimeoutReaderWithContext {
	tr := &TimeoutReaderWithContext{
		reader:      r,
		idleTimeout: idleTimeout,
		ctx:         ctx,
		lastRead:    time.Now(),
		done:        make(chan struct{}),
	}
	go tr.watchTimeout()
	return tr
}

func (tr *TimeoutReaderWithContext) watchTimeout() {
	checkInterval := tr.idleTimeout / 10
	if checkInterval < 100*time.Millisecond {
		checkInterval = 100 * time.Millisecond
	}
	if checkInterval > time.Second {
		checkInterval = time.Second
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-tr.done:
			return
		case <-tr.ctx.Done():
			tr.mu.Lock()
			tr.err = fmt.Errorf("download cancelled")
			tr.mu.Unlock()
			return
		case <-ticker.C:
			tr.mu.Lock()
			elapsed := time.Since(tr.lastRead)
			if elapsed > tr.idleTimeout {
				tr.err = fmt.Errorf("remote server stopped sending data (idle timeout after %v)", tr.idleTimeout)
				tr.mu.Unlock()
				return
			}
			tr.mu.Unlock()
		}
	}
}

type readResult struct {
	n   int
	err error
}

// Read implements io.Reader with timeout and cancellation support
func (tr *TimeoutReaderWithContext) Read(p []byte) (n int, err error) {
	// Check for existing error or cancellation
	tr.mu.Lock()
	if tr.err != nil {
		err := tr.err
		tr.mu.Unlock()
		return 0, err
	}
	tr.mu.Unlock()

	select {
	case <-tr.ctx.Done():
		return 0, fmt.Errorf("download cancelled")
	default:
	}

	// Run read in goroutine so we can interrupt it on timeout or cancellation
	resultCh := make(chan readResult, 1)
	go func() {
		n, err := tr.reader.Read(p)
		resultCh <- readResult{n, err}
	}()

	// Wait for read to complete, timeout, or cancellation
	for {
		select {
		case result := <-resultCh:
			if result.n > 0 {
				tr.mu.Lock()
				tr.lastRead = time.Now()
				tr.mu.Unlock()
			}
			return result.n, result.err
		case <-tr.ctx.Done():
			return 0, fmt.Errorf("download cancelled")
		case <-tr.done:
			return 0, fmt.Errorf("remote server stopped sending data (idle timeout after %v)", tr.idleTimeout)
		default:
			tr.mu.Lock()
			err := tr.err
			tr.mu.Unlock()
			if err != nil {
				return 0, err
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// Close signals the reader to stop monitoring
func (tr *TimeoutReaderWithContext) Close() error {
	select {
	case <-tr.done:
		// Already closed
	default:
		close(tr.done)
	}
	return nil
}
