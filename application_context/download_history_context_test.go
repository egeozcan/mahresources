package application_context

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
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
		&models.DownloadHistoryEntry{},
		&models.PluginSchedule{}, &models.User{}, &models.Group{},
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

// The retries filter answers "was this download ever run again", which is true
// of both shapes a rerun takes: an in-place retry, which bumps attempts, and a
// resubmission, which links back through last_retry_job_id. Every row must land
// under exactly one of the two answers — including one whose retry column is
// NULL, which a plain `<> ”` comparison would drop from both.
func TestDownloadHistoryRetriedFilter(t *testing.T) {
	ctx := newHistoryTestContext(t, nil)
	now := time.Now()
	mustRecord(t, ctx, download_queue.HistoryRecord{JobID: "fresh", Status: models.DownloadHistoryStatusFailed, CreatedAt: now, CompletedAt: &now})
	// Recorded twice: the second terminal outcome for one job is the in-place
	// retry, which is what bumps attempts to 2.
	mustRecord(t, ctx, download_queue.HistoryRecord{JobID: "inplace", Status: models.DownloadHistoryStatusFailed, CreatedAt: now, CompletedAt: &now})
	mustRecord(t, ctx, download_queue.HistoryRecord{JobID: "inplace", Status: models.DownloadHistoryStatusFailed, CreatedAt: now, CompletedAt: &now})
	mustRecord(t, ctx, download_queue.HistoryRecord{JobID: "resubmitted", Status: models.DownloadHistoryStatusFailed, CreatedAt: now, CompletedAt: &now})
	if err := ctx.db.Model(&models.DownloadHistoryEntry{}).Where("job_id = ?", "resubmitted").
		Update("last_retry_job_id", "abc123").Error; err != nil {
		t.Fatalf("link the retry: %v", err)
	}
	// A row written before the column existed holds NULL, not "".
	if err := ctx.db.Model(&models.DownloadHistoryEntry{}).Where("job_id = ?", "fresh").
		Update("last_retry_job_id", gorm.Expr("NULL")).Error; err != nil {
		t.Fatalf("null the retry column: %v", err)
	}

	jobIDs := func(q *query_models.DownloadHistoryQuery) []string {
		t.Helper()
		entries, err := ctx.GetDownloadHistory(0, 50, q)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		out := make([]string, 0, len(entries))
		for _, e := range entries {
			out = append(out, e.JobID)
		}
		sort.Strings(out)
		return out
	}

	retried := jobIDs(&query_models.DownloadHistoryQuery{Retried: query_models.DownloadRetriedYes})
	if !reflect.DeepEqual(retried, []string{"inplace", "resubmitted"}) {
		t.Fatalf("retried = %v, want [inplace resubmitted]", retried)
	}
	never := jobIDs(&query_models.DownloadHistoryQuery{Retried: query_models.DownloadRetriedNo})
	if !reflect.DeepEqual(never, []string{"fresh"}) {
		t.Fatalf("never retried = %v, want [fresh]", never)
	}
	all := jobIDs(&query_models.DownloadHistoryQuery{})
	if len(all) != 3 {
		t.Fatalf("unfiltered = %v, want all three rows", all)
	}

	count, err := ctx.GetDownloadHistoryCount(&query_models.DownloadHistoryQuery{Retried: query_models.DownloadRetriedYes})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("retried count = %d, want 2 — the count must match the listing", count)
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

// The finished-at window bounds completed_at, not created_at: a download queued
// on Monday and finished on Tuesday belongs to Tuesday's window and to Monday's
// submission range, and a filter that confused the two would answer "what
// finished last night?" with what was *asked for* last night.
func TestDownloadHistoryCompletedDateRangeBoundsCompletedAt(t *testing.T) {
	ctx := newHistoryTestContext(t, nil)
	submitted := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	monday := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	wednesday := time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)

	mustRecord(t, ctx, download_queue.HistoryRecord{JobID: "early", Status: models.DownloadHistoryStatusCompleted, CreatedAt: submitted, CompletedAt: &monday})
	mustRecord(t, ctx, download_queue.HistoryRecord{JobID: "late", Status: models.DownloadHistoryStatusCompleted, CreatedAt: submitted, CompletedAt: &wednesday})

	got := historyJobIDs(t, ctx, &query_models.DownloadHistoryQuery{CompletedAfter: "2026-03-03"})
	if !reflect.DeepEqual(got, []string{"late"}) {
		t.Fatalf("CompletedAfter=2026-03-03 = %v, want [late]", got)
	}
	got = historyJobIDs(t, ctx, &query_models.DownloadHistoryQuery{CompletedBefore: "2026-03-03"})
	if !reflect.DeepEqual(got, []string{"early"}) {
		t.Fatalf("CompletedBefore=2026-03-03 = %v, want [early]", got)
	}
	// The submission range still asks about created_at, so a window that contains
	// neither completion keeps both rows.
	got = historyJobIDs(t, ctx, &query_models.DownloadHistoryQuery{CreatedAfter: "2026-03-01", CreatedBefore: "2026-03-02"})
	if !reflect.DeepEqual(got, []string{"early", "late"}) {
		t.Fatalf("created range = %v, want both rows — the two ranges are different questions", got)
	}
}

// A row that never finished — one recorded with no completion time — is outside
// every finished-at window rather than inside all of them, which is what a NULL
// comparison gives and what the page needs.
func TestDownloadHistoryCompletedRangeExcludesRowsWithNoCompletion(t *testing.T) {
	ctx := newHistoryTestContext(t, nil)
	submitted := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	done := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	mustRecord(t, ctx, download_queue.HistoryRecord{JobID: "unfinished", Status: models.DownloadHistoryStatusFailed, CreatedAt: submitted})
	mustRecord(t, ctx, download_queue.HistoryRecord{JobID: "finished", Status: models.DownloadHistoryStatusCompleted, CreatedAt: submitted, CompletedAt: &done})

	for _, q := range []*query_models.DownloadHistoryQuery{
		{CompletedAfter: "2020-01-01"},
		{CompletedBefore: "2030-01-01"},
	} {
		if got := historyJobIDs(t, ctx, q); !reflect.DeepEqual(got, []string{"finished"}) {
			t.Fatalf("%+v = %v, want [finished]", q, got)
		}
	}
}

// Each bucket claims the errors the download path actually produces, and claims
// no other bucket's. The strings below are copied from the constructors named in
// query_models.DownloadFailureReasons.
func TestDownloadHistoryReasonFilterClassifiesRealErrors(t *testing.T) {
	ctx := newHistoryTestContext(t, nil)
	now := time.Now()
	samples := map[string]string{
		query_models.DownloadReasonHTTP:    "HTTP 404: 404 Not Found",
		query_models.DownloadReasonTimeout: "remote server stopped sending data (idle timeout after 1m0s)",
		// The host-hiding form, which is what an ordinary loopback URL produces:
		// the refusal omits the host rather than reporting what it resolved to.
		query_models.DownloadReasonBlocked:     "blocked request: it resolves to an address this server is not permitted to fetch from",
		query_models.DownloadReasonUnsupported: "this HLS stream is protected by DRM (SAMPLE-AES) and cannot be downloaded",
		query_models.DownloadReasonLimit:       "this HLS playlist lists 9000 segments, which is over this server's limit of 5000",
		query_models.DownloadReasonStorage:     "could not write the local playlist: no space left on device",
		query_models.DownloadReasonCancelled:   "Download cancelled",
	}
	for key, message := range samples {
		mustRecord(t, ctx, download_queue.HistoryRecord{
			JobID: key, Status: models.DownloadHistoryStatusFailed, Error: message,
			CreatedAt: now, CompletedAt: &now,
		})
	}
	// Nothing in the table claims this one, so it is what "Other" is for.
	mustRecord(t, ctx, download_queue.HistoryRecord{
		JobID: query_models.DownloadReasonOther, Status: models.DownloadHistoryStatusFailed,
		Error: "the remote closed the connection unexpectedly", CreatedAt: now, CompletedAt: &now,
	})
	// A completed download carries no error, so it belongs to no reason at all —
	// including "Other", which is for failures the classification has not learned
	// about rather than for everything unclassified.
	mustRecord(t, ctx, download_queue.HistoryRecord{
		JobID: "clean", Status: models.DownloadHistoryStatusCompleted, CreatedAt: now, CompletedAt: &now,
	})

	for key := range samples {
		got := historyJobIDs(t, ctx, &query_models.DownloadHistoryQuery{Reason: key})
		if !reflect.DeepEqual(got, []string{key}) {
			t.Errorf("Reason=%s = %v, want [%s] — %q", key, got, key, samples[key])
		}
	}
	// Any non-2xx is stamped as "HTTP <code>: ...", 3xx included.
	mustRecord(t, ctx, download_queue.HistoryRecord{
		JobID: "redirect", Status: models.DownloadHistoryStatusFailed,
		Error: "HTTP 304: 304 Not Modified", CreatedAt: now, CompletedAt: &now,
	})
	if got := historyJobIDs(t, ctx, &query_models.DownloadHistoryQuery{Reason: query_models.DownloadReasonHTTP}); len(got) != 2 {
		t.Fatalf("Reason=http = %v, want the 4xx and the 3xx", got)
	}

	// The form that does name the host is the same bucket.
	mustRecord(t, ctx, download_queue.HistoryRecord{
		JobID: "named-host", Status: models.DownloadHistoryStatusFailed,
		Error:     "blocked request to metadata.internal (169.254.169.254): private address",
		CreatedAt: now, CompletedAt: &now,
	})
	if got := historyJobIDs(t, ctx, &query_models.DownloadHistoryQuery{Reason: query_models.DownloadReasonBlocked}); len(got) != 2 {
		t.Fatalf("Reason=blocked = %v, want both refusal forms", got)
	}

	got := historyJobIDs(t, ctx, &query_models.DownloadHistoryQuery{Reason: query_models.DownloadReasonOther})
	if !reflect.DeepEqual(got, []string{query_models.DownloadReasonOther}) {
		t.Fatalf("Reason=other = %v, want the one unclassifiable failure", got)
	}
	// An unrecognised key is lenient, as Retried is.
	if got := historyJobIDs(t, ctx, &query_models.DownloadHistoryQuery{Reason: "not-a-reason"}); len(got) != len(samples)+4 {
		t.Fatalf("Reason=not-a-reason = %v, want every row", got)
	}
}

// The error box is a substring search over the stored message, case-insensitive
// on both engines because it goes through GetLikeOperator, and AND-ed with the
// category rather than replacing it.
func TestDownloadHistoryErrorFilter(t *testing.T) {
	ctx := newHistoryTestContext(t, nil)
	now := time.Now()
	mustRecord(t, ctx, download_queue.HistoryRecord{JobID: "notfound", Status: models.DownloadHistoryStatusFailed, Error: "HTTP 404: 404 Not Found", CreatedAt: now, CompletedAt: &now})
	mustRecord(t, ctx, download_queue.HistoryRecord{JobID: "server", Status: models.DownloadHistoryStatusFailed, Error: "HTTP 503: 503 Service Unavailable", CreatedAt: now, CompletedAt: &now})
	mustRecord(t, ctx, download_queue.HistoryRecord{JobID: "stalled", Status: models.DownloadHistoryStatusFailed, Error: "the server stopped sending data for 1m0s", CreatedAt: now, CompletedAt: &now})

	if got := historyJobIDs(t, ctx, &query_models.DownloadHistoryQuery{Error: "not found"}); !reflect.DeepEqual(got, []string{"notfound"}) {
		t.Fatalf("Error=\"not found\" = %v, want [notfound] — the match is case-insensitive", got)
	}
	if got := historyJobIDs(t, ctx, &query_models.DownloadHistoryQuery{Reason: query_models.DownloadReasonHTTP, Error: "503"}); !reflect.DeepEqual(got, []string{"server"}) {
		t.Fatalf("reason+error = %v, want [server] — the two filters are AND-ed", got)
	}
	// A wildcard is a literal: the box searches for text, not for a pattern.
	if got := historyJobIDs(t, ctx, &query_models.DownloadHistoryQuery{Error: "%"}); len(got) != 0 {
		t.Fatalf("Error=%% = %v, want no rows — the pattern characters are escaped", got)
	}
	// A NUL is answered here rather than sent to the engine: Postgres refuses one
	// inside a text parameter (SQLSTATE 22021), so `?error=%00` was an HTTP 500
	// where the honest answer is that nothing matches.
	// Bytes Postgres refuses inside a text parameter: a NUL, and invalid UTF-8.
	// Both are answered as "no matches" rather than sent to the engine, which
	// answered HTTP 500.
	for _, q := range []query_models.DownloadHistoryQuery{
		{Error: "\x00"}, {URL: "a\x00b"}, {Error: "\xff"}, {URL: "a\xffb"},
	} {
		if got := historyJobIDs(t, ctx, &q); len(got) != 0 {
			t.Fatalf("%+v = %v, want no rows", q, got)
		}
	}
}

func historyJobIDs(t *testing.T, ctx *MahresourcesContext, q *query_models.DownloadHistoryQuery) []string {
	t.Helper()
	entries, err := ctx.GetDownloadHistory(0, 50, q)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	count, err := ctx.GetDownloadHistoryCount(q)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if int(count) != len(entries) {
		t.Fatalf("count = %d but the listing returned %d rows — the two must apply the same filters", count, len(entries))
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.JobID)
	}
	sort.Strings(out)
	return out
}

// An RFC 3339 bound must select the same rows a bare date does, on both engines.
//
// SQLite has no date type: the time is stored as text with a space where RFC
// 3339 writes a `T`, and a space sorts below every digit — so before this was
// normalised, `CompletedBefore=<13:00>` *kept* a download that finished at 14:00
// and `CompletedAfter=<13:00>` dropped it. Both bounds were wrong, in opposite
// directions, and neither reported an error. The API documents RFC 3339 for
// every date filter, so this pins created_at as well as completed_at.
func TestDownloadHistoryRFC3339BoundsAreNotCompletedLexically(t *testing.T) {
	ctx := newHistoryTestContext(t, nil)
	afternoon := time.Date(2026, 3, 3, 14, 0, 0, 0, time.UTC)
	mustRecord(t, ctx, download_queue.HistoryRecord{
		JobID: "afternoon", Status: models.DownloadHistoryStatusCompleted,
		CreatedAt: afternoon, CompletedAt: &afternoon,
	})

	cases := []struct {
		name  string
		query query_models.DownloadHistoryQuery
		want  []string
	}{
		{"finished before an earlier instant", query_models.DownloadHistoryQuery{CompletedBefore: "2026-03-03T13:00:00Z"}, []string{}},
		{"finished before a later instant", query_models.DownloadHistoryQuery{CompletedBefore: "2026-03-03T15:00:00Z"}, []string{"afternoon"}},
		{"finished after an earlier instant", query_models.DownloadHistoryQuery{CompletedAfter: "2026-03-03T13:00:00Z"}, []string{"afternoon"}},
		{"finished after a later instant", query_models.DownloadHistoryQuery{CompletedAfter: "2026-03-03T15:00:00Z"}, []string{}},
		// The same bound written in another offset names the same instant.
		{"same instant, other offset", query_models.DownloadHistoryQuery{CompletedAfter: "2026-03-03T15:30:00+02:00"}, []string{"afternoon"}},
		// A bound is accurate to the second, and the second it names is wholly
		// inside the range: sub-second precision is truncated on both sides,
		// because SQLite's millisecond field cannot be matched from Go (see
		// sqliteInstantFormat). A bound one millisecond past the row therefore
		// still contains it; one second past does not.
		{"one second before", query_models.DownloadHistoryQuery{CompletedAfter: "2026-03-03T13:59:59Z"}, []string{"afternoon"}},
		{"one millisecond after, same second", query_models.DownloadHistoryQuery{CompletedAfter: "2026-03-03T14:00:00.001Z"}, []string{"afternoon"}},
		{"one second after", query_models.DownloadHistoryQuery{CompletedAfter: "2026-03-03T14:00:01Z"}, []string{}},
		// Go's RFC 3339 admits a comma fraction; SQLite's own date functions
		// answer NULL for one, which would silently match nothing. The bound is
		// parsed in Go for that reason.
		{"comma fraction", query_models.DownloadHistoryQuery{CompletedAfter: "2026-03-03T13:00:00,5Z"}, []string{"afternoon"}},
		// Postgres rejects a comma fraction as timestamptz input, so the bound is
		// re-emitted in canonical form there rather than passed through.
		{"comma fraction, upper bound", query_models.DownloadHistoryQuery{CompletedBefore: "2026-03-03T13:00:00,5Z"}, []string{}},
		{"submitted before an earlier instant", query_models.DownloadHistoryQuery{CreatedBefore: "2026-03-03T13:00:00Z"}, []string{}},
		{"submitted after an earlier instant", query_models.DownloadHistoryQuery{CreatedAfter: "2026-03-03T13:00:00Z"}, []string{"afternoon"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := historyJobIDs(t, ctx, &tc.query); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("= %v, want %v", got, tc.want)
			}
		})
	}
}

// Every sentence download_queue stamps on a cancellation belongs to the
// cancelled bucket. Enumerating them was wrong the day it was written: the
// message a cancel lands on depends on where in the transfer it arrived, and the
// one that was missing filed a cancelled download under "Other".
func TestDownloadHistoryCancelledReasonCoversEveryCancellationMessage(t *testing.T) {
	ctx := newHistoryTestContext(t, nil)
	now := time.Now()
	messages := []string{
		"Download cancelled",
		"Cancelled before starting",
		"Cancelled after the file had been saved",
	}
	for i, message := range messages {
		mustRecord(t, ctx, download_queue.HistoryRecord{
			JobID: fmt.Sprintf("c%d", i), Status: models.DownloadHistoryStatusCancelled,
			Error: message, CreatedAt: now, CompletedAt: &now,
		})
	}
	if got := historyJobIDs(t, ctx, &query_models.DownloadHistoryQuery{Reason: query_models.DownloadReasonCancelled}); len(got) != len(messages) {
		t.Fatalf("Reason=cancelled = %v, want all %d cancellation messages", got, len(messages))
	}
	if got := historyJobIDs(t, ctx, &query_models.DownloadHistoryQuery{Reason: query_models.DownloadReasonOther}); len(got) != 0 {
		t.Fatalf("Reason=other = %v, want none — a cancellation is not an unclassified failure", got)
	}
}

// The buckets overlap on purpose, and a test that only checked "each sample is
// in its own bucket" would not say so. A gateway timeout is an HTTP error and a
// timeout, and a user who filtered for either should find it.
func TestDownloadHistoryReasonBucketsMayOverlap(t *testing.T) {
	ctx := newHistoryTestContext(t, nil)
	now := time.Now()
	mustRecord(t, ctx, download_queue.HistoryRecord{
		JobID: "gateway", Status: models.DownloadHistoryStatusFailed,
		Error: "HTTP 504: 504 Gateway Timeout", CreatedAt: now, CompletedAt: &now,
	})
	for _, reason := range []string{query_models.DownloadReasonHTTP, query_models.DownloadReasonTimeout} {
		if got := historyJobIDs(t, ctx, &query_models.DownloadHistoryQuery{Reason: reason}); !reflect.DeepEqual(got, []string{"gateway"}) {
			t.Fatalf("Reason=%s = %v, want [gateway]", reason, got)
		}
	}
	// A row any bucket claims is not "Other", however many claim it.
	if got := historyJobIDs(t, ctx, &query_models.DownloadHistoryQuery{Reason: query_models.DownloadReasonOther}); len(got) != 0 {
		t.Fatalf("Reason=other = %v, want none", got)
	}
}

// A bound naming the very instant a row was stamped at must include it, and
// SQLite's millisecond field is why that is not free. strftime's %f rounds —
// .9009 reads back as .901 — except at a second boundary, where it clamps: .9999
// reads back as .999, not the next second. No Go layout does both, so a bound
// formatted to match one rule disagreed with the column under the other, and a
// completion at 14:00:59.9999 was excluded by a bound at its own instant. Both
// sides truncate to the second instead.
func TestDownloadHistorySubSecondBoundsIncludeTheirOwnInstant(t *testing.T) {
	ctx := newHistoryTestContext(t, nil)
	instants := map[string]time.Time{
		// Rounds up inside the second.
		"midsecond": time.Date(2026, 3, 3, 14, 0, 0, 900900000, time.UTC),
		// Would carry into the next second, which %f refuses to do.
		"carry": time.Date(2026, 3, 3, 14, 0, 59, 999900000, time.UTC),
	}
	bounds := map[string]string{
		"midsecond": "2026-03-03T14:00:00.9009Z",
		"carry":     "2026-03-03T14:00:59.9999Z",
	}
	for name, at := range instants {
		at := at
		t.Run(name, func(t *testing.T) {
			ctx := ctx
			mustRecord(t, ctx, download_queue.HistoryRecord{
				JobID: name, Status: models.DownloadHistoryStatusCompleted, CreatedAt: at, CompletedAt: &at,
			})
			for _, q := range []query_models.DownloadHistoryQuery{
				{CompletedAfter: bounds[name], URL: ""},
				{CompletedBefore: bounds[name]},
			} {
				got := historyJobIDs(t, ctx, &q)
				found := false
				for _, id := range got {
					if id == name {
						found = true
					}
				}
				if !found {
					t.Fatalf("%+v = %v, want it to contain %q — a bound at the row's own instant excludes it", q, got, name)
				}
			}
		})
	}
}

// Go's RFC 3339 is wider than the databases underneath it at both ends, and both
// extremes reached the query as a wrong answer rather than an error. Year zero
// exists to Go and not to Postgres; and `9999-12-31T23:00:00-14:00` is year
// 10000 once it is in UTC, whose five-digit string breaks the fixed-width
// lexical ordering SQLite compares on — including the row under a lower bound
// that is meant to be above every row, and excluding it under an upper bound
// that is meant to be below none.
func TestDownloadHistoryBoundsOutsideEveryStorableYearAreAnsweredByTheirEnd(t *testing.T) {
	ctx := newHistoryTestContext(t, nil)
	at := time.Date(2026, 3, 3, 14, 0, 0, 0, time.UTC)
	mustRecord(t, ctx, download_queue.HistoryRecord{JobID: "row", Status: models.DownloadHistoryStatusCompleted, CreatedAt: at, CompletedAt: &at})
	// Rows sitting exactly on the representable extremes. Clamping such a bound
	// to the nearest instant instead of answering by its end put each of these on
	// the wrong side of a bound naming something beyond it.
	edges := map[string]time.Time{
		"firstInstant": time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC),
		"lastInstant":  time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC),
	}
	for id, edge := range edges {
		edge := edge
		mustRecord(t, ctx, download_queue.HistoryRecord{JobID: id, Status: models.DownloadHistoryStatusCompleted, CreatedAt: edge, CompletedAt: &edge})
	}
	// An unfinished download is outside every window that names a finish time,
	// including one no row can be outside of.
	mustRecord(t, ctx, download_queue.HistoryRecord{JobID: "unfinished", Status: models.DownloadHistoryStatusFailed, CreatedAt: at})

	cases := []struct {
		name  string
		query query_models.DownloadHistoryQuery
		want  []string
	}{
		{"below every year, as a lower bound", query_models.DownloadHistoryQuery{CompletedAfter: "0000-01-01T00:00:00Z"}, []string{"firstInstant", "lastInstant", "row"}},
		{"below every year, as an upper bound", query_models.DownloadHistoryQuery{CompletedBefore: "0000-01-01T00:00:00Z"}, []string{}},
		{"above every year, as a lower bound", query_models.DownloadHistoryQuery{CompletedAfter: "9999-12-31T23:00:00-14:00"}, []string{}},
		{"above every year, as an upper bound", query_models.DownloadHistoryQuery{CompletedBefore: "9999-12-31T23:00:00-14:00"}, []string{"firstInstant", "lastInstant", "row"}},
		{"below every year, as a bare date", query_models.DownloadHistoryQuery{CompletedBefore: "0000-01-01"}, []string{}},
		// An offset Go admits and Postgres refuses outright. Normalised to UTC.
		{"an offset past what Postgres accepts", query_models.DownloadHistoryQuery{CompletedAfter: "2026-03-03T23:00:00+23:00"}, []string{"lastInstant", "row"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := historyJobIDs(t, ctx, &tc.query); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("= %v, want %v", got, tc.want)
			}
		})
	}
}

// A refusal hls writes as a sentence must land in "unsupported" rather than
// falling through to "Other": a live stream says "only complete recordings can
// be downloaded", which reads straight past a "cannot be downloaded" pattern.
func TestDownloadHistoryUnsupportedBucketCoversTheRefusalsHLSWrites(t *testing.T) {
	ctx := newHistoryTestContext(t, nil)
	now := time.Now()
	refusals := []string{
		"this is a live HLS stream (the playlist has no #EXT-X-ENDLIST); only complete recordings can be downloaded",
		"this HLS stream is protected by DRM (SAMPLE-AES) and cannot be downloaded",
		"this HLS stream's initialization section uses a byte range with no explicit offset, which this server cannot place",
		"this HLS stream changes its initialization segment part-way through, which this server cannot download",
		"the URL is an HLS playlist of a kind this server does not handle",
		"this HLS master playlist lists no playable video renditions",
		"this URL points at an HLS trick-play (I-frames only) rendition rather than a playable stream",
		"this HLS playlist contains no media segments",
		"this HLS master playlist points at another master playlist more than 3 levels deep",
	}
	for i, message := range refusals {
		mustRecord(t, ctx, download_queue.HistoryRecord{
			JobID: fmt.Sprintf("u%d", i), Status: models.DownloadHistoryStatusFailed,
			Error: message, CreatedAt: now, CompletedAt: &now,
		})
	}
	if got := historyJobIDs(t, ctx, &query_models.DownloadHistoryQuery{Reason: query_models.DownloadReasonUnsupported}); len(got) != len(refusals) {
		t.Fatalf("Reason=unsupported = %v, want all %d refusals", got, len(refusals))
	}
	if got := historyJobIDs(t, ctx, &query_models.DownloadHistoryQuery{Reason: query_models.DownloadReasonOther}); len(got) != 0 {
		t.Fatalf("Reason=other = %v, want none", got)
	}
}

// A filesystem write failure reaches the row as the raw *os.PathError
// AddResource never wraps, which is the ordinary shape of a storage failure.
func TestDownloadHistoryStorageBucketCoversRawFilesystemErrors(t *testing.T) {
	ctx := newHistoryTestContext(t, nil)
	now := time.Now()
	failures := []string{
		"open /srv/files/ab/cd/hash.bin: permission denied",
		"open /srv/files/ab/cd/hash.bin: read-only file system",
		"write /srv/files/ab/cd/hash.bin: no space left on device",
	}
	for i, message := range failures {
		mustRecord(t, ctx, download_queue.HistoryRecord{
			JobID: fmt.Sprintf("s%d", i), Status: models.DownloadHistoryStatusFailed,
			Error: message, CreatedAt: now, CompletedAt: &now,
		})
	}
	if got := historyJobIDs(t, ctx, &query_models.DownloadHistoryQuery{Reason: query_models.DownloadReasonStorage}); len(got) != len(failures) {
		t.Fatalf("Reason=storage = %v, want all %d write failures", got, len(failures))
	}
}
