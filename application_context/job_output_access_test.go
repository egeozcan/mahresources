package application_context

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
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

func writeGroupExportArchiveForTest(t *testing.T, ctx *MahresourcesContext, jobID string, groupIDs, resourceIDs, noteIDs, seriesIDs []uint) string {
	t.Helper()
	var buf bytes.Buffer
	writer, err := archive.NewWriter(&buf, false)
	if err != nil {
		t.Fatalf("create test export writer: %v", err)
	}
	manifest := &archive.Manifest{SchemaVersion: archive.SchemaVersion}
	manifest.Counts = archive.Counts{
		Groups: len(groupIDs), Resources: len(resourceIDs), Notes: len(noteIDs), Series: len(seriesIDs),
	}
	manifest.Entries.Groups = make([]archive.GroupEntry, 0, len(groupIDs))
	for i, id := range groupIDs {
		ref := fmt.Sprintf("g%06d", i+1)
		manifest.Entries.Groups = append(manifest.Entries.Groups, archive.GroupEntry{
			ExportID: ref, Name: "group", SourceID: id, Path: "groups/" + ref + ".json",
		})
		if i == 0 {
			manifest.Roots = []string{ref}
		}
	}
	manifest.Entries.Resources = make([]archive.ResourceEntry, 0, len(resourceIDs))
	for i, id := range resourceIDs {
		ref := fmt.Sprintf("r%06d", i+1)
		manifest.Entries.Resources = append(manifest.Entries.Resources, archive.ResourceEntry{
			ExportID: ref, Name: "resource", SourceID: id, Path: "resources/" + ref + ".json",
		})
	}
	manifest.Entries.Notes = make([]archive.NoteEntry, 0, len(noteIDs))
	for i, id := range noteIDs {
		ref := fmt.Sprintf("n%06d", i+1)
		manifest.Entries.Notes = append(manifest.Entries.Notes, archive.NoteEntry{
			ExportID: ref, Name: "note", SourceID: id, Path: "notes/" + ref + ".json",
		})
	}
	manifest.Entries.Series = make([]archive.SeriesEntry, 0, len(seriesIDs))
	for i, id := range seriesIDs {
		ref := fmt.Sprintf("s%06d", i+1)
		manifest.Entries.Series = append(manifest.Entries.Series, archive.SeriesEntry{
			ExportID: ref, Name: "series", SourceID: id, Path: "series/" + ref + ".json",
		})
	}
	if err := writer.WriteManifest(manifest); err != nil {
		t.Fatalf("write test export manifest: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close test export archive: %v", err)
	}
	path := exportArchivePath(jobID, false)
	if err := afero.WriteFile(ctx.GetDefaultFs(), path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write test export archive: %v", err)
	}
	return path
}

func publishScopedGroupExportOutputForTest(t *testing.T, ctx *MahresourcesContext, jobID string, groupIDs, resourceIDs []uint) string {
	t.Helper()
	path := writeGroupExportArchiveForTest(t, ctx, jobID, groupIDs, resourceIDs, nil, nil)
	reference, err := json.Marshal(queueArtifactReference{
		Path: path, ScopeManifestVersion: jobExportScopeManifestVersion,
	})
	if err != nil {
		t.Fatalf("encode scoped export reference: %v", err)
	}
	publishTestOutput(t, ctx, jobID, jobExportArtifactOutput, jobs.OutputTypeArtifact,
		string(reference), jobs.OutputAvailable, nil)
	return path
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
	publishScopedGroupExportOutputForTest(t, ctx, job.ID, []uint{1}, nil)

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

func TestGroupExportOutputFailsClosedWhenKindVersionAdapterIsMissing(t *testing.T) {
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
	publishScopedGroupExportOutputForTest(t, ctx, job.ID, []uint{1}, nil)

	// This is a supported persisted upgrade state: the Job and its artifact were
	// written by an older binary, while this binary no longer has its adapter.
	if err := ctx.db.Model(&models.Job{}).Where("id = ?", job.ID).Update("kind_version", jobExportKindVersion+100).Error; err != nil {
		t.Fatalf("mark export as an unsupported adapter version: %v", err)
	}

	// The owner can still see the Job, but has since lost scope to its root group.
	revoked := ctx.WithPrincipal(&auth.Principal{UserID: 7, Role: models.RoleUser, ScopeGroupID: jobUintPtr(2)})
	outputs, err := revoked.GetOpenableJobOutputs(job.ID)
	if err != nil || len(outputs) != 0 {
		t.Fatalf("unsupported-version outputs = %#v, err=%v; want no advertised output", outputs, err)
	}
	if _, err := revoked.OpenJobOutput(context.Background(), job.ID, jobExportArtifactOutput); !errors.Is(err, ErrJobOutputForbidden) {
		t.Fatalf("unsupported-version output open error = %v, want forbidden", err)
	}
}

func TestGroupExportOutputRechecksEveryExportedGroupAfterMove(t *testing.T) {
	ctx := newJobContext(t)
	if err := ctx.db.Exec("CREATE TABLE groups (id integer PRIMARY KEY, owner_id integer)").Error; err != nil {
		t.Fatalf("create test group scope table: %v", err)
	}
	if err := ctx.db.Exec("INSERT INTO groups(id, owner_id) VALUES (1, NULL), (2, NULL), (3, 1)").Error; err != nil {
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
	publishScopedGroupExportOutputForTest(t, ctx, job.ID, []uint{1, 3}, nil)

	principal := &auth.Principal{UserID: 7, Role: models.RoleUser, ScopeGroupID: jobUintPtr(1)}
	inside := ctx.WithPrincipal(principal)
	if outputs, err := inside.GetOpenableJobOutputs(job.ID); err != nil || len(outputs) != 1 {
		t.Fatalf("in-scope outputs = %#v, err=%v; want the export", outputs, err)
	}
	content, err := inside.OpenJobOutput(context.Background(), job.ID, "artifact")
	if err != nil {
		t.Fatalf("in-scope export could not be opened: %v", err)
	}
	_ = content.Body.Close()

	if err := ctx.db.Exec("UPDATE groups SET owner_id = 2 WHERE id = 3").Error; err != nil {
		t.Fatalf("move exported child group out of current scope: %v", err)
	}
	revoked := ctx.WithPrincipal(principal)
	if outputs, err := revoked.GetOpenableJobOutputs(job.ID); err != nil || len(outputs) != 0 {
		t.Fatalf("outputs after exported child moved = %#v, err=%v; want no advertised output", outputs, err)
	}
	if _, err := revoked.OpenJobOutput(context.Background(), job.ID, "artifact"); !errors.Is(err, ErrJobOutputForbidden) {
		t.Fatalf("output open after exported child moved = %v, want forbidden", err)
	}
}

func TestGroupExportScopeAuthorizationBatchesLargeScopedManifest(t *testing.T) {
	ctx := newJobContext(t)
	if err := ctx.db.Exec("CREATE TABLE groups (id integer PRIMARY KEY, owner_id integer)").Error; err != nil {
		t.Fatalf("create test group scope table: %v", err)
	}
	const entityCount = 2200
	for start := 1; start <= entityCount; start += 300 {
		end := start + 300
		if end > entityCount+1 {
			end = entityCount + 1
		}
		values := make([]string, 0, end-start)
		args := make([]any, 0, (end-start)*2)
		for id := start; id < end; id++ {
			values = append(values, "(?, ?)")
			args = append(args, id)
			if id == 1 {
				args = append(args, nil)
			} else {
				args = append(args, 1)
			}
		}
		if err := ctx.db.Exec("INSERT INTO groups(id, owner_id) VALUES "+strings.Join(values, ","), args...).Error; err != nil {
			t.Fatalf("create test groups %d through %d: %v", start, end-1, err)
		}
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
	groupIDs := make([]uint, entityCount)
	for i := range groupIDs {
		groupIDs[i] = uint(i + 1)
	}
	publishScopedGroupExportOutputForTest(t, ctx, job.ID, groupIDs, nil)
	scoped := ctx.WithPrincipal(&auth.Principal{UserID: 7, Role: models.RoleUser, ScopeGroupID: jobUintPtr(1)})
	if outputs, err := scoped.GetOpenableJobOutputs(job.ID); err != nil || len(outputs) != 1 {
		t.Fatalf("large scoped export outputs = %d, err=%v; want one visible output", len(outputs), err)
	}
	content, err := scoped.OpenJobOutput(context.Background(), job.ID, jobExportArtifactOutput)
	if err != nil {
		t.Fatalf("open large scoped export: %v", err)
	}
	_ = content.Body.Close()
}

func TestGroupExportOutputRechecksEveryExportedResourceAfterMove(t *testing.T) {
	ctx := newJobContext(t)
	if err := ctx.db.Exec("CREATE TABLE groups (id integer PRIMARY KEY, owner_id integer)").Error; err != nil {
		t.Fatalf("create test group scope table: %v", err)
	}
	if err := ctx.db.Exec("INSERT INTO groups(id, owner_id) VALUES (1, NULL), (2, NULL)").Error; err != nil {
		t.Fatalf("create test groups: %v", err)
	}
	if err := ctx.db.Exec("CREATE TABLE resources (id integer PRIMARY KEY, owner_id integer, series_id integer)").Error; err != nil {
		t.Fatalf("create test resource scope table: %v", err)
	}
	if err := ctx.db.Exec("INSERT INTO resources(id, owner_id) VALUES (10, 1)").Error; err != nil {
		t.Fatalf("create test resource: %v", err)
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
	publishScopedGroupExportOutputForTest(t, ctx, job.ID, []uint{1}, []uint{10})

	principal := &auth.Principal{UserID: 7, Role: models.RoleUser, ScopeGroupID: jobUintPtr(1)}
	inside := ctx.WithPrincipal(principal)
	if outputs, err := inside.GetOpenableJobOutputs(job.ID); err != nil || len(outputs) != 1 {
		t.Fatalf("in-scope outputs = %#v, err=%v; want the export", outputs, err)
	}
	content, err := inside.OpenJobOutput(context.Background(), job.ID, "artifact")
	if err != nil {
		t.Fatalf("in-scope export could not be opened: %v", err)
	}
	_ = content.Body.Close()

	if err := ctx.db.Exec("UPDATE resources SET owner_id = 2 WHERE id = 10").Error; err != nil {
		t.Fatalf("move exported resource out of current scope: %v", err)
	}
	revoked := ctx.WithPrincipal(principal)
	if outputs, err := revoked.GetOpenableJobOutputs(job.ID); err != nil || len(outputs) != 0 {
		t.Fatalf("outputs after exported resource moved = %#v, err=%v; want no advertised output", outputs, err)
	}
	if _, err := revoked.OpenJobOutput(context.Background(), job.ID, "artifact"); !errors.Is(err, ErrJobOutputForbidden) {
		t.Fatalf("output open after exported resource moved = %v, want forbidden", err)
	}
}

func TestGroupExportOutputHidesArtifactsWithoutScopeProofMarker(t *testing.T) {
	ctx := newJobContext(t)
	if err := ctx.db.Exec("CREATE TABLE groups (id integer PRIMARY KEY, owner_id integer)").Error; err != nil {
		t.Fatalf("create test group scope table: %v", err)
	}
	if err := ctx.db.Exec("INSERT INTO groups(id, owner_id) VALUES (1, NULL)").Error; err != nil {
		t.Fatalf("create test group: %v", err)
	}
	request, err := json.Marshal(exportJobInput{Request: ExportRequest{RootGroupIDs: []uint{1}}})
	if err != nil {
		t.Fatalf("encode export input: %v", err)
	}
	job := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindGroupExport, KindVersion: jobExportKindVersion, State: jobs.StateQueued,
		Origin: "api", OwnerUserID: jobUintPtr(7), Title: "Export of one group",
		Replay: jobs.ReplayInput{Input: request},
	})
	path := writeGroupExportArchiveForTest(t, ctx, job.ID, []uint{1}, nil, nil, nil)
	reference, err := json.Marshal(queueArtifactReference{Path: path})
	if err != nil {
		t.Fatalf("encode legacy output reference: %v", err)
	}
	publishTestOutput(t, ctx, job.ID, jobExportArtifactOutput, jobs.OutputTypeArtifact,
		string(reference), jobs.OutputAvailable, nil)
	principal := &auth.Principal{UserID: 7, Role: models.RoleUser, ScopeGroupID: jobUintPtr(1)}
	scoped := ctx.WithPrincipal(principal)
	if outputs, err := scoped.GetOpenableJobOutputs(job.ID); err != nil || len(outputs) != 0 {
		t.Fatalf("unproved export outputs = %#v, err=%v; want hidden", outputs, err)
	}
	if _, err := scoped.OpenJobOutput(context.Background(), job.ID, jobExportArtifactOutput); !errors.Is(err, ErrJobOutputForbidden) {
		t.Fatalf("unproved export open error = %v, want forbidden", err)
	}
}

func TestGroupExportOutputRechecksEveryExportedNoteAfterMove(t *testing.T) {
	ctx := newJobContext(t)
	if err := ctx.db.Exec("CREATE TABLE groups (id integer PRIMARY KEY, owner_id integer)").Error; err != nil {
		t.Fatalf("create test group scope table: %v", err)
	}
	if err := ctx.db.Exec("INSERT INTO groups(id, owner_id) VALUES (1, NULL), (2, NULL)").Error; err != nil {
		t.Fatalf("create test groups: %v", err)
	}
	if err := ctx.db.Exec("CREATE TABLE notes (id integer PRIMARY KEY, owner_id integer)").Error; err != nil {
		t.Fatalf("create test note scope table: %v", err)
	}
	if err := ctx.db.Exec("INSERT INTO notes(id, owner_id) VALUES (50, 1)").Error; err != nil {
		t.Fatalf("create test note: %v", err)
	}
	request, err := json.Marshal(exportJobInput{Request: ExportRequest{RootGroupIDs: []uint{1}}})
	if err != nil {
		t.Fatalf("encode export input: %v", err)
	}
	job := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindGroupExport, KindVersion: jobExportKindVersion, State: jobs.StateQueued,
		Origin: "api", OwnerUserID: jobUintPtr(7), Title: "Export of one group",
		Replay: jobs.ReplayInput{Input: request},
	})
	path := writeGroupExportArchiveForTest(t, ctx, job.ID, []uint{1}, nil, []uint{50}, nil)
	reference, err := json.Marshal(queueArtifactReference{Path: path, ScopeManifestVersion: jobExportScopeManifestVersion})
	if err != nil {
		t.Fatalf("encode scoped export reference: %v", err)
	}
	publishTestOutput(t, ctx, job.ID, jobExportArtifactOutput, jobs.OutputTypeArtifact,
		string(reference), jobs.OutputAvailable, nil)
	principal := &auth.Principal{UserID: 7, Role: models.RoleUser, ScopeGroupID: jobUintPtr(1)}
	scoped := ctx.WithPrincipal(principal)
	if outputs, err := scoped.GetOpenableJobOutputs(job.ID); err != nil || len(outputs) != 1 {
		t.Fatalf("in-scope output = %#v, err=%v; want the export", outputs, err)
	}
	if err := ctx.db.Exec("UPDATE notes SET owner_id = 2 WHERE id = 50").Error; err != nil {
		t.Fatalf("move exported note out of current scope: %v", err)
	}
	if outputs, err := scoped.GetOpenableJobOutputs(job.ID); err != nil || len(outputs) != 0 {
		t.Fatalf("outputs after exported note moved = %#v, err=%v; want hidden", outputs, err)
	}
	if _, err := scoped.OpenJobOutput(context.Background(), job.ID, jobExportArtifactOutput); !errors.Is(err, ErrJobOutputForbidden) {
		t.Fatalf("output open after exported note moved = %v, want forbidden", err)
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
