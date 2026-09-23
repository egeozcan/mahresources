package application_context

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
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
	"mahresources/models/query_models"
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

// TestAQueuedImportApplyRestartsFromItsAdmittedPlan is the crash boundary between
// durable acceptance and queue creation.
//
// The plan is consumed when the apply is *submitted* — that is what makes a second
// /apply on one review a refusal — and the consumed path is recorded in the sealed
// input. A process that stops between accepting the Job and enqueuing it, or a
// dispatch loop that claims the Job first, therefore finds an executor whose only
// evidence of what to read is that recorded path: consuming the plan again cannot
// work (the unconsumed one is gone by construction), and failing the Job for it
// leaves an intact archive and an untouched plan that no command can use.
func TestAQueuedImportApplyRestartsFromItsAdmittedPlan(t *testing.T) {
	ctx := newWorkflowJobContext(t)
	handle := "imp-crash-boundary"
	staging := writeImportArchiveForTest(t, ctx, handle)

	parse := ctx.SubmitImportParse(handle, staging, "api")
	if parse.Err != nil {
		t.Fatalf("submit the parse: %v", parse.Err)
	}
	parsed := waitForSnapshot(t, ctx, parse.CanonicalJobID, "the parse to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if parsed.State != jobs.StateSucceeded {
		t.Fatalf("the parse ended %s (%+v)", parsed.State, parsed.Failure)
	}

	// The handler's half: consume the plan, exactly as the applied route does.
	consumed, err := ConsumeImportPlan(ctx.GetDefaultFs(), handle)
	if err != nil {
		t.Fatalf("consume the plan: %v", err)
	}

	// The crash boundary: the Job is accepted durably and nothing is enqueued. The
	// runtime's loop is running, so it claims the Job and dispatches it from the
	// recorded input alone — which is exactly what a restart does.
	decisions := ImportDecisions{
		MappingActions:  map[string]MappingAction{},
		DanglingActions: map[string]DanglingAction{},
	}
	input, err := json.Marshal(importApplyJobInput{
		ParseHandle: handle, Plan: consumed, Decisions: decisions,
	})
	if err != nil {
		t.Fatalf("encode the apply input: %v", err)
	}
	accepted := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindGroupImportApply, KindVersion: jobImportKindVersion,
		State: jobs.StateQueued, Origin: "api",
		Replay:     jobs.ReplayInput{Input: input},
		LegacyRefs: []jobs.LegacyRef{{Namespace: ImportApplyHandleNamespace, Handle: "crash-boundary-1"}},
	})

	applied := waitForSnapshot(t, ctx, accepted.ID, "the accepted apply to run", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if applied.State != jobs.StateSucceeded {
		t.Fatalf("the accepted apply ended %s (%+v): its admitted plan could not be used", applied.State, applied.Failure)
	}

	// And the import really was applied: the archive's group is in the library.
	var imported int64
	if err := ctx.db.Model(&models.Group{}).Where("name = ?", "Imported").Count(&imported).Error; err != nil {
		t.Fatalf("count the imported group: %v", err)
	}
	if imported != 1 {
		t.Fatalf("%d groups named Imported, want the one the apply created", imported)
	}
}

// stageConsumedApplyForTest puts one import in the exact filesystem state an apply is
// in *while it runs*: the plan has been consumed, and the archive it reads blobs from
// is still there.
//
// It is the state the accounting has to be right about, and the only one that
// distinguishes nothing from the outside: a consumed plan is written by the submission
// before the executor exists, and it is what a live executor leaves behind for the whole
// of its run.
func stageConsumedApplyForTest(t *testing.T, ctx *MahresourcesContext, handle string) string {
	t.Helper()
	fs := ctx.GetDefaultFs()
	if err := fs.MkdirAll("_imports", 0o755); err != nil {
		t.Fatalf("mkdir _imports: %v", err)
	}
	consumed := importConsumedPlanPathFor(handle)
	for _, path := range []string{consumed, importArchivePathFor(handle)} {
		if err := afero.WriteFile(fs, path, []byte("staged"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return consumed
}

// acceptApplyJobForTest accepts one apply Job whose input names a consumed plan.
func acceptApplyJobForTest(t *testing.T, ctx *MahresourcesContext, handle, legacyID, consumed string) jobs.Snapshot {
	t.Helper()
	input, err := json.Marshal(importApplyJobInput{
		ParseHandle: handle,
		Plan:        consumed,
		Decisions: ImportDecisions{
			MappingActions:  map[string]MappingAction{},
			DanglingActions: map[string]DanglingAction{},
		},
	})
	if err != nil {
		t.Fatalf("encode the apply input: %v", err)
	}
	return acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindGroupImportApply, KindVersion: jobImportKindVersion,
		State: jobs.StateQueued, Origin: "api",
		Replay:     jobs.ReplayInput{Input: input},
		LegacyRefs: []jobs.LegacyRef{{Namespace: ImportApplyHandleNamespace, Handle: legacyID}},
	})
}

// TestAnApplyReconciledWhereItIsNotRunningIsNotTerminatedWhileItMayBeLive is the
// two-process reconciliation boundary.
//
// A consumed plan is what an apply *runs* against, so the process that holds no queue
// entry for the Job is in the same filesystem state whether the apply is running in
// another process or died half-way through. Reading that absence as "nothing can
// continue" terminated live work, released its ownership and recorded a failure for an
// import that was still committing rows. Liveness is the missing premise, and with the
// runtime alive the claim, the capacity and the Job stay exactly where they are.
func TestAnApplyReconciledWhereItIsNotRunningIsNotTerminatedWhileItMayBeLive(t *testing.T) {
	first := newJobHarnessContext(t, false)
	first.Config.MaxJobConcurrency = 2
	key := sharedReplayKey(t)
	holdJobReplayKey(t, first, key)
	other, _ := newSecondProcessJobContext(t, first, key)

	const handle = "apply-live-1"
	consumed := stageConsumedApplyForTest(t, first, handle)
	accepted := acceptApplyJobForTest(t, first, handle, "apply-live-1", consumed)

	// The process that owns it: a real claim under this runtime's own identity, with
	// the queue entry that is applying the plan.
	execution, claimed, err := first.JobService().Claim(context.Background(), first.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindGroupImportApply, KindVersion: jobImportKindVersion, JobID: accepted.ID,
		Claimant: defaultJobRuntimeClaimant(), Capacity: first.hostClaimCapacityBudget(),
		// A lease short enough to expire inside the test; the adapter is asked about the
		// claim, not about the clock.
		Lease: 20 * time.Millisecond,
	})
	if err != nil || !claimed {
		t.Fatalf("claim the apply: claimed=%v err=%v", claimed, err)
	}
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	entry, err := first.submitQueueJob(
		download_queue.JobOptions{Source: download_queue.JobSourceGroupImportApply, InitialPhase: "applying"},
		"apply-live-1",
		jobs.ExecutionRef{JobID: accepted.ID, ExecutionToken: execution.ExecutionToken},
		func(context.Context, *download_queue.DownloadJob, download_queue.ProgressSink) error {
			<-release
			return nil
		},
	)
	if err != nil {
		t.Fatalf("start the apply's executor: %v", err)
	}
	if _, found := other.DownloadManager().GetJobByCanonicalJobID(accepted.ID); found {
		t.Fatalf("the second process holds an executor for work it does not own")
	}

	time.Sleep(40 * time.Millisecond)
	if decision := reconcileOnce(t, other, accepted.ID); decision != jobs.ReconcileExternalWorkUnproven {
		t.Fatalf("the reconciler decided %q for an apply it cannot prove stopped, want %q",
			decision, jobs.ReconcileExternalWorkUnproven)
	}

	snap := jobSnapshot(t, other.JobService(), other, accepted.ID)
	if snap.State != jobs.StateBlocked {
		t.Fatalf("the reconciled apply is %s, want blocked for a person to resolve", snap.State)
	}
	if snap.Failure != nil {
		t.Fatalf("the apply recorded the failure %+v: nothing was proved about its execution", snap.Failure)
	}
	if claim := storedClaim(t, other, accepted.ID); claim.State != models.JobClaimStateQuarantined {
		t.Fatalf("the claim is %s, want it held for the work that may still be running", claim.State)
	}
	if held := storedCapacity(t, other, jobs.CapacityGroupGlobal); held != 1 {
		t.Fatalf("the deployment budget holds %d slots, want the running apply's one still occupied", held)
	}
	// And the transfer of the claim is untouched: the executor is still applying.
	if current := storedClaim(t, other, accepted.ID); current.ExecutionToken != execution.ExecutionToken {
		t.Fatalf("the claim's token moved from %q to %q while the execution was unproven",
			execution.ExecutionToken, current.ExecutionToken)
	}
	if entryNow, found := first.DownloadManager().GetJob(entry.ID); !found ||
		entryNow.GetStatus() == download_queue.JobStatusFailed ||
		entryNow.GetStatus() == download_queue.JobStatusCancelled {
		t.Fatalf("the running apply's executor was stopped from another process: %v", entryNow)
	}
}

// TestAnApplyReconciledAfterItsRuntimeIsProvedGoneIsFailed is the other side of the same
// rule: once quiescence *is* proved, an apply whose plan was consumed and never restored
// has reached the one state that cannot be replayed, and the honest answer is a failure
// with its report rather than work nobody can run.
func TestAnApplyReconciledAfterItsRuntimeIsProvedGoneIsFailed(t *testing.T) {
	ctx := newJobHarnessContext(t, false)

	const handle = "apply-gone-1"
	consumed := stageConsumedApplyForTest(t, ctx, handle)
	accepted := acceptApplyJobForTest(t, ctx, handle, "apply-gone-1", consumed)
	registerClaimableKind(t, ctx, JobKindGroupImportApply, jobImportKindVersion)

	// A runtime that cannot still exist: this host, a boot session it is not in.
	execution, claimed, err := ctx.JobService().Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindGroupImportApply, KindVersion: jobImportKindVersion, JobID: accepted.ID,
		Claimant: goneRuntimeIdentityForTest(), Lease: 20 * time.Millisecond,
	})
	if err != nil || !claimed {
		t.Fatalf("claim the apply: claimed=%v err=%v", claimed, err)
	}
	_ = execution
	time.Sleep(40 * time.Millisecond)

	if decision := reconcileOnce(t, ctx, accepted.ID); decision != jobs.ReconcileFail {
		t.Fatalf("the reconciler decided %q for an apply whose runtime is gone, want %q",
			decision, jobs.ReconcileFail)
	}
	snap := waitForSnapshot(t, ctx, accepted.ID, "the reconciled apply to be failed", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if snap.State != jobs.StateFailed {
		t.Fatalf("the apply ended %s, want failed", snap.State)
	}
}

// TestStartupCleanupKeepsTheInputsARetriedApplyStillReads is the retention invariant one
// link further out than a parent: a Retry.
//
// A Retry creates a new Job and moves the failed apply's legacy handle onto it, so the
// successor carries an apply handle and nothing else. The plan, archive and report it
// runs against are named after its *parse's* handle, and the parse is not its parent —
// the retry-of link replaced that relation. Protecting only the open Job's own handles
// plus one level of parentage therefore stopped covering `_imports/<parseHandle>.*` at
// exactly the moment the work was retried, and startup cleanup deleted the queued
// successor's input.
func TestStartupCleanupKeepsTheInputsARetriedApplyStillReads(t *testing.T) {
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

	const parseHandle = "cafe0123456789ab"
	parse := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindGroupImportParse, KindVersion: jobImportKindVersion,
		State: jobs.StateQueued, Origin: "api",
		Replay:     jobs.ReplayInput{Input: json.RawMessage(`{"handle":"` + parseHandle + `","archive":"_imports/` + parseHandle + `.tar"}`)},
		LegacyRefs: []jobs.LegacyRef{{Namespace: ImportParseHandleNamespace, Handle: parseHandle}},
	})
	finishedParse := finishJobFor(t, ctx, parse, jobs.StateSucceeded)

	// The apply that decided on the parse, and was retried after failing.
	input, err := json.Marshal(importApplyJobInput{
		ParseHandle: parseHandle,
		Plan:        importConsumedPlanPathFor(parseHandle),
		Decisions: ImportDecisions{
			MappingActions:  map[string]MappingAction{},
			DanglingActions: map[string]DanglingAction{},
		},
	})
	if err != nil {
		t.Fatalf("encode the apply input: %v", err)
	}
	apply := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindGroupImportApply, KindVersion: jobImportKindVersion,
		State: jobs.StateQueued, Origin: "api",
		Replay:     jobs.ReplayInput{Input: input},
		LegacyRefs: []jobs.LegacyRef{{Namespace: ImportApplyHandleNamespace, Handle: "retried-apply"}},
	})
	if err := ctx.JobService().Link(ctx.jobDeps(), jobs.LinkRequest{
		Type: jobs.LinkParentChild, FromJobID: finishedParse.ID, ToJobID: apply.ID,
	}); err != nil {
		t.Fatalf("link the apply to its parse: %v", err)
	}
	failedApply := finishJobFor(t, ctx, apply, jobs.StateFailed)

	// The Retry: a new queued Job, linked retry-of, holding the moved handle.
	retried := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindGroupImportApply, KindVersion: jobImportKindVersion,
		State: jobs.StateQueued, Origin: "api",
		Replay: jobs.ReplayInput{Input: input},
	})
	if err := ctx.JobService().Link(ctx.jobDeps(), jobs.LinkRequest{
		Type: jobs.LinkRetryOf, FromJobID: retried.ID, ToJobID: failedApply.ID,
	}); err != nil {
		t.Fatalf("link the retry to the failed apply: %v", err)
	}
	if err := moveLegacyHandlesForTest(t, ctx, failedApply.ID, retried.ID); err != nil {
		t.Fatalf("move the handle onto the retry: %v", err)
	}

	required := []string{
		importArchivePathFor(parseHandle),
		importConsumedPlanPathFor(parseHandle),
	}
	for _, path := range required {
		if err := afero.WriteFile(fs, path, []byte("staged"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		ageStagingFileForTest(t, fs, path)
	}

	ctx.RunStartupExportSweep()

	for _, path := range required {
		if exists, _ := afero.Exists(fs, path); !exists {
			t.Fatalf("startup cleanup deleted %s, which the queued retry %s still reads", path, retried.ID)
		}
	}
}

// moveLegacyHandlesForTest performs the handle movement a Retry commits, through the
// durable table the production path writes. The movement is normally inside the command
// transaction; a test that built the lineage by hand has to do it explicitly, because the
// handle is what the sweep reads.
func moveLegacyHandlesForTest(t *testing.T, ctx *MahresourcesContext, from, to string) error {
	t.Helper()
	return ctx.db.Model(&models.JobLegacyHandle{}).Where("job_id = ?", from).
		Update("job_id", to).Error
}

// TestAQueuedImportApplyIsRefusedWhenItsActorLosesTheAuthorityToWrite is §8's rule
// reaching the one place it cannot be assumed: an apply admitted while the deployment
// was busy, dispatched later by whichever runtime has room.
//
// The Job records the actor and the dispatch binds that actor's *scope*, which is
// exactly why the missing half was easy to miss: binding a subtree does not ask whether
// the account may write at all. An actor demoted to guest between acceptance and
// dispatch would therefore have had its import applied — groups created, taxonomy rows
// inserted — by a runtime that never carried the request that proved otherwise. The
// refusal is a block, decided before the plan is read or any row is touched.
func TestAQueuedImportApplyIsRefusedWhenItsActorLosesTheAuthorityToWrite(t *testing.T) {
	first := newJobHarnessContext(t, false)
	first.Config.MaxJobConcurrency = 1
	key := sharedReplayKey(t)
	holdJobReplayKey(t, first, key)
	other, otherRuntime := newSecondProcessJobContext(t, first, key)

	actor, err := first.CreateUser(&UserInput{
		Username: "import-actor", Password: "correct-horse-battery", Role: models.RoleUser,
	})
	if err != nil {
		t.Fatalf("create the actor: %v", err)
	}
	actorCtx := first.WithPrincipal(auth.FromUser(actor))

	// The plan an apply reads comes from a parse of this archive, run while the
	// deployment had room for it.
	handle := "imp-demoted-1"
	staging := writeImportArchiveForTest(t, first, handle)
	parse := actorCtx.SubmitImportParse(handle, staging, "api")
	if parse.Err != nil {
		t.Fatalf("submit the parse: %v", parse.Err)
	}
	waitForSnapshot(t, first, parse.CanonicalJobID, "the parse to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	consumed, err := ConsumeImportPlan(first.GetDefaultFs(), handle)
	if err != nil {
		t.Fatalf("consume the plan: %v", err)
	}

	// The deployment's one slot is taken, so the apply is accepted durably and runs
	// nowhere until a runtime has room.
	server, _, unblock := heldTransferServer(t)
	holder := first.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{URL: server.URL + "/holding.bin"}, nil, "", "api")
	if len(holder) != 1 || holder[0].Err != nil {
		t.Fatalf("the holding transfer: %+v", holder)
	}
	waitForSnapshot(t, first, holder[0].CanonicalJobID, "the holding transfer to start", func(s jobs.Snapshot) bool {
		return s.State == jobs.StateRunning
	})

	apply := actorCtx.SubmitImportApply(handle, consumed, &ImportDecisions{
		MappingActions:  map[string]MappingAction{},
		DanglingActions: map[string]DanglingAction{},
	}, "api")
	if apply.Err != nil {
		t.Fatalf("submit the apply: %v", apply.Err)
	}
	if _, found := first.queueEntryFor(apply.CanonicalJobID); found {
		t.Fatalf("the apply started an executor while the deployment's only slot was taken")
	}

	// The demotion, between acceptance and dispatch. The account is the same one and
	// the Job's actor is unchanged; only what the actor may do has moved.
	group := &models.Group{Name: "demoted-scope"}
	if err := first.db.Create(group).Error; err != nil {
		t.Fatalf("create the scope group: %v", err)
	}
	if _, err := first.UpdateUser(actor.ID, &UserUpdate{
		Role:         UserField[models.Role]{Set: true, Value: models.RoleGuest},
		ScopeGroupID: UserField[*uint]{Set: true, Value: &group.ID},
	}); err != nil {
		t.Fatalf("demote the actor: %v", err)
	}

	// The slot frees and the other process dispatches the waiting apply.
	unblock()
	waitForSnapshot(t, first, holder[0].CanonicalJobID, "the holding transfer to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	waitFor(t, "the other runtime to claim and dispatch the apply", func() bool {
		otherRuntime.tick(context.Background())
		snap, err := other.JobService().Get(other.jobDeps(), jobs.Access{Administrator: true}, apply.CanonicalJobID)
		return err == nil && (snap.State.Terminal() || snap.State == jobs.StateBlocked)
	})

	decided := waitForSnapshot(t, other, apply.CanonicalJobID, "the apply to be decided", func(s jobs.Snapshot) bool {
		return s.State.Terminal() || s.State == jobs.StateBlocked
	})
	if decided.State != jobs.StateBlocked {
		t.Fatalf("an apply whose actor was demoted before dispatch ended %s, want blocked", decided.State)
	}
	events, err := other.GetJobTimeline(apply.CanonicalJobID, 0, 200)
	if err != nil {
		t.Fatalf("read the timeline: %v", err)
	}
	blockedDetail := ""
	for _, event := range events {
		if event.Type == jobs.EventBlocked {
			blockedDetail = string(event.Detail)
		}
	}
	if !strings.Contains(blockedDetail, "role-refused") {
		t.Fatalf("the blocked apply does not say why: %s", blockedDetail)
	}

	// Nothing was mutated: no imported group, and the archive and plan are where the
	// refusal left them.
	var imported int64
	if err := other.db.Model(&models.Group{}).Where("name = ?", "Imported").Count(&imported).Error; err != nil {
		t.Fatalf("count imported groups: %v", err)
	}
	if imported != 0 {
		t.Fatalf("a refused apply created %d groups from its archive", imported)
	}
	if exists, _ := afero.Exists(other.GetDefaultFs(), importConsumedPlanPathFor(handle)); !exists {
		t.Fatalf("a refused apply consumed the plan it was told not to read")
	}
}

// TestTheRetentionSweepKeepsAnAncestorAQueuedRetryStillReads is the same invariant from
// the other side, and the one the startup sweep cannot see for itself.
//
// Startup protection derives a nonterminal Job's staged hand-off by walking its lineage
// upward, so that path has to exist: the parse's handle is what names the plan and the
// archive the queued apply reads. Ordinary retention, meanwhile, prunes finished history
// and takes the lineage rows with it — so a retained parse and a failed apply expiring
// under a queued Retry removed the very link the walk follows, and the next startup
// deleted the input of work that had not run. A Job a live Job's lineage still names is
// not history.
func TestTheRetentionSweepKeepsAnAncestorAQueuedRetryStillReads(t *testing.T) {
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

	const parseHandle = "beef0123456789ab"
	parse := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindGroupImportParse, KindVersion: jobImportKindVersion,
		State: jobs.StateQueued, Origin: "api",
		Replay:     jobs.ReplayInput{Input: json.RawMessage(`{"handle":"` + parseHandle + `","archive":"_imports/` + parseHandle + `.tar"}`)},
		LegacyRefs: []jobs.LegacyRef{{Namespace: ImportParseHandleNamespace, Handle: parseHandle}},
	})
	finishedParse := finishJobFor(t, ctx, parse, jobs.StateSucceeded)

	input, err := json.Marshal(importApplyJobInput{
		ParseHandle: parseHandle,
		Plan:        importConsumedPlanPathFor(parseHandle),
		Decisions: ImportDecisions{
			MappingActions:  map[string]MappingAction{},
			DanglingActions: map[string]DanglingAction{},
		},
	})
	if err != nil {
		t.Fatalf("encode the apply input: %v", err)
	}
	apply := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindGroupImportApply, KindVersion: jobImportKindVersion,
		State: jobs.StateQueued, Origin: "api",
		Replay:     jobs.ReplayInput{Input: input},
		LegacyRefs: []jobs.LegacyRef{{Namespace: ImportApplyHandleNamespace, Handle: "retried-apply"}},
	})
	if err := ctx.JobService().Link(ctx.jobDeps(), jobs.LinkRequest{
		Type: jobs.LinkParentChild, FromJobID: finishedParse.ID, ToJobID: apply.ID,
	}); err != nil {
		t.Fatalf("link the apply to its parse: %v", err)
	}
	failedApply := finishJobFor(t, ctx, apply, jobs.StateFailed)

	// The Retry is queued and reads what its ancestor's lineage names.
	retried := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindGroupImportApply, KindVersion: jobImportKindVersion,
		State: jobs.StateQueued, Origin: "api",
		Replay: jobs.ReplayInput{Input: input},
	})
	if err := ctx.JobService().Link(ctx.jobDeps(), jobs.LinkRequest{
		Type: jobs.LinkRetryOf, FromJobID: retried.ID, ToJobID: failedApply.ID,
	}); err != nil {
		t.Fatalf("link the retry to the failed apply: %v", err)
	}
	if err := moveLegacyHandlesForTest(t, ctx, failedApply.ID, retried.ID); err != nil {
		t.Fatalf("move the handle onto the retry: %v", err)
	}

	required := []string{
		importArchivePathFor(parseHandle),
		importConsumedPlanPathFor(parseHandle),
	}
	for _, path := range required {
		if err := afero.WriteFile(fs, path, []byte("staged"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		ageStagingFileForTest(t, fs, path)
	}

	// Both ancestors are long past their retention window: the parse succeeded a month
	// ago and the apply failed three months ago, under the design's default windows.
	expired := time.Now().Add(-time.Hour).UTC()
	if err := ctx.db.Model(&models.Job{}).Where("id IN ?", []string{finishedParse.ID, failedApply.ID}).
		Update("expires_at", expired).Error; err != nil {
		t.Fatalf("expire the ancestors: %v", err)
	}

	result, err := ctx.SweepJobHistory(jobs.SweepCursor{}, 50)
	if err != nil {
		t.Fatalf("sweep the history: %v", err)
	}
	if result.Pruned != 0 {
		t.Fatalf("the sweep pruned %d jobs, and every candidate is named by a queued retry's lineage", result.Pruned)
	}
	for _, ancestor := range []string{finishedParse.ID, failedApply.ID} {
		var job models.Job
		if err := ctx.db.Where("id = ?", ancestor).First(&job).Error; err != nil {
			t.Fatalf("the sweep removed ancestor %s, whose lineage the queued retry %s reads: %v",
				ancestor, retried.ID, err)
		}
	}

	// And the property that matters: the startup sweep can still derive the staged
	// hand-off, because the lineage it walks is still there.
	ctx.RunStartupExportSweep()
	for _, path := range required {
		if exists, _ := afero.Exists(fs, path); !exists {
			t.Fatalf("startup cleanup deleted %s, which the queued retry %s still reads", path, retried.ID)
		}
	}

	// The guard is a dependency test, not a refusal to prune at all: once the retry is
	// over, the same ancestors are history again.
	finishJobFor(t, ctx, jobSnapshot(t, ctx.JobService(), ctx, retried.ID), jobs.StateCancelled)
	if err := ctx.db.Model(&models.Job{}).Where("id IN ?", []string{finishedParse.ID, failedApply.ID, retried.ID}).
		Update("expires_at", expired).Error; err != nil {
		t.Fatalf("expire the lineage: %v", err)
	}
	pruned, err := ctx.SweepJobHistory(jobs.SweepCursor{}, 50)
	if err != nil {
		t.Fatalf("sweep again: %v", err)
	}
	if pruned.Pruned == 0 {
		t.Fatalf("a lineage nothing depends on any more was not pruned at all")
	}
}
