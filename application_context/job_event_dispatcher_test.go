package application_context

import (
	"testing"
	"time"

	"mahresources/download_queue"
	"mahresources/plugin_system"
)

// The three statuses the catalogue names, and nothing else. A status with no
// event must not invent one: the drift scan pins the catalogue against these
// literals in both directions.
func TestJobEventNameCoversExactlyTheCataloguedEvents(t *testing.T) {
	for status, want := range map[string]string{
		"completed": "after_job_completed",
		"failed":    "after_job_failed",
		"cancelled": "after_job_cancelled",
	} {
		got, ok := jobEventName(status)
		if !ok || got != want {
			t.Errorf("jobEventName(%q) = %q, %v; want %q", status, got, ok, want)
		}
		if !plugin_system.IsHookEvent(want) {
			t.Errorf("%q is dispatched but not in AllHookEvents, so mah.on would refuse it", want)
		}
		if !plugin_system.IsJobHookEvent(want) {
			t.Errorf("%q is not classified as a job event, so it would be gated by hooks rather than job_events", want)
		}
	}

	for _, status := range []string{"downloading", "paused", "queued", ""} {
		if got, ok := jobEventName(status); ok {
			t.Errorf("jobEventName(%q) = %q; a non-terminal status must not get an event", status, got)
		}
	}
}

// The sink runs on the worker goroutine that just finished a job, immediately
// after the history write. It must never block there: a bookkeeping
// notification that waits is a download that waits.
//
// Built without its goroutine on purpose. The first version of this test used
// NewJobEventDispatcher and flooded it, and proved nothing -- the worker drained
// as fast as the test could send, so the buffer never filled and the branch
// under test never ran. With no worker, the 257th send has nowhere to go, which
// is the condition a loaded deployment actually produces.
func TestRecordJobEventDropsRatherThanBlockingWhenFull(t *testing.T) {
	ctx := createCoverageTestContext(t, "job_event_nonblocking")
	d := &JobEventDispatcher{
		ctx:    ctx,
		events: make(chan download_queue.JobEventRecord, jobEventQueueSize),
		done:   make(chan struct{}),
	}

	done := make(chan struct{})
	go func() {
		for i := 0; i < jobEventQueueSize*4; i++ {
			d.RecordJobEvent(download_queue.JobEventRecord{JobID: "flood", Status: "completed"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("RecordJobEvent blocked on a full queue: it must drop, or it stalls the download worker that called it")
	}

	if len(d.events) != jobEventQueueSize {
		t.Errorf("queue holds %d, want it capped at %d", len(d.events), jobEventQueueSize)
	}
	d.dropMu.Lock()
	dropped := d.dropped
	d.dropMu.Unlock()
	if dropped == 0 {
		t.Error("nothing was recorded as dropped, so the overflow went somewhere unaccounted for")
	}
}

// Stop must return even when nothing is listening, and must be safe twice --
// main defers it, and a second call from a test or a restart path must not
// panic on a closed channel.
func TestJobEventDispatcherStopIsBoundedAndIdempotent(t *testing.T) {
	ctx := createCoverageTestContext(t, "job_event_stop")
	d := NewJobEventDispatcher(ctx)

	d.RecordJobEvent(download_queue.JobEventRecord{JobID: "before-stop", Status: "completed"})

	start := time.Now()
	d.Stop()
	d.Stop()
	if elapsed := time.Since(start); elapsed > jobEventDrainTimeout+5*time.Second {
		t.Errorf("Stop took %s, want at most about %s", elapsed, jobEventDrainTimeout)
	}

	// Recording after Stop is a no-op rather than a panic on a closed channel.
	d.RecordJobEvent(download_queue.JobEventRecord{JobID: "after-stop", Status: "completed"})
}

// A nil dispatcher is what a deployment with no plugin manager effectively has,
// and every method must tolerate it -- main installs the sink unconditionally.
func TestJobEventDispatcherNilIsSafe(t *testing.T) {
	var d *JobEventDispatcher
	d.RecordJobEvent(download_queue.JobEventRecord{JobID: "nil", Status: "completed"})
	d.Stop()
}
