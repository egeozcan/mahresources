package application_context

import (
	"encoding/json"
	"testing"
	"time"

	"mahresources/download_queue"
	"mahresources/models"
	"mahresources/models/query_models"
)

func makeWriterEpochUnavailable(t *testing.T, ctx *MahresourcesContext) {
	t.Helper()
	if err := ctx.db.AutoMigrate(&models.JobWriterEpoch{}); err != nil {
		t.Fatalf("migrate writer epoch: %v", err)
	}
	if err := models.EnsureJobWriterEpoch(ctx.db); err != nil {
		t.Fatalf("seed writer epoch: %v", err)
	}
	if err := ctx.db.Migrator().DropTable(&models.JobWriterEpoch{}); err != nil {
		t.Fatalf("drop writer epoch: %v", err)
	}
}

func TestDownloadHistoryPayloadFailsClosedWhenWriterEpochCannotBeRead(t *testing.T) {
	ctx := newHistoryTestContext(t, nil)
	makeWriterEpochUnavailable(t, ctx)
	payload, err := json.Marshal(query_models.ResourceFromRemoteCreator{URL: "https://secret.example/file"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.DownloadHistoryPayload(&models.DownloadHistoryEntry{Payload: payload}); err == nil {
		t.Fatal("DownloadHistoryPayload returned legacy input when the writer epoch was unavailable")
	}
}

func TestScheduledDownloadPayloadFailsClosedWhenWriterEpochCannotBeRead(t *testing.T) {
	ctx := newScheduledDownloadTestContext(t)
	makeWriterEpochUnavailable(t, ctx)
	payload, err := json.Marshal(query_models.ResourceFromRemoteCreator{URL: "https://secret.example/file"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.ScheduledDownloadPayload(&models.ScheduledDownload{Payload: payload}); err == nil {
		t.Fatal("ScheduledDownloadPayload returned legacy input when the writer epoch was unavailable")
	}
}

func TestRecordTerminalDownloadFailsClosedWhenWriterEpochCannotBeRead(t *testing.T) {
	ctx := newHistoryTestContext(t, nil)
	makeWriterEpochUnavailable(t, ctx)
	now := time.Now().UTC()
	err := ctx.RecordTerminalDownload(download_queue.HistoryRecord{
		JobID: "epoch-error-download", URL: "https://secret.example/file",
		Payload: []byte(`{"token":"legacy-secret"}`),
		Status:  models.DownloadHistoryStatusFailed, CreatedAt: now, CompletedAt: &now,
	})
	if err == nil {
		t.Fatal("RecordTerminalDownload succeeded when it could not determine whether plaintext writes were fenced")
	}
	var count int64
	if queryErr := ctx.db.Model(&models.DownloadHistoryEntry{}).Where("job_id = ?", "epoch-error-download").Count(&count).Error; queryErr != nil {
		t.Fatal(queryErr)
	}
	if count != 0 {
		t.Fatalf("RecordTerminalDownload persisted %d row(s) after the writer epoch read failed", count)
	}
}

func TestCreateScheduledDownloadFailsClosedWhenWriterEpochCannotBeRead(t *testing.T) {
	ctx := newScheduledDownloadTestContext(t)
	owner := createDownloadOwner(t, ctx)
	makeWriterEpochUnavailable(t, ctx)
	_, err := ctx.CreateScheduledDownload("feeds", owner.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://secret.example/file", Headers: map[string]string{"Authorization": "secret"}},
		time.Now().Add(time.Hour))
	if err == nil {
		t.Fatal("CreateScheduledDownload succeeded when it could not determine whether plaintext writes were fenced")
	}
	var count int64
	if queryErr := ctx.db.Model(&models.ScheduledDownload{}).Count(&count).Error; queryErr != nil {
		t.Fatal(queryErr)
	}
	if count != 0 {
		t.Fatalf("CreateScheduledDownload persisted %d row(s) after the writer epoch read failed", count)
	}
}
