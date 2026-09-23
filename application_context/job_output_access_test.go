package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/spf13/afero"
	"mahresources/archive"
	"mahresources/auth"
	"mahresources/contracts"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/types"
)

type jobOutputOpenerTestAdapter struct {
	*runtimeTestAdapter
	request JobOutputOpenRequest
}

func (a *jobOutputOpenerTestAdapter) OpenJobOutput(_ context.Context, request JobOutputOpenRequest) (contracts.JobOutputContent, error) {
	a.request = request
	return contracts.JobOutputContent{Data: json.RawMessage(`{"tail":"redacted"}`), ContentType: "application/json"}, nil
}

func (a *jobOutputOpenerTestAdapter) AuthorizeJobOutput(_ context.Context, request JobOutputOpenRequest) error {
	if request.Principal == nil || !request.Principal.CanWrite() {
		return ErrJobOutputForbidden
	}
	return nil
}

func publishTestOutput(t *testing.T, ctx *MahresourcesContext, jobID, key, kind string, reference string, availability jobs.OutputAvailability, expiresAt *time.Time) {
	t.Helper()
	now := time.Now().UTC()
	row := models.JobOutput{
		ID: types.NewUUIDv7(), JobID: jobID, Key: key, Type: kind, Label: key,
		Reference: types.JSON(reference), Availability: string(availability), Version: 1,
		CreatedAt: now, UpdatedAt: now, ExpiresAt: expiresAt,
	}
	if err := ctx.db.Create(&row).Error; err != nil {
		t.Fatalf("create output row: %v", err)
	}
}

func TestOpenJobOutputRechecksJobAndOutputAuthorization(t *testing.T) {
	ctx := newJobContext(t)
	job := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: "remote-download", KindVersion: 1, State: jobs.StateQueued, Origin: "api",
		OwnerUserID: jobUintPtr(7), Title: "mine", Replay: jobs.ReplayInput{NonReplayable: true},
	})
	if err := afero.WriteFile(ctx.GetDefaultFs(), "_exports/archive.tar", []byte("private archive"), 0o600); err != nil {
		t.Fatalf("write test artifact: %v", err)
	}
	publishTestOutput(t, ctx, job.ID, "artifact", jobs.OutputTypeArtifact, `{"path":"_exports/archive.tar"}`, jobs.OutputAvailable, nil)

	owner := ctx.WithPrincipal(&auth.Principal{UserID: 7, Role: models.RoleUser})
	ownerOutputs, err := owner.GetOpenableJobOutputs(job.ID)
	if err != nil || len(ownerOutputs) != 1 || ownerOutputs[0].Key != "artifact" {
		t.Fatalf("owner openable outputs = %#v, err=%v; want artifact", ownerOutputs, err)
	}
	content, err := owner.OpenJobOutput(context.Background(), job.ID, "artifact")
	if err != nil {
		t.Fatalf("owner could not open output: %v", err)
	}
	if content.Filename != "artifact" {
		t.Fatalf("opened output metadata = %#v", content)
	}
	body, err := io.ReadAll(content.Body)
	_ = content.Body.Close()
	if err != nil || string(body) != "private archive" {
		t.Fatalf("opened body = %q, err=%v", body, err)
	}

	guest := ctx.WithPrincipal(&auth.Principal{UserID: 7, Role: models.RoleGuest})
	guestOutputs, err := guest.GetOpenableJobOutputs(job.ID)
	if err != nil || len(guestOutputs) != 0 {
		t.Fatalf("guest openable outputs = %#v, err=%v; want no advertised outputs", guestOutputs, err)
	}
	if _, err := guest.OpenJobOutput(context.Background(), job.ID, "artifact"); !errors.Is(err, ErrJobOutputForbidden) {
		t.Fatalf("guest output access error = %v, want forbidden", err)
	}
	other := ctx.WithPrincipal(&auth.Principal{UserID: 8, Role: models.RoleUser})
	if _, err := other.OpenJobOutput(context.Background(), job.ID, "artifact"); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("foreign Job output error = %v, want not found", err)
	}
}

func TestGroupExportOutputRechecksCurrentRootGroupScope(t *testing.T) {
	ctx := newJobContext(t)
	if err := ctx.db.Exec("CREATE TABLE groups (id integer PRIMARY KEY, owner_id integer)").Error; err != nil {
		t.Fatalf("create test group scope table: %v", err)
	}
	if err := ctx.db.Exec("INSERT INTO groups(id, owner_id) VALUES (1, NULL), (2, NULL)").Error; err != nil {
		t.Fatalf("create test groups: %v", err)
	}
	request, err := json.Marshal(exportJobInput{Request: ExportRequest{
		RootGroupIDs: []uint{1}, Scope: archive.ExportScope{Subtree: true},
	}})
	if err != nil {
		t.Fatalf("encode export input: %v", err)
	}
	job := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindGroupExport, KindVersion: jobExportKindVersion, State: jobs.StateQueued,
		Origin: "api", OwnerUserID: jobUintPtr(7), Title: "Export of one group",
		Replay: jobs.ReplayInput{Input: request},
	})
	if err := afero.WriteFile(ctx.GetDefaultFs(), "_exports/group.tar", []byte("private export"), 0o600); err != nil {
		t.Fatalf("write test export: %v", err)
	}
	publishTestOutput(t, ctx, job.ID, "artifact", jobs.OutputTypeArtifact, `{"path":"_exports/group.tar"}`, jobs.OutputAvailable, nil)

	inside := ctx.WithPrincipal(&auth.Principal{UserID: 7, Role: models.RoleUser, ScopeGroupID: jobUintPtr(1)})
	insideOutputs, err := inside.GetOpenableJobOutputs(job.ID)
	if err != nil || len(insideOutputs) != 1 {
		t.Fatalf("in-scope outputs = %#v, err=%v; want the export", insideOutputs, err)
	}
	content, err := inside.OpenJobOutput(context.Background(), job.ID, "artifact")
	if err != nil {
		t.Fatalf("in-scope export could not be opened: %v", err)
	}
	_ = content.Body.Close()

	revoked := ctx.WithPrincipal(&auth.Principal{UserID: 7, Role: models.RoleUser, ScopeGroupID: jobUintPtr(2)})
	revokedOutputs, err := revoked.GetOpenableJobOutputs(job.ID)
	if err != nil || len(revokedOutputs) != 0 {
		t.Fatalf("revoked-scope outputs = %#v, err=%v; want no advertised output", revokedOutputs, err)
	}
	if _, err := revoked.OpenJobOutput(context.Background(), job.ID, "artifact"); !errors.Is(err, ErrJobOutputForbidden) {
		t.Fatalf("revoked-scope output open error = %v, want forbidden", err)
	}
}

func TestOpenJobOutputChecksExpiryAndRejectsTraversal(t *testing.T) {
	ctx := newJobContext(t)
	job := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: "remote-download", KindVersion: 1, State: jobs.StateQueued, Origin: "api",
		OwnerUserID: jobUintPtr(7), Title: "mine", Replay: jobs.ReplayInput{NonReplayable: true},
	})
	if err := afero.WriteFile(ctx.GetDefaultFs(), "_exports/archive.tar", []byte("old archive"), 0o600); err != nil {
		t.Fatalf("write test artifact: %v", err)
	}
	ctx = ctx.WithPrincipal(&auth.Principal{UserID: 7, Role: models.RoleUser})

	expiredAt := time.Now().UTC().Add(-time.Minute)
	publishTestOutput(t, ctx, job.ID, "expired", jobs.OutputTypeArtifact, `{"path":"_exports/archive.tar"}`, jobs.OutputAvailable, &expiredAt)
	if _, err := ctx.OpenJobOutput(context.Background(), job.ID, "expired"); !errors.Is(err, ErrJobOutputUnavailable) {
		t.Fatalf("expired output error = %v, want unavailable", err)
	}

	publishTestOutput(t, ctx, job.ID, "traversal", jobs.OutputTypeArtifact, `{"path":"../secret"}`, jobs.OutputAvailable, nil)
	if _, err := ctx.OpenJobOutput(context.Background(), job.ID, "traversal"); !errors.Is(err, ErrJobOutputInvalid) {
		t.Fatalf("traversal output error = %v, want invalid", err)
	}
}

func TestOpenJobOutputPassesStoredRunReferenceThroughKindSeam(t *testing.T) {
	ctx := newJobContext(t)
	base := newRuntimeTestAdapter()
	base.def.Kind = facadeTestKind
	adapter := &jobOutputOpenerTestAdapter{runtimeTestAdapter: base}
	if err := ctx.JobService().RegisterAdapter(adapter); err != nil {
		t.Fatalf("register output opener Kind: %v", err)
	}
	job := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: facadeTestKind, KindVersion: 1, State: jobs.StateQueued, Origin: "plugin",
		OwnerUserID: jobUintPtr(7), Title: "command history", Replay: jobs.ReplayInput{NonReplayable: true},
	})
	publishTestOutput(t, ctx, job.ID, "command-history", jobs.OutputTypeLog, `{"runId":"stored-run-1"}`, jobs.OutputAvailable, nil)

	owner := ctx.WithPrincipal(&auth.Principal{UserID: 7, Role: models.RoleUser})
	content, err := owner.OpenJobOutput(context.Background(), job.ID, "command-history")
	if err != nil {
		t.Fatalf("Kind output could not be opened: %v", err)
	}
	if adapter.request.Snapshot.ID != job.ID || adapter.request.Output.Reference == nil ||
		string(adapter.request.Output.Reference) != `{"runId":"stored-run-1"}` || adapter.request.Principal.UserID != 7 {
		t.Fatalf("Kind opener request = %#v, want visible Job, stored run reference, and current owner", adapter.request)
	}
	if string(content.Data) != `{"tail":"redacted"}` {
		t.Fatalf("Kind output = %s, want sanitized response", content.Data)
	}
}
