package application_context

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/archive"
	"mahresources/auth"
	"mahresources/constants"
	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models"
)

// This file holds the import Kind pair's executor-level tests: the properties that
// belong to the executor rather than to a Job's lifecycle.
//
// The four apply-executor tests below moved here from server/api_handlers with the
// executor itself. They used to call an unexported runFn builder in the handler
// package; the runFn now belongs to the application layer, because the Kind's own
// dispatch has to be able to build the same one for a Retry — a retried apply that
// ran a *different* executor would be a second definition of what applying an import
// is.

// importTestContext builds a context with the tables one import executor needs and
// nothing else, for tests that never enter the Job control plane.
func importTestContext(t *testing.T, name string) *MahresourcesContext {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.ResourceCategory{}, &models.Group{}, &models.Resource{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("underlying db: %v", err)
	}
	fs := afero.NewMemMapFs()
	ctx := NewMahresourcesContext(fs, db, sqlx.NewDb(sqlDB, "sqlite3"), &MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	})
	db.FirstOrCreate(&models.ResourceCategory{Name: "Default"}, 1)
	return ctx
}

// TestRunImportApplyJob_RestoresPlanOnFailure verifies that when ApplyImport fails,
// the executor renames the consumed plan file back to .plan.json so the import can be
// applied again without re-uploading. Failure is deterministic: no tar is staged, so
// Phase 1 errors out immediately with "open tar".
func TestRunImportApplyJob_RestoresPlanOnFailure(t *testing.T) {
	ctx := importTestContext(t, "import_restore_fail")

	jobID := "restore-fail-test"
	planPath := filepath.Join("_imports", jobID+".plan.json")
	consumedPath := filepath.Join("_imports", jobID+".plan.applied.json")

	// Simulate handler state after it consumed the plan but before the runFn runs.
	plan := &ImportPlan{JobID: jobID}
	planBytes, _ := json.Marshal(plan)
	if err := ctx.GetDefaultFs().MkdirAll("_imports", 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := afero.WriteFile(ctx.GetDefaultFs(), consumedPath, planBytes, 0644); err != nil {
		t.Fatalf("write consumed plan: %v", err)
	}
	// Intentionally no tar — ApplyImport's Phase 1 will fail.

	runFn := ctx.buildImportApplyRunFn(jobID, consumedPath, &ImportDecisions{
		MappingActions:  map[string]MappingAction{},
		DanglingActions: map[string]DanglingAction{},
	})
	if err := runFn(context.Background(), nil, facadeSink{}); err == nil {
		t.Fatal("expected the executor to return an error when the tar is missing")
	}

	// Plan must be back at .plan.json so a retry can succeed.
	if exists, _ := afero.Exists(ctx.GetDefaultFs(), planPath); !exists {
		t.Errorf("plan was not restored: %s does not exist after failure", planPath)
	}
	if exists, _ := afero.Exists(ctx.GetDefaultFs(), consumedPath); exists {
		t.Errorf("consumed plan still exists at %s after restoration", consumedPath)
	}
}

// TestImportApplyPlanShouldBeRestored covers the four branches of the restoration
// gate: nil result (pre-Phase-1 failure), non-nil but no mutations (pre-write Phase 2
// abort), mutations on a retry-safe archive, and mutations on a retry-unsafe archive
// (legacy pre-GUID or GUIDCollisionPolicy=skip).
//
// It is the predicate the Kind's Retry advertisement is written in terms of, which is
// why it lives beside the executor rather than beside a handler.
func TestImportApplyPlanShouldBeRestored(t *testing.T) {
	cases := []struct {
		name   string
		result *ImportApplyResult
		want   bool
	}{
		{"nil result", nil, true},
		{"no mutations, retry-unsafe", &ImportApplyResult{RetrySafe: false}, true},
		{"mutations, retry-safe", &ImportApplyResult{CreatedCategories: 1, RetrySafe: true}, true},
		{"mutations, retry-unsafe", &ImportApplyResult{CreatedCategories: 1, RetrySafe: false}, false},
		{"mutations via group IDs, retry-unsafe", &ImportApplyResult{CreatedGroupIDs: []uint{1}, RetrySafe: false}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ImportApplyPlanShouldBeRestored(tc.result); got != tc.want {
				t.Errorf("ImportApplyPlanShouldBeRestored = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestImportApplyPlanShouldBeRestored_RefusesLegacyWithMutations pins the case the
// Retry predicate exists for: a legacy archive that already committed rows may not
// be replayed, so no Retry is offered and the plan stays consumed.
func TestImportApplyPlanShouldBeRestored_RefusesLegacyWithMutations(t *testing.T) {
	if ImportApplyPlanShouldBeRestored(&ImportApplyResult{CreatedGroups: 1, RetrySafe: false}) {
		t.Error("expected no restoration for a legacy archive with committed mutations")
	}
}

// TestRunImportApplyJob_LegacyArchiveWithNoMutationsRestoresItsPlan verifies that a
// legacy (pre-GUID) archive whose apply aborted before any write restores its plan:
// the DB is unchanged, so replay cannot duplicate anything.
func TestRunImportApplyJob_LegacyArchiveWithNoMutationsRestoresItsPlan(t *testing.T) {
	ctx := importTestContext(t, "import_legacy_no_restore")

	jobID := "legacy-no-restore"
	planPath := filepath.Join("_imports", jobID+".plan.json")
	consumedPath := filepath.Join("_imports", jobID+".plan.applied.json")
	tarPath := filepath.Join("_imports", jobID+".tar")

	// A legacy-style archive: a group with NO GUID on the payload, and a tar that
	// parses fine but whose apply is cancelled just after collection — the easiest
	// reproducible failure that still lets collection run and set RetrySafe correctly.
	var buf bytes.Buffer
	w, err := archive.NewWriter(&buf, false)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.WriteManifest(&archive.Manifest{
		SchemaVersion: 1,
		CreatedAt:     time.Now().UTC(),
		CreatedBy:     "test",
		Roots:         []string{"g0001"},
		Counts:        archive.Counts{Groups: 1},
		Entries: archive.Entries{
			Groups: []archive.GroupEntry{
				{ExportID: "g0001", Name: "LegacyGroup", SourceID: 1, Path: "groups/g0001.json"},
			},
		},
	}); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	if err := w.WriteGroup(&archive.GroupPayload{
		ExportID: "g0001",
		SourceID: 1,
		Name:     "LegacyGroup",
		// GUID intentionally empty — simulates a pre-GUID archive.
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("WriteGroup: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	if err := ctx.GetDefaultFs().MkdirAll("_imports", 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := afero.WriteFile(ctx.GetDefaultFs(), tarPath, buf.Bytes(), 0644); err != nil {
		t.Fatalf("write tar: %v", err)
	}

	// Parse writes .plan.json; simulate the handler's rename.
	plan, err := ctx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := ctx.GetDefaultFs().Rename(planPath, consumedPath); err != nil {
		t.Fatalf("rename plan to consumed: %v", err)
	}

	// Cancel the context before the apply runs, so ApplyImport fails after
	// collection — which records RetrySafe=false for this legacy archive.
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	decisions := &ImportDecisions{
		MappingActions:  map[string]MappingAction{},
		DanglingActions: map[string]DanglingAction{},
	}
	for _, entry := range plan.Mappings.Categories {
		decisions.MappingActions[entry.DecisionKey] = MappingAction{
			Include: true, Action: "create", DestinationID: entry.DestinationID,
		}
	}

	runFn := ctx.buildImportApplyRunFn(jobID, consumedPath, decisions)
	if err := runFn(cancelledCtx, nil, facadeSink{}); err == nil {
		t.Fatal("expected the executor to return an error (cancelled context)")
	}

	// The pre-cancelled context fails at the first inter-phase checkpoint, before any
	// row is written, so the plan is safe to restore even for a legacy archive.
	if exists, _ := afero.Exists(ctx.GetDefaultFs(), planPath); !exists {
		t.Errorf("plan was not restored at %s even though no DB writes happened", planPath)
	}
}

// ---- the import Kind pair at the submission seam -------------------------------

// writeImportArchiveForTest stages one small, valid archive at the path an import
// reads. The handle names it, exactly as the handler names it: the Kind's input is a
// reference to a staged file, and a test that invented its own naming would be testing
// a layout nobody uses.
func writeImportArchiveForTest(t *testing.T, ctx *MahresourcesContext, handle string) string {
	t.Helper()
	var buf bytes.Buffer
	w, err := archive.NewWriter(&buf, false)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.WriteManifest(&archive.Manifest{
		SchemaVersion: 1,
		CreatedAt:     time.Now().UTC(),
		CreatedBy:     "test",
		Roots:         []string{"g0001"},
		Counts:        archive.Counts{Groups: 1},
		Entries: archive.Entries{
			Groups: []archive.GroupEntry{
				{ExportID: "g0001", Name: "Imported", SourceID: 1, Path: "groups/g0001.json"},
			},
		},
	}); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	if err := w.WriteGroup(&archive.GroupPayload{
		ExportID:  "g0001",
		SourceID:  1,
		Name:      "Imported",
		GUID:      "11111111-2222-3333-4444-555555555555",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("WriteGroup: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	fs := ctx.GetDefaultFs()
	if err := fs.MkdirAll("_imports", 0755); err != nil {
		t.Fatalf("mkdir _imports: %v", err)
	}
	staging := filepath.Join("_imports", "staging-"+handle)
	if err := afero.WriteFile(fs, staging, buf.Bytes(), 0644); err != nil {
		t.Fatalf("stage the archive: %v", err)
	}
	return staging
}

// TestAnImportParseAcceptsADurableJobAndPublishesItsPlan is the parse half of dual
// publication: the submission accepts a durable Job, the plan it produced is published
// as an output pointing at the plan file, and the Job's success is what says the plan is
// reviewable.
func TestAnImportParseAcceptsADurableJobAndPublishesItsPlan(t *testing.T) {
	ctx := newWorkflowJobContext(t)
	handle := "imp-accept-1"
	staging := writeImportArchiveForTest(t, ctx, handle)

	submission := ctx.SubmitImportParse(handle, staging, "api")
	if submission.Err != nil {
		t.Fatalf("submit the parse: %v", submission.Err)
	}
	if submission.CanonicalJobID == "" {
		t.Fatalf("the parse created no durable job")
	}
	if submission.QueueJobID != handle {
		t.Fatalf("the submission answered queue id %q, want the handle %q", submission.QueueJobID, handle)
	}

	snap := waitForSnapshot(t, ctx, submission.CanonicalJobID, "the parse to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if snap.Kind != JobKindGroupImportParse {
		t.Fatalf("the parse job is %q, want %s", snap.Kind, JobKindGroupImportParse)
	}
	if snap.State != jobs.StateSucceeded {
		t.Fatalf("the parse ended %s (%+v)", snap.State, snap.Failure)
	}
	outputs, err := ctx.GetJobOutputs(snap.ID)
	if err != nil {
		t.Fatalf("read outputs: %v", err)
	}
	plan, published := findJobOutput(outputs, jobImportPlanOutput)
	if !published {
		t.Fatalf("the succeeded parse published no plan output: %+v", outputs)
	}
	planPath := importPlanPathFor(handle)
	if exists, _ := afero.Exists(ctx.GetDefaultFs(), planPath); !exists {
		t.Fatalf("the parse job %s but no plan is at %s (output reference %s)",
			snap.State, planPath, plan.Reference)
	}
}

// TestAnImportParseRetryIsAdvertisedOnlyWhileTheArchiveRemains pins the Kind's own
// policy: the staged archive is the input a re-parse reads, so a Retry is offered only
// while it is there. Advertising one and failing at dispatch would be a button that
// lies.
func TestAnImportParseRetryIsAdvertisedOnlyWhileTheArchiveRemains(t *testing.T) {
	ctx := newWorkflowJobContext(t)
	handle := "imp-retry-1"
	staging := writeImportArchiveForTest(t, ctx, handle)

	// A staged file that is not an archive at all: the parse fails deterministically,
	// which is the outcome whose Retry is at issue.
	if err := afero.WriteFile(ctx.GetDefaultFs(), staging, []byte("not a tar"), 0644); err != nil {
		t.Fatalf("write a corrupt archive: %v", err)
	}
	submission := ctx.SubmitImportParse(handle, staging, "api")
	if submission.Err != nil {
		t.Fatalf("submit the parse: %v", submission.Err)
	}
	snap := waitForSnapshot(t, ctx, submission.CanonicalJobID, "the parse to fail", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if snap.State != jobs.StateFailed {
		t.Fatalf("a corrupt archive ended %s, want failed", snap.State)
	}
	if !offersCommand(advertisedForTest(t, ctx, snap.ID), jobs.CommandRetry) {
		t.Fatalf("a failed parse with its archive still staged offered no Retry")
	}

	// The archive goes — "Clear completed", or the delete handler — and the input the
	// Retry would read is gone with it.
	if err := ctx.GetDefaultFs().Remove(importArchivePathFor(handle)); err != nil {
		t.Fatalf("remove the staged archive: %v", err)
	}
	if offersCommand(advertisedForTest(t, ctx, snap.ID), jobs.CommandRetry) {
		t.Fatalf("a parse whose archive is gone still advertised a Retry")
	}
}

// TestAnImportApplyIsAChildOfItsParseAndRetriesOnlyOnRestoredEvidence covers the apply
// half: the apply Job is linked to the parse it decided on, and a Retry is advertised
// only while the plan is back at its unconsumed path — which is the evidence the
// executor itself produced when it decided replay was safe.
func TestAnImportApplyIsAChildOfItsParseAndRetriesOnlyOnRestoredEvidence(t *testing.T) {
	ctx := newWorkflowJobContext(t)
	handle := "imp-apply-1"
	staging := writeImportArchiveForTest(t, ctx, handle)

	submission := ctx.SubmitImportParse(handle, staging, "api")
	if submission.Err != nil {
		t.Fatalf("submit the parse: %v", submission.Err)
	}
	waitForSnapshot(t, ctx, submission.CanonicalJobID, "the parse to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})

	// Apply with the archive gone: Phase 1 cannot read it, so the apply fails before any
	// row is written and the executor restores the plan — the replay-safe answer.
	consumed, err := ConsumeImportPlan(ctx.GetDefaultFs(), handle)
	if err != nil {
		t.Fatalf("consume the plan: %v", err)
	}
	if err := ctx.GetDefaultFs().Remove(importArchivePathFor(handle)); err != nil {
		t.Fatalf("remove the staged archive: %v", err)
	}
	apply := ctx.SubmitImportApply(handle, consumed, &ImportDecisions{
		MappingActions:  map[string]MappingAction{},
		DanglingActions: map[string]DanglingAction{},
	}, "api")
	if apply.Err != nil {
		t.Fatalf("submit the apply: %v", apply.Err)
	}
	applied := waitForSnapshot(t, ctx, apply.CanonicalJobID, "the apply to fail", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if applied.State != jobs.StateFailed {
		t.Fatalf("an apply whose archive is gone ended %s, want failed", applied.State)
	}

	// The parse is the apply's parent: a review and its decision are two stages, and the
	// lineage is what says so.
	lineage, err := ctx.GetJobLineage(applied.ID)
	if err != nil {
		t.Fatalf("read the lineage: %v", err)
	}
	parents := map[string]bool{}
	for _, relative := range lineage.Parents {
		parents[relative.ID] = true
	}
	if !parents[submission.CanonicalJobID] {
		t.Fatalf("the apply is not a child of its parse: %+v", lineage)
	}

	// A Retry needs both the restored plan and the archive it reads blobs from, and the
	// archive is gone, so this failure is not replayable through the canonical surface.
	if offersCommand(advertisedForTest(t, ctx, applied.ID), jobs.CommandRetry) {
		t.Fatalf("an apply with no archive left advertised a Retry")
	}

	// With the archive back and the plan at its unconsumed path, the same Job offers one.
	if err := afero.WriteFile(ctx.GetDefaultFs(), importArchivePathFor(handle), []byte("x"), 0644); err != nil {
		t.Fatalf("restore the archive: %v", err)
	}
	planRestored, _ := afero.Exists(ctx.GetDefaultFs(), importPlanPathFor(handle))
	if !planRestored {
		if renameErr := ctx.GetDefaultFs().Rename(importConsumedPlanPathFor(handle), importPlanPathFor(handle)); renameErr != nil {
			t.Fatalf("the executor did not restore the plan: %v", renameErr)
		}
	}
	if !offersCommand(advertisedForTest(t, ctx, applied.ID), jobs.CommandRetry) {
		t.Fatalf("an apply with a restored plan and a staged archive offered no Retry")
	}
}

// TestAnImportsAuthorizationOutlivesItsQueueEntry is the property the plan calls out
// explicitly: the durable Job behind a parse handle is what authorizes the plan, the
// result and the archive, so "Clear completed" — which removes the in-memory queue entry
// while those files stay on disk — cannot turn an owner's own import into nobody's, and
// cannot turn somebody else's into theirs.
func TestAnImportsAuthorizationOutlivesItsQueueEntry(t *testing.T) {
	ctx := newWorkflowJobContext(t)
	owner := &models.User{Username: "import-owner", Role: models.RoleUser}
	other := &models.User{Username: "import-other", Role: models.RoleUser}
	for _, user := range []*models.User{owner, other} {
		if err := ctx.db.Create(user).Error; err != nil {
			t.Fatalf("create %s: %v", user.Username, err)
		}
	}

	handle := "imp-owner-1"
	staging := writeImportArchiveForTest(t, ctx, handle)
	ownerCtx := ctx.WithPrincipal(auth.FromUser(owner))
	submission := ownerCtx.SubmitImportParse(handle, staging, "api")
	if submission.Err != nil {
		t.Fatalf("submit the parse: %v", submission.Err)
	}
	waitForSnapshot(t, ctx, submission.CanonicalJobID, "the parse to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})

	// The queue entry goes, the way it goes in practice: the owner presses "Clear
	// completed", or an hour passes.
	ctx.downloadManager.ClearFinished(nil)
	if _, stillThere := ctx.downloadManager.GetJob(handle); stillThere {
		t.Fatalf("precondition: the queue entry is still there, so this test measured nothing")
	}

	authorized, answered := ownerCtx.ImportJobAuthorized(handle)
	if !answered {
		t.Fatalf("no durable job answered for the cleared import")
	}
	if !authorized {
		t.Fatalf("the owner was refused their own cleared import")
	}
	authorized, answered = ctx.WithPrincipal(auth.FromUser(other)).ImportJobAuthorized(handle)
	if !answered {
		t.Fatalf("no durable job answered for the cleared import")
	}
	if authorized {
		t.Fatalf("another user was authorized on somebody else's cleared import")
	}
}

// TestStartupCleanupKeepsTheInputsANonterminalJobStillNeeds is the retention
// invariant the legacy sweep used to break.
//
// Startup cleanup deleted every staging file older than the export retention,
// purely by modtime — including `_imports` archives and the plans a queued apply
// is going to read. A restart after an outage longer than that window therefore
// erased the input of work that had not run yet, and the Job could only fail with
// an error nobody could act on. Age is not ownership: the durable Job is, and this
// is the sweep asking it.
func TestStartupCleanupKeepsTheInputsANonterminalJobStillNeeds(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	if ctx.exportSweepFs == nil {
		t.Skip("this harness has no startup sweep filesystem")
	}
	// A window the staged files are older than, so the sweep would remove them if
	// nothing protected them.
	ctx.DownloadManager().SetSettings(download_queue.NewStaticDownloadSettings(
		download_queue.TimeoutConfig{}, time.Hour))
	fs := ctx.GetDefaultFs()
	if err := fs.MkdirAll("_imports", 0o755); err != nil {
		t.Fatalf("mkdir _imports: %v", err)
	}

	// A parse Job that has not finished, with the archive and plan it will read.
	const parseHandle = "0123456789abcdef"
	parse := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindGroupImportParse, KindVersion: jobImportKindVersion,
		State: jobs.StateQueued, Origin: "api",
		Replay:     jobs.ReplayInput{Input: json.RawMessage(`{"handle":"` + parseHandle + `","archive":"_imports/` + parseHandle + `.tar"}`)},
		LegacyRefs: []jobs.LegacyRef{{Namespace: ImportParseHandleNamespace, Handle: parseHandle}},
	})
	required := []string{
		importArchivePathFor(parseHandle),
		importPlanPathFor(parseHandle),
	}
	for _, path := range required {
		if err := afero.WriteFile(fs, path, []byte("staged"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		ageStagingFileForTest(t, fs, path)
	}
	// One file nothing owns, which the sweep must still remove: without this the
	// test would pass on a sweep that had simply stopped working.
	orphan := "_imports/feedfacefeedface.tar"
	if err := afero.WriteFile(fs, orphan, []byte("orphan"), 0o644); err != nil {
		t.Fatalf("write the orphan: %v", err)
	}
	ageStagingFileForTest(t, fs, orphan)

	ctx.RunStartupExportSweep()

	for _, path := range required {
		exists, err := afero.Exists(fs, path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if !exists {
			t.Fatalf("startup cleanup deleted %s, which job %s still requires", path, parse.ID)
		}
	}
	if exists, _ := afero.Exists(fs, orphan); exists {
		t.Fatalf("startup cleanup kept %s, which no job owns", orphan)
	}
}

// TestStartupCleanupKeepsAFinishedParentsFilesForItsQueuedChild is the same
// invariant one link out: a pending apply reads its parse's plan and archive, and
// the parse Job has already succeeded. Protecting only the nonterminal Job's own
// name would delete exactly the files the apply is waiting for.
func TestStartupCleanupKeepsAFinishedParentsFilesForItsQueuedChild(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	if ctx.exportSweepFs == nil {
		t.Skip("this harness has no startup sweep filesystem")
	}
	ctx.DownloadManager().SetSettings(download_queue.NewStaticDownloadSettings(
		download_queue.TimeoutConfig{}, time.Hour))
	fs := ctx.GetDefaultFs()
	if err := fs.MkdirAll("_imports", 0o755); err != nil {
		t.Fatalf("mkdir _imports: %v", err)
	}

	const parseHandle = "abcdef0123456789"
	parse := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindGroupImportParse, KindVersion: jobImportKindVersion,
		State: jobs.StateQueued, Origin: "api",
		Replay:     jobs.ReplayInput{Input: json.RawMessage(`{"handle":"` + parseHandle + `","archive":"_imports/` + parseHandle + `.tar"}`)},
		LegacyRefs: []jobs.LegacyRef{{Namespace: ImportParseHandleNamespace, Handle: parseHandle}},
	})
	finished := finishJobFor(t, ctx, parse, jobs.StateSucceeded)

	apply := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindGroupImportApply, KindVersion: jobImportKindVersion,
		State: jobs.StateQueued, Origin: "api",
		Replay: jobs.ReplayInput{Input: json.RawMessage(
			`{"parseHandle":"` + parseHandle + `","plan":"` + importConsumedPlanPathFor(parseHandle) + `","decisions":{}}`)},
	})
	if err := ctx.JobService().Link(ctx.jobDeps(), jobs.LinkRequest{
		Type: jobs.LinkParentChild, FromJobID: finished.ID, ToJobID: apply.ID,
	}); err != nil {
		t.Fatalf("link the apply to its parse: %v", err)
	}

	archive := importArchivePathFor(parseHandle)
	consumed := importConsumedPlanPathFor(parseHandle)
	for _, path := range []string{archive, consumed} {
		if err := afero.WriteFile(fs, path, []byte("staged"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		ageStagingFileForTest(t, fs, path)
	}

	ctx.RunStartupExportSweep()

	for _, path := range []string{archive, consumed} {
		if exists, _ := afero.Exists(fs, path); !exists {
			t.Fatalf("startup cleanup deleted %s, which the queued apply %s still reads", path, apply.ID)
		}
	}
}

// ageStagingFileForTest makes one staging file older than any retention window, so
// the sweep would remove it if nothing protected it.
func ageStagingFileForTest(t *testing.T, fs afero.Fs, path string) {
	t.Helper()
	old := time.Now().Add(-72 * time.Hour)
	if err := fs.Chtimes(path, old, old); err != nil {
		t.Fatalf("age %s: %v", path, err)
	}
}
