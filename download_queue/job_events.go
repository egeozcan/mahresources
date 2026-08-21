package download_queue

import (
	"sync"
	"time"
)

// JobEventRecord is one job that has just reached a terminal state, as handed to
// whatever observes such things.
//
// A plain value for the same reason HistoryRecord is one: the live *DownloadJob
// carries a mutex, a context and the attempt machinery, and handing it across
// this seam would invite an observer that takes its lock on another goroutine.
// This is a point-in-time statement about a job that has finished.
type JobEventRecord struct {
	JobID  string
	Source string
	Status string
	Name   string
	URL    string
	Error  string
	// ResourceID is set only for a download that produced one.
	ResourceID  *uint
	TotalSize   int64
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	// OwnerUserID is the submitter, and is what an observer's handler runs as.
	OwnerUserID *uint
}

// JobEventSink observes jobs reaching a terminal state.
//
// Declared here and implemented by application_context, the way HistoryRecorder
// and ResourceCreator already are: this package must not reach up into the layer
// that owns plugins, and the observer must not need to know how jobs work.
//
// Unlike recordTerminal, this fires for **every** job kind the queue runs, not
// only real downloads. A history row for an export could do nothing useful — its
// Retry button would have no URL — but "the export you asked for has finished" is
// exactly the thing a plugin wants to hear about, and the terminal edge is
// already shared by every kind.
type JobEventSink interface {
	RecordJobEvent(JobEventRecord)
}

// SetJobEventSink installs the observer. Optional: with no sink, emitJobEvent is
// a nil check and nothing else, which is what every deployment with no plugin
// listening gets.
func (dm *DownloadManager) SetJobEventSink(s JobEventSink) {
	dm.jobEventMu.Lock()
	defer dm.jobEventMu.Unlock()
	dm.jobEventSink = s
}

func (dm *DownloadManager) currentJobEventSink() JobEventSink {
	dm.jobEventMu.RLock()
	defer dm.jobEventMu.RUnlock()
	return dm.jobEventSink
}

// jobEventState is embedded in DownloadManager.
type jobEventState struct {
	jobEventMu   sync.RWMutex
	jobEventSink JobEventSink
}

// emitJobEvent announces a job that has just reached a terminal state.
//
// Called from the same four places that stamp a history row, under the same two
// rules, for the same reasons: never under dm.mu or j.mu, because the sink hands
// off to a dispatcher and holding a lock across that would serialise every other
// job's progress behind it; and from the snapshot the stamping transition took
// under the job's own lock rather than a fresh read, because by the time this
// runs the job is reachable by its controls again and a Retry landing first
// would otherwise be what gets announced.
//
// The sink must not block. This is a bookkeeping notification on a worker
// goroutine, and an observer that stalls here stalls the queue.
func (dm *DownloadManager) emitJobEvent(job *DownloadJob, snap *DownloadJob) {
	if job == nil || snap == nil {
		return
	}
	sink := dm.currentJobEventSink()
	if sink == nil {
		return
	}

	rec := JobEventRecord{
		JobID:       snap.ID,
		Source:      snap.Source,
		Status:      string(snap.Status),
		URL:         snap.URL,
		Error:       snap.Error,
		ResourceID:  snap.ResourceID,
		TotalSize:   snap.TotalSize,
		CreatedAt:   snap.CreatedAt,
		StartedAt:   snap.StartedAt,
		CompletedAt: snap.CompletedAt,
		OwnerUserID: snap.ownerUserID,
	}
	// A generic job (export, import, plugin action) carries no creator, so its
	// name is simply absent; the observer has the id and the source either way.
	if creator := job.creatorCopy(); creator != nil {
		rec.Name = creator.Name
	}

	sink.RecordJobEvent(rec)
}
