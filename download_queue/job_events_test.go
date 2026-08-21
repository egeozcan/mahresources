package download_queue

import (
	"sync"
	"testing"
	"time"
)

type recordingJobEvents struct {
	mu   sync.Mutex
	recs []JobEventRecord
}

func (r *recordingJobEvents) RecordJobEvent(rec JobEventRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recs = append(r.recs, rec)
}

func (r *recordingJobEvents) all() []JobEventRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]JobEventRecord, len(r.recs))
	copy(out, r.recs)
	return out
}

// Item 4.2. Every job kind the queue runs announces its terminal state, not just
// real downloads: a history row for an export could do nothing useful, since its
// Retry button would have no URL, but "the export you asked for has finished" is
// exactly what a plugin wants to hear.
func TestEmitJobEventFiresForEveryJobKind(t *testing.T) {
	for _, source := range []string{"download", "group-export", "plugin"} {
		t.Run(source, func(t *testing.T) {
			sink := &recordingJobEvents{}
			dm := createTestManager()
			dm.SetJobEventSink(sink)

			now := time.Now()
			job := &DownloadJob{
				ID:          "job-" + source,
				Source:      source,
				Status:      JobStatusCompleted,
				URL:         "http://example.invalid/x",
				CompletedAt: &now,
			}
			dm.emitJobEvent(job, job)

			got := sink.all()
			if len(got) != 1 {
				t.Fatalf("expected 1 event for source %q, got %d", source, len(got))
			}
			if got[0].JobID != job.ID || got[0].Source != source {
				t.Errorf("event = %+v, want job %q source %q", got[0], job.ID, source)
			}
			if got[0].Status != string(JobStatusCompleted) {
				t.Errorf("status = %q", got[0].Status)
			}
		})
	}
}

// No sink is the default, and must cost nothing and panic on nothing: the CLI's
// and the tests' bare managers run that way.
func TestEmitJobEventWithNoSinkIsInert(t *testing.T) {
	dm := createTestManager()
	job := &DownloadJob{ID: "no-sink", Source: "download", Status: JobStatusCompleted}
	dm.emitJobEvent(job, job)
	dm.emitJobEvent(nil, nil)
	dm.emitJobEvent(job, nil)
}
