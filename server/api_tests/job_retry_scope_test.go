package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/jobs"
	"mahresources/models"
)

// A Retry is refused up front, with the reason, when the download it would start
// targets a group the asker has since lost: accepted, it could only block at
// dispatch with no reason shown, and each Resume would block it again.
func TestRetryOfADownloadWhoseTargetLeftTheAskersScopeIsRefusedWithTheReason(t *testing.T) {
	tc := setupAuthEnv(t)
	installJobControlPlane(t, tc)
	before := &models.Group{Name: "retry-scope-before"}
	after := &models.Group{Name: "retry-scope-after"}
	for _, group := range []*models.Group{before, after} {
		if err := tc.DB.Create(group).Error; err != nil {
			t.Fatalf("create group: %v", err)
		}
	}
	user, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "retry-scope-user", Password: "password1", Role: models.RoleUser, ScopeGroupId: &before.ID,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	raw, _, err := tc.AppCtx.CreateApiToken(user.ID, "t", nil)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	bearer := map[string]string{"Authorization": "Bearer " + raw, "Accept": "application/json"}

	// A loopback address the fetch policy refuses, so the download fails at once.
	submitted := postJSON(tc, "/v1/download/submit",
		fmt.Sprintf(`{"URL":"http://127.0.0.1:9/retry-scope.bin","OwnerId":%d}`, before.ID), bearer)
	if submitted.Code != http.StatusAccepted {
		t.Fatalf("submit answered %d: %s", submitted.Code, submitted.Body.String())
	}
	var accepted struct {
		Jobs []struct {
			CanonicalJobID string `json:"canonicalJobId"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(submitted.Body.Bytes(), &accepted); err != nil || len(accepted.Jobs) != 1 {
		t.Fatalf("decode submit: %v: %s", err, submitted.Body.String())
	}
	jobID := accepted.Jobs[0].CanonicalJobID
	deadline := time.Now().Add(20 * time.Second)
	var failed jobs.Snapshot
	for time.Now().Before(deadline) {
		failed, err = tc.AppCtx.JobService().Get(jobs.Deps{DB: tc.DB}, jobs.Access{Administrator: true}, jobID)
		if err == nil && failed.State == jobs.StateFailed {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if failed.State != jobs.StateFailed {
		t.Fatalf("the download ended %s, want failed", failed.State)
	}

	if _, err := tc.AppCtx.UpdateUser(user.ID, &application_context.UserUpdate{
		ScopeGroupID: application_context.UserField[*uint]{Set: true, Value: &after.ID},
	}); err != nil {
		t.Fatalf("move the user's scope: %v", err)
	}

	retry := postJSON(tc, "/v1/jobs/"+jobID+"/commands/retry",
		fmt.Sprintf(`{"expectedVersion":%d,"idempotencyKey":"retry-after-scope-loss"}`, failed.Version), bearer)
	if retry.Code != http.StatusConflict {
		t.Fatalf("the retry answered %d, want 409: %s", retry.Code, retry.Body.String())
	}
	if !strings.Contains(retry.Body.String(), "outside your permitted scope") || !strings.Contains(retry.Body.String(), `"code":"refused"`) {
		t.Fatalf("the refusal does not say why: %s", retry.Body.String())
	}
	var successors int64
	if err := tc.DB.Model(&models.JobLink{}).Where("to_job_id = ?", jobID).Count(&successors).Error; err != nil {
		t.Fatalf("count successors: %v", err)
	}
	if successors != 0 {
		t.Fatalf("the refused retry created %d successors", successors)
	}
}
