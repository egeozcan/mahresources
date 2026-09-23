package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/spf13/afero"
	"mahresources/application_context"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/types"
)

func TestCanonicalJobAPIFailsClosedForOutputWithoutKindVersionAdapter(t *testing.T) {
	tc := setupAuthEnv(t)
	root := &models.Group{Name: "missing-adapter-export-root"}
	outside := &models.Group{Name: "missing-adapter-export-outside"}
	if err := tc.DB.Create(root).Error; err != nil {
		t.Fatalf("create export root: %v", err)
	}
	if err := tc.DB.Create(outside).Error; err != nil {
		t.Fatalf("create out-of-scope group: %v", err)
	}
	owner, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "missing-adapter-export-owner", Password: "password1", Role: models.RoleUser,
		ScopeGroupId: &outside.ID,
	})
	if err != nil {
		t.Fatalf("create scoped export owner: %v", err)
	}
	token, _, err := tc.AppCtx.CreateApiToken(owner.ID, "test", nil)
	if err != nil {
		t.Fatalf("create API token: %v", err)
	}

	accepted, err := tc.AppCtx.JobService().Accept(jobs.Deps{DB: tc.DB}, jobs.Acceptance{
		Kind: application_context.JobKindGroupExport, KindVersion: 1, State: jobs.StateQueued,
		Origin: "api", OwnerUserID: &owner.ID, Title: "Export of one group",
		Replay: jobs.ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("accept export Job: %v", err)
	}
	path := "_exports/" + accepted.ID + ".tar"
	const privateArchive = "private export archive"
	if err := tc.Fs.MkdirAll("_exports", 0o755); err != nil {
		t.Fatalf("create export directory: %v", err)
	}
	if err := afero.WriteFile(tc.Fs, path, []byte(privateArchive), 0o600); err != nil {
		t.Fatalf("write export archive: %v", err)
	}
	ref, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		t.Fatalf("encode output reference: %v", err)
	}
	now := time.Now().UTC()
	output := models.JobOutput{
		ID: types.NewUUIDv7(), JobID: accepted.ID, Key: "artifact", Type: jobs.OutputTypeArtifact,
		Label: "Export", Reference: types.JSON(ref), Availability: string(jobs.OutputAvailable),
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := tc.DB.Create(&output).Error; err != nil {
		t.Fatalf("publish export artifact: %v", err)
	}
	// Simulate reading a persisted Job after an upgrade whose adapter no longer
	// supports this version. The stored artifact remains, but its scope policy
	// cannot be evaluated by this process.
	if err := tc.DB.Model(&models.Job{}).Where("id = ?", accepted.ID).
		Update("kind_version", uint(101)).Error; err != nil {
		t.Fatalf("mark export as an unsupported adapter version: %v", err)
	}

	headers := map[string]string{"Authorization": "Bearer " + token, "Accept": "application/json"}
	detail := doReq(tc, http.MethodGet, fmt.Sprintf("/v1/jobs/%s", accepted.ID), headers, nil, nil)
	var response struct {
		Outputs []json.RawMessage `json:"outputs"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode Job detail (%d): %v: %s", detail.Code, err, detail.Body.String())
	}
	opened := doReq(tc, http.MethodGet,
		fmt.Sprintf("/v1/jobs/%s/outputs?key=artifact", accepted.ID), headers, nil, nil)

	if detail.Code != http.StatusOK || len(response.Outputs) != 0 {
		t.Errorf("detail response code=%d outputs=%d; want 200 and no outputs", detail.Code, len(response.Outputs))
	}
	if opened.Code != http.StatusNotFound || opened.Body.String() == privateArchive {
		t.Errorf("output open response code=%d body=%q; want 404 without artifact bytes", opened.Code, opened.Body.String())
	}
}
