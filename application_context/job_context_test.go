package application_context

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"mahresources/auth"
	"mahresources/constants"
	"mahresources/jobs"
	"mahresources/models"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
)

// This file drives the application seam of the Job Center: the request-scoped
// context methods the HTTP and template layers call. They are tested through the
// same seam production uses — a real context with a real database, a bound
// principal, and the live settings service — because the properties worth
// pinning are about who is asking and what the deployment configured, not about
// how the query is assembled.

func jobUintPtr(v uint) *uint { return &v }

func jobBoolPtr(v bool) *bool { return &v }

// newJobContext builds a context whose database holds the durable job core and
// whose settings service is live, so a facade call reads the deployment's
// configuration the way a request does.
func newJobContext(t *testing.T) *MahresourcesContext {
	t.Helper()

	dsn := filepath.Join(t.TempDir(), "job-context.db")
	db, _, err := models.CreateDatabaseConnection(constants.DbTypeSqlite, dsn, "", 0)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("underlying database: %v", err)
	}
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(
		&models.Job{}, &models.JobEvent{}, &models.JobEventSequence{}, &models.JobLink{},
		&models.JobOutput{}, &models.JobReplayEnvelope{},
		&models.JobClaim{}, &models.JobCapacityLease{}, &models.JobPreference{}, &models.JobPinGuard{},
		&models.JobCommandRequest{}, &models.JobLegacyHandle{},
		&models.RuntimeSetting{}, &models.LogEntry{},
	); err != nil {
		t.Fatalf("migrate job core: %v", err)
	}

	cfg := &MahresourcesConfig{DbType: constants.DbTypeSqlite}
	ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, sqlx.NewDb(sqlDB, "sqlite3"), cfg)
	settings := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), BuildDefaultsFromConfig(cfg))
	if err := settings.Load(); err != nil {
		t.Fatalf("load settings: %v", err)
	}
	ctx.SetSettings(settings)
	// A replay keyring, because a Job accepted with input has to have somewhere to
	// seal it: the control plane refuses replayable input with no stable key rather
	// than storing it in the clear.
	ring, err := jobs.LoadReplayKeyring(jobs.ReplayKeyConfig{Dialect: constants.DbTypeSqlite, Ephemeral: true})
	if err != nil {
		t.Fatalf("build replay keyring: %v", err)
	}
	ctx.SetJobReplayKeyring(ring)
	ctx.SetJobService(jobs.NewService())
	return ctx
}

// facadeTestKind is the Kind the facade tests register their own adapter under.
// It is deliberately not one of the Kinds a real context registers: these tests are
// about the facade binding the asker, and a Kind the download adapter already owns
// would test that adapter instead.
const facadeTestKind = "facade-test-work"

// acceptJobFor accepts one Job through the installed control plane, as a Kind
// adapter would.
func acceptJobFor(t *testing.T, ctx *MahresourcesContext, acceptance jobs.Acceptance) jobs.Snapshot {
	t.Helper()
	snap, err := ctx.JobService().Accept(ctx.jobDeps(), acceptance)
	if err != nil {
		t.Fatalf("accept job: %v", err)
	}
	return snap
}

// registerClaimableKind teaches the context's control plane to run one Kind, so a
// test can walk a Job through the lifecycle the way a runtime does. A Kind that is
// already registered is left as it is: registering it again is what the second
// caller would be doing.
func registerClaimableKind(t *testing.T, ctx *MahresourcesContext, kind string, version uint) {
	t.Helper()
	if _, ok := ctx.JobService().AdapterFor(kind, version); ok {
		return
	}
	adapter := newRuntimeTestAdapter()
	adapter.def.Kind = kind
	adapter.def.KindVersion = version
	if err := ctx.JobService().RegisterAdapter(adapter); err != nil {
		t.Fatalf("register %s v%d: %v", kind, version, err)
	}
}

// finishJobFor walks one Job to a terminal state through the real lifecycle: a Job
// reaches running through a claim and nothing else, so the helper claims it — the
// Kind is registered for the test — and then ends it as the execution that owns it.
func finishJobFor(t *testing.T, ctx *MahresourcesContext, snap jobs.Snapshot, outcome jobs.State) jobs.Snapshot {
	t.Helper()
	deps := ctx.jobDeps()
	registerClaimableKind(t, ctx, snap.Kind, snap.KindVersion)

	execution, ok, err := ctx.JobService().Claim(context.Background(), deps, jobs.ClaimRequest{
		Kind: snap.Kind, KindVersion: snap.KindVersion, Claimant: "test-runtime",
	})
	if err != nil {
		t.Fatalf("claim %s: %v", snap.ID, err)
	}
	if !ok {
		t.Fatalf("the accepted Job %s was not claimable", snap.ID)
	}
	if execution.JobID != snap.ID {
		t.Fatalf("claimed %s while finishing %s", execution.JobID, snap.ID)
	}

	request := jobs.FinishRequest{
		ExecutionRef:    jobs.ExecutionRef{JobID: snap.ID, ExecutionToken: execution.ExecutionToken},
		ExpectedVersion: execution.Version,
		Outcome:         outcome,
	}
	if outcome == jobs.StateFailed {
		request.Failure = &jobs.Failure{Code: "boom", Class: jobs.FailureClassInternal}
	}
	finished, err := ctx.JobService().Finish(deps, request)
	if err != nil {
		t.Fatalf("finish as %s: %v", outcome, err)
	}
	return finished
}

// TestVisibilityFollowsTheBoundPrincipalAtTheFacade proves the facade asks the visibility
// question as the principal this context carries: an ordinary user sees their own
// owner-class work, the no-auth principal is the implicit administrator, and a
// Job the asker may not see is not-found rather than forbidden.
func TestVisibilityFollowsTheBoundPrincipalAtTheFacade(t *testing.T) {
	ctx := newJobContext(t)

	mine := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: "remote-download", KindVersion: 1, State: jobs.StateQueued, Origin: "api",
		OwnerUserID: jobUintPtr(7), Title: "mine",
		Replay: jobs.ReplayInput{NonReplayable: true},
	})
	adminOnly := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: "plugin-command", KindVersion: 1, State: jobs.StateQueued, Origin: "api",
		OwnerUserID: jobUintPtr(7), Title: "a command run", Visibility: jobs.VisibilityAdmin,
		Replay: jobs.ReplayInput{NonReplayable: true},
	})

	asUser := ctx.WithPrincipal(&auth.Principal{UserID: 7, Username: "user", Role: models.RoleUser})
	page, err := asUser.ListJobs(jobs.Filter{}, jobs.Cursor{}, 0)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(page.Jobs) != 1 || page.Jobs[0].ID != mine.ID {
		t.Fatalf("an ordinary user listed %d jobs, want their own", len(page.Jobs))
	}
	if _, err := asUser.GetJob(adminOnly.ID); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("an ordinary user read an admin-class job: %v", err)
	}
	if _, err := asUser.GetJobTimeline(mine.ID, 0, 0); err != nil {
		t.Fatalf("an ordinary user could not read their own timeline: %v", err)
	}

	// The no-auth principal is the implicit administrator, so a deployment
	// without accounts sees the whole Job Center — the same rule every other
	// surface follows when authentication is off.
	page, err = ctx.ListJobs(jobs.Filter{}, jobs.Cursor{}, 0)
	if err != nil {
		t.Fatalf("ListJobs without a principal: %v", err)
	}
	if len(page.Jobs) != 2 {
		t.Fatalf("an administrator listed %d jobs, want both", len(page.Jobs))
	}
	if _, err := ctx.GetJob(adminOnly.ID); err != nil {
		t.Fatalf("an administrator could not read an admin-class job: %v", err)
	}
}

// TestPreferenceAndRetentionFollowTheDeploymentSettings proves the facade rebuilds
// its per-call handle from the live settings: an operator's change applies to the
// next call without a restart, which is the whole reason the pin limit and the
// retention windows are runtime-editable.
func TestPreferenceAndRetentionFollowTheDeploymentSettings(t *testing.T) {
	ctx := newJobContext(t)

	if got := ctx.JobPinLimit(); got != jobs.DefaultPinLimit {
		t.Fatalf("default pin limit = %d, want %d", got, jobs.DefaultPinLimit)
	}
	if got := ctx.JobHistoryRetention(); got != jobs.DefaultHistoryRetention {
		t.Fatalf("default history retention = %v, want %v", got, jobs.DefaultHistoryRetention)
	}
	if got := ctx.JobAttentionRetention(); got != jobs.DefaultAttentionRetention {
		t.Fatalf("default attention retention = %v, want %v", got, jobs.DefaultAttentionRetention)
	}

	first := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: "remote-download", KindVersion: 1, State: jobs.StateQueued, Origin: "api",
		OwnerUserID: jobUintPtr(7), Title: "first",
		Replay: jobs.ReplayInput{NonReplayable: true},
	})
	second := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: "remote-download", KindVersion: 1, State: jobs.StateQueued, Origin: "api",
		OwnerUserID: jobUintPtr(7), Title: "second",
		Replay: jobs.ReplayInput{NonReplayable: true},
	})
	viewer := ctx.WithPrincipal(&auth.Principal{UserID: 7, Username: "user", Role: models.RoleUser})

	if err := ctx.settings.Set(KeyJobPinLimit, "1", "test", "127.0.0.1"); err != nil {
		t.Fatalf("set pin limit: %v", err)
	}
	if got := ctx.JobPinLimit(); got != 1 {
		t.Fatalf("pin limit after the override = %d, want 1", got)
	}
	pin := func(job jobs.Snapshot) error {
		return viewer.SetJobPreference(jobs.PreferenceRequest{JobID: job.ID, Pinned: jobBoolPtr(true)})
	}
	if err := pin(first); err != nil {
		t.Fatalf("pin the first job: %v", err)
	}
	if err := pin(second); !errors.Is(err, jobs.ErrPinLimitReached) {
		t.Fatalf("the second pin under a limit of 1 = %v, want ErrPinLimitReached", err)
	}

	// Raising it takes effect on the next call, with no restart and no
	// re-instantiation of the control plane.
	if err := ctx.settings.Set(KeyJobPinLimit, "5", "test", "127.0.0.1"); err != nil {
		t.Fatalf("raise pin limit: %v", err)
	}
	if err := pin(second); err != nil {
		t.Fatalf("pin under the raised limit: %v", err)
	}

	// The retention windows reach the lifecycle the same way: a Job that finishes
	// under a shortened history window carries that deadline, not the default.
	if err := ctx.settings.Set(KeyJobHistoryRetention, "1h", "test", "127.0.0.1"); err != nil {
		t.Fatalf("set history retention: %v", err)
	}
	if got := ctx.JobHistoryRetention(); got != time.Hour {
		t.Fatalf("history retention = %v, want 1h", got)
	}
	finished := finishJobFor(t, ctx, acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: "group-export", KindVersion: 1, State: jobs.StateQueued, Origin: "api", Title: "an export",
		Replay: jobs.ReplayInput{NonReplayable: true},
	}), jobs.StateSucceeded)
	if finished.ExpiresAt == nil {
		t.Fatal("a finished job carries no deadline")
	}
	if want := finished.FinishedAt.Add(time.Hour); !finished.ExpiresAt.Equal(want) {
		t.Fatalf("deadline = %v, want finished_at + the configured hour (%v)", finished.ExpiresAt, want)
	}
}

// TestCommandSurfaceFollowsTheBoundPrincipalAtTheFacade is the application seam's
// half of §8: the command surface asks as the principal this context carries,
// never as the one a request names, a Job the principal may not see is answered
// exactly as a Job that does not exist, and the same holds for the bulk surface
// where each Job is answered on its own.
func TestCommandSurfaceFollowsTheBoundPrincipalAtTheFacade(t *testing.T) {
	ctx := newJobContext(t)
	owner := uint(7)

	adapter := newRuntimeTestAdapter()
	adapter.def.Kind = facadeTestKind
	adapter.def.KindVersion = 1
	var askedAs []jobs.Access
	adapter.advertise = func(_ context.Context, commandContext jobs.CommandContext) ([]jobs.Command, error) {
		askedAs = append(askedAs, commandContext.Access)
		return []jobs.Command{{Key: jobs.CommandCancel, Label: "Cancel"}}, nil
	}
	if err := ctx.JobService().RegisterAdapter(adapter); err != nil {
		t.Fatalf("register the kind's adapter: %v", err)
	}

	mine := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: facadeTestKind, KindVersion: 1, State: jobs.StateQueued, Origin: "api",
		OwnerUserID: jobUintPtr(owner), ActorUserID: jobUintPtr(owner), Title: "mine",
		Replay: jobs.ReplayInput{NonReplayable: true},
	})
	execution, ok, err := ctx.JobService().Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: facadeTestKind, KindVersion: 1, Claimant: "facade-test",
	})
	if err != nil || !ok || execution.JobID != mine.ID {
		t.Fatalf("claiming the running job: %v (ok=%v, claimed %s)", err, ok, execution.JobID)
	}

	theirs := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: facadeTestKind, KindVersion: 1, State: jobs.StateQueued, Origin: "api",
		OwnerUserID: jobUintPtr(owner + 1), ActorUserID: jobUintPtr(owner + 1), Title: "theirs",
		Replay: jobs.ReplayInput{NonReplayable: true},
	})

	asUser := ctx.WithPrincipal(&auth.Principal{UserID: owner, Username: "user", Role: models.RoleUser})

	// A Job the principal may not see is not-found through every command surface.
	if _, err := asUser.AdvertisedJobCommands(context.Background(), theirs.ID); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("advertising a foreign job's commands = %v, want ErrNotFound", err)
	}
	if _, err := asUser.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
		JobID: theirs.ID, Key: jobs.CommandCancel, IdempotencyKey: "idem-foreign", ExpectedVersion: 1, Origin: "api",
	}); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("running a command on a foreign job = %v, want ErrNotFound", err)
	}

	// The actor is the context's principal, not the one the request names: a request
	// claiming to be an administrator still asks as this user.
	snapshot, err := asUser.GetJob(mine.ID)
	if err != nil {
		t.Fatalf("read the running job: %v", err)
	}
	result, err := asUser.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
		JobID: mine.ID, Key: jobs.CommandCancel, IdempotencyKey: "idem-facade",
		ExpectedVersion: snapshot.Version,
		Actor:           jobs.Access{UserID: owner + 1, Administrator: true},
		Origin:          "api",
	})
	if err != nil {
		t.Fatalf("running a command through the facade: %v", err)
	}
	if result.Code != jobs.CommandCodeRequested {
		t.Fatalf("the cancellation recorded %s/%s, want a durable request", result.Status, result.Code)
	}

	commands := adapter.commandExecutions()
	if len(commands) != 1 || commands[0].Access.UserID != owner {
		t.Fatalf("the adapter was asked to run %d commands with access %+v, want one as user %d",
			len(commands), commands, owner)
	}
	for _, access := range askedAs {
		if access.UserID != owner {
			t.Fatalf("the adapter was asked for a job's commands as %+v, want the bound principal", access)
		}
	}

	var recorded []models.JobCommandRequest
	if err := ctx.db.Where("job_id = ?", mine.ID).Find(&recorded).Error; err != nil {
		t.Fatalf("read the recorded command: %v", err)
	}
	if len(recorded) != 1 || recorded[0].ActorUserID != owner {
		t.Fatalf("the recorded command names actor %v, want the bound principal %d", recorded, owner)
	}

	// The bulk surface is the same rule per Job: the principal's own Job is acted on
	// and the foreign one is not found, with an outcome for each.
	results := asUser.ExecuteBulkJobCommand(context.Background(), jobs.BulkCommandRequest{
		JobIDs: []string{mine.ID, theirs.ID}, Key: jobs.CommandPin,
		IdempotencyKey: "idem-facade-bulk", Actor: jobs.Access{UserID: owner + 1}, Origin: "api",
	})
	if len(results) != 2 {
		t.Fatalf("the bulk command answered %d results, want one per job", len(results))
	}
	if results[0].Status != jobs.CommandStatusSucceeded || results[0].Code != jobs.CommandCodeApplied {
		t.Fatalf("the principal's own job = %s/%s, want it pinned", results[0].Status, results[0].Code)
	}
	if results[1].Code != jobs.CommandCodeNotFound {
		t.Fatalf("the foreign job = %s/%s, want not-found rather than a refusal that names it",
			results[1].Status, results[1].Code)
	}

	var preference models.JobPreference
	if err := ctx.db.Where("job_id = ?", mine.ID).First(&preference).Error; err != nil {
		t.Fatalf("read the preference: %v", err)
	}
	if preference.UserID != owner || preference.PinnedAt == nil {
		t.Fatalf("the pin = %+v, want it recorded for the bound principal %d", preference, owner)
	}
}

// TestRetentionSweepUsesTheFacadesConfiguredWindows proves the sweep is driven by the
// deployment's own windows through the facade: shortening the history retention
// expires finished work on the next pass, and the pass is bounded by the batch it
// is given.
func TestRetentionSweepUsesTheFacadesConfiguredWindows(t *testing.T) {
	ctx := newJobContext(t)

	kept := finishJobFor(t, ctx, acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: "group-export", KindVersion: 1, State: jobs.StateQueued, Origin: "api", Title: "kept",
		Replay: jobs.ReplayInput{NonReplayable: true},
	}), jobs.StateSucceeded)

	// Nothing is due: the default window is a month.
	result, err := ctx.SweepJobHistory(jobs.SweepCursor{}, 10)
	if err != nil {
		t.Fatalf("SweepJobHistory: %v", err)
	}
	if result.Pruned != 0 {
		t.Fatalf("the default window pruned %d jobs", result.Pruned)
	}

	// A Job whose own deadline has passed — the shape a shortened window produces
	// for work that already finished — is what the next pass takes.
	due := finishJobFor(t, ctx, acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: "group-export", KindVersion: 1, State: jobs.StateQueued, Origin: "api", Title: "due",
		Replay: jobs.ReplayInput{NonReplayable: true},
	}), jobs.StateSucceeded)
	if err := ctx.db.Model(&models.Job{}).Where("id = ?", due.ID).
		Update("expires_at", due.FinishedAt.Add(-time.Minute)).Error; err != nil {
		t.Fatalf("age the job's deadline: %v", err)
	}

	result, err = ctx.SweepJobHistory(jobs.SweepCursor{}, 10)
	if err != nil {
		t.Fatalf("SweepJobHistory: %v", err)
	}
	if result.Pruned != 1 {
		t.Fatalf("pruned %d jobs, want 1", result.Pruned)
	}
	if _, err := ctx.GetJob(due.ID); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("the expired job is still readable: %v", err)
	}
	if _, err := ctx.GetJob(kept.ID); err != nil {
		t.Fatalf("the job inside its window was pruned: %v", err)
	}
}
