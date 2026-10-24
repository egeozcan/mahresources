//go:build unix

package application_context

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO opened for reading blocks until a writer appears, so a compile that
// returns promptly proves the reference was never opened.
func TestValidateMetaSchemaDoesNotOpenAFIFORef(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipe.json")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	fileURL := "file://" + filepath.ToSlash(path)

	done := make(chan error, 1)
	go func() {
		done <- ValidateMetaSchema(`{"$ref":"` + fileURL + `"}`)
	}()
	select {
	case err := <-done:
		requireRefusedByLoader(t, err, fileURL)
	case <-time.After(5 * time.Second):
		unblockFIFOReader(path)
		t.Fatal("ValidateMetaSchema blocked on a FIFO $ref: it opened the file")
	}
}

// unblockFIFOReader releases a reader stuck opening path, without blocking
// itself. A non-blocking writer open fails with ENXIO while no reader has the
// FIFO open (compilation stalled before opening it), so it retries for a
// bounded time and then gives up.
func unblockFIFOReader(path string) {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		f, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			_ = f.Close()
			return
		}
		if !errors.Is(err, syscall.ENXIO) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
