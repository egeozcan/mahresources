package jobs

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/types"

	"gorm.io/gorm"
)

// uuidV7Pattern is the identity contract: an opaque, time-ordered UUIDv7 that
// encodes no kind, owner, state or authorization decision.
var uuidV7Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// jobCoreTables is every table one test database for the durable job core needs,
// in the order they are migrated. It is the whole core rather than the subset a
// single test happens to write: a statement against a table that does not exist
// is an error rather than a missing match, and on PostgreSQL it aborts the
// surrounding transaction — which is exactly what Accept's post-commit
// availability read did to TestJobPublishOrdersOutOfOrderCommitsPG once replay
// envelopes arrived.
func jobCoreTables() []any {
	return []any{
		&models.Job{}, &models.JobEvent{}, &models.JobEventSequence{}, &models.JobLink{},
		&models.JobOutput{}, &models.JobReplayEnvelope{},
		&models.JobClaim{}, &models.JobCapacityLease{},
		&models.JobPreference{},
		&models.JobPinGuard{},
	}
}

// newTestDeps opens a real file-backed SQLite database through the production
// driver configuration — the same PRAGMAs (WAL, foreign keys, busy timeout) a
// deployment runs with — and migrates the durable job core plus one domain model
// used to prove a Job commits and rolls back with its caller's transaction.
//
// A file rather than :memory: because two connections must see the same
// database: a transaction on one connection and a read on another is exactly
// what the acceptance and publisher tests exercise.
func newTestDeps(t *testing.T) Deps {
	t.Helper()
	deps, _ := newFileDeps(t)
	return deps
}

// newFileDeps is newTestDeps plus the DSN it opened, so a test that needs a
// second connection — an interleaving driven from another transaction, which is
// what makes it an interleaving rather than a sequence — can open one against
// the very same file.
func newFileDeps(t *testing.T) (Deps, string) {
	t.Helper()

	dsn := filepath.Join(t.TempDir(), "jobs.db")
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

	if err := db.AutoMigrate(append(jobCoreTables(), &models.PluginKV{})...); err != nil {
		t.Fatalf("migrate job core: %v", err)
	}
	return Deps{DB: db}, dsn
}

// openSecondHandle opens another connection to one test database. It is how a
// test performs a write in the middle of another transaction's statement.
func openSecondHandle(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, _, err := models.CreateDatabaseConnection(constants.DbTypeSqlite, dsn, "", 0)
	if err != nil {
		t.Fatalf("open second handle: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("underlying second handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(2)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func uintPtr(v uint) *uint { return &v }

func int64Ptr(v int64) *int64 { return &v }

// jobRow reads the stored row directly. Assertions about stored columns are the
// point of a persistence test; the Service's own reads are asserted separately.
func jobRow(t *testing.T, deps Deps, id string) models.Job {
	t.Helper()
	var job models.Job
	if err := deps.DB.Where("id = ?", id).First(&job).Error; err != nil {
		t.Fatalf("load job %s: %v", id, err)
	}
	return job
}

func jobEvents(t *testing.T, deps Deps, id string) []models.JobEvent {
	t.Helper()
	var events []models.JobEvent
	if err := deps.DB.Where("job_id = ?", id).Order("sequence").Find(&events).Error; err != nil {
		t.Fatalf("load events for %s: %v", id, err)
	}
	return events
}

// eventsOfType filters a timeline or a delivered page down to one event type.
// Assertions about output facts come as pairs — the publication, the expiry, the
// removal — so they read the type rather than a position in the list.
func eventsOfType(events []Event, eventType string) []Event {
	matched := make([]Event, 0, len(events))
	for _, event := range events {
		if event.Type == eventType {
			matched = append(matched, event)
		}
	}
	return matched
}

func TestJobAcceptStoresUUIDv7IdentityAndAcceptedEventAtomically(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	accepted := time.Date(2031, 3, 4, 5, 6, 7, 0, time.UTC)
	deps.Now = func() time.Time { return accepted }

	snap, err := svc.Accept(deps, Acceptance{
		Kind:        "remote-download",
		KindVersion: 1,
		State:       StateQueued,
		OwnerUserID: uintPtr(7),
		ActorUserID: uintPtr(7),
		Origin:      "api",
		Title:       "an example download",
		Summary:     json.RawMessage(`{"scheme":"https","host":"example.test"}`),
		Replay:      ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}

	if !uuidV7Pattern.MatchString(snap.ID) {
		t.Errorf("job id %q is not a UUIDv7", snap.ID)
	}
	if snap.State != StateQueued || snap.Version != 1 {
		t.Errorf("accepted snapshot = %s v%d, want queued v1", snap.State, snap.Version)
	}
	if !snap.AcceptedAt.Equal(accepted) || snap.AcceptedAt.Location() != time.UTC {
		t.Errorf("AcceptedAt = %v (%v), want %v in UTC", snap.AcceptedAt, snap.AcceptedAt.Location(), accepted)
	}
	if snap.Terminal() {
		t.Errorf("a queued job is not terminal")
	}
	if snap.Visibility != VisibilityOwner {
		t.Errorf("visibility = %q, want %q for a submission that named no class", snap.Visibility, VisibilityOwner)
	}
	if string(snap.Summary) != `{"scheme":"https","host":"example.test"}` {
		t.Errorf("summary = %s", snap.Summary)
	}

	job := jobRow(t, deps, snap.ID)
	if job.Kind != "remote-download" || job.KindVersion != 1 || job.State != string(StateQueued) {
		t.Errorf("stored job = %+v", job)
	}
	if job.AcceptedAt.Location() != time.UTC || !job.AcceptedAt.Equal(accepted) {
		t.Errorf("stored AcceptedAt = %v (%v)", job.AcceptedAt, job.AcceptedAt.Location())
	}
	if job.CreatedAt.Location() != time.UTC || job.UpdatedAt.Location() != time.UTC {
		t.Errorf("stored CreatedAt/UpdatedAt are %v/%v, want UTC", job.CreatedAt.Location(), job.UpdatedAt.Location())
	}
	if job.StateEnteredAt == nil || !job.StateEnteredAt.Equal(accepted) {
		t.Errorf("stored StateEnteredAt = %v, want %v", job.StateEnteredAt, accepted)
	}
	if job.QueuedAt == nil || !job.QueuedAt.Equal(accepted) {
		t.Errorf("stored QueuedAt = %v, want the acceptance instant %v", job.QueuedAt, accepted)
	}
	// The acceptance declared its input non-replayable, which is the only way a
	// Job is stored without an envelope and the only way a caller may name the
	// summary itself: a replayable Job's summary comes from its Kind's
	// sanitizer.
	if job.ReplayClass != string(ReplayClassNonReplayable) {
		t.Errorf("stored ReplayClass = %q, want %q", job.ReplayClass, ReplayClassNonReplayable)
	}
	if job.Summary == nil {
		t.Error("stored summary is empty")
	}

	events := jobEvents(t, deps, snap.ID)
	if len(events) != 1 {
		t.Fatalf("recorded %d events, want exactly the accepted event", len(events))
	}
	event := events[0]
	if event.Type != EventAccepted || event.Sequence != 1 || event.JobVersion != 1 {
		t.Errorf("accepted event = %+v", event)
	}
	if !event.ReservedHost {
		t.Error("the accepted event must be a reserved host event")
	}
	if !event.CreatedAt.Equal(accepted) {
		t.Errorf("event CreatedAt = %v, want %v", event.CreatedAt, accepted)
	}
	if event.DeliverySequence != nil {
		t.Errorf("acceptance must not allocate a delivery sequence, got %d", *event.DeliverySequence)
	}
}

func TestJobAcceptRefusesMalformedAcceptance(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	now := time.Date(2031, 3, 4, 5, 6, 7, 0, time.UTC)
	deps.Now = func() time.Time { return now }

	scheduledFor := now.Add(time.Hour)
	valid := Acceptance{
		Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "ui",
		Replay: ReplayInput{NonReplayable: true},
	}

	tests := []struct {
		name       string
		mutate     func(*Acceptance)
		wantTarget error
	}{
		{"no kind", func(a *Acceptance) { a.Kind = "" }, ErrInvalidAcceptance},
		{"kind over its ceiling", func(a *Acceptance) { a.Kind = string(make([]byte, MaxKindBytes+1)) }, ErrInvalidAcceptance},
		{"no kind version", func(a *Acceptance) { a.KindVersion = 0 }, ErrInvalidAcceptance},
		{"no origin", func(a *Acceptance) { a.Origin = "" }, ErrInvalidAcceptance},
		{"origin over its ceiling", func(a *Acceptance) { a.Origin = string(make([]byte, MaxOriginBytes+1)) }, ErrInvalidAcceptance},
		{"unknown state", func(a *Acceptance) { a.State = "starting" }, ErrUnknownState},
		{"terminal initial state", func(a *Acceptance) { a.State = StateSucceeded }, ErrInvalidAcceptance},
		{"scheduled without a time", func(a *Acceptance) { a.State = StateScheduled }, ErrInvalidAcceptance},
		{"scheduled with a time", func(a *Acceptance) {
			a.State = StateScheduled
			a.ScheduledFor = &scheduledFor
		}, nil},
		{"summary over its ceiling", func(a *Acceptance) {
			a.Summary = json.RawMessage(`"` + string(make([]byte, MaxSummaryBytes+1)) + `"`)
		}, ErrInvalidAcceptance},
		{"malformed summary", func(a *Acceptance) { a.Summary = json.RawMessage(`{"unterminated"`) }, ErrInvalidAcceptance},
		{"title over its ceiling", func(a *Acceptance) { a.Title = string(make([]byte, MaxTitleBytes+1)) }, ErrInvalidAcceptance},
		{"unknown visibility class", func(a *Acceptance) { a.Visibility = "public" }, ErrInvalidAcceptance},
		{"admin visibility class", func(a *Acceptance) { a.Visibility = VisibilityAdmin }, nil},
		{"legacy ref without a namespace", func(a *Acceptance) {
			a.LegacyRefs = []LegacyRef{{Handle: "17"}}
		}, ErrInvalidAcceptance},
		{"duplicate legacy refs", func(a *Acceptance) {
			a.LegacyRefs = []LegacyRef{{Namespace: "download", Handle: "17"}, {Namespace: "download", Handle: "17"}}
		}, ErrInvalidAcceptance},
		{"distinct legacy refs", func(a *Acceptance) {
			a.LegacyRefs = []LegacyRef{{Namespace: "download", Handle: "17"}, {Namespace: "action", Handle: "17"}}
		}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			acceptance := valid
			tt.mutate(&acceptance)

			var before int64
			deps.DB.Model(&models.Job{}).Count(&before)

			snap, err := svc.Accept(deps, acceptance)
			switch {
			case tt.wantTarget == nil:
				if err != nil {
					t.Fatalf("Accept: %v", err)
				}
				if snap.ID == "" {
					t.Fatal("accepted snapshot has no identity")
				}
			case !errors.Is(err, tt.wantTarget):
				t.Fatalf("Accept error = %v, want %v", err, tt.wantTarget)
			}

			var after int64
			deps.DB.Model(&models.Job{}).Count(&after)
			if tt.wantTarget != nil && after != before {
				t.Fatalf("a refused acceptance stored %d job rows", after-before)
			}
		})
	}
}

func TestJobAcceptRollsBackWithTheCallersTransaction(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()

	rollback := errors.New("domain write failed")
	err := deps.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&models.PluginKV{PluginName: "p", Key: "k", Value: "v"}).Error; err != nil {
			return err
		}
		if _, err := svc.Accept(Deps{DB: tx}, Acceptance{
			Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "ui",
			Replay: ReplayInput{NonReplayable: true},
		}); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("transaction error = %v, want %v", err, rollback)
	}

	var jobs, kvs, events int64
	deps.DB.Model(&models.Job{}).Count(&jobs)
	deps.DB.Model(&models.JobEvent{}).Count(&events)
	deps.DB.Model(&models.PluginKV{}).Count(&kvs)
	if jobs != 0 || events != 0 || kvs != 0 {
		t.Fatalf("rollback left %d jobs, %d events, %d domain rows behind", jobs, events, kvs)
	}
}

// TestJobAcceptUsesTheDeclaredTables is a guard on the storage contract itself:
// the durable core must migrate on both supported databases, so the tables and
// authorization-relevant columns the Service writes are the ones the models
// declare.
func TestJobAcceptUsesTheDeclaredTables(t *testing.T) {
	deps := newTestDeps(t)
	migrator := deps.DB.Migrator()
	for _, table := range []string{"jobs", "job_events", "job_event_sequences", "job_links", "job_outputs"} {
		if !migrator.HasTable(table) {
			t.Errorf("table %s was not created by AutoMigrate", table)
		}
	}
	for _, column := range []string{
		"visibility_class", "owner_user_id", "actor_user_id", "state", "accepted_at",
		"version", "execution_token", "replay_class", "finished_at",
		"progress_completed", "progress_total", "progress_unit", "progress_message", "progress_eta",
	} {
		if !migrator.HasColumn(&models.Job{}, column) {
			t.Errorf("jobs.%s is missing from the durable core", column)
		}
	}
	for _, column := range []string{
		"key", "type", "label", "reference", "required", "availability",
		"expires_at", "removed_at", "version", "job_id",
	} {
		if !migrator.HasColumn(&models.JobOutput{}, column) {
			t.Errorf("job_outputs.%s is missing from the durable core", column)
		}
	}
}

// TestJobRunningAdmissionIsTheClaimProtocol pins the one door into running.
//
// §3 makes the claim atomic with the state: the row moves to running, the fencing
// token, the claim and the capacity that admitted it are written together, so a
// running Job is always one an execution owns and one a reconciliation can find.
// A transition that entered running on its own would leave a Job that is neither
// claimable — Claim takes queued or scheduled work — nor reconciled, because the
// expiry scan looks for held claims: work stranded with nobody able to pick it up,
// across a restart or not.
func TestJobRunningAdmissionIsTheClaimProtocol(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 5, 7, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }
	registerTestAdapter(t, svc, testDefinition())

	for _, from := range []State{StateScheduled, StateQueued, StatePaused, StateRunning} {
		t.Run(string(from), func(t *testing.T) {
			job := seedJob(t, deps, from, clock, 1)

			_, err := svc.Transition(deps, Transition{JobID: job.ID, ExpectedVersion: 1, To: StateRunning})
			if !errors.Is(err, ErrRunningRequiresClaim) {
				t.Fatalf("transition %s -> running = %v, want ErrRunningRequiresClaim", from, err)
			}
			stored := jobRow(t, deps, job.ID)
			if stored.State != string(from) || stored.Version != 1 || stored.ExecutionToken != "" {
				t.Fatalf("a refused admission wrote %s v%d token %q", stored.State, stored.Version, stored.ExecutionToken)
			}
			if events := jobEvents(t, deps, job.ID); len(events) != 0 {
				t.Fatalf("a refused admission recorded %d events", len(events))
			}
		})
	}

	// The protocol itself is what admits work: a queued Job reaches running with
	// the token, the claim and the capacity a reconciliation and a release are
	// about, in one write.
	accepted := acceptQueued(t, svc, deps, nil)
	execution, ok := claimOnce(t, svc, deps, "runtime-a")
	if !ok {
		t.Fatal("the queued Job was not claimed")
	}
	if execution.JobID != accepted.ID {
		t.Fatalf("claimed %s, want the queued Job %s", execution.JobID, accepted.ID)
	}
	stored := jobRow(t, deps, accepted.ID)
	if stored.State != string(StateRunning) || stored.ExecutionToken != execution.ExecutionToken {
		t.Fatalf("claimed Job = %s token %q, want running under the claim's token", stored.State, stored.ExecutionToken)
	}
	if claim := claimRow(t, deps, accepted.ID); claim.State != models.JobClaimStateHeld {
		t.Fatalf("claimed Job's claim = %s, want held", claim.State)
	}
	if leases := capacityRows(t, deps, accepted.ID); len(leases) != 1 {
		t.Fatalf("claimed Job holds %d capacity leases, want the one that admitted it", len(leases))
	}
}

func TestJobStateVocabularyIsClosed(t *testing.T) {
	if len(AllStates) != 9 {
		t.Fatalf("AllStates has %d entries, want the nine normalized states", len(AllStates))
	}
	for _, state := range AllStates {
		if !state.Valid() {
			t.Errorf("state %q is listed but not Valid", state)
		}
	}
	if State("starting").Valid() {
		t.Error("an unlisted state must not be valid")
	}
	for _, terminal := range []State{StateSucceeded, StateFailed, StateCancelled, StateInterrupted} {
		if !terminal.Terminal() {
			t.Errorf("%q must be terminal", terminal)
		}
	}
	for _, nonterminal := range []State{StateScheduled, StateQueued, StateRunning, StatePaused, StateBlocked} {
		if nonterminal.Terminal() {
			t.Errorf("%q must not be terminal", nonterminal)
		}
	}
}

// legalTransitionSpec is the state machine as the design states it, written out
// independently of the implementation so the two have to agree. Terminal states
// appear with empty targets: a terminal Job never reopens, and continuation is a
// new Job. Running is a target of nothing — it is what a claim writes, and
// TestJobRunningAdmissionIsTheClaimProtocol is the test that says so.
var legalTransitionSpec = map[State][]State{
	StateScheduled:   {StateQueued, StateBlocked, StateCancelled, StateFailed, StateInterrupted},
	StateQueued:      {StateBlocked, StateCancelled, StateFailed, StateInterrupted},
	StateRunning:     {StateQueued, StatePaused, StateBlocked, StateSucceeded, StateFailed, StateCancelled, StateInterrupted},
	StatePaused:      {StateQueued, StateBlocked, StateCancelled, StateFailed, StateInterrupted},
	StateBlocked:     {StateQueued, StateCancelled, StateFailed, StateInterrupted},
	StateSucceeded:   {},
	StateFailed:      {},
	StateCancelled:   {},
	StateInterrupted: {},
}

func specAllows(from, to State) bool {
	for _, candidate := range legalTransitionSpec[from] {
		if candidate == to {
			return true
		}
	}
	return false
}

// seedJob writes one Job row directly in a state, which is setup rather than a
// behavior under test: the state machine's own reachability is covered by the
// table test below.
func seedJob(t *testing.T, deps Deps, state State, enteredAt time.Time, version uint64) models.Job {
	t.Helper()
	started := enteredAt.Add(-time.Minute)
	job := models.Job{
		ID:              types.NewUUIDv7(),
		Kind:            "group-export",
		KindVersion:     1,
		State:           string(state),
		Origin:          "ui",
		VisibilityClass: string(VisibilityOwner),
		ReplayClass:     string(ReplayClassReplayable),
		Version:         version,
		AcceptedAt:      enteredAt.Add(-time.Hour),
		StateEnteredAt:  &enteredAt,
	}
	if state == StateRunning || state == StatePaused || state == StateBlocked {
		job.StartedAt = &started
	}
	if err := deps.DB.Create(&job).Error; err != nil {
		t.Fatalf("seed job in %s: %v", state, err)
	}
	return job
}

func TestJobTransitionAppliesExactlyTheLegalTransitions(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }
	enteredAt := clock.Add(-5 * time.Minute)

	for _, from := range AllStates {
		for _, to := range AllStates {
			t.Run(string(from)+"_to_"+string(to), func(t *testing.T) {
				job := seedJob(t, deps, from, enteredAt, 1)

				// A failed transition always carries a failure; the state
				// machine is what this table is about, so the request is
				// otherwise well-formed.
				request := Transition{JobID: job.ID, ExpectedVersion: 1, To: to}
				if to == StateFailed {
					request.Failure = &Failure{Code: "test-failure", Class: FailureClassInternal}
				}

				snap, err := svc.Transition(deps, request)

				stored := jobRow(t, deps, job.ID)
				events := jobEvents(t, deps, job.ID)
				// Running is refused whatever the Job is in now, and by its own rule:
				// a claim is what admits it.
				if to == StateRunning {
					if !errors.Is(err, ErrRunningRequiresClaim) {
						t.Fatalf("transition %s -> running = %v, want ErrRunningRequiresClaim", from, err)
					}
					if stored.State != string(from) || stored.Version != 1 {
						t.Fatalf("a refused admission changed the row to %s v%d", stored.State, stored.Version)
					}
					if len(events) != 0 {
						t.Fatalf("a refused admission recorded %d events", len(events))
					}
					return
				}
				if !specAllows(from, to) {
					if !errors.Is(err, ErrIllegalTransition) {
						t.Fatalf("transition %s -> %s = %v, want ErrIllegalTransition", from, to, err)
					}
					if stored.State != string(from) || stored.Version != 1 {
						t.Fatalf("a refused transition changed the row to %s v%d", stored.State, stored.Version)
					}
					if len(events) != 0 {
						t.Fatalf("a refused transition recorded %d events", len(events))
					}
					return
				}

				if err != nil {
					t.Fatalf("transition %s -> %s: %v", from, to, err)
				}
				if snap.State != to || snap.Version != 2 {
					t.Fatalf("snapshot = %s v%d, want %s v2", snap.State, snap.Version, to)
				}
				if snap.Terminal() != to.Terminal() {
					t.Fatalf("snapshot.Terminal() = %v for state %s", snap.Terminal(), snap.State)
				}
				if stored.State != string(to) || stored.Version != 2 {
					t.Fatalf("stored row = %s v%d, want %s v2", stored.State, stored.Version, to)
				}
				if stored.StateEnteredAt == nil || !stored.StateEnteredAt.Equal(clock) {
					t.Fatalf("state entered at %v, want the injected clock %v", stored.StateEnteredAt, clock)
				}
				if len(events) != 1 {
					t.Fatalf("recorded %d events, want one", len(events))
				}
				event := events[0]
				if want := wantEventType(job, to); event.Type != want {
					t.Errorf("event type = %q, want %q", event.Type, want)
				}
				if event.Sequence != 1 || event.JobVersion != 2 {
					t.Errorf("event = sequence %d, job version %d, want 1 and 2", event.Sequence, event.JobVersion)
				}
				if !event.ReservedHost {
					t.Error("a lifecycle event must be a reserved host event")
				}
				if !event.CreatedAt.Equal(clock) {
					t.Errorf("event created at %v, want %v", event.CreatedAt, clock)
				}
				if to.Terminal() && (stored.FinishedAt == nil || !stored.FinishedAt.Equal(clock)) {
					t.Errorf("terminal transition left FinishedAt = %v, want %v", stored.FinishedAt, clock)
				}
				if !to.Terminal() && stored.FinishedAt != nil {
					t.Errorf("nonterminal transition set FinishedAt = %v", stored.FinishedAt)
				}
			})
		}
	}
}

// wantEventType is the lifecycle event name the transition should record: the
// state it enters, except that entering running is "started" the first time and
// "resumed" afterwards.
func wantEventType(job models.Job, to State) string {
	if to == StateRunning {
		if job.StartedAt == nil {
			return EventStarted
		}
		return EventResumed
	}
	return string(to)
}

func TestJobTransitionAccumulatesDurationsAndStartTimestamps(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }
	registerTestAdapter(t, svc, testDefinition())

	// The Job reaches running the only way a Job reaches running: a claim, which
	// names the execution every write from here on is fenced by.
	accepted := acceptQueued(t, svc, deps, nil)
	first, ok := claimOnce(t, svc, deps, "runtime-a")
	if !ok {
		t.Fatal("the queued Job was not claimed")
	}
	if err := deps.DB.Model(&models.Job{}).Where("id = ?", accepted.ID).
		Update("running_duration", 90*time.Second).Error; err != nil {
		t.Fatalf("seed running duration: %v", err)
	}
	started := jobRow(t, deps, accepted.ID).StartedAt

	// running -> paused: the elapsed running time is banked, and the pause
	// begins at the injected clock rather than at wall-clock now.
	clock = clock.Add(10 * time.Second)
	if _, err := svc.Transition(deps, Transition{
		JobID: accepted.ID, ExpectedVersion: first.Version, ExecutionToken: first.ExecutionToken, To: StatePaused,
	}); err != nil {
		t.Fatalf("running -> paused: %v", err)
	}
	paused := jobRow(t, deps, accepted.ID)
	if paused.RunningDuration != 100*time.Second {
		t.Errorf("RunningDuration = %v, want 100s", paused.RunningDuration)
	}
	if paused.PausedDuration != 0 {
		t.Errorf("PausedDuration = %v before any pause elapsed", paused.PausedDuration)
	}

	clock = clock.Add(25 * time.Second)
	if _, err := svc.Transition(deps, Transition{
		JobID: accepted.ID, ExpectedVersion: paused.Version, To: StateQueued,
	}); err != nil {
		t.Fatalf("paused -> queued: %v", err)
	}
	queued := jobRow(t, deps, accepted.ID)
	if queued.PausedDuration != 25*time.Second {
		t.Errorf("PausedDuration = %v, want 25s", queued.PausedDuration)
	}
	if queued.QueuedAt == nil || accepted.QueuedAt == nil || !queued.QueuedAt.Equal(*accepted.QueuedAt) {
		t.Errorf("QueuedAt = %v, want the first instant the Job was queueable (%v)", queued.QueuedAt, accepted.QueuedAt)
	}
	if queued.StartedAt == nil || started == nil || !queued.StartedAt.Equal(*started) {
		t.Errorf("StartedAt = %v, want the first start %v kept", queued.StartedAt, started)
	}
	if queued.ExecutionToken != "" {
		t.Errorf("a Job that left running still holds the token %q", queued.ExecutionToken)
	}

	// A queued Job is claimed again rather than transitioned into running: the
	// second execution is what resumes it, and the queue wait it banked is the
	// gap between leaving running and being admitted once more.
	clock = clock.Add(6 * time.Second)
	if _, ok := claimOnce(t, svc, deps, "runtime-b"); !ok {
		t.Fatal("the requeued Job was not claimed again")
	}
	resumed := jobRow(t, deps, accepted.ID)
	if resumed.QueueDuration != 6*time.Second {
		t.Errorf("QueueDuration = %v, want 6s", resumed.QueueDuration)
	}
	if resumed.LastResumedAt == nil || !resumed.LastResumedAt.Equal(clock) {
		t.Errorf("LastResumedAt = %v, want %v", resumed.LastResumedAt, clock)
	}
	if resumed.StartedAt == nil || started == nil || !resumed.StartedAt.Equal(*started) {
		t.Errorf("StartedAt = %v, want the first start %v", resumed.StartedAt, started)
	}
}

func TestJobTransitionVersionConflictWritesNothing(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	job := seedJob(t, deps, StateRunning, clock, 4)

	if _, err := svc.Transition(deps, Transition{JobID: job.ID, ExpectedVersion: 3, To: StateSucceeded}); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale version = %v, want ErrVersionConflict", err)
	}
	stored := jobRow(t, deps, job.ID)
	if stored.Version != 4 || stored.State != string(StateRunning) {
		t.Fatalf("a refused transition wrote %s v%d", stored.State, stored.Version)
	}
	if events := jobEvents(t, deps, job.ID); len(events) != 0 {
		t.Fatalf("a refused transition recorded %d events", len(events))
	}

	// The winner of the race is the one whose version still matches: a second
	// transition at the now-stale version 4 is refused after the first commits
	// version 5.
	if _, err := svc.Transition(deps, Transition{JobID: job.ID, ExpectedVersion: 4, To: StateSucceeded}); err != nil {
		t.Fatalf("first transition: %v", err)
	}
	if _, err := svc.Transition(deps, Transition{JobID: job.ID, ExpectedVersion: 4, To: StateFailed, Failure: &Failure{Code: "x", Class: FailureClassInternal}}); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("second transition at a consumed version = %v, want ErrVersionConflict", err)
	}
	if events := jobEvents(t, deps, job.ID); len(events) != 1 {
		t.Fatalf("recorded %d events, want exactly the winner's", len(events))
	}
}

func TestJobTerminalStateCannotChange(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	for _, terminal := range []State{StateSucceeded, StateFailed, StateCancelled, StateInterrupted} {
		job := seedJob(t, deps, terminal, clock, 9)
		for _, to := range AllStates {
			request := Transition{JobID: job.ID, ExpectedVersion: 9, To: to}
			if to == StateFailed {
				request.Failure = &Failure{Code: "test-failure", Class: FailureClassInternal}
			}
			_, err := svc.Transition(deps, request)
			// Running is refused by its own rule, whatever the Job is in: a claim is
			// what admits it, and a terminal Job is no exception to that.
			want := ErrIllegalTransition
			if to == StateRunning {
				want = ErrRunningRequiresClaim
			}
			if !errors.Is(err, want) {
				t.Fatalf("%s -> %s = %v, want %v", terminal, to, err, want)
			}
		}
		stored := jobRow(t, deps, job.ID)
		if stored.State != string(terminal) || stored.Version != 9 {
			t.Fatalf("terminal job changed to %s v%d", stored.State, stored.Version)
		}
	}
}

func TestJobTransitionRequiresAndRecordsAFailureTaxonomy(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	t.Run("failed without a failure", func(t *testing.T) {
		job := seedJob(t, deps, StateRunning, clock, 1)
		if _, err := svc.Transition(deps, Transition{JobID: job.ID, ExpectedVersion: 1, To: StateFailed}); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("= %v, want ErrInvalidTransition", err)
		}
	})
	t.Run("failure class outside the taxonomy", func(t *testing.T) {
		job := seedJob(t, deps, StateRunning, clock, 1)
		_, err := svc.Transition(deps, Transition{
			JobID: job.ID, ExpectedVersion: 1, To: StateFailed,
			Failure: &Failure{Code: "http-403", Class: "network", Message: "refused"},
		})
		if !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("= %v, want ErrInvalidTransition", err)
		}
	})
	t.Run("failure message over its ceiling", func(t *testing.T) {
		job := seedJob(t, deps, StateRunning, clock, 1)
		_, err := svc.Transition(deps, Transition{
			JobID: job.ID, ExpectedVersion: 1, To: StateFailed,
			Failure: &Failure{Code: "http-403", Class: FailureClassDependency, Message: strings.Repeat("x", MaxFailureMessageBytes+1)},
		})
		if !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("= %v, want ErrInvalidTransition", err)
		}
	})
	t.Run("failure on a non-failing transition", func(t *testing.T) {
		job := seedJob(t, deps, StateRunning, clock, 1)
		_, err := svc.Transition(deps, Transition{
			JobID: job.ID, ExpectedVersion: 1, To: StateSucceeded,
			Failure: &Failure{Code: "http-403", Class: FailureClassDependency},
		})
		if !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("= %v, want ErrInvalidTransition", err)
		}
	})
	t.Run("recorded failure", func(t *testing.T) {
		job := seedJob(t, deps, StateRunning, clock, 1)
		snap, err := svc.Transition(deps, Transition{
			JobID: job.ID, ExpectedVersion: 1, To: StateFailed,
			Failure: &Failure{Code: "http-403", Class: FailureClassDependency, Message: "the origin refused", DiagnosticRef: "diag-17"},
		})
		if err != nil {
			t.Fatalf("running -> failed: %v", err)
		}
		if snap.Failure == nil || snap.Failure.Code != "http-403" || snap.Failure.Class != FailureClassDependency {
			t.Fatalf("snapshot failure = %+v", snap.Failure)
		}
		if snap.Failure.DiagnosticRef != "diag-17" || snap.Failure.Message != "the origin refused" {
			t.Fatalf("snapshot failure = %+v", snap.Failure)
		}
		stored := jobRow(t, deps, job.ID)
		if stored.FailureCode != "http-403" || stored.FailureClass != FailureClassDependency {
			t.Fatalf("stored failure = %+v", stored)
		}
	})
}

func TestJobTransitionRefusesAStaleExecutionToken(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 5, 6, 7, 8, 9, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	job := seedJob(t, deps, StateRunning, clock, 1)
	if err := deps.DB.Model(&models.Job{}).Where("id = ?", job.ID).
		Update("execution_token", "claim-a").Error; err != nil {
		t.Fatalf("seed claim: %v", err)
	}

	if _, err := svc.Transition(deps, Transition{JobID: job.ID, ExpectedVersion: 1, ExecutionToken: "claim-b", To: StateSucceeded}); !errors.Is(err, ErrStaleExecution) {
		t.Fatalf("stale token = %v, want ErrStaleExecution", err)
	}
	if _, err := svc.Transition(deps, Transition{JobID: job.ID, ExpectedVersion: 1, To: StateSucceeded}); !errors.Is(err, ErrStaleExecution) {
		t.Fatalf("absent token against a claimed job = %v, want ErrStaleExecution", err)
	}
	if _, err := svc.Transition(deps, Transition{JobID: job.ID, ExpectedVersion: 1, ExecutionToken: "claim-a", To: StateSucceeded}); err != nil {
		t.Fatalf("matching token: %v", err)
	}
}

func TestJobGetHonoursVisibility(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()

	owner := uint(11)
	ownerJob, err := svc.Accept(deps, Acceptance{
		Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api", OwnerUserID: &owner,
		Replay: ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("accept owner job: %v", err)
	}
	adminOnly, err := svc.Accept(deps, Acceptance{
		Kind: "plugin-command", KindVersion: 1, State: StateQueued, Origin: "plugin",
		OwnerUserID: &owner, Visibility: VisibilityAdmin,
		Replay: ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("accept admin-class job: %v", err)
	}
	ownerless, err := svc.Accept(deps, Acceptance{
		Kind: "similarity-recompute", KindVersion: 1, State: StateQueued, Origin: "system",
		Replay: ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("accept ownerless job: %v", err)
	}

	tests := []struct {
		name   string
		access Access
		jobID  string
		hidden bool
	}{
		{"the owner sees their own job", Access{UserID: owner}, ownerJob.ID, false},
		{"another user does not", Access{UserID: 12}, ownerJob.ID, true},
		{"an anonymous principal does not", Access{}, ownerJob.ID, true},
		{"an administrator sees any owned job", Access{Administrator: true}, ownerJob.ID, false},
		{"an admin-class job is hidden from its own owner", Access{UserID: owner}, adminOnly.ID, true},
		{"an admin-class job is visible to an administrator", Access{Administrator: true}, adminOnly.ID, false},
		{"an ownerless job is hidden from ordinary users", Access{UserID: owner}, ownerless.ID, true},
		{"an ownerless job is visible to an administrator", Access{Administrator: true}, ownerless.ID, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap, err := svc.Get(deps, tt.access, tt.jobID)
			if tt.hidden {
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("Get = %+v, %v; want ErrNotFound and no snapshot", snap, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if snap.ID != tt.jobID {
				t.Fatalf("Get returned %s, want %s", snap.ID, tt.jobID)
			}
		})
	}

	if _, err := svc.Get(deps, Access{Administrator: true}, "0193b0a0-0000-7000-8000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a missing job = %v, want ErrNotFound", err)
	}
}

// TestJobPublishAssignsDeliverySequencesOnlyAfterCommit covers what SQLite can
// honestly show: an event is invisible to everybody — including the publisher —
// until its transaction commits, and the publisher then hands out strictly
// increasing delivery sequences, so a subscriber that has consumed up to a
// cursor still receives what committed after it.
//
// The interleaving that actually inverts commit order — transaction A records
// first but commits after B — needs two concurrent writers, which SQLite does
// not have. That regression lives in service_pg_test.go.
func TestJobPublishAssignsDeliverySequencesOnlyAfterCommit(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 6, 7, 8, 9, 10, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	uncommitted := deps.DB.Begin()
	if uncommitted.Error != nil {
		t.Fatalf("begin: %v", uncommitted.Error)
	}
	first, err := svc.Accept(Deps{DB: uncommitted, Now: deps.Now}, Acceptance{
		Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "ui",
		Replay: ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("accept uncommitted job: %v", err)
	}

	// Nobody else can see it yet: not the publisher, not a listing, not a
	// subscriber's catch-up.
	var visible int64
	if err := deps.DB.Model(&models.Job{}).Where("id = ?", first.ID).Count(&visible).Error; err != nil {
		t.Fatalf("read uncommitted job: %v", err)
	}
	if visible != 0 {
		t.Fatalf("an uncommitted acceptance is visible to other connections (%d rows)", visible)
	}

	if committed := uncommitted.Commit(); committed.Error != nil {
		t.Fatalf("commit: %v", committed.Error)
	}
	published, err := svc.PublishPendingEvents(deps, DefaultPublishBatch)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if published != 1 {
		t.Fatalf("published %d events after the commit, want 1", published)
	}
	firstEvent := jobEvents(t, deps, first.ID)[0]
	if firstEvent.DeliverySequence == nil {
		t.Fatal("the committed event has no delivery sequence")
	}
	cursor := *firstEvent.DeliverySequence

	// A second Job is accepted and committed after that cursor was handed out.
	clock = clock.Add(time.Minute)
	second, err := svc.Accept(deps, Acceptance{
		Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "ui",
		Replay: ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("accept committed job: %v", err)
	}
	if _, err := svc.PublishPendingEvents(deps, DefaultPublishBatch); err != nil {
		t.Fatalf("publish after the second commit: %v", err)
	}

	later := jobEvents(t, deps, second.ID)[0]
	if later.DeliverySequence == nil {
		t.Fatal("the second commit was never published")
	}
	if *later.DeliverySequence <= cursor {
		t.Fatalf("the later commit took sequence %d, at or below the subscriber cursor %d: that is exactly the fact a cursor skips",
			*later.DeliverySequence, cursor)
	}

	// A catch-up read from the cursor must still yield the later fact, and must
	// not re-deliver what the subscriber already has.
	var catchUp []models.JobEvent
	if err := deps.DB.Where("delivery_sequence > ?", cursor).Order("delivery_sequence").Find(&catchUp).Error; err != nil {
		t.Fatalf("catch-up: %v", err)
	}
	if len(catchUp) != 1 || catchUp[0].ID != later.ID {
		t.Fatalf("catch-up returned %d events, want exactly the later commit %s", len(catchUp), later.ID)
	}
}

func TestJobPublishIsBoundedAndIdempotent(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	for i := 0; i < 3; i++ {
		if _, err := svc.Accept(deps, Acceptance{
			Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "ui",
			Replay: ReplayInput{NonReplayable: true},
		}); err != nil {
			t.Fatalf("accept %d: %v", i, err)
		}
	}

	published, err := svc.PublishPendingEvents(deps, 2)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if published != 2 {
		t.Fatalf("published %d events with a limit of 2", published)
	}

	published, err = svc.PublishPendingEvents(deps, 2)
	if err != nil {
		t.Fatalf("second publish: %v", err)
	}
	if published != 1 {
		t.Fatalf("second publish took %d events, want the one left", published)
	}

	published, err = svc.PublishPendingEvents(deps, 2)
	if err != nil {
		t.Fatalf("third publish: %v", err)
	}
	if published != 0 {
		t.Fatalf("third publish re-published %d events", published)
	}

	var sequences []uint64
	if err := deps.DB.Model(&models.JobEvent{}).Order("delivery_sequence").Pluck("delivery_sequence", &sequences).Error; err != nil {
		t.Fatalf("read sequences: %v", err)
	}
	if len(sequences) != 3 {
		t.Fatalf("%d events carry a delivery sequence, want 3", len(sequences))
	}
	for i, sequence := range sequences {
		if sequence != uint64(i+1) {
			t.Fatalf("delivery sequences = %v, want consecutive values from 1", sequences)
		}
	}
}

// TestJobTerminalTransitionAllocatesItsEventSequenceInsideTheTransaction covers
// the one write in a terminal transition that must not be decided outside the
// transaction that performs it.
//
// The event sequence is a Job's timeline position, and an executor appending a
// phase event between the decision and the commit takes that same position:
// AppendEvent reads the maximum inside its own transaction and does not move the
// Job version, so the terminal transition's guarded update still matches and the
// insert then collides on (job_id, sequence). The whole completion rolls back —
// a Job that cannot finish because its own executor was reporting progress.
func TestJobTerminalTransitionAllocatesItsEventSequenceInsideTheTransaction(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	clock := time.Date(2031, 8, 9, 10, 11, 12, 0, time.UTC)
	deps.Now = func() time.Time { return clock }

	job := seededExecution(t, deps, StateRunning, "claim-a")
	ref := ExecutionRef{JobID: job.ID, ExecutionToken: "claim-a"}
	transition := Transition{
		JobID: job.ID, ExpectedVersion: job.Version, ExecutionToken: "claim-a",
		To: StateFailed, Failure: &Failure{Code: "gave-up", Class: FailureClassInternal},
	}

	prepared, err := prepareTransition(deps, transition)
	if err != nil {
		t.Fatalf("prepareTransition: %v", err)
	}

	// The interleaving: an event that lands after the transition was decided and
	// before it commits. This is the real concurrency the sequence has to
	// survive, driven here through the same public append an adapter uses.
	if err := svc.AppendEvent(deps, ref, EventInput{
		Type: "phase", Detail: json.RawMessage(`{"segment":9}`),
	}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	if _, err := svc.commitTransition(deps, prepared, nil); err != nil {
		t.Fatalf("commitTransition: %v", err)
	}

	events := jobEvents(t, deps, job.ID)
	if len(events) != 2 {
		t.Fatalf("timeline has %d events, want the appended phase event and the terminal one", len(events))
	}
	if events[0].Type != "phase" || events[1].Type != EventFailed {
		t.Fatalf("timeline = %s, %s; want the appended event first and the terminal event after it",
			events[0].Type, events[1].Type)
	}
	if events[1].Sequence != events[0].Sequence+1 {
		t.Fatalf("terminal event sequence = %d after %d, want the next position",
			events[1].Sequence, events[0].Sequence)
	}
	if stored := jobRow(t, deps, job.ID); stored.State != string(StateFailed) {
		t.Fatalf("state = %s, want failed", stored.State)
	}
}

// TestJobVisibilityIsFixedByTheRegisteredKind closes the one way a Kind's
// visibility could be escaped: the class was a field on the acceptance, so an
// admin-only Kind whose adapter omitted it produced owner-visible Jobs, and a
// Job's class is exactly what the shared visibility predicate reads.
//
// The registered Definition is the only thing that knows whether a Kind is
// operator-only, so acceptance takes the class from it — and refuses an
// acceptance that claims something else.
func TestJobVisibilityIsFixedByTheRegisteredKind(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	registerTestAdapter(t, svc, Definition{
		Kind: testKind, KindVersion: 1, Restorable: true, Visibility: VisibilityAdmin,
	})

	accepted, err := svc.Accept(deps, Acceptance{
		Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: uintPtr(7), ActorUserID: uintPtr(7), Title: "an operator-only run",
		Replay: ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if accepted.Visibility != VisibilityAdmin {
		t.Fatalf("visibility = %q, want %q from the Kind's own definition", accepted.Visibility, VisibilityAdmin)
	}
	if stored := jobRow(t, deps, accepted.ID); stored.VisibilityClass != string(VisibilityAdmin) {
		t.Fatalf("stored visibility class = %q, want %q", stored.VisibilityClass, VisibilityAdmin)
	}

	// The submitter cannot see it, whatever they asked for.
	if _, err := svc.Get(deps, Access{UserID: 7}, accepted.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the submitter read an admin-class Job: %v", err)
	}
	if _, err := svc.Get(deps, Access{Administrator: true}, accepted.ID); err != nil {
		t.Fatalf("an administrator could not read it: %v", err)
	}

	// An acceptance that contradicts its own Kind is refused rather than
	// believed: nothing may name a class the registered Kind does not have.
	if _, err := svc.Accept(deps, Acceptance{
		Kind: testKind, KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: uintPtr(7), Visibility: VisibilityOwner,
		Replay: ReplayInput{NonReplayable: true},
	}); !errors.Is(err, ErrInvalidAcceptance) {
		t.Fatalf("an acceptance that contradicts its Kind's visibility = %v, want ErrInvalidAcceptance", err)
	}
}

// TestJobExecutionPrincipalVocabularyIsClosed pins the class dispatch acts from:
// every listed spelling round-trips into the durable row and back out of a
// snapshot, and a spelling nothing resolves is refused rather than stored.
//
// It matters because the class is the memory of who a Job acts as after the
// live reference beside it has been cleared — a class nobody can resolve would
// be a Job whose execution authority nobody can decide.
func TestJobExecutionPrincipalVocabularyIsClosed(t *testing.T) {
	deps := newTestDeps(t)
	svc := NewService()
	deps.Now = func() time.Time { return time.Date(2031, 9, 10, 11, 12, 13, 0, time.UTC) }

	if len(PrincipalClasses) != 3 {
		t.Fatalf("PrincipalClasses has %d entries, want actor, owner and host", len(PrincipalClasses))
	}
	for _, class := range PrincipalClasses {
		accepted, err := svc.Accept(deps, Acceptance{
			Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api",
			OwnerUserID: uintPtr(7), ActorUserID: uintPtr(7), ExecutionPrincipal: class,
			Replay: ReplayInput{NonReplayable: true},
		})
		if err != nil {
			t.Fatalf("Accept as %s: %v", class, err)
		}
		if accepted.ExecutionPrincipal != class {
			t.Fatalf("snapshot principal = %q, want %q", accepted.ExecutionPrincipal, class)
		}
		if stored := jobRow(t, deps, accepted.ID); stored.ExecutionPrincipal != string(class) {
			t.Fatalf("stored principal = %q, want %q", stored.ExecutionPrincipal, class)
		}
	}

	if _, err := svc.Accept(deps, Acceptance{
		Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: uintPtr(7), ExecutionPrincipal: PrincipalClass("root"),
		Replay: ReplayInput{NonReplayable: true},
	}); !errors.Is(err, ErrInvalidAcceptance) {
		t.Fatalf("an unknown execution principal = %v, want ErrInvalidAcceptance", err)
	}

	// The derived class is the one dispatch used to apply at run time: the actor
	// when there is one, else the owner, else the host.
	for _, testCase := range []struct {
		name      string
		owner     *uint
		actor     *uint
		wantClass PrincipalClass
	}{
		{"an actor is recorded", uintPtr(7), uintPtr(8), PrincipalActor},
		{"only an owner is recorded", uintPtr(7), nil, PrincipalOwner},
		{"no principal is recorded", nil, nil, PrincipalHost},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			accepted, err := svc.Accept(deps, Acceptance{
				Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "api",
				OwnerUserID: testCase.owner, ActorUserID: testCase.actor,
				Replay: ReplayInput{NonReplayable: true},
			})
			if err != nil {
				t.Fatalf("Accept: %v", err)
			}
			if accepted.ExecutionPrincipal != testCase.wantClass {
				t.Fatalf("principal = %q, want %q", accepted.ExecutionPrincipal, testCase.wantClass)
			}
		})
	}
}
