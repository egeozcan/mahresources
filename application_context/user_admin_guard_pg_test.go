//go:build postgres && json1 && fts5

package application_context

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"

	"mahresources/auth"
	"mahresources/constants"
	"mahresources/jobs"
	"mahresources/models"

	"gorm.io/gorm"
)

// The preference fence is observable on PostgreSQL alone, and that is the point
// of the test below rather than an accident of the fixture: SQLite has a single
// writer, so a deletion and an admission reaching the same viewer can never be
// inside their transactions at the same time there, and the tombstone read is
// what carries the rule. On PostgreSQL two writers can be in flight at once, and
// only a fence the deletion takes as well keeps an admission that began first
// from committing last.

// newPostgresUserJobContext builds a context that can both delete an account and
// admit a preference, because DeleteUser sweeps every table a live user
// reference sits on: the fixture is the whole stamped set rather than the two
// tables the assertions read.
func newPostgresUserJobContext(t *testing.T) *MahresourcesContext {
	t.Helper()

	db, dsn := pgContainer.CreateTestDBWithDSN(t)
	if err := db.AutoMigrate(
		&models.Query{}, &models.Series{}, &models.Tag{}, &models.Category{},
		&models.ResourceCategory{}, &models.NoteType{}, &models.SavedMRQLQuery{},
		&models.TemplatePartial{}, &models.Group{}, &models.GroupRelationType{},
		&models.Resource{}, &models.Note{}, &models.ResourceVersion{}, &models.NoteBlock{},
		&models.GroupRelation{}, &models.User{}, &models.SavedSearch{}, &models.UserSetting{},
		&models.Session{}, &models.ApiToken{},
		&models.DownloadHistoryEntry{}, &models.ScheduledDownload{}, &models.PluginSchedule{},
		&models.ResourceReduction{}, &models.PluginCommandRun{}, &models.PluginCommandImport{},
		&models.Job{}, &models.JobEvent{}, &models.JobEventSequence{}, &models.JobLink{},
		&models.JobOutput{}, &models.JobReplayEnvelope{}, &models.JobClaim{}, &models.JobCapacityLease{},
		&models.JobPreference{}, &models.JobPinGuard{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	readOnly, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("open read-only handle: %v", err)
	}
	t.Cleanup(func() { readOnly.Close() })

	ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, readOnly, &MahresourcesConfig{
		DbType:      constants.DbTypePosgres,
		AuthEnabled: true,
	})
	ctx.SetJobService(jobs.NewService())
	return ctx
}

// waitForABlockedQueryPG reports whether some statement in this test's database
// is waiting on a lock, which is how the interleaving below observes that one
// transaction has reached the row another holds, without sleeping for a guessed
// interval. It answers rather than failing, because the test asserts the
// preference that survives it, not the observation itself.
func waitForABlockedQueryPG(t *testing.T, ctx *MahresourcesContext, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var waiting int64
		if err := ctx.db.Raw("SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND datname = current_database()").
			Scan(&waiting).Error; err != nil {
			t.Fatalf("read pg_stat_activity: %v", err)
		}
		if waiting > 0 {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return false
}

// TestJobPreferenceIsFencedAgainstUserDeletionPG is the other half of the
// correction the SQLite regression pins: the tombstone makes a later admission
// impossible, and the fence is what makes an *earlier* one impossible to lose.
//
// The admission is held once it holds the viewer's fence and is about to take the
// Job's row — the instant a deletion has to arrive at to be serialized against it.
// The account is deleted from another connection, which must wait on that fence
// rather than run past it: were the deletion to sweep first and this admission to
// commit afterwards, the Job would be left pinned for everybody by a viewer that
// no longer exists, and no account could ever unpin it. Here the deletion waits,
// the admission commits, and the sweep that follows takes the row it wrote.
func TestJobPreferenceIsFencedAgainstUserDeletionPG(t *testing.T) {
	ctx := newPostgresUserJobContext(t)
	makeAdmin(t, ctx, "keeper") // so removing the other admin is not the last-admin case
	doomed := makeAdmin(t, ctx, "doomed")
	owner, err := ctx.CreateUser(&UserInput{Username: "owner", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	job, err := ctx.JobService().Accept(ctx.jobDeps(), jobs.Acceptance{
		Kind: "remote-download", KindVersion: 1, State: jobs.StateQueued, Origin: "api",
		OwnerUserID: &owner.ID, Title: "another user's download",
		Replay: jobs.ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("accept job: %v", err)
	}
	admin := ctx.WithPrincipal(&auth.Principal{UserID: doomed.ID, Username: doomed.Username, Role: models.RoleAdmin})

	// The viewer already holds a preference, so their fence row exists and is
	// committed: what holds the fence during the interleaving below is the
	// admission's own lock on that row rather than the insert that creates it.
	if err := admin.SetJobPreference(jobs.PreferenceRequest{JobID: job.ID, Dismissed: boolPointer(true)}); err != nil {
		t.Fatalf("dismiss: %v", err)
	}

	held := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	// A failing assertion must not leave the admission holding its transaction.
	t.Cleanup(unblock)

	var once sync.Once
	const hook = "test:hold-the-preference-admission"
	if err := ctx.db.Callback().Query().After("gorm:query").Register(hook, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Table != "jobs" ||
			!strings.Contains(tx.Statement.SQL.String(), "FOR UPDATE") {
			return
		}
		for _, variable := range tx.Statement.Vars {
			if named, ok := variable.(string); ok && named == job.ID {
				once.Do(func() {
					close(held)
					<-release
				})
				return
			}
		}
	}); err != nil {
		t.Fatalf("register the interleaving hook: %v", err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(hook) })

	admitted := make(chan error, 1)
	go func() {
		admitted <- admin.SetJobPreference(jobs.PreferenceRequest{JobID: job.ID, Pinned: boolPointer(true)})
	}()

	select {
	case <-held:
	case err := <-admitted:
		t.Fatalf("the admission returned without holding its fence: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("the admission never reached the Job's row, so nothing is holding the fence")
	}

	deleted := make(chan error, 1)
	go func() { deleted <- ctx.DeleteUser(doomed.ID) }()

	blocked := waitForABlockedQueryPG(t, ctx, 5*time.Second)
	unblock()

	if err := <-admitted; err != nil {
		t.Fatalf("admission: %v", err)
	}
	if err := <-deleted; err != nil {
		t.Fatalf("delete: %v", err)
	}

	if rows := jobPreferenceRows(t, ctx, doomed.ID); rows != 0 {
		t.Fatalf("the deletion and the admission in flight did not serialize: %d preference rows survive for the viewer it removed (the deletion waited on the fence: %v)", rows, blocked)
	}
	if !blocked {
		t.Fatal("the deletion never waited on the fence the in-flight admission held, so the two are not serialized by anything")
	}

	// The tombstone then refuses the next admission outright, which is the
	// sequential half of the same rule.
	err = admin.SetJobPreference(jobs.PreferenceRequest{JobID: job.ID, Pinned: boolPointer(true)})
	if !errors.Is(err, jobs.ErrViewerDeleted) {
		t.Fatalf("an admission after the viewer was deleted = %v, want ErrViewerDeleted", err)
	}
	if rows := jobPreferenceRows(t, ctx, doomed.ID); rows != 0 {
		t.Fatalf("the refused admission wrote %d preference rows", rows)
	}
}
