package application_context

import (
	"sync"
	"testing"
	"time"

	"mahresources/jobs"
	"mahresources/models/jobmetrics"
	"mahresources/plugin_commands"
)

type recordingProgressWriter struct {
	mu     sync.Mutex
	writes []jobs.Progress
	delay  time.Duration
}

func (w *recordingProgressWriter) Progress(progress jobs.Progress) (jobs.Snapshot, error) {
	time.Sleep(w.delay)
	w.mu.Lock()
	w.writes = append(w.writes, progress)
	w.mu.Unlock()
	return jobs.Snapshot{}, nil
}

func (w *recordingProgressWriter) written() []jobs.Progress {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]jobs.Progress(nil), w.writes...)
}

func i64(v int64) *int64      { return &v }
func str(v string) *string    { return &v }
func f64p(v float64) *float64 { return &v }

func TestMergeCommandProgressKeepsOmittedFields(t *testing.T) {
	progress := mergeCommandProgress(jobs.Progress{}, plugin_commands.ProgressReport{Percent: f64p(40), Message: str("Encoding")})
	if progress.Unit != "percent" || *progress.Completed != 40 || *progress.Total != 100 || progress.Message != "Encoding" {
		t.Fatalf("percent report = %+v; want 40 of 100 percent", progress)
	}
	progress = mergeCommandProgress(progress, plugin_commands.ProgressReport{Completed: i64(3), Total: i64(12), Unit: str("items")})
	if progress.Unit != "items" || *progress.Completed != 3 || *progress.Total != 12 || progress.Message != "Encoding" {
		t.Fatalf("counted report = %+v; want counts replacing the percent and the message kept", progress)
	}
	progress = mergeCommandProgress(progress, plugin_commands.ProgressReport{Percent: f64p(90)})
	if progress.Unit != "items" || *progress.Completed != 3 {
		t.Fatalf("a bare percent overwrote counts: %+v", progress)
	}
	metrics := []jobmetrics.Metric{{Key: "fps", Label: "Frames/s", Value: 30, Graph: true}}
	progress = mergeCommandProgress(progress, plugin_commands.ProgressReport{Metrics: &metrics})
	progress = mergeCommandProgress(progress, plugin_commands.ProgressReport{Completed: i64(4)})
	if len(progress.Metrics) != 1 || *progress.Completed != 4 || *progress.Total != 12 {
		t.Fatalf("omitting metrics dropped them: %+v", progress)
	}
	empty := []jobmetrics.Metric{}
	if cleared := mergeCommandProgress(progress, plugin_commands.ProgressReport{Metrics: &empty}); len(cleared.Metrics) != 0 {
		t.Fatalf("an empty metrics list kept %+v", cleared.Metrics)
	}
}

func TestCommandProgressMirrorThrottlesAndFlushesTheLastReport(t *testing.T) {
	writer := &recordingProgressWriter{}
	mirror := &commandProgressMirror{jobID: "job-1", target: writer}
	for i := int64(1); i <= 50; i++ {
		mirror.report(plugin_commands.ProgressReport{Completed: i64(i), Total: i64(50), Unit: str("items")})
	}
	mirror.close()
	writes := writer.written()
	if len(writes) == 0 || len(writes) > 3 {
		t.Fatalf("%d writes for 50 reports made at once; want the throttle to coalesce them", len(writes))
	}
	if last := writes[len(writes)-1]; last.Completed == nil || *last.Completed != 50 {
		t.Fatalf("last write = %+v; want the final report", last)
	}
	mirror.report(plugin_commands.ProgressReport{Completed: i64(99)})
	time.Sleep(commandProgressInterval + 50*time.Millisecond)
	if got := len(writer.written()); got != len(writes) {
		t.Fatalf("a report after close was written (%d writes, was %d)", got, len(writes))
	}
}

func TestCommandProgressMirrorCloseWaitsForAWriteInFlightAndWritesTheNewest(t *testing.T) {
	writer := &recordingProgressWriter{delay: 150 * time.Millisecond}
	mirror := &commandProgressMirror{jobID: "job-1", target: writer}
	mirror.report(plugin_commands.ProgressReport{Completed: i64(1), Total: i64(3)})
	time.Sleep(30 * time.Millisecond) // the timer write of 1 is now in flight
	mirror.report(plugin_commands.ProgressReport{Completed: i64(3)})
	mirror.close()
	writes := writer.written()
	if len(writes) == 0 || *writes[len(writes)-1].Completed != 3 {
		t.Fatalf("writes = %+v; close must return after the newest report is written", writes)
	}
	for i := 1; i < len(writes); i++ {
		if *writes[i].Completed < *writes[i-1].Completed {
			t.Fatalf("an older snapshot was written after a newer one: %+v", writes)
		}
	}
}
