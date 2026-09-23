//go:build postgres && json1 && fts5

package application_context

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"mahresources/archive"
	"mahresources/auth"
	"mahresources/constants"
	"mahresources/jobs"
	"mahresources/models"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	pgdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestGroupExportScopeAuthorizationBatchesOnPostgres(t *testing.T) {
	ctx, _, _ := newPostgresOwnershipFixture(t, 2)
	rootID := createExportGroupForTest(t, ctx, "pg-scoped-export-root")
	children := make([]models.Group, 600)
	for i := range children {
		children[i] = models.Group{Name: "pg-scoped-export-child", OwnerId: &rootID}
	}
	if err := ctx.db.CreateInBatches(&children, 200).Error; err != nil {
		t.Fatalf("create export descendants: %v", err)
	}
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
	groupIDs := make([]uint, 1+len(children))
	groupIDs[0] = rootID
	for i := range children {
		groupIDs[i+1] = children[i].ID
	}
	publishScopedGroupExportOutputForTest(t, ctx, job.ID, groupIDs, nil)
	scoped := ctx.WithPrincipal(&auth.Principal{
		UserID: 7, Role: models.RoleUser, ScopeGroupID: &rootID,
	})
	if outputs, err := scoped.GetOpenableJobOutputs(job.ID); err != nil || len(outputs) != 1 {
		t.Fatalf("large Postgres scoped export outputs = %d, err=%v; want one visible output", len(outputs), err)
	}
	content, err := scoped.OpenJobOutput(context.Background(), job.ID, jobExportArtifactOutput)
	if err != nil {
		t.Fatalf("open large Postgres scoped export: %v", err)
	}
	_ = content.Body.Close()
}

// The engine parity of the executor-ownership contract. What differs between SQLite
// and PostgreSQL here is the whole admission: PostgreSQL runs several writers, so the
// capacity budget is serialized with a transaction-scoped advisory lock and the claim
// row is taken with a conditional upsert, while SQLite relies on being the only writer.
// A claim that two runtimes can both take, or a capacity slot that two admissions can
// both occupy, would show on PostgreSQL and nowhere else.
//
// The SQLite arms live in job_queue_ownership_test.go; these drive the same seams over
// one PostgreSQL database with two application contexts, which is the shape a duplicate
// executor is observable in at all.
//
// The transfers of the SQLite file are replaced here by an export: what is under test
// is which process owns the Job, and a queue entry that writes an archive exercises
// exactly the same admission without a server standing in for the network.

func newPostgresOwnershipContext(t *testing.T, dsn string, key jobs.ReplayKey, filesystem afero.Fs, concurrency int) *MahresourcesContext {
	t.Helper()

	db, err := gorm.Open(pgdriver.Open(dsn), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Silent),
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open the process's database: %v", err)
	}
	readOnly, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("open read-only handle: %v", err)
	}
	t.Cleanup(func() { readOnly.Close() })

	ctx := NewMahresourcesContext(filesystem, db, readOnly, &MahresourcesConfig{
		DbType:            constants.DbTypePosgres,
		MaxJobConcurrency: concurrency,
	})
	holdJobReplayKey(t, ctx, key)
	ctx.SetJobService(jobs.NewService())
	return ctx
}

// newPostgresOwnershipFixture builds the two processes of one PostgreSQL deployment
// and the tables both of them touch, over one database and one filesystem.
func newPostgresOwnershipFixture(t *testing.T, concurrency int) (*MahresourcesContext, *MahresourcesContext, *JobRuntime) {
	t.Helper()

	db, dsn := pgContainer.CreateTestDBWithDSN(t)
	if err := db.AutoMigrate(
		&models.Resource{}, &models.ResourceVersion{}, &models.ResourceCategory{},
		&models.Series{}, &models.Tag{}, &models.Group{}, &models.Note{}, &models.NoteType{},
		&models.Category{}, &models.Preview{}, &models.ImageHash{}, &models.GroupRelation{},
		&models.GroupRelationType{}, &models.NoteBlock{}, &models.User{}, &models.LogEntry{},
		&models.PluginKV{}, &models.PluginState{}, &models.RuntimeSetting{},
		&models.DownloadHistoryEntry{}, &models.ScheduledDownload{},
		&models.Query{}, &models.SavedMRQLQuery{}, &models.SavedSearch{}, &models.UserSetting{},
		&models.Session{}, &models.ApiToken{}, &models.TemplatePartial{}, &models.ResourceSimilarity{},
		&models.PluginSchedule{}, &models.PluginCommandRun{}, &models.PluginCommandImport{},
		&models.ResourceReduction{},
		&models.Job{}, &models.JobResourceReceipt{}, &models.JobEvent{}, &models.JobEventSequence{}, &models.JobLink{},
		&models.JobOutput{}, &models.JobReplayEnvelope{}, &models.JobClaim{},
		&models.JobCapacityLease{}, &models.JobPreference{}, &models.JobPinGuard{},
		&models.JobCommandRequest{}, &models.JobLegacyHandle{}, &models.JobImportCommandFact{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	key := sharedReplayKey(t)
	filesystem := afero.NewMemMapFs()
	first := newPostgresOwnershipContext(t, dsn, key, filesystem, concurrency)
	other := newPostgresOwnershipContext(t, dsn, key, filesystem, concurrency)
	return first, other, NewJobRuntime(other, other.JobService(), JobRuntimeConfig{
		Claimant: "second-process", Interval: time.Hour,
	})
}

// TestAQueueBackedSubmissionIsOwnedAcrossProcessesOnPostgres drives the whole contract
// on the engine where several writers contend for it: a submission whose deployment
// budget is already full is accepted queued with no executor anywhere, the other
// process cannot start one either, and when the slot frees that other process claims
// the Job, starts the executor from the durable input alone and publishes the outcome.
//
// It ends on the second half of the same property: with the budget free, a submission
// is owned by the process that accepted it, and the other process's pass finds nothing
// of it to run.
func TestAQueueBackedSubmissionIsOwnedAcrossProcessesOnPostgres(t *testing.T) {
	first, other, otherRuntime := newPostgresOwnershipFixture(t, 1)
	groupID := createExportGroupForTest(t, first, "pg-owned-export")

	// The deployment is at its one slot, held by work that is nobody's submission.
	holder := holdTheDeploymentBudgetIn(t, first)

	submission := first.SubmitGroupExport(exportRequestForTest(groupID), "api")
	if submission.Err != nil {
		t.Fatalf("submit the export: %v", submission.Err)
	}
	if submission.CanonicalJobID == "" {
		t.Fatalf("the export created no durable job")
	}
	handle := submission.QueueJobID
	if handle == "" {
		t.Fatalf("the submission answered no id at all")
	}
	if handle == submission.CanonicalJobID {
		t.Fatalf("the submission answered the canonical id where a legacy handle belongs")
	}

	// Accepted, visible and controllable under the id the client was handed.
	queued, err := first.GetJob(submission.CanonicalJobID)
	if err != nil {
		t.Fatalf("read the accepted job: %v", err)
	}
	if queued.State != jobs.StateQueued {
		t.Fatalf("the export is %s while the deployment has no room for it, want queued", queued.State)
	}
	resolved, err := first.ResolveJobHandle(GroupExportHandleNamespace, handle)
	if err != nil {
		t.Fatalf("the answered id resolves to no job: %v", err)
	}
	if resolved.ID != queued.ID {
		t.Fatalf("the answered id resolves to %s, want %s", resolved.ID, queued.ID)
	}
	if _, found := first.DownloadManager().GetJob(handle); found {
		t.Fatalf("the accepted export started a queue entry it had no capacity to run")
	}
	if _, found := other.DownloadManager().GetJob(handle); found {
		t.Fatalf("another process started an executor for work with no capacity to admit it")
	}

	// The other process's pass has nothing to run either: every claim asks for the same
	// budget, and the budget is what is full.
	otherRuntime.tick(context.Background())
	if entries := other.DownloadManager().GetJobs(); len(entries) != 0 {
		t.Fatalf("a full budget admitted %d executors", len(entries))
	}

	// The slot frees, and the next process with room runs it — from the durable input
	// alone, under the id the client was handed.
	if _, err := first.JobService().ReleaseClaim(first.jobDeps(), jobs.ReleaseRequest{
		ExecutionRef: jobs.ExecutionRef{JobID: holder.JobID, ExecutionToken: holder.ExecutionToken},
		// Left queued: what this test needs is the slot, and a holder Job nothing
		// dispatches is free to wait for one it will never get.
		To:     jobs.StateQueued,
		Reason: "test released the budget",
	}); err != nil {
		t.Fatalf("release the budget holder: %v", err)
	}
	otherRuntime.tick(context.Background())

	waitFor(t, "the queued export's executor to appear", func() bool {
		entry, found := other.DownloadManager().GetJob(handle)
		return found && entry.CanonicalJobID == queued.ID
	})
	finished := waitForSnapshot(t, other, queued.ID, "the queued export to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if finished.State != jobs.StateSucceeded {
		t.Fatalf("the queued export ended %s (%+v)", finished.State, finished.Failure)
	}
	outputs, err := other.GetJobOutputs(finished.ID)
	if err != nil {
		t.Fatalf("read the outputs: %v", err)
	}
	if _, published := findJobOutput(outputs, jobExportArtifactOutput); !published {
		t.Fatalf("a succeeded export published no archive to hand over: %+v", outputs)
	}
	if claim := storedClaim(t, other, finished.ID); claim.State != models.JobClaimStateReleased {
		t.Fatalf("the claim is %s after the export ended, want released", claim.State)
	}
	if held := storedCapacity(t, other, jobs.CapacityGroupGlobal); held != 0 {
		t.Fatalf("the deployment budget still holds %d slots after the export ended", held)
	}

	// With room again, a submission is owned by the process that accepted it: the other
	// process's pass finds nothing of it, and its outcome is the submitter's to publish.
	owned := first.SubmitGroupExport(exportRequestForTest(groupID), "api")
	if owned.Err != nil {
		t.Fatalf("submit the second export: %v", owned.Err)
	}
	if owned.CanonicalJobID == "" {
		t.Fatalf("the second export created no durable job")
	}
	ownedFinished := waitForSnapshot(t, first, owned.CanonicalJobID, "the owned export to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if ownedFinished.State != jobs.StateSucceeded {
		t.Fatalf("the owned export ended %s (%+v)", ownedFinished.State, ownedFinished.Failure)
	}
	otherRuntime.tick(context.Background())
	if _, found := other.DownloadManager().GetJob(owned.QueueJobID); found {
		t.Fatalf("another process started an executor for work the submitter already owned")
	}
	if held := storedCapacity(t, other, jobs.CapacityGroupGlobal); held != 0 {
		t.Fatalf("a finished export left %d capacity slots held", held)
	}
}
