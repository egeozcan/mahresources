package application_context

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
)

// This file is the restart contract of a deferred download on a database whose
// Job sources are retired: a row that came due in one process must not stop the
// next process from starting on the same database, and the retirement barrier
// must still refuse a row that regained its plaintext.

// newRetiredDeferredDownloadContext builds a context over a file database, with a
// replay key a second process can share, and a deferred row created before the
// Job source migration ran. The migration is then driven to completion, so the
// writer epoch is retired and that row is scrubbed while it is still pending.
func newRetiredDeferredDownloadContext(t *testing.T) (*MahresourcesContext, jobs.ReplayKey, models.User, models.ScheduledDownload) {
	t.Helper()
	ctx := newJobHarnessContext(t, false)
	key := sharedReplayKey(t)
	holdJobReplayKey(t, ctx, key)
	enableDownloadTestPlugin(t, ctx)
	actor, err := ctx.CreateUser(&UserInput{Username: "deferred-owner", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create the acting user: %v", err)
	}
	beforeRetirement, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/before-retirement.bin?token=private"},
		time.Now().Add(-time.Second))
	if err != nil {
		t.Fatalf("create the deferred download before retirement: %v", err)
	}
	result, err := ctx.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 50, MaxBatches: 20, WritersDrained: true})
	if err != nil || !result.Complete {
		t.Fatalf("retire the job sources = %+v, %v", result, err)
	}
	return ctx, key, *actor, *beforeRetirement
}

// fireDueDeferredDownloads runs one scheduler sweep the way the plugin scheduler
// tick does, with the plugin reported available. A row backed by a durable Job is
// materialized rather than submitted, so the submit seam must never be reached.
func fireDueDeferredDownloads(t *testing.T, ctx *MahresourcesContext, now time.Time) int {
	t.Helper()
	fired, err := ctx.FireDueScheduledDownloads(ScheduledDownloadFireConfig{
		Now:             now,
		PluginAvailable: func(string) bool { return true },
		Submit: func(*query_models.ResourceFromRemoteCreator, *uint, string) (string, error) {
			t.Fatalf("a deferred row with a durable Job was submitted to the queue instead of materialized")
			return "", nil
		},
	})
	if err != nil {
		t.Fatalf("fire due deferred downloads: %v", err)
	}
	return fired
}

// restartJobProcess stops the process that owns ctx and starts another over the
// same database file and replay key, which sees only what was committed.
func restartJobProcess(t *testing.T, ctx *MahresourcesContext, key jobs.ReplayKey) *MahresourcesContext {
	t.Helper()
	sqlDB, err := ctx.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close the first process's database: %v", err)
	}
	next, _ := newSecondProcessJobContext(t, ctx, key)
	return next
}

// requireCleanBoot runs what startup runs before it admits traffic and asserts
// what the Job Center cutover gate asserts.
func requireCleanBoot(t *testing.T, ctx *MahresourcesContext) {
	t.Helper()
	result, err := ctx.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 50, MaxBatches: 20})
	if err != nil {
		t.Fatalf("startup migration after restart: %v", err)
	}
	readiness, err := ctx.GetJobMigrationReadiness()
	if err != nil {
		t.Fatalf("readiness after restart: %v", err)
	}
	if !result.Complete || !readiness.Ready || readiness.Phase != models.JobMigrationPhaseComplete ||
		readiness.WriterEpoch < models.JobWriterEpochRetiredPlaintext {
		var mappings []models.JobSourceMapping
		_ = ctx.db.Where("source_kind = ?", jobMigrationScheduledDownload).Order("source_id").Find(&mappings).Error
		t.Fatalf("the restarted process would refuse to start: migration=%+v readiness=%+v mappings=%+v", result, readiness, mappings)
	}
}

func scheduledDownloadMapping(t *testing.T, ctx *MahresourcesContext, rowID uint) models.JobSourceMapping {
	t.Helper()
	var mapping models.JobSourceMapping
	if err := ctx.db.Where("source_kind = ? AND source_id = ?", jobMigrationScheduledDownload,
		strconv.FormatUint(uint64(rowID), 10)).First(&mapping).Error; err != nil {
		t.Fatalf("read the mapping of scheduled download %d: %v", rowID, err)
	}
	return mapping
}

// TestAFiredDeferredDownloadDoesNotBlockTheNextBoot covers both ways a live row
// is scrubbed — by the migration while it was pending, and at creation after the
// sources were retired — and both then come due in the running process.
func TestAFiredDeferredDownloadDoesNotBlockTheNextBoot(t *testing.T) {
	ctx, key, actor, beforeRetirement := newRetiredDeferredDownloadContext(t)
	afterRetirement, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/after-retirement.bin?token=private"},
		time.Now().Add(-time.Second))
	if err != nil {
		t.Fatalf("create the deferred download after retirement: %v", err)
	}
	for _, row := range []models.ScheduledDownload{beforeRetirement, *afterRetirement} {
		if mapping := scheduledDownloadMapping(t, ctx, row.ID); mapping.Status != models.JobSourceMappingScrubbed {
			t.Fatalf("setup: scheduled download %d is %s before it fires, want scrubbed", row.ID, mapping.Status)
		}
	}

	if fired := fireDueDeferredDownloads(t, ctx, time.Now()); fired != 2 {
		t.Fatalf("the sweep fired %d rows, want 2", fired)
	}
	for _, row := range []models.ScheduledDownload{beforeRetirement, *afterRetirement} {
		current := scheduledDownloadRow(t, ctx, row.ID)
		if current.Status != models.ScheduledDownloadStatusSubmitted || current.JobID == "" {
			t.Fatalf("setup: scheduled download %d is %s with job %q after its due time, want submitted with its Job",
				row.ID, current.Status, current.JobID)
		}
	}

	ctx = restartJobProcess(t, ctx, key)
	requireCleanBoot(t, ctx)
	for _, row := range []models.ScheduledDownload{beforeRetirement, *afterRetirement} {
		if mapping := scheduledDownloadMapping(t, ctx, row.ID); mapping.Status != models.JobSourceMappingScrubbed {
			t.Fatalf("scheduled download %d is %s (%s) after the restart, want scrubbed", row.ID, mapping.Status, mapping.BlockerCode)
		}
	}
}

// TestABootBlockedByAFiredDeferredDownloadRecovers starts from what an earlier
// release left behind when it started over a fired row: its readiness check read
// the fire-time job id as a restored source, its re-arm could not prove the
// scrubbed URL against the canonical input and quarantined the mapping, and every
// boot after that stopped at the drain fence. The quarantine kept the scrub
// marker, and the row itself still carries no plaintext, so the next start must
// recover it without anybody editing a row.
func TestABootBlockedByAFiredDeferredDownloadRecovers(t *testing.T) {
	cases := []struct {
		name        string
		blockerCode string
		phase       string
	}{
		// The first boot quarantined the mapping in its re-arm and reset the
		// checkpoint to copy; the copy pass then re-read the changed row, failed to
		// prove it and rewrote the blocker, and the pass stopped at the fence.
		{name: "stopped at the fence", blockerCode: "source-canonical-replay-mismatch", phase: models.JobMigrationPhaseDrainFence},
		// A process that died between the re-arm and the copy pass.
		{name: "stopped after the re-arm", blockerCode: "restored-source-not-proven", phase: models.JobMigrationPhaseCopy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, key, _, row := newRetiredDeferredDownloadContext(t)
			if fired := fireDueDeferredDownloads(t, ctx, time.Now()); fired != 1 {
				t.Fatalf("the sweep fired %d rows, want 1", fired)
			}
			fired := scheduledDownloadRow(t, ctx, row.ID)
			quarantineAsAnEarlierReleaseDid(t, ctx, fired, tc.blockerCode, tc.phase)

			ctx = restartJobProcess(t, ctx, key)
			requireCleanBoot(t, ctx)
			if mapping := scheduledDownloadMapping(t, ctx, row.ID); mapping.Status != models.JobSourceMappingScrubbed || mapping.BlockerCode != "" {
				t.Fatalf("the recovered mapping is %s (%q), want scrubbed", mapping.Status, mapping.BlockerCode)
			}
		})
	}
}

// TestARestoredPlaintextDeferredRowStaysQuarantined is the control for the
// recovery above: a quarantined row whose plaintext came back is not the row that
// was scrubbed, and the next start must keep refusing it.
func TestARestoredPlaintextDeferredRowStaysQuarantined(t *testing.T) {
	ctx, key, _, row := newRetiredDeferredDownloadContext(t)
	if fired := fireDueDeferredDownloads(t, ctx, time.Now()); fired != 1 {
		t.Fatalf("the sweep fired %d rows, want 1", fired)
	}
	dropScheduledDownloadBarriers(t, ctx)
	restored := row.URL + "&restored=from-backup"
	if err := ctx.db.Model(&models.ScheduledDownload{}).Where("id = ?", row.ID).
		Updates(map[string]any{"url": restored, "payload": mustScheduledPayload(t, &query_models.ResourceFromRemoteCreator{URL: restored})}).Error; err != nil {
		t.Fatalf("restore plaintext into the scrubbed row: %v", err)
	}
	reinstallScheduledDownloadBarriers(t, ctx)
	quarantineAsAnEarlierReleaseDid(t, ctx, scheduledDownloadRow(t, ctx, row.ID), "source-canonical-replay-mismatch", models.JobMigrationPhaseDrainFence)

	ctx = restartJobProcess(t, ctx, key)
	if _, err := ctx.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 50, MaxBatches: 20}); err != nil {
		t.Fatalf("startup migration: %v", err)
	}
	if mapping := scheduledDownloadMapping(t, ctx, row.ID); mapping.Status != models.JobSourceMappingQuarantined {
		t.Fatalf("a row that regained plaintext is %s after the restart, want still quarantined", mapping.Status)
	}
	readiness, err := ctx.GetJobMigrationReadiness()
	if err != nil {
		t.Fatal(err)
	}
	if readiness.Ready {
		t.Fatalf("a row that regained plaintext was accepted as retired: %+v", readiness)
	}
}

// TestEveryRestoredDeferredRowIsReexaminedInOnePass restores plaintext into two
// scrubbed rows: the first cannot be proved against its canonical input and the
// second can. The first one's quarantine must not leave the second one unexamined
// behind a mapping that still says it is scrubbed.
func TestEveryRestoredDeferredRowIsReexaminedInOnePass(t *testing.T) {
	ctx, _, actor, unprovable := newRetiredDeferredDownloadContext(t)
	provableCreator := &query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/provable.bin?token=private"}
	provable, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID, provableCreator, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create the second deferred download: %v", err)
	}
	dropScheduledDownloadBarriers(t, ctx)
	wrong := "https://example.invalid/somebody-else.bin?token=other"
	if err := ctx.db.Model(&models.ScheduledDownload{}).Where("id = ?", unprovable.ID).
		Updates(map[string]any{"url": wrong, "payload": mustScheduledPayload(t, &query_models.ResourceFromRemoteCreator{URL: wrong})}).Error; err != nil {
		t.Fatalf("restore unprovable plaintext: %v", err)
	}
	if err := ctx.db.Model(&models.ScheduledDownload{}).Where("id = ?", provable.ID).
		Updates(map[string]any{"url": provableCreator.URL, "payload": mustScheduledPayload(t, provableCreator)}).Error; err != nil {
		t.Fatalf("restore provable plaintext: %v", err)
	}
	reinstallScheduledDownloadBarriers(t, ctx)

	if _, err := ctx.RunJobMigrationToGate(JobMigrationOptions{BatchSize: 50, MaxBatches: 20}); err != nil {
		t.Fatalf("startup migration: %v", err)
	}
	if mapping := scheduledDownloadMapping(t, ctx, unprovable.ID); mapping.Status != models.JobSourceMappingQuarantined {
		t.Fatalf("the unprovable restored row is %s, want quarantined", mapping.Status)
	}
	mapping := scheduledDownloadMapping(t, ctx, provable.ID)
	if mapping.Status == models.JobSourceMappingScrubbed {
		t.Fatalf("the provable restored row still claims to be scrubbed while it holds plaintext: %+v", mapping)
	}
	if mapping.Status != models.JobSourceMappingVerified {
		t.Fatalf("the provable restored row is %s (%q), want verified against its canonical input", mapping.Status, mapping.BlockerCode)
	}
}

// quarantineAsAnEarlierReleaseDid writes the mapping and checkpoint an earlier
// release left when it read a fired row as a restored source. Each field is the
// one its code wrote: the re-arm's Save kept the scrub marker and post-scrub
// hash, and the copy pass's quarantine upsert replaced only status, blocker code,
// source hash and update time.
func quarantineAsAnEarlierReleaseDid(t *testing.T, ctx *MahresourcesContext, row models.ScheduledDownload, blockerCode, phase string) {
	t.Helper()
	mapping := scheduledDownloadMapping(t, ctx, row.ID)
	if mapping.ScrubbedAt == nil || mapping.PostScrubHash == "" {
		t.Fatalf("setup: the mapping has no scrub marker to keep: %+v", mapping)
	}
	updates := map[string]any{"status": models.JobSourceMappingQuarantined, "blocker_code": blockerCode, "updated_at": time.Now().UTC()}
	if blockerCode == "source-canonical-replay-mismatch" {
		updates["source_hash"] = hashScheduledDownload(row)
	}
	if err := ctx.db.Model(&models.JobSourceMapping{}).
		Where("source_kind = ? AND source_id = ?", jobMigrationScheduledDownload, mapping.SourceID).
		Updates(updates).Error; err != nil {
		t.Fatalf("quarantine the mapping: %v", err)
	}
	checkpoint := map[string]any{"phase": phase, "source_kind": "", "cursor_id": "", "completed_at": nil,
		"last_error": "source conversion is quarantined; writer drain and scrub are blocked"}
	if phase == models.JobMigrationPhaseCopy {
		checkpoint["source_kind"] = jobMigrationSourceKinds[0]
		checkpoint["last_error"] = ""
	}
	if err := ctx.db.Model(&models.JobMigrationCheckpoint{}).Where("id = ?", models.JobMigrationCheckpointRowID).
		Updates(checkpoint).Error; err != nil {
		t.Fatalf("move the checkpoint: %v", err)
	}
}

// reinstallScheduledDownloadBarriers puts the write triggers back, so readiness
// judges the restored rows rather than the missing barrier.
func reinstallScheduledDownloadBarriers(t *testing.T, ctx *MahresourcesContext) {
	t.Helper()
	if err := installLegacySourceBarriers(ctx.db); err != nil {
		t.Fatalf("reinstall the source barriers: %v", err)
	}
}

// dropScheduledDownloadBarriers removes the retired-source write triggers, which
// is what restoring the table from a backup taken before retirement amounts to.
func dropScheduledDownloadBarriers(t *testing.T, ctx *MahresourcesContext) {
	t.Helper()
	for _, trigger := range []string{"job_barrier_scheduled_download_insert", "job_barrier_scheduled_download_update"} {
		if err := ctx.db.Exec("DROP TRIGGER IF EXISTS " + trigger).Error; err != nil {
			t.Fatal(err)
		}
	}
}

// TestAFiredDeferredRowMustNameItsOwnJob pins what the fire-time exception
// accepts: the JobID a fire writes is the Job the row's handle named, which is
// the Job it was mapped to or a Retry successor of it. A scrubbed row that names
// some other Job is not the row that was scrubbed.
func TestAFiredDeferredRowMustNameItsOwnJob(t *testing.T) {
	ctx, _, actor, row := newRetiredDeferredDownloadContext(t)
	if fired := fireDueDeferredDownloads(t, ctx, time.Now()); fired != 1 {
		t.Fatalf("the sweep fired %d rows, want 1", fired)
	}
	other, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/other.bin"}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create another deferred download: %v", err)
	}
	unrelated := deferredDownloadJob(t, ctx, other.ID)
	if err := ctx.db.Model(&models.ScheduledDownload{}).Where("id = ?", row.ID).Update("job_id", unrelated.ID).Error; err != nil {
		t.Fatalf("point the fired row at another Job: %v", err)
	}
	readiness, err := ctx.GetJobMigrationReadiness()
	if err != nil {
		t.Fatal(err)
	}
	if readiness.Ready || readiness.Blockers["source-retirement-hash-mismatch/"+jobMigrationScheduledDownload] == 0 {
		t.Fatalf("a fired row naming an unrelated Job was accepted as retired: %+v", readiness)
	}

	// A Repeat of the row's own Job is a branch, not the Job its handle moves to.
	mapped := scheduledDownloadMapping(t, ctx, row.ID)
	if err := ctx.JobService().Link(ctx.jobDeps(), jobs.LinkRequest{Type: jobs.LinkRepeatOf, FromJobID: unrelated.ID, ToJobID: mapped.JobID}); err != nil {
		t.Fatalf("link the other Job as a repeat of the row's Job: %v", err)
	}
	readiness, err = ctx.GetJobMigrationReadiness()
	if err != nil {
		t.Fatal(err)
	}
	if readiness.Ready || readiness.Blockers["source-retirement-hash-mismatch/"+jobMigrationScheduledDownload] == 0 {
		t.Fatalf("a fired row naming a repeat of its Job was accepted as retired: %+v", readiness)
	}
}

// A deferral can be cancelled and rescheduled any number of times before it
// fires, and its row then names the last Job of that chain. The retirement check
// follows the whole chain back to the Job the row was mapped to.
func TestAFiredDeferredRowAtTheEndOfALongRetryChainIsRetired(t *testing.T) {
	ctx, key, _, row := newRetiredDeferredDownloadContext(t)
	if fired := fireDueDeferredDownloads(t, ctx, time.Now()); fired != 1 {
		t.Fatalf("the sweep fired %d rows, want 1", fired)
	}
	mapped := scheduledDownloadMapping(t, ctx, row.ID)
	previous := mapped.JobID
	leaf := ""
	for i := 0; i < 100; i++ {
		leaf = fmt.Sprintf("retry-chain-%03d", i)
		if err := ctx.db.Create(&models.JobLink{FromJobID: leaf, ToJobID: previous, Type: string(jobs.LinkRetryOf), CreatedAt: time.Now()}).Error; err != nil {
			t.Fatalf("link retry %d: %v", i, err)
		}
		previous = leaf
	}
	if err := ctx.db.Model(&models.ScheduledDownload{}).Where("id = ?", row.ID).Update("job_id", leaf).Error; err != nil {
		t.Fatalf("name the last Job of the chain: %v", err)
	}
	ctx = restartJobProcess(t, ctx, key)
	requireCleanBoot(t, ctx)
}
