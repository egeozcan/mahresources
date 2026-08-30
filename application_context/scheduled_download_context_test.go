package application_context

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
)

// newScheduledDownloadTestContext uses a temp-file WAL database for the same
// reason the plugin schedule tests do: the claim is a real database race, and an
// in-memory private-cache database would hand each pooled connection its own
// empty copy.
func newScheduledDownloadTestContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scheduled-downloads.db")
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=10000&_synchronous=NORMAL", path)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.ScheduledDownload{}, &models.User{}, &models.Group{}, &models.Note{},
		&models.RuntimeSetting{}, &models.LogEntry{}, &models.Session{}, &models.ApiToken{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, _ := db.DB()
	cfg := &MahresourcesConfig{DbType: constants.DbTypeSqlite}
	return NewMahresourcesContext(afero.NewMemMapFs(), db, sqlx.NewDb(sqlDB, "sqlite3"), cfg)
}

func mustScheduledPayload(t *testing.T, creator *query_models.ResourceFromRemoteCreator) []byte {
	t.Helper()
	payload, err := scheduledDownloadPayload(creator)
	if err != nil {
		t.Fatalf("encode scheduled payload: %v", err)
	}
	return payload
}

func seedScheduledDownload(t *testing.T, ctx *MahresourcesContext, due time.Time, owner *uint) models.ScheduledDownload {
	t.Helper()
	payload := mustScheduledPayload(t, &query_models.ResourceFromRemoteCreator{URL: "https://example.test/file"})
	row := models.ScheduledDownload{
		PluginName:      "feeds",
		URL:             "https://example.test/file",
		Payload:         payload,
		DueAt:           due,
		Status:          models.ScheduledDownloadStatusPending,
		CreatedByUserId: owner,
	}
	if err := ctx.db.Create(&row).Error; err != nil {
		t.Fatalf("seed scheduled download: %v", err)
	}
	return row
}

func scheduledDownloadRow(t *testing.T, ctx *MahresourcesContext, id uint) models.ScheduledDownload {
	t.Helper()
	var row models.ScheduledDownload
	if err := ctx.db.Where("id = ?", id).First(&row).Error; err != nil {
		t.Fatalf("read scheduled download %d: %v", id, err)
	}
	return row
}

func createDownloadOwner(t *testing.T, ctx *MahresourcesContext) models.User {
	t.Helper()
	user := models.User{Username: fmt.Sprintf("owner-%d", time.Now().UnixNano()), Role: models.RoleUser, PasswordHash: "x"}
	if err := ctx.db.Create(&user).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	return user
}

func TestCreateScheduledDownloadBindsTheSubmitterAsOwner(t *testing.T) {
	ctx := newScheduledDownloadTestContext(t)
	admin := models.User{Username: "root", Role: models.RoleAdmin, PasswordHash: "x"}
	if err := ctx.db.Create(&admin).Error; err != nil {
		t.Fatalf("seed root admin: %v", err)
	}
	ctx.refreshRootAdmin()
	submitter := models.User{Username: "submitter", Role: models.RoleUser, PasswordHash: "x"}
	if err := ctx.db.Create(&submitter).Error; err != nil {
		t.Fatalf("seed submitter: %v", err)
	}
	if admin.ID == submitter.ID {
		t.Fatalf("fixture is broken: root and submitter share id %d", admin.ID)
	}

	row, err := ctx.CreateScheduledDownload("feeds", submitter.ID, &query_models.ResourceFromRemoteCreator{
		URL:     "https://example.test/file",
		Headers: map[string]string{"Referer": "https://example.test/"},
	}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create scheduled download: %v", err)
	}

	got := scheduledDownloadRow(t, ctx, row.ID)
	if got.CreatedByUserId == nil || *got.CreatedByUserId != submitter.ID {
		t.Fatalf("owner = %v, want submitter %d, not the default actor %d", got.CreatedByUserId, submitter.ID, admin.ID)
	}
	if got.Status != models.ScheduledDownloadStatusPending {
		t.Fatalf("status = %q, want pending", got.Status)
	}
	creator, err := ctx.ScheduledDownloadPayload(&got)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if creator.Headers["Referer"] != "https://example.test/" {
		t.Fatalf("payload headers were not preserved: %#v", creator.Headers)
	}
}

func TestScheduledDownloadWithNoOwnerIsNeverClaimed(t *testing.T) {
	ctx := newScheduledDownloadTestContext(t)
	row := seedScheduledDownload(t, ctx, time.Now().Add(-time.Minute), nil)

	claimed, err := ctx.ClaimScheduledDownload(row.ID, "tick", time.Now())
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed {
		t.Fatal("an ownerless scheduled download was claimed; it would fire without an identity to re-validate")
	}

	owner := uint(7)
	if err := ctx.db.Model(&models.ScheduledDownload{}).Where("id = ?", row.ID).
		Update("created_by_user_id", owner).Error; err != nil {
		t.Fatalf("restore owner: %v", err)
	}
	if claimed, err := ctx.ClaimScheduledDownload(row.ID, "tick", time.Now()); err != nil || !claimed {
		t.Fatalf("an owned, due scheduled download was not claimed: claimed=%v err=%v", claimed, err)
	}
}

func TestClaimScheduledDownloadReclaimsAnAbandonedClaimButNotALiveOne(t *testing.T) {
	ctx := newScheduledDownloadTestContext(t)
	owner := uint(1)
	row := seedScheduledDownload(t, ctx, time.Now().Add(-time.Minute), &owner)

	if claimed, err := ctx.ClaimScheduledDownload(row.ID, "holder", time.Now()); err != nil || !claimed {
		t.Fatalf("seed claim: claimed=%v err=%v", claimed, err)
	}
	if claimed, err := ctx.ClaimScheduledDownload(row.ID, "thief", time.Now()); err != nil {
		t.Fatalf("live-claim contest: %v", err)
	} else if claimed {
		t.Fatal("a live scheduled-download claim was stolen while its submit may still be in flight")
	}

	abandoned := time.Now().Add(-(ScheduledDownloadClaimTTL + time.Minute))
	if err := ctx.db.Model(&models.ScheduledDownload{}).Where("id = ?", row.ID).
		Update("claimed_at", abandoned).Error; err != nil {
		t.Fatalf("age claim: %v", err)
	}
	if claimed, err := ctx.ClaimScheduledDownload(row.ID, "reclaimer", time.Now()); err != nil {
		t.Fatalf("reclaim: %v", err)
	} else if !claimed {
		t.Fatal("a stale scheduled-download claim was not reclaimed")
	}
}

func TestClaimScheduledDownloadUnderConcurrencyAdmitsExactlyOne(t *testing.T) {
	ctx := newScheduledDownloadTestContext(t)
	owner := uint(1)
	row := seedScheduledDownload(t, ctx, time.Now().Add(-time.Minute), &owner)

	const racers = 8
	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	done.Add(racers)

	var mu sync.Mutex
	wins := 0
	errs := []error{}
	for i := 0; i < racers; i++ {
		go func(i int) {
			defer done.Done()
			start.Wait()
			claimed, err := ctx.ClaimScheduledDownload(row.ID, fmt.Sprintf("claim-%d", i), time.Now())
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			if claimed {
				wins++
			}
		}(i)
	}
	start.Done()
	done.Wait()

	for _, err := range errs {
		t.Errorf("claim errored under contention: %v", err)
	}
	if wins != 1 {
		t.Fatalf("%d of %d racers claimed one scheduled download; exactly one may", wins, racers)
	}
}

func TestFireScheduledDownloadRevalidatesTheStoredActorScope(t *testing.T) {
	ctx := newScheduledDownloadTestContext(t)
	ctx.Config.AuthEnabled = true
	groupA := models.Group{Name: "A"}
	groupB := models.Group{Name: "B"}
	if err := ctx.db.Create(&groupA).Error; err != nil {
		t.Fatalf("create group A: %v", err)
	}
	if err := ctx.db.Create(&groupB).Error; err != nil {
		t.Fatalf("create group B: %v", err)
	}
	user := models.User{Username: "scoped", Role: models.RoleUser, PasswordHash: "x", ScopeGroupId: &groupA.ID}
	if err := ctx.db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	row, err := ctx.CreateScheduledDownload("feeds", user.ID, &query_models.ResourceFromRemoteCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{OwnerId: groupA.ID},
		URL:               "https://example.test/file",
	}, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("create scheduled download: %v", err)
	}
	if err := ctx.db.Model(&models.User{}).Where("id = ?", user.ID).Update("scope_group_id", groupB.ID).Error; err != nil {
		t.Fatalf("move user's scope: %v", err)
	}

	submits := 0
	fired, err := ctx.FireDueScheduledDownloads(ScheduledDownloadFireConfig{
		Now:             time.Now(),
		PluginAvailable: func(string) bool { return true },
		Submit: func(*query_models.ResourceFromRemoteCreator, *uint, string) (string, error) {
			submits++
			return "job-should-not-exist", nil
		},
	})
	if err != nil {
		t.Fatalf("fire due downloads: %v", err)
	}
	if fired != 0 {
		t.Fatalf("fired = %d, want 0 for a row refused during re-validation", fired)
	}
	if submits != 0 {
		t.Fatalf("submit was called %d times after the actor lost access", submits)
	}
	got := scheduledDownloadRow(t, ctx, row.ID)
	if got.Status != models.ScheduledDownloadStatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.LastError == "" {
		t.Fatal("fire-time scope refusal did not record an error")
	}
}

func TestFireScheduledDownloadDoesNotSubmitForDisabledPlugin(t *testing.T) {
	ctx := newScheduledDownloadTestContext(t)
	owner := uint(7)
	row := seedScheduledDownload(t, ctx, time.Now().Add(-time.Minute), &owner)

	submits := 0
	fired, err := ctx.FireDueScheduledDownloads(ScheduledDownloadFireConfig{
		Now:             time.Now(),
		PluginAvailable: func(string) bool { return false },
		Submit: func(*query_models.ResourceFromRemoteCreator, *uint, string) (string, error) {
			submits++
			return "job-should-not-exist", nil
		},
	})
	if err != nil {
		t.Fatalf("fire due downloads: %v", err)
	}
	if fired != 0 || submits != 0 {
		t.Fatalf("disabled plugin fired=%d submits=%d; the row must not be submitted", fired, submits)
	}
	got := scheduledDownloadRow(t, ctx, row.ID)
	if got.Status != models.ScheduledDownloadStatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
}

func TestFireScheduledDownloadSubmitsOneClaimWinnerAndMarksSubmitted(t *testing.T) {
	ctx := newScheduledDownloadTestContext(t)
	ownerUser := createDownloadOwner(t, ctx)
	owner := ownerUser.ID
	row := seedScheduledDownload(t, ctx, time.Now().Add(-time.Minute), &owner)

	fired, err := ctx.FireDueScheduledDownloads(ScheduledDownloadFireConfig{
		Now:             time.Now(),
		PluginAvailable: func(string) bool { return true },
		Submit: func(creator *query_models.ResourceFromRemoteCreator, ownerUserID *uint, pluginName string) (string, error) {
			if creator.URL != row.URL {
				t.Fatalf("submitted URL = %q, want %q", creator.URL, row.URL)
			}
			if ownerUserID == nil || *ownerUserID != owner {
				t.Fatalf("submit owner = %v, want %d", ownerUserID, owner)
			}
			if pluginName != row.PluginName {
				t.Fatalf("pluginName = %q, want %q", pluginName, row.PluginName)
			}
			return "job-123", nil
		},
	})
	if err != nil {
		t.Fatalf("fire due downloads: %v", err)
	}
	if fired != 1 {
		t.Fatalf("fired = %d, want 1", fired)
	}
	got := scheduledDownloadRow(t, ctx, row.ID)
	if got.Status != models.ScheduledDownloadStatusSubmitted || got.JobID != "job-123" {
		t.Fatalf("row status/job = %q/%q, want submitted/job-123", got.Status, got.JobID)
	}
	if got.ClaimToken != "" || got.ClaimedAt != nil {
		t.Fatalf("submitted row kept a claim: token=%q claimedAt=%v", got.ClaimToken, got.ClaimedAt)
	}
	if got.Attempts != 1 {
		t.Fatalf("Attempts = %d, want 1", got.Attempts)
	}
}

func TestFireScheduledDownloadDefersWhenURLIsAlreadyDownloading(t *testing.T) {
	ctx := newScheduledDownloadTestContext(t)
	ownerUser := createDownloadOwner(t, ctx)
	owner := ownerUser.ID
	row := seedScheduledDownload(t, ctx, time.Now().Add(-time.Minute), &owner)

	fired, err := ctx.FireDueScheduledDownloads(ScheduledDownloadFireConfig{
		Now:             time.Now(),
		PluginAvailable: func(string) bool { return true },
		ActiveDownload: func(url string) (string, bool) {
			if url != row.URL {
				t.Fatalf("active check URL = %q, want %q", url, row.URL)
			}
			return "live-job", true
		},
		Submit: func(*query_models.ResourceFromRemoteCreator, *uint, string) (string, error) {
			t.Fatal("submit was called while the same URL is already downloading")
			return "", nil
		},
	})
	if err != nil {
		t.Fatalf("fire due downloads: %v", err)
	}
	if fired != 0 {
		t.Fatalf("fired = %d, want 0 while live download exists", fired)
	}
	got := scheduledDownloadRow(t, ctx, row.ID)
	if got.Status != models.ScheduledDownloadStatusPending {
		t.Fatalf("status = %q, want pending so the next tick can retry", got.Status)
	}
	if got.ClaimToken != "" || got.ClaimedAt != nil {
		t.Fatalf("deferred row kept a claim: token=%q claimedAt=%v", got.ClaimToken, got.ClaimedAt)
	}
	if got.Attempts != 0 {
		t.Fatalf("Attempts = %d, want 0 when nothing was submitted", got.Attempts)
	}
}

func TestFireScheduledDownloadMarksSubmitFailureTerminal(t *testing.T) {
	ctx := newScheduledDownloadTestContext(t)
	ownerUser := createDownloadOwner(t, ctx)
	owner := ownerUser.ID
	row := seedScheduledDownload(t, ctx, time.Now().Add(-time.Minute), &owner)

	fired, err := ctx.FireDueScheduledDownloads(ScheduledDownloadFireConfig{
		Now:             time.Now(),
		PluginAvailable: func(string) bool { return true },
		Submit: func(*query_models.ResourceFromRemoteCreator, *uint, string) (string, error) {
			return "", errors.New("queue full")
		},
	})
	if err != nil {
		t.Fatalf("fire due downloads: %v", err)
	}
	if fired != 0 {
		t.Fatalf("fired = %d, want 0", fired)
	}
	got := scheduledDownloadRow(t, ctx, row.ID)
	if got.Status != models.ScheduledDownloadStatusFailed {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if got.Attempts != 1 || got.LastError != "queue full" {
		t.Fatalf("attempts/error = %d/%q, want 1/queue full", got.Attempts, got.LastError)
	}
}

func TestCancelScheduledDownloadDoesNotStealALiveClaim(t *testing.T) {
	ctx := newScheduledDownloadTestContext(t)
	owner := uint(7)
	row := seedScheduledDownload(t, ctx, time.Now().Add(-time.Minute), &owner)
	if claimed, err := ctx.ClaimScheduledDownload(row.ID, "tick", time.Now()); err != nil || !claimed {
		t.Fatalf("claim: claimed=%v err=%v", claimed, err)
	}

	cancelled, err := ctx.CancelScheduledDownload(row.ID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cancelled {
		t.Fatal("cancel cleared a live claim; the tick could already be submitting a queue job")
	}
	got := scheduledDownloadRow(t, ctx, row.ID)
	if got.Status != models.ScheduledDownloadStatusPending || got.ClaimToken != "tick" {
		t.Fatalf("status/token = %q/%q, want pending/tick", got.Status, got.ClaimToken)
	}

	stale := time.Now().Add(-(ScheduledDownloadClaimTTL + time.Minute))
	if err := ctx.db.Model(&models.ScheduledDownload{}).Where("id = ?", row.ID).Update("claimed_at", stale).Error; err != nil {
		t.Fatalf("age claim: %v", err)
	}
	cancelled, err = ctx.CancelScheduledDownload(row.ID)
	if err != nil {
		t.Fatalf("cancel stale claim: %v", err)
	}
	if !cancelled {
		t.Fatal("a stale claimed row could not be cancelled")
	}
}

func TestScheduledDownloadIsInTheUserDeletionSweep(t *testing.T) {
	for _, m := range stampedModels() {
		if _, ok := m.(*models.ScheduledDownload); ok {
			return
		}
	}
	t.Fatal("models.ScheduledDownload is not in stampedModels(), so deleting its owner leaves a deferred download able to fire as a deleted account")
}
