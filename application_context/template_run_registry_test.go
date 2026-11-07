package application_context

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestRunRegistryLimitsLiveRunsPerAccount(t *testing.T) {
	r := newTemplateRunRegistry()
	a, err := r.start("user:1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.start("user:1", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := r.start("user:1", nil); !errors.Is(err, ErrTemplateRunLimit) {
		t.Fatalf("the third live run must be refused, got %v", err)
	}
	if _, err := r.start("user:2", nil); err != nil {
		t.Fatalf("another account is unaffected: %v", err)
	}
	a.Release()
	a.Release() // idempotent
	if r.live("user:1") != 1 {
		t.Fatalf("live = %d", r.live("user:1"))
	}
	if _, err := r.start("user:1", nil); err != nil {
		t.Fatalf("a released run frees its slot: %v", err)
	}
}

func TestRunRegistryFinishNeedsTheOwningAccount(t *testing.T) {
	r := newTemplateRunRegistry()
	h, _ := r.start("user:1", nil)
	if r.finish("nope", "user:1") || r.finish(h.ID, "user:2") {
		t.Fatal("an unknown run or another account's run is not found")
	}
	if h.FinishRequested() {
		t.Fatal("a refused finish must not set the flag")
	}
	if !r.finish(h.ID, "user:1") || !h.FinishRequested() {
		t.Fatal("the owner can request Finish now")
	}
	h.Release()
	if r.finish(h.ID, "user:1") {
		t.Fatal("a released run is gone")
	}
}

func TestRunRegistryCancelAndConcurrentUse(t *testing.T) {
	r := newTemplateRunRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	h, _ := r.start("ip:1.2.3.4", cancel)
	h.Cancel()
	if ctx.Err() == nil {
		t.Fatal("Cancel must cancel the run's context")
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if hh, err := r.start("ip:9", nil); err == nil {
				r.finish(hh.ID, "ip:9")
				hh.Release()
			}
		}()
	}
	wg.Wait()
	if r.live("ip:9") != 0 {
		t.Fatalf("leaked entries: %d", r.live("ip:9"))
	}
	if TemplateRunAccountKey(7, "x") != "user:7" || TemplateRunAccountKey(0, "1.1.1.1") != "ip:1.1.1.1" {
		t.Fatal("account keys")
	}
}
