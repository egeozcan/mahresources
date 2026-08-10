package application_context

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/constants"
	"mahresources/download_queue"
	"mahresources/models"
	"mahresources/models/query_models"
)

// newHistoryTestContext opens a temp-file SQLite DB rather than an in-memory
// one. `mode=memory&cache=private` gives every pooled connection its own
// database, so a row written on one connection is invisible on the next — which
// silently breaks any test that creates a user and then reads it back (the user
// comes back with a fresh id 1, and an attribution assertion passes for the
// wrong reason).
func newHistoryTestContext(t *testing.T, cfg *MahresourcesConfig) *MahresourcesContext {
	t.Helper()
	path := filepath.Join(t.TempDir(), "history.db")
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=10000&_synchronous=NORMAL", path)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.DownloadHistoryEntry{}, &models.User{}, &models.Group{},
		&models.RuntimeSetting{}, &models.LogEntry{}, &models.Session{}, &models.ApiToken{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	if cfg == nil {
		cfg = &MahresourcesConfig{}
	}
	cfg.DbType = constants.DbTypeSqlite
	return NewMahresourcesContext(afero.NewMemMapFs(), db, readOnlyDB, cfg)
}

func mustRecord(t *testing.T, ctx *MahresourcesContext, rec download_queue.HistoryRecord) {
	t.Helper()
	if err := ctx.RecordTerminalDownload(rec); err != nil {
		t.Fatalf("record %s: %v", rec.JobID, err)
	}
}

func historyRow(t *testing.T, ctx *MahresourcesContext, jobID string) models.DownloadHistoryEntry {
	t.Helper()
	var entry models.DownloadHistoryEntry
	if err := ctx.db.Where("job_id = ?", jobID).First(&entry).Error; err != nil {
		t.Fatalf("load %s: %v", jobID, err)
	}
	return entry
}

func TestRecordTerminalDownloadPersistsOwnerAndPayload(t *testing.T) {
	ctx := newHistoryTestContext(t, nil)
	now := time.Now()
	owner := uint(4)
	payload, _ := json.Marshal(query_models.ResourceFromRemoteCreator{URL: "http://example.invalid/a.bin"})

	mustRecord(t, ctx, download_queue.HistoryRecord{
		JobID:           "job-a",
		URL:             "http://example.invalid/a.bin",
		Name:            "a.bin",
		Status:          models.DownloadHistoryStatusFailed,
		Error:           "HTTP 404",
		CreatedAt:       now,
		CompletedAt:     &now,
		CreatedByUserId: &owner,
		Payload:         payload,
	})

	entry := historyRow(t, ctx, "job-a")
	if entry.Status != models.DownloadHistoryStatusFailed {
		t.Errorf("status = %q", entry.Status)
	}
	if entry.CreatedByUserId == nil || *entry.CreatedByUserId != owner {
		t.Errorf("owner = %v, want %d — the stamp callback must not overwrite it", entry.CreatedByUserId, owner)
	}
	if entry.Attempts != 1 {
		t.Errorf("attempts = %d, want 1", entry.Attempts)
	}
	creator, err := ctx.DownloadHistoryPayload(&entry)
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	if creator.URL != "http://example.invalid/a.bin" {
		t.Errorf("payload url = %q", creator.URL)
	}
}

// A recorded owner survives the create callback.
//
// The callback overwrites CreatedByUserId from the db context's acting user,
// falling back to the default actor — which, with a root admin present, is root.
// The recording runs on a worker goroutine that carries no principal, so without
// binding the submitter explicitly the row is reassigned to root. Since a
// non-admin sees only the rows it owns, that reassignment would hide a user's own
// failed download from them.
//
// Exercised with auth off, where the default actor is non-zero and the
// overwrite actually happens; with auth on the default is 0 and the callback
// returns early. Binding the actor covers both, and keeps covering them if that
// default ever changes.
func TestRecordTerminalDownloadKeepsOwnerOverDefaultActor(t *testing.T) {
	ctx := newHistoryTestContext(t, nil)
	admin := makeAdmin(t, ctx, "root-for-history")
	submitter, err := ctx.CreateUser(&UserInput{
		Username: "submitter", Password: "hunter22!", Role: models.RoleUser,
	})
	if err != nil {
		t.Fatalf("create submitter: %v", err)
	}
	if submitter.ID == admin.ID {
		t.Fatalf("fixture is broken: submitter and admin share id %d", admin.ID)
	}
	if got := ctx.defaultActorID(); got != admin.ID {
		t.Fatalf("default actor = %d, want the root admin (%d) — otherwise this test cannot observe the overwrite", got, admin.ID)
	}

	// Captured before the call, and the pointer handed over is the caller's own —
	// which is what the manager does, since a job holds its owner as a *uint.
	// Comparing against submitter.ID afterwards would be no assertion at all: the
	// defect being guarded against writes *through* that pointer, so the expected
	// value would move to match the wrong stored one and the test would pass.
	wantID := submitter.ID
	now := time.Now()
	mustRecord(t, ctx, download_queue.HistoryRecord{
		JobID: "owned", URL: "http://example.invalid/o", Status: models.DownloadHistoryStatusFailed,
		CreatedAt: now, CompletedAt: &now, CreatedByUserId: &submitter.ID,
	})

	entry := historyRow(t, ctx, "owned")
	if entry.CreatedByUserId == nil || *entry.CreatedByUserId != wantID {
		t.Fatalf("owner = %v, want the submitter (%d), not the default actor (%d)", entry.CreatedByUserId, wantID, admin.ID)
	}
	if submitter.ID != wantID {
		t.Fatalf("recording rewrote the caller's user id (%d → %d) — the manager's job holds that same pointer as its owner", wantID, submitter.ID)
	}
}

// A job that fails and is then retried in place is one download; the row it
// already has is updated, and the attempt counter moves.
func TestRecordTerminalDownloadUpsertsOnJobID(t *testing.T) {
	ctx := newHistoryTestContext(t, nil)
	now := time.Now()

	mustRecord(t, ctx, download_queue.HistoryRecord{
		JobID: "job-b", URL: "http://example.invalid/b", Status: models.DownloadHistoryStatusFailed,
		Error: "boom", CreatedAt: now, CompletedAt: &now,
	})
	resourceID := uint(99)
	mustRecord(t, ctx, download_queue.HistoryRecord{
		JobID: "job-b", URL: "http://example.invalid/b", Status: models.DownloadHistoryStatusCompleted,
		ResourceID: &resourceID, CreatedAt: now, CompletedAt: &now,
	})

	var count int64
	if err := ctx.db.Model(&models.DownloadHistoryEntry{}).Where("job_id = ?", "job-b").Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("rows for job-b = %d, want 1", count)
	}

	entry := historyRow(t, ctx, "job-b")
	if entry.Status != models.DownloadHistoryStatusCompleted {
		t.Errorf("status = %q, want completed", entry.Status)
	}
	if entry.Attempts != 2 {
		t.Errorf("attempts = %d, want 2", entry.Attempts)
	}
	if entry.ResourceID == nil || *entry.ResourceID != resourceID {
		t.Errorf("resource id = %v, want %d", entry.ResourceID, resourceID)
	}
	// The failed attempt's message must not survive onto a completed row.
	if entry.Error != "" {
		t.Errorf("error = %q, want it cleared by the successful attempt", entry.Error)
	}
}

// The two retention windows are separate, and the sweep reads them per call — so
// a failed download outlives a completed one by default, and a runtime change
// takes effect on the very next sweep.
func TestSweepDownloadHistoryHonoursBothRetentions(t *testing.T) {
	ctx := newHistoryTestContext(t, &MahresourcesConfig{
		DownloadFailedRetention:  7 * 24 * time.Hour,
		DownloadHistoryRetention: time.Hour,
	})

	old := time.Now().Add(-3 * time.Hour)
	recent := time.Now().Add(-time.Minute)
	rows := []download_queue.HistoryRecord{
		{JobID: "old-failed", Status: models.DownloadHistoryStatusFailed, CreatedAt: old, CompletedAt: &old},
		{JobID: "old-completed", Status: models.DownloadHistoryStatusCompleted, CreatedAt: old, CompletedAt: &old},
		{JobID: "recent-completed", Status: models.DownloadHistoryStatusCompleted, CreatedAt: recent, CompletedAt: &recent},
	}
	for _, r := range rows {
		mustRecord(t, ctx, r)
	}

	removed, err := ctx.SweepDownloadHistory()
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1 (only the old completed row is past its window)", removed)
	}

	var jobIDs []string
	if err := ctx.db.Model(&models.DownloadHistoryEntry{}).Order("job_id").Pluck("job_id", &jobIDs).Error; err != nil {
		t.Fatalf("list: %v", err)
	}
	want := map[string]bool{"old-failed": true, "recent-completed": true}
	if len(jobIDs) != 2 {
		t.Fatalf("remaining = %v, want the failed row and the recent completed one", jobIDs)
	}
	for _, id := range jobIDs {
		if !want[id] {
			t.Errorf("unexpected surviving row %q", id)
		}
	}
}

// A three-hour-old failure is swept once the window is shortened below three
// hours, with no restart in between — that is what "runtime config" has to mean.
func TestSweepDownloadHistoryPicksUpRuntimeChange(t *testing.T) {
	ctx := newHistoryTestContext(t, &MahresourcesConfig{
		DownloadFailedRetention:  7 * 24 * time.Hour,
		DownloadHistoryRetention: 7 * 24 * time.Hour,
	})
	settings := NewRuntimeSettings(ctx.db, NewStdlibSettingsLogger(), BuildSpecsExported(), BuildDefaultsFromConfig(ctx.Config))
	ctx.SetSettings(settings)

	old := time.Now().Add(-3 * time.Hour)
	mustRecord(t, ctx, download_queue.HistoryRecord{
		JobID: "stale-failure", Status: models.DownloadHistoryStatusFailed, CreatedAt: old, CompletedAt: &old,
	})

	if removed, err := ctx.SweepDownloadHistory(); err != nil || removed != 0 {
		t.Fatalf("first sweep: removed=%d err=%v, want the row kept", removed, err)
	}

	if err := settings.Set(KeyDownloadFailedRetention, "1h", "test", "tester"); err != nil {
		t.Fatalf("set retention: %v", err)
	}

	removed, err := ctx.SweepDownloadHistory()
	if err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1 after shortening the window", removed)
	}
}

// A zero retention in the config means "never configured", not "expire on
// write". Every test fixture that builds MahresourcesConfig{} directly relies on
// this, and an operator who has never touched the flags does too.
func TestSweepDownloadHistoryZeroRetentionFallsBack(t *testing.T) {
	ctx := newHistoryTestContext(t, &MahresourcesConfig{})
	now := time.Now()
	mustRecord(t, ctx, download_queue.HistoryRecord{
		JobID: "fresh", Status: models.DownloadHistoryStatusFailed, CreatedAt: now, CompletedAt: &now,
	})

	removed, err := ctx.SweepDownloadHistory()
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if removed != 0 {
		t.Fatalf("removed = %d, want 0 — a zero retention must not empty the table", removed)
	}
}

// The owner predicate is part of the scope, so a restricted caller sees only its
// own rows and can delete only its own — including the fail-closed treatment of
// rows with no owner.
func TestDownloadHistoryOwnerScoping(t *testing.T) {
	ctx := newHistoryTestContext(t, nil)
	now := time.Now()
	mine, theirs := uint(1), uint(2)

	mustRecord(t, ctx, download_queue.HistoryRecord{JobID: "mine", Status: models.DownloadHistoryStatusFailed, CreatedAt: now, CompletedAt: &now, CreatedByUserId: &mine})
	mustRecord(t, ctx, download_queue.HistoryRecord{JobID: "theirs", Status: models.DownloadHistoryStatusFailed, CreatedAt: now, CompletedAt: &now, CreatedByUserId: &theirs})
	mustRecord(t, ctx, download_queue.HistoryRecord{JobID: "ownerless", Status: models.DownloadHistoryStatusFailed, CreatedAt: now, CompletedAt: &now})

	restricted := &query_models.DownloadHistoryQuery{OwnerUserID: &mine, OwnerRestricted: true}
	entries, err := ctx.GetDownloadHistory(0, 50, restricted)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 || entries[0].JobID != "mine" {
		t.Fatalf("restricted listing = %+v, want only the caller's own row", entries)
	}

	all, err := ctx.GetDownloadHistory(0, 50, &query_models.DownloadHistoryQuery{})
	if err != nil {
		t.Fatalf("admin list: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("admin listing = %d rows, want 3", len(all))
	}

	// A restricted caller cannot delete a row it cannot see, even naming its id.
	var theirRow models.DownloadHistoryEntry
	if err := ctx.db.Where("job_id = ?", "theirs").First(&theirRow).Error; err != nil {
		t.Fatalf("load theirs: %v", err)
	}
	deleted, err := ctx.DeleteDownloadHistoryEntries([]uint{theirRow.ID}, &mine, true, time.Now())
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("deleted = %d, want 0 — another user's row must be untouchable", deleted)
	}
}

// A URL filter searches the name as well: which of the two carried the word the
// user remembers is not a distinction worth making them draw.
func TestDownloadHistoryURLFilterMatchesName(t *testing.T) {
	ctx := newHistoryTestContext(t, nil)
	now := time.Now()
	mustRecord(t, ctx, download_queue.HistoryRecord{JobID: "one", URL: "http://example.invalid/x1", Name: "holiday-photos.zip", Status: models.DownloadHistoryStatusFailed, CreatedAt: now, CompletedAt: &now})
	mustRecord(t, ctx, download_queue.HistoryRecord{JobID: "two", URL: "http://example.invalid/holiday", Name: "b.bin", Status: models.DownloadHistoryStatusFailed, CreatedAt: now, CompletedAt: &now})
	mustRecord(t, ctx, download_queue.HistoryRecord{JobID: "three", URL: "http://example.invalid/z", Name: "c.bin", Status: models.DownloadHistoryStatusFailed, CreatedAt: now, CompletedAt: &now})

	entries, err := ctx.GetDownloadHistory(0, 50, &query_models.DownloadHistoryQuery{URL: "holiday"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("matches = %d, want 2 (one by name, one by url)", len(entries))
	}
}

// Two requests that both find no live attempt would both resubmit an evicted
// download — two jobs, two resources, one row whose marker records whichever
// wrote last. The retry slot is therefore claimed, not merely recorded: the
// compare-and-set is on the marker the caller read, so exactly one wins.
func TestClaimDownloadHistoryRetryAdmitsOneWinner(t *testing.T) {
	ctx := newHistoryTestContext(t, &MahresourcesConfig{})
	now := time.Now()
	mustRecord(t, ctx, download_queue.HistoryRecord{
		JobID: "contended", URL: "http://example.invalid/x", Status: models.DownloadHistoryStatusFailed,
		CreatedAt: now, CompletedAt: &now,
	})
	entry := historyRow(t, ctx, "contended")

	claimed, err := ctx.ClaimDownloadHistoryRetry(entry.ID, "", models.DownloadHistoryStatusFailed, "claim-a", time.Now())
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if !claimed {
		t.Fatal("the first claim was refused")
	}

	// The second request read the same row before either wrote, so it offers the
	// same expected marker — and must lose.
	claimed, err = ctx.ClaimDownloadHistoryRetry(entry.ID, "", models.DownloadHistoryStatusFailed, "claim-b", time.Now())
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if claimed {
		t.Fatal("both requests claimed the same retry slot; the download would run twice")
	}

	// The status the caller decided from is part of the claim. An in-place retry
	// that finished while the request was in flight has already turned the row into
	// `completed`, and resubmitting then downloads a file that now exists.
	if err := ctx.db.Model(&models.DownloadHistoryEntry{}).Where("id = ?", entry.ID).
		Updates(map[string]any{"status": models.DownloadHistoryStatusCompleted, "last_retry_job_id": ""}).Error; err != nil {
		t.Fatalf("simulate the in-place retry completing: %v", err)
	}
	claimed, err = ctx.ClaimDownloadHistoryRetry(entry.ID, "", models.DownloadHistoryStatusFailed, "claim-c", time.Now())
	if err != nil {
		t.Fatalf("stale-status claim: %v", err)
	}
	if claimed {
		t.Fatal("a request that read `failed` claimed a row that has since completed")
	}
	if err := ctx.db.Model(&models.DownloadHistoryEntry{}).Where("id = ?", entry.ID).
		Updates(map[string]any{"status": models.DownloadHistoryStatusFailed, "last_retry_job_id": "claim-a"}).Error; err != nil {
		t.Fatalf("restore: %v", err)
	}

	// Handing the slot back is the same operation with the arguments reversed, so a
	// submit that fails does not leave the row claimed forever.
	released, err := ctx.ClaimDownloadHistoryRetry(entry.ID, "claim-a", "", "", time.Now())
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if !released {
		t.Fatal("the holder could not hand its own claim back")
	}
}

// A row whose retry slot was claimed after the deleting request read it survives:
// the decision to delete was made from a copy that predates the claim, and the
// row is the only record of a download that is starting right now.
func TestDeleteDownloadHistorySkipsRowsRetriedSinceTheRead(t *testing.T) {
	ctx := newHistoryTestContext(t, &MahresourcesConfig{})
	now := time.Now()
	mustRecord(t, ctx, download_queue.HistoryRecord{
		JobID: "deleted-mid-retry", URL: "http://example.invalid/x", Status: models.DownloadHistoryStatusFailed,
		CreatedAt: now, CompletedAt: &now,
	})
	entry := historyRow(t, ctx, "deleted-mid-retry")

	loadedAt := time.Now()
	time.Sleep(2 * time.Millisecond)
	if _, err := ctx.ClaimDownloadHistoryRetry(entry.ID, "", models.DownloadHistoryStatusFailed, "claim-a", time.Now()); err != nil {
		t.Fatalf("claim: %v", err)
	}

	deleted, err := ctx.DeleteDownloadHistoryEntries([]uint{entry.ID}, nil, false, loadedAt)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deleted != 0 {
		t.Fatal("the row was deleted although a retry had claimed it since the read")
	}

	// A read taken after the claim deletes it, so this is a rule about ordering
	// rather than a row that has become undeletable.
	deleted, err = ctx.DeleteDownloadHistoryEntries([]uint{entry.ID}, nil, false, time.Now())
	if err != nil {
		t.Fatalf("second delete: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
}

// An expired row that someone has just retried is not idle history: its own
// status still describes the attempt that failed, so both expiry predicates match
// it while the attempt launched from it is downloading — and the row would vanish
// from under the user who pressed the button.
func TestSweepDownloadHistorySpareRowsRetriedInsideTheWindow(t *testing.T) {
	ctx := newHistoryTestContext(t, &MahresourcesConfig{
		DownloadFailedRetention:  time.Hour,
		DownloadHistoryRetention: time.Hour,
	})
	old := time.Now().Add(-48 * time.Hour)
	mustRecord(t, ctx, download_queue.HistoryRecord{
		JobID: "expired-but-retried", URL: "http://example.invalid/x",
		Status: models.DownloadHistoryStatusFailed, CreatedAt: old, CompletedAt: &old,
	})
	mustRecord(t, ctx, download_queue.HistoryRecord{
		JobID: "expired-and-idle", URL: "http://example.invalid/y",
		Status: models.DownloadHistoryStatusFailed, CreatedAt: old, CompletedAt: &old,
	})

	retried := historyRow(t, ctx, "expired-but-retried")
	if err := ctx.MarkDownloadHistoryRetried(retried.ID, "the-new-job", time.Now()); err != nil {
		t.Fatalf("mark retried: %v", err)
	}

	deleted, err := ctx.SweepDownloadHistory()
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("swept %d rows, want 1", deleted)
	}

	var survivors int64
	ctx.db.Model(&models.DownloadHistoryEntry{}).Where("job_id = ?", "expired-but-retried").Count(&survivors)
	if survivors != 1 {
		t.Fatal("the sweep deleted a row whose retry is running")
	}
}
