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
// Implements contracts.File (io.Reader + io.Closer)
type ProgressReader struct {
	reader     io.Reader
	downloaded int64
	onProgress func(downloaded int64)
	onComplete func()
	completed  bool
	completeMu sync.Mutex
}

// NewProgressReader creates a new progress-tracking reader
func NewProgressReader(r io.Reader, onProgress func(downloaded int64), onComplete func()) *ProgressReader {
	return &ProgressReader{
		reader:     r,
		onProgress: onProgress,
		onComplete: onComplete,
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
	// Call onComplete when EOF is reached (download finished)
	if err == io.EOF {
		pr.completeMu.Lock()
		if !pr.completed && pr.onComplete != nil {
			pr.completed = true
			pr.onComplete()
		}
		pr.completeMu.Unlock()
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

	// The underlying read runs on a goroutine this reader may walk away from — on
	// cancellation, or on the idle timeout — and that goroutine keeps writing wherever
	// it was told to for as long as the remote takes to answer. It used to be told to
	// write into the caller's own p, so a cancelled download scribbled into a buffer
	// the caller had taken back. Not theoretical: `io.Copy` gets its buffer from a
	// pool for some destinations, so that memory can already belong to an unrelated
	// transfer by then. Found by -race under load in the 2026-07-29 round-3 audit,
	// as two downloads writing to one address.
	//
	// buf is this reader's own landing pad; pending is the read still outstanding on
	// it, kept so a caller that reads again after an abandoned attempt waits for that
	// read rather than starting a second concurrent one on the same body; and ready is
	// what has arrived and not yet been handed over.
	buf        []byte
	pending    chan readResult
	ready      []byte
	pendingErr error
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

// Read implements io.Reader with timeout and cancellation support.
//
// Called from one goroutine at a time — it is the body of a single download — so the
// pending-read bookkeeping needs no lock of its own. tr.mu guards only the fields the
// timeout watcher also touches.
func (tr *TimeoutReaderWithContext) Read(p []byte) (n int, err error) {
	// Anything already delivered goes out first, even after a cancellation: those
	// bytes came off the wire before it and throwing them away would be a corrupt
	// transfer rather than an abandoned one.
	if len(tr.ready) > 0 {
		n = tr.drain(p)
		if len(tr.ready) > 0 {
			return n, nil
		}
		err, tr.pendingErr = tr.pendingErr, nil
		return n, err
	}

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

	// Run read in goroutine so we can interrupt it on timeout or cancellation. It
	// reads into tr.buf and never into p: this call may return before the goroutine
	// does, and p belongs to the caller again the moment it does.
	if tr.pending == nil {
		if cap(tr.buf) < len(p) {
			tr.buf = make([]byte, len(p))
		}
		into := tr.buf[:len(p)]
		resultCh := make(chan readResult, 1)
		tr.pending = resultCh
		go func() {
			n, err := tr.reader.Read(into)
			resultCh <- readResult{n, err}
		}()
	}

	// Wait for read to complete, timeout, or cancellation
	for {
		select {
		case result := <-tr.pending:
			tr.pending = nil
			if result.n > 0 {
				tr.mu.Lock()
				tr.lastRead = time.Now()
				tr.mu.Unlock()
				tr.ready = tr.buf[:result.n]
				tr.pendingErr = result.err
				n = tr.drain(p)
				if len(tr.ready) > 0 {
					// The caller asked for less than the outstanding read delivered,
					// which only a caller that shrank its buffer after an abandoned
					// attempt can do. The rest waits for the next call.
					return n, nil
				}
				err = tr.pendingErr
				tr.pendingErr = nil
				return n, err
			}
			return 0, result.err
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

// drain hands over as much of what has arrived as p will hold.
func (tr *TimeoutReaderWithContext) drain(p []byte) int {
	n := copy(p, tr.ready)
	tr.ready = tr.ready[n:]
	return n
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
