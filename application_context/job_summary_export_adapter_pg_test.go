//go:build postgres && json1 && fts5

package application_context

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"mahresources/auth"
	"mahresources/jobs"
	"mahresources/models"
)

func TestSummaryExportKeepsOwnerScopeAcrossPromotionAndRestartOnPostgres(t *testing.T) {
	first, second, _ := newPostgresOwnershipFixture(t, 1)
	owner := &models.User{Username: "pg-summary-scope-owner", Role: models.RoleEditor}
	other := &models.User{Username: "pg-summary-scope-other", Role: models.RoleUser}
	if err := first.db.Create(owner).Error; err != nil {
		t.Fatalf("create export owner: %v", err)
	}
	if err := first.db.Create(other).Error; err != nil {
		t.Fatalf("create other owner: %v", err)
	}

	acceptDownload := func(user *models.User) {
		t.Helper()
		userID := user.ID
		if _, err := first.JobService().Accept(first.jobDeps(), jobs.Acceptance{
			Kind: JobKindRemoteDownload, KindVersion: jobDownloadKindVersion,
			State: jobs.StateQueued, OwnerUserID: &userID, ActorUserID: &userID,
			Origin: "api", Title: "historical download", Replay: jobs.ReplayInput{NonReplayable: true},
		}); err != nil {
			t.Fatalf("accept remote download for %d: %v", userID, err)
		}
	}
	acceptDownload(owner)
	acceptDownload(other)

	ownerCtx := first.WithPrincipal(auth.FromUser(owner))
	from := time.Now().UTC().Add(-181 * 24 * time.Hour)
	to := time.Now().UTC().Add(time.Hour)
	accepted, err := ownerCtx.SubmitJobSummaryExport(
		jobs.Filter{Kinds: []string{JobKindRemoteDownload}}, from, to, "json", "api",
	)
	if err != nil {
		t.Fatalf("submit owner-scoped summary export: %v", err)
	}
	if err := first.db.Model(&models.User{}).Where("id = ?", owner.ID).Update("role", models.RoleAdmin).Error; err != nil {
		t.Fatalf("promote export owner before dispatch: %v", err)
	}

	execution, claimed, err := first.JobService().Claim(context.Background(), first.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindSummaryExport, KindVersion: jobSummaryExportVersion, Claimant: "pg-summary-export-test",
	})
	if err != nil || !claimed || execution.JobID != accepted.ID {
		t.Fatalf("claim summary export = (%+v, %t, %v)", execution, claimed, err)
	}
	adapter, registered := first.JobService().AdapterFor(JobKindSummaryExport, jobSummaryExportVersion)
	if !registered {
		t.Fatal("summary export adapter is not registered")
	}
	if err := adapter.Dispatch(context.Background(), execution); err != nil {
		t.Fatalf("dispatch summary export: %v", err)
	}

	// The second context has a separate service and adapter registry, like a new
	// process after restart. It must authorize from the persisted artifact scope,
	// which remains readable after the replay input expires or is forgotten.
	owner.Role = models.RoleAdmin
	restarted := second.WithPrincipal(auth.FromUser(owner))
	if _, err := first.JobService().ForgetReplay(first.jobDeps(), jobs.Access{UserID: owner.ID}, accepted.ID); err != nil {
		t.Fatalf("forget completed export input: %v", err)
	}
	outputs, err := restarted.GetOpenableJobOutputs(accepted.ID)
	if err != nil || len(outputs) != 1 || outputs[0].Key != jobSummaryExportOutput {
		t.Fatalf("restarted administrator outputs = %#v, err=%v; want persisted scoped artifact", outputs, err)
	}
	content, err := restarted.OpenJobOutput(context.Background(), accepted.ID, jobSummaryExportOutput)
	if err != nil {
		t.Fatalf("restarted administrator could not open output: %v", err)
	}
	body, err := io.ReadAll(content.Body)
	_ = content.Body.Close()
	if err != nil {
		t.Fatalf("read restarted output: %v", err)
	}
	var summary jobs.Summary
	if err := json.Unmarshal(body, &summary); err != nil {
		t.Fatalf("decode restarted output: %v (%s)", err, body)
	}
	if summary.Total != 1 {
		t.Fatalf("owner-scoped export total after promotion = %d, want 1 despite another user's Job", summary.Total)
	}
}
