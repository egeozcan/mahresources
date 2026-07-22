package mrqlbench

import (
	"context"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm/logger"
)

func TestCollectorIsolatesSamplesAndRedactsSQLValues(t *testing.T) {
	collector := NewCollector(logger.Discard)
	ctxA := WithSample(context.Background(), "a")
	ctxB := WithSample(context.Background(), "b")

	collector.Trace(ctxA, time.Now(), func() (string, int64) {
		return `SELECT * FROM resources WHERE name = 'secret-one' AND id = 123`, 1
	}, nil)
	collector.Trace(ctxB, time.Now(), func() (string, int64) {
		return `SELECT * FROM resources WHERE name = 'secret-two' AND id = 999`, 2
	}, nil)

	a := collector.Snapshot("a")
	b := collector.Snapshot("b")
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("unexpected snapshots: %#v %#v", a, b)
	}
	if a[0].Fingerprint != b[0].Fingerprint {
		t.Fatalf("bound values changed SQL shape: %q != %q", a[0].Fingerprint, b[0].Fingerprint)
	}
	if a[0].Rows != 1 || b[0].Rows != 2 {
		t.Fatalf("rows leaked across samples: %#v %#v", a, b)
	}
}

func TestCollectorIsConcurrencySafe(t *testing.T) {
	collector := NewCollector(logger.Discard)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			collector.Trace(WithSample(context.Background(), "shared"), time.Now(), func() (string, int64) {
				return "SELECT 1", 1
			}, nil)
		}()
	}
	wg.Wait()
	if got := len(collector.Snapshot("shared")); got != 100 {
		t.Fatalf("observations = %d, want 100", got)
	}
}

func TestCollectorIgnoresUnmeasuredContexts(t *testing.T) {
	collector := NewCollector(logger.Discard)
	collector.Trace(context.Background(), time.Now(), func() (string, int64) { return "SELECT 1", 1 }, nil)
	if got := collector.Snapshot(""); len(got) != 0 {
		t.Fatalf("unmeasured trace was recorded: %#v", got)
	}
}
