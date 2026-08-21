package application_context

import (
	"log"
	"sync"
	"time"

	"mahresources/download_queue"
	"mahresources/plugin_system"
)

const (
	// jobEventQueueSize bounds what a stalled dispatcher can hold. The queue's
	// worker must never block on this seam, so a full buffer drops rather than
	// waits — the same trade the subscriber broadcast already makes, and for the
	// same reason: a bookkeeping notification must not hold a download.
	jobEventQueueSize = 256

	// jobEventDrainTimeout bounds Stop, so a handler that ignores its context
	// cannot hold a deployment open. Mirrors the scheduler's drain.
	jobEventDrainTimeout = 5 * time.Second
)

// JobEventDispatcher turns terminal jobs into plugin hook dispatches.
//
// It exists because download_queue must not reach up into the layer that owns
// plugins: the queue publishes a plain value through its JobEventSink seam, and
// this is what listens. The same shape as HistoryRecorder, one layer up.
//
// Asynchronous by construction, unlike the entity hooks. An entity after-hook is
// dispatched inline because the caller is a request that can afford to wait and
// because ordering against the write matters. Here the caller is a download
// worker: blocking it would serialise the queue behind plugin VMs, so the record
// is queued and a single goroutine dispatches. That also makes ordering explicit
// — one worker, so events reach handlers in the order the queue finished them.
type JobEventDispatcher struct {
	ctx      *MahresourcesContext
	events   chan download_queue.JobEventRecord
	done     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup

	// dropped counts records discarded because the buffer was full, so the
	// condition is reportable rather than silent.
	dropMu  sync.Mutex
	dropped int
}

// NewJobEventDispatcher starts the dispatcher's goroutine.
func NewJobEventDispatcher(ctx *MahresourcesContext) *JobEventDispatcher {
	d := &JobEventDispatcher{
		ctx:    ctx,
		events: make(chan download_queue.JobEventRecord, jobEventQueueSize),
		done:   make(chan struct{}),
	}
	d.wg.Add(1)
	go d.run()
	return d
}

// RecordJobEvent implements download_queue.JobEventSink.
//
// Non-blocking, always. This runs on the worker goroutine that just finished a
// job, immediately after the history write, and the queue's contract for that
// seam is that it does not wait.
func (d *JobEventDispatcher) RecordJobEvent(rec download_queue.JobEventRecord) {
	if d == nil {
		return
	}
	select {
	case <-d.done:
		return
	default:
	}

	select {
	case d.events <- rec:
	default:
		d.dropMu.Lock()
		d.dropped++
		n := d.dropped
		d.dropMu.Unlock()
		// Every dropped event is a notification a plugin will not receive. Say
		// so: the alternative is a plugin that silently stops hearing about jobs
		// under load and no way to find out why.
		log.Printf("warning: job event queue full, dropped notification for job %s (%d dropped so far)", rec.JobID, n)
	}
}

func (d *JobEventDispatcher) run() {
	defer d.wg.Done()
	for {
		select {
		case <-d.done:
			// Drain what is already queued before leaving, so a shutdown does
			// not discard events the queue already handed over.
			for {
				select {
				case rec := <-d.events:
					d.dispatch(rec)
				default:
					return
				}
			}
		case rec := <-d.events:
			d.dispatch(rec)
		}
	}
}

// jobEventName maps a terminal status to its event.
//
// The three literals live here, in a file that mentions RunAfterHooks, which is
// what internal/arch/plugin_catalogue_drift_test.go scans: a catalogue entry
// nothing dispatches fails that test, so the names cannot drift apart from the
// dispatch without the build noticing.
func jobEventName(status string) (string, bool) {
	switch status {
	case "completed":
		return "after_job_completed", true
	case "failed":
		return "after_job_failed", true
	case "cancelled":
		return "after_job_cancelled", true
	}
	return "", false
}

func (d *JobEventDispatcher) dispatch(rec download_queue.JobEventRecord) {
	event, ok := jobEventName(rec.Status)
	if !ok {
		// Not a terminal status this announces. Paused and downloading reach the
		// sink only if a future caller adds a site; refusing here keeps the
		// catalogue honest rather than inventing a name for them.
		return
	}

	pm := d.ctx.PluginManager()
	if pm == nil {
		return
	}

	data := map[string]any{
		"job_id": rec.JobID,
		"source": rec.Source,
		"status": rec.Status,
		"name":   rec.Name,
		"url":    rec.URL,
		"error":  rec.Error,
	}
	if rec.ResourceID != nil {
		data["resource_id"] = float64(*rec.ResourceID)
	}
	if rec.TotalSize > 0 {
		data["total_size"] = float64(rec.TotalSize)
	}

	// The handler runs as whoever submitted the job, matching how the history
	// row is attributed and how an async action job runs. A job with no
	// submitter (auth-off, or a system-started export) dispatches with no actor,
	// which is the same thing every other principal-less host path does.
	inv := plugin_system.NewInvocation(0)
	if rec.OwnerUserID != nil {
		inv = plugin_system.NewInvocation(*rec.OwnerUserID)
	}

	pm.RunAfterHooks(inv, event, data)
}

// Stop ends the dispatcher and waits, bounded, for the goroutine to finish what
// it is holding. Bounded for the reason PluginScheduler.Stop is: a plugin
// handler that ignores its context must not hold a deployment open.
func (d *JobEventDispatcher) Stop() {
	if d == nil {
		return
	}
	d.stopOnce.Do(func() { close(d.done) })

	drained := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(drained)
	}()
	select {
	case <-drained:
	case <-time.After(jobEventDrainTimeout):
		log.Printf("warning: job event dispatcher still running after %s; "+
			"the events it holds are not delivered", jobEventDrainTimeout)
	}
}
