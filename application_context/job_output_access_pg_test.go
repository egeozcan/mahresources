//go:build postgres && json1 && fts5

package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"mahresources/archive"
	"mahresources/auth"
	"mahresources/jobs"
	"mahresources/models"
)

func TestGroupExportOutputFailsClosedWhenKindVersionAdapterIsMissingOnPostgres(t *testing.T) {
	ctx, _, _ := newPostgresOwnershipFixture(t, 1)
	rootID := createExportGroupForTest(t, ctx, "pg-missing-adapter-root")
	outsideID := createExportGroupForTest(t, ctx, "pg-missing-adapter-outside")
	request, err := json.Marshal(exportJobInput{Request: ExportRequest{
		RootGroupIDs: []uint{rootID}, Scope: archive.ExportScope{Subtree: true},
	}})
	if err != nil {
		t.Fatalf("encode export input: %v", err)
	}
	job := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindGroupExport, KindVersion: jobExportKindVersion, State: jobs.StateQueued,
		Origin: "api", OwnerUserID: jobUintPtr(7), Title: "Export of one group",
		Replay: jobs.ReplayInput{Input: request},
	})
	publishScopedGroupExportOutputForTest(t, ctx, job.ID, []uint{rootID}, nil)
	if err := ctx.db.Model(&models.Job{}).Where("id = ?", job.ID).
		Update("kind_version", jobExportKindVersion+100).Error; err != nil {
		t.Fatalf("mark export as an unsupported adapter version: %v", err)
	}

	revoked := ctx.WithPrincipal(&auth.Principal{
		UserID: 7, Role: models.RoleUser, ScopeGroupID: jobUintPtr(outsideID),
	})
	outputs, err := revoked.GetOpenableJobOutputs(job.ID)
	if err != nil || len(outputs) != 0 {
		t.Fatalf("unsupported-version Postgres outputs = %#v, err=%v; want no advertised output", outputs, err)
	}
	if _, err := revoked.OpenJobOutput(context.Background(), job.ID, jobExportArtifactOutput); !errors.Is(err, ErrJobOutputForbidden) {
		t.Fatalf("unsupported-version Postgres output open error = %v, want forbidden", err)
	}
}
