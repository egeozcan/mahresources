package download_queue

// The io.Reader contract, for the reader a download actually runs through.
//
// TimeoutReaderWithContext is the riskiest thing in this package: it runs the
// underlying read on a goroutine it can walk away from, carries undelivered bytes
// between calls, and decides what a cancellation means for bytes already in flight.
// Round-4 review put two real defects in it and both are pinned below. Everything
// else here is the contract itself — n>0 with io.EOF, an error delivered with data, a
// zero-length read, byte-for-byte fidelity across ragged chunk boundaries — because a
// reader in the middle of a file upload that silently drops or duplicates a chunk
// corrupts a resource rather than failing one.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

// chunker returns fixed chunks, then a final chunk together with an error.
type chunker struct {
	chunks [][]byte
	final  error
	i      int
}

func (c *chunker) Read(p []byte) (int, error) {
	if c.i >= len(c.chunks) {
		return 0, c.final
	}
	n := copy(p, c.chunks[c.i])
	if n < len(c.chunks[c.i]) {
		c.chunks[c.i] = c.chunks[c.i][n:]
		return n, nil
	}
	c.i++
	if c.i >= len(c.chunks) {
		return n, c.final
	}
	return n, nil
}

// n>0 delivered together with io.EOF must not lose the bytes or the EOF.
func TestReaderContract_EOFWithData(t *testing.T) {
	src := &chunker{chunks: [][]byte{[]byte("abcdefgh")}, final: io.EOF}
	tr := NewTimeoutReaderWithContext(src, time.Minute, context.Background())
	defer tr.Close()
	p := make([]byte, 16)
	n, err := tr.Read(p)
	if n != 8 || err != io.EOF {
		t.Fatalf("got n=%d err=%v, want 8/EOF", n, err)
	}
	if string(p[:n]) != "abcdefgh" {
		t.Fatalf("got %q", p[:n])
	}
}

// A non-EOF error delivered with data.
func TestReaderContract_ErrorWithData(t *testing.T) {
	boom := errors.New("boom")
	src := &chunker{chunks: [][]byte{[]byte("xy")}, final: boom}
	tr := NewTimeoutReaderWithContext(src, time.Minute, context.Background())
	defer tr.Close()
	p := make([]byte, 4)
	n, err := tr.Read(p)
	if n != 2 || !errors.Is(err, boom) {
		t.Fatalf("got n=%d err=%v, want 2/boom", n, err)
	}
}

// Many small reads through io.Copy must reproduce the stream byte for byte.
func TestReaderContract_StreamIsExact(t *testing.T) {
	want := bytes.Repeat([]byte("0123456789abcdef"), 4096) // 64 KiB
	src := &chunker{chunks: [][]byte{want[:1], want[1:7], want[7:5000], want[5000:]}, final: io.EOF}
	tr := NewTimeoutReaderWithContext(src, time.Minute, context.Background())
	defer tr.Close()
	got, err := io.ReadAll(tr)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("stream differs: got %d bytes want %d; first mismatch near %d", len(got), len(want), firstDiff(got, want))
	}
}

// A zero-length read must not hang or consume anything.
func TestReaderContract_ZeroLengthRead(t *testing.T) {
	src := &chunker{chunks: [][]byte{[]byte("hello")}, final: io.EOF}
	tr := NewTimeoutReaderWithContext(src, time.Minute, context.Background())
	defer tr.Close()
	done := make(chan struct{})
	go func() { defer close(done); _, _ = tr.Read(nil) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("a zero-length Read hung")
	}
	got, err := io.ReadAll(tr)
	if err != nil {
		t.Fatalf("ReadAll after zero-length read: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("a zero-length read consumed data: got %q", got)
	}
}

// The caller shrinks its buffer between calls: the surplus must be carried, not lost.
func TestReaderContract_ShrinkingBufferKeepsEveryByte(t *testing.T) {
	src := &chunker{chunks: [][]byte{[]byte("ABCDEFGH")}, final: io.EOF}
	tr := NewTimeoutReaderWithContext(src, time.Minute, context.Background())
	defer tr.Close()

	big := make([]byte, 8)
	n, err := tr.Read(big)
	if n != 8 {
		t.Fatalf("first read n=%d err=%v", n, err)
	}
	small := make([]byte, 3)
	var out []byte
	out = append(out, big[:n]...)
	for i := 0; i < 5; i++ {
		n, err = tr.Read(small)
		out = append(out, small[:n]...)
		if err != nil {
			break
		}
	}
	if string(out) != "ABCDEFGH" {
		t.Fatalf("got %q, want ABCDEFGH", out)
	}
}

func firstDiff(a, b []byte) int {
	for i := range a {
		if i >= len(b) || a[i] != b[i] {
			return i
		}
	}
	return len(a)
}

// One read per 10ms was the old cost of learning about an idle timeout by polling.
func TestReaderContract_ReadIsNotSchedulerBound(t *testing.T) {
	// 512 reads of a reader that always has data ready. The old busy-poll charged
	// ~10ms to each, i.e. >5s for this loop.
	src := &chunker{chunks: make([][]byte, 512), final: io.EOF}
	for i := range src.chunks {
		src.chunks[i] = bytes.Repeat([]byte("x"), 64)
	}
	tr := NewTimeoutReaderWithContext(src, time.Minute, context.Background())
	defer tr.Close()

	start := time.Now()
	n, err := io.Copy(io.Discard, tr)
	elapsed := time.Since(start)

	if err != nil || n != 512*64 {
		t.Fatalf("copied %d bytes, err=%v", n, err)
	}
	// Generous by two orders of magnitude against the old ~5.1s, and still far below
	// anything a correct implementation would need.
	if elapsed > time.Second {
		t.Errorf("512 ready reads took %v — each one is paying a polling quantum it does not need", elapsed)
	}
	t.Logf("512 reads in %v (%.1f us/read)", elapsed, float64(elapsed.Microseconds())/512)
}

// cancelSource cancels the transfer and then answers with the last chunk of the body
// and io.EOF — the shape that matters, because the chunk carrying EOF is the one that
// lets AddResource succeed.
type cancelSource struct {
	cancel context.CancelFunc
	done   bool
}

func (c *cancelSource) Read(p []byte) (int, error) {
	if c.done {
		return 0, io.EOF
	}
	c.done = true
	c.cancel()
	return copy(p, []byte("LASTCHUNK")), io.EOF
}

// A cancelled transfer must not hand its caller more bytes.
//
// The wait select had `case result := <-tr.pending` beside `case <-tr.ctx.Done()`,
// and Go picks uniformly when both are ready — so a cancelled download delivered its
// final chunk about half the time, and that chunk is the one carrying io.EOF.
// AddResource then sees a complete body and the job reports `completed`. That is a
// different thing from the argued case in docs/todo.md: there the resource already
// existed when the cancel was accepted, here the cancel is accepted first and the
// transfer finishes anyway because of which case the runtime picked.
//
// Repeated rather than sequenced: the two become ready a few instructions apart, so
// what is asserted is that the outcome no longer depends on which one wins.
func TestReaderContract_ACancelledTransferNeverDeliversMoreBytes(t *testing.T) {
	delivered := 0
	const iterations = 60
	for i := 0; i < iterations; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		src := &cancelSource{cancel: cancel}
		tr := NewTimeoutReaderWithContext(src, time.Minute, ctx)

		p := make([]byte, 32)
		n, err := tr.Read(p)
		if n > 0 || err == nil {
			delivered++
		}
		tr.Close()
		cancel()
	}
	if delivered > 0 {
		t.Errorf("a cancelled transfer handed over its final chunk in %d of %d attempts — that chunk carries io.EOF, so the download completes after the user was told it would not",
			delivered, iterations)
	}
}
