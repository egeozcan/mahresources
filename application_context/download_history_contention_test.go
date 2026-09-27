package application_context

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"mahresources/download_queue"
	"mahresources/models"
)

// Recording a finished download reads the job writer epoch and then upserts the
// history row, and the recorder only logs a failure: a transaction refused at its
// write loses the row without anyone seeing an error. A commit from another
// connection right before that write (the queue's next job, a Job's bookkeeping)
// must not be able to refuse it.
func TestATerminalDownloadIsRecordedDespiteACommitBeforeItsWrite(t *testing.T) {
	ctx := newWALTestContext(t, 0)
	require.NoError(t, ctx.db.AutoMigrate(&models.DownloadHistoryEntry{}, &models.JobWriterEpoch{}))
	require.NoError(t, models.EnsureJobWriterEpoch(ctx.db))

	fired := commitBeforeEveryWriteTo(t, ctx, "download_history_entries")
	completed := time.Now().UTC()
	require.NoError(t, ctx.RecordTerminalDownload(download_queue.HistoryRecord{
		JobID: "contended-download", URL: "https://example.com/a.bin", Name: "a.bin",
		Status: "completed", CreatedAt: completed, CompletedAt: &completed,
	}))
	require.NotZero(t, fired.Load(), "the recorder wrote nothing, so the interleave never happened")

	var stored int64
	require.NoError(t, ctx.db.Model(&models.DownloadHistoryEntry{}).Where("job_id = ?", "contended-download").Count(&stored).Error)
	require.EqualValues(t, 1, stored, "the history row was lost")
}
