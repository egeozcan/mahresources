package download_queue

import (
	"encoding/json"
	"time"
)

// HistoryRecord is one terminal download, as handed to the durable store.
//
// A plain value rather than the *DownloadJob itself: the job carries a mutex, a
// context and the attempt machinery, none of which mean anything to a store, and
// handing the live job across the seam would invite a reader that takes its lock
// on some other goroutine. This is a point-in-time statement about a job that has
// finished, and nothing about it can change afterwards.
type HistoryRecord struct {
	JobID           string
	URL             string
	Name            string
	Status          string
	Error           string
	ResourceID      *uint
	TotalSize       int64
	Progress        int64
	CreatedAt       time.Time
	StartedAt       *time.Time
	CompletedAt     *time.Time
	CreatedByUserId *uint
	// Payload is the submitted query_models.ResourceFromRemoteCreator as JSON, so a
	// retry is possible once the job itself has left the in-memory queue.
	Payload []byte
}

// HistoryRecorder is the durable store for terminal downloads.
//
// Declared here and implemented by application_context, the way ResourceCreator
// already is: the manager must not reach up into the layer that owns the DB, and
// the store must not need to know how jobs work.
type HistoryRecorder interface {
	RecordTerminalDownload(HistoryRecord) error
}

// HistoryLogger is the optional error sink for history writes. A failed history
// write is not the download's problem, so it is reported here and swallowed.
type HistoryLogger interface {
	Warn(format string, args ...any)
}

// SetHistoryRecorder installs the durable store for terminal downloads. Optional:
// with no recorder the manager behaves exactly as it did before history existed,
// which is what keeps the CLI's and the tests' bare managers working.
func (dm *DownloadManager) SetHistoryRecorder(r HistoryRecorder) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.history = r
}

// SetHistoryLogger installs the sink that failed history writes are reported to.
func (dm *DownloadManager) SetHistoryLogger(l HistoryLogger) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.historyLog = l
}

// SetHistorySweepFn registers the function cleanupOldJobs calls to delete expired
// history rows. Mirrors SetExportSweepFn: the retention values themselves are read
// by the sweep, per call, so a runtime change takes effect without a restart.
func (dm *DownloadManager) SetHistorySweepFn(fn func()) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.historySweepFn = fn
}

func (dm *DownloadManager) currentHistory() (HistoryRecorder, HistoryLogger) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.history, dm.historyLog
}

// recordTerminal persists a job that has just reached a terminal state.
//
// Called from the four places that stamp one — processJob's finish, processJob's
// cancelled-before-starting branch, Cancel's paused branch, and Shutdown, which
// abandons a paused download that has no worker left to stamp anything — and from
// none of them under a lock: it performs a database write, and holding dm.mu or j.mu
// across it would serialise every other job's progress against the store's
// latency.
//
// snap is the capture the stamping transition took under the job's own lock, not
// a fresh read: by the time this runs the job is reachable by its controls again,
// and a Retry that lands first would otherwise be what gets recorded.
//
// Only real downloads are recorded. Generic jobs (group export/import, plugin
// actions) have no URL to retry, their artifacts have their own retention, and a
// history row for one would be a row whose Retry button could do nothing.
func (dm *DownloadManager) recordTerminal(job *DownloadJob, snap *DownloadJob) {
	if job == nil || snap == nil || job.Source != JobSourceDownload || job.runFn != nil {
		return
	}
	recorder, logger := dm.currentHistory()
	if recorder == nil {
		return
	}
	// The user deleted this download's row while this write was on its way. Writing
	// it now would resurrect the row they just discarded.
	if job.isDiscarded() {
		return
	}

	rec := HistoryRecord{
		JobID:           snap.ID,
		URL:             snap.URL,
		Status:          string(snap.Status),
		Error:           snap.Error,
		ResourceID:      snap.ResourceID,
		TotalSize:       snap.TotalSize,
		Progress:        snap.Progress,
		CreatedAt:       snap.CreatedAt,
		StartedAt:       snap.StartedAt,
		CompletedAt:     snap.CompletedAt,
		CreatedByUserId: snap.ownerUserID,
	}

	if creator := job.creatorCopy(); creator != nil {
		rec.Name = creator.Name
		if payload, err := json.Marshal(creator); err == nil {
			rec.Payload = payload
		} else if logger != nil {
			logger.Warn("download history: encode payload for job %s: %v", snap.ID, err)
		}
	}

	if err := recorder.RecordTerminalDownload(rec); err != nil && logger != nil {
		logger.Warn("download history: record job %s: %v", snap.ID, err)
	}
}
