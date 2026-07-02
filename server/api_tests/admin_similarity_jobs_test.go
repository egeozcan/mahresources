package api_tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"mahresources/hash_worker"
	"mahresources/models"
)

func TestAdminRetryFailedHashes_Endpoint(t *testing.T) {
	tc := SetupTestEnv(t)

	ver := hash_worker.HashVersionV2
	failed := models.ImageHash{ResourceId: func() *uint { v := uint(1); return &v }(), HashVersion: &ver, Status: models.HashStatusFailed}
	if err := tc.DB.Create(&failed).Error; err != nil {
		t.Fatal(err)
	}

	rec := tc.MakeRequest(http.MethodPost, "/v1/admin/similarity/retry-failed", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Reset int64 `json:"reset"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Reset != 1 {
		t.Errorf("reset = %d, want 1", resp.Reset)
	}

	var reloaded models.ImageHash
	tc.DB.First(&reloaded, failed.ID)
	if reloaded.HashVersion != nil {
		t.Errorf("failed row should be reset to NULL hash_version")
	}
}

func TestAdminRecomputeSimilarities_ConflictWhileRunning(t *testing.T) {
	tc := SetupTestEnv(t)

	// Simulate an in-flight recompute holding the process-wide guard.
	if !hash_worker.TryBeginRecompute() {
		t.Fatal("expected to acquire recompute guard")
	}
	defer hash_worker.EndRecompute()

	rec := tc.MakeRequest(http.MethodPost, "/v1/admin/similarity/recompute", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminRecomputeSimilarities_Endpoint(t *testing.T) {
	tc := SetupTestEnv(t)

	rec := tc.MakeRequest(http.MethodPost, "/v1/admin/similarity/recompute", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		JobID string `json:"jobId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.JobID == "" {
		t.Errorf("expected a jobId in response, got %s", rec.Body.String())
	}
}
