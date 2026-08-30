package application_context

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/models/query_models"
)

func writeDownloadOnlyPlugin(t *testing.T, dir, name string) {
	t.Helper()
	pluginDir := filepath.Join(dir, name)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir plugin: %v", err)
	}
	src := `
plugin = { api_version = 1, name = "` + name + `", version = "1.0",
           capabilities = { "db:write" }, network = { "example.test" } }
function init() end
`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.lua"), []byte(src), 0o644); err != nil {
		t.Fatalf("write plugin: %v", err)
	}
}

func scheduledDownloadSchedulerTest(t *testing.T) (*MahresourcesContext, *PluginScheduler, models.User) {
	t.Helper()
	dir := t.TempDir()
	writeDownloadOnlyPlugin(t, dir, "feeds")
	ctx := schedulerTestContext(t, dir)
	if err := ctx.db.AutoMigrate(&models.ScheduledDownload{}); err != nil {
		t.Fatalf("migrate scheduled downloads: %v", err)
	}
	pm := ctx.PluginManager()
	if pm == nil {
		t.Fatal("no plugin manager")
	}
	if err := pm.EnablePlugin("feeds"); err != nil {
		t.Fatalf("enable plugin: %v", err)
	}
	owner := models.User{Username: "owner", Role: models.RoleAdmin, PasswordHash: "x"}
	if err := ctx.db.Create(&owner).Error; err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	ctx.refreshRootAdmin()
	return ctx, NewPluginScheduler(ctx, time.Minute), owner
}

func createDueScheduledDownloadForTick(t *testing.T, ctx *MahresourcesContext, owner models.User) *models.ScheduledDownload {
	t.Helper()
	row, err := ctx.CreateScheduledDownload("feeds", owner.ID, &query_models.ResourceFromRemoteCreator{
		URL: "https://example.test/file.mp4",
	}, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatalf("create scheduled download: %v", err)
	}
	return row
}

func TestPluginSchedulerTickFiresDueScheduledDownload(t *testing.T) {
	ctx, scheduler, owner := scheduledDownloadSchedulerTest(t)
	row := createDueScheduledDownloadForTick(t, ctx, owner)

	calls := 0
	scheduler.scheduledDownloadSubmit = func(creator *query_models.ResourceFromRemoteCreator, ownerUserID *uint, pluginName string) (string, error) {
		calls++
		if creator.URL != row.URL {
			t.Fatalf("submitted URL = %q, want %q", creator.URL, row.URL)
		}
		if ownerUserID == nil || *ownerUserID != owner.ID {
			t.Fatalf("owner = %v, want %d", ownerUserID, owner.ID)
		}
		if pluginName != "feeds" {
			t.Fatalf("plugin name = %q, want feeds", pluginName)
		}
		return "job-from-scheduler", nil
	}

	scheduler.Tick(time.Now())

	if calls != 1 {
		t.Fatalf("scheduled submit calls = %d, want 1", calls)
	}
	got := scheduledDownloadRow(t, ctx, row.ID)
	if got.Status != models.ScheduledDownloadStatusSubmitted || got.JobID != "job-from-scheduler" {
		t.Fatalf("status/job = %q/%q, want submitted/job-from-scheduler", got.Status, got.JobID)
	}
}

func TestPluginSchedulerTickMarksScheduledDownloadSubmitFailure(t *testing.T) {
	ctx, scheduler, owner := scheduledDownloadSchedulerTest(t)
	row := createDueScheduledDownloadForTick(t, ctx, owner)
	scheduler.scheduledDownloadSubmit = func(*query_models.ResourceFromRemoteCreator, *uint, string) (string, error) {
		return "", errors.New("queue refused")
	}

	scheduler.Tick(time.Now())

	got := scheduledDownloadRow(t, ctx, row.ID)
	if got.Status != models.ScheduledDownloadStatusFailed || got.LastError != "queue refused" {
		t.Fatalf("status/error = %q/%q, want failed/queue refused", got.Status, got.LastError)
	}
}

func TestPluginSchedulerTickDoesNotFireScheduledDownloadForDisabledPlugin(t *testing.T) {
	ctx, scheduler, owner := scheduledDownloadSchedulerTest(t)
	row := createDueScheduledDownloadForTick(t, ctx, owner)
	if err := ctx.PluginManager().DisablePlugin("feeds"); err != nil {
		t.Fatalf("disable plugin: %v", err)
	}
	scheduler.scheduledDownloadSubmit = func(*query_models.ResourceFromRemoteCreator, *uint, string) (string, error) {
		t.Fatal("disabled plugin reached scheduled download submit")
		return "", nil
	}

	scheduler.Tick(time.Now())

	got := scheduledDownloadRow(t, ctx, row.ID)
	if got.Status != models.ScheduledDownloadStatusFailed || got.LastError == "" {
		t.Fatalf("disabled plugin status/error = %q/%q, want failed with an error", got.Status, got.LastError)
	}
}

func TestPluginSchedulerTickDefersScheduledDownloadWhenURLIsActive(t *testing.T) {
	ctx, scheduler, owner := scheduledDownloadSchedulerTest(t)
	row := createDueScheduledDownloadForTick(t, ctx, owner)
	scheduler.scheduledDownloadActive = func(url string) (string, bool) {
		if url != row.URL {
			t.Fatalf("active check URL = %q, want %q", url, row.URL)
		}
		return "live-job", true
	}
	scheduler.scheduledDownloadSubmit = func(*query_models.ResourceFromRemoteCreator, *uint, string) (string, error) {
		t.Fatal("scheduled download submitted while the same URL is active")
		return "", nil
	}

	scheduler.Tick(time.Now())

	got := scheduledDownloadRow(t, ctx, row.ID)
	if got.Status != models.ScheduledDownloadStatusPending || got.ClaimToken != "" || got.Attempts != 0 {
		t.Fatalf("status/token/attempts = %q/%q/%d, want pending/released/0", got.Status, got.ClaimToken, got.Attempts)
	}
}
