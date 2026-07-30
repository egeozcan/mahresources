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

	// failed is closed by the watcher when it sets err, so a Read waiting on a remote
	// that has gone quiet is woken instead of discovering it on the next poll. There
	// was no such signal: Read learned about an idle timeout only from a `default:`
	// branch that slept 10ms and looked again, which cost every read of the transfer
	// a scheduler quantum it did not need.
	failed chan struct{}
}

// NewTimeoutReaderWithContext creates a new timeout reader with context cancellation
func NewTimeoutReaderWithContext(r io.Reader, idleTimeout time.Duration, ctx context.Context) *TimeoutReaderWithContext {
	tr := &TimeoutReaderWithContext{
		reader:      r,
		idleTimeout: idleTimeout,
		ctx:         ctx,
		lastRead:    time.Now(),
		done:        make(chan struct{}),
		failed:      make(chan struct{}),
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
			close(tr.failed)
			return
		case <-ticker.C:
			tr.mu.Lock()
			elapsed := time.Since(tr.lastRead)
			if elapsed > tr.idleTimeout {
				tr.err = fmt.Errorf("remote server stopped sending data (idle timeout after %v)", tr.idleTimeout)
				tr.mu.Unlock()
				close(tr.failed)
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

	// Abandonment outranks a result that arrived alongside it. A plain select over
	// both is a coin flip when both are ready, and losing it is not cosmetic: the
	// chunk it hands over can be the one carrying io.EOF, so a download the user
	// cancelled — or one the idle watchdog gave up on — reaches AddResource with a
	// complete body and reports `completed`. That is a different thing from the
	// argued case in docs/todo.md, where AddResource had already succeeded before the
	// cancel was accepted; here the cancel is accepted first and the transfer
	// finishes anyway, purely because of which case the runtime picked.
	if err := tr.abandoned(); err != nil {
		return 0, err
	}

	// Wait for the read to complete, or for one of the three ways out. No polling
	// branch: this used to fall through to a `default:` that slept 10ms and looked
	// again, which is how it learned about an idle timeout, and it charged that 10ms
	// to *every* read whose goroutine had not finished by the time the select ran —
	// which is almost all of them. At io.Copy's 32 KiB that is a ceiling of about
	// 3 MB/s regardless of the network. The watcher closes tr.failed now, so this can
	// simply block.
	select {
	case result := <-tr.pending:
		tr.pending = nil
		// Checked once more, because the select above does not rank its cases: a
		// result and a cancellation that become ready together are a coin flip, and
		// the priority check before the select only covers what was already ready when
		// this call reached it. Measured with the pre-check alone: 1 of 60 cancelled
		// transfers still handed over its final chunk, and that chunk is the one
		// carrying io.EOF.
		//
		// Cancellation only, and deliberately not the idle watchdog. An idle timeout
		// asserts that nothing arrived for N seconds — and something did arrive, we
		// simply had not handed it over yet, so the assertion is false in this instant.
		// Discarding these bytes would fail a download that finished on the boundary,
		// on the strength of a claim the bytes themselves disprove. If the remote
		// really has gone quiet, the *next* read says so.
		if err := tr.cancelled(); err != nil {
			return 0, err
		}
		if result.n > 0 {
			tr.mu.Lock()
			tr.lastRead = time.Now()
			tr.mu.Unlock()
			tr.ready = tr.buf[:result.n]
			tr.pendingErr = result.err
			n = tr.drain(p)
			if len(tr.ready) > 0 {
				// The caller asked for less than the outstanding read delivered. The
				// rest waits for the next call rather than being dropped.
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
	case <-tr.failed:
		return 0, tr.watcherErr()
	}
}

// abandoned reports the reason this transfer has been given up on, or nil.
//
// Abandonment outranks anything the remote has to say. A plain select over both is a
// coin flip when both are ready, and losing it is not cosmetic: the chunk it hands
// over can be the one carrying io.EOF, so a download the user cancelled — or one the
// idle watchdog gave up on — reaches AddResource with a complete body and reports
// `completed`. Measured before this existed: 37 of 60 cancelled transfers delivered
// their last chunk. That is a different thing from the argued case in docs/todo.md,
// where AddResource had already succeeded before the cancel was accepted; here the
// cancel is accepted first and the transfer finishes anyway.
// cancelled reports whether the transfer has been given up on by *decision* — a
// Cancel, a Pause, or a Shutdown — as opposed to by inactivity.
//
// Note what this cannot promise, and does not try to. Reading a context and then
// acting on it is check-then-act by construction; nothing at this layer can make the
// two atomic against another goroutine's cancel(), because a cancellation that lands
// between them is indistinguishable from one that lands a nanosecond later. What the
// checks here remove is the part that was *not* a race: a `select` between a ready
// result and an already-cancelled context, which Go resolves by coin flip and which
// delivered the final chunk 37 times in 60. With them, 0 in 20 000. The remainder is
// the ordinary meaning of asynchronous cancellation, and it is the same thing
// docs/todo.md argues about AddResource: a cancel that lands too late has landed too
// late.
func (tr *TimeoutReaderWithContext) cancelled() error {
	select {
	case <-tr.ctx.Done():
		return fmt.Errorf("download cancelled")
	default:
		return nil
	}
}

func (tr *TimeoutReaderWithContext) abandoned() error {
	if err := tr.cancelled(); err != nil {
		return err
	}
	select {
	case <-tr.done:
		return fmt.Errorf("remote server stopped sending data (idle timeout after %v)", tr.idleTimeout)
	default:
	}
	select {
	case <-tr.failed:
		return tr.watcherErr()
	default:
	}
	return nil
}

// watcherErr reports what the timeout watcher gave up with.
func (tr *TimeoutReaderWithContext) watcherErr() error {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.err
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
