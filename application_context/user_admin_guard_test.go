package application_context

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"mahresources/auth"
	"mahresources/constants"
	"mahresources/jobs"
	"mahresources/models"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newSharedFileContext opens a temp-file SQLite DB (WAL + busy_timeout) shared
// across pool connections, so concurrent goroutines see the same data. Used for
// the last-admin concurrency test (the in-memory cache=private DB gives each
// connection its own database and cannot be shared).
func newSharedFileContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=10000&_synchronous=NORMAL", path)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Group{}, &models.Resource{}, &models.Note{}, &models.Tag{},
		&models.Category{}, &models.ResourceCategory{}, &models.NoteType{},
		&models.Series{}, &models.Query{}, &models.SavedMRQLQuery{}, &models.TemplatePartial{},
		&models.NoteBlock{}, &models.GroupRelation{}, &models.GroupRelationType{},
		&models.ResourceVersion{}, &models.User{}, &models.SavedSearch{}, &models.UserSetting{}, &models.Session{}, &models.ApiToken{},
		&models.DownloadHistoryEntry{},
		&models.ScheduledDownload{},
		&models.PluginSchedule{},
		&models.ResourceReduction{},
		&models.PluginCommandRun{}, &models.PluginCommandImport{},
		// The durable job core: DeleteUser's sweep nulls a Job's owner and
		// actor, and deletes the viewer-keyed preferences beside it, so these
		// tables exist wherever a user can be deleted.
		&models.Job{}, &models.JobEvent{}, &models.JobEventSequence{}, &models.JobLink{},
		&models.JobPreference{}, &models.JobPinGuard{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(4)
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	return NewMahresourcesContext(afero.NewMemMapFs(), db, readOnlyDB, &MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	})
}

func TestLastAdmin_DeleteSoleAdminBlocked(t *testing.T) {
	ctx := newStampTestContext(t, true)
	admin := makeAdmin(t, ctx, "solo")
	if err := ctx.DeleteUser(admin.ID); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("deleting sole admin: want ErrLastAdmin, got %v", err)
	}
	// The admin must still exist.
	if _, err := ctx.GetUser(admin.ID); err != nil {
		t.Fatalf("sole admin should still exist: %v", err)
	}
}

func TestLastAdmin_DemoteSoleAdminBlocked(t *testing.T) {
	ctx := newStampTestContext(t, true)
	admin := makeAdmin(t, ctx, "solo")
	_, err := ctx.UpdateUser(admin.ID, FullUserUpdate(&UserInput{Username: "solo", Role: models.RoleEditor}))
	if !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demoting sole admin: want ErrLastAdmin, got %v", err)
	}
	got, _ := ctx.GetUser(admin.ID)
	if got.Role != models.RoleAdmin || got.Disabled {
		t.Fatalf("sole admin must remain an enabled admin, got role=%q disabled=%v", got.Role, got.Disabled)
	}
}

func TestLastAdmin_DisableSoleAdminBlocked(t *testing.T) {
	ctx := newStampTestContext(t, true)
	admin := makeAdmin(t, ctx, "solo")
	_, err := ctx.UpdateUser(admin.ID, FullUserUpdate(&UserInput{Username: "solo", Role: models.RoleAdmin, Disabled: true}))
	if !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("disabling sole admin: want ErrLastAdmin, got %v", err)
	}
	got, _ := ctx.GetUser(admin.ID)
	if got.Disabled {
		t.Fatalf("sole admin must remain enabled")
	}
}

func TestLastAdmin_WithTwoAdminsEachOperationSucceeds(t *testing.T) {
	t.Run("delete", func(t *testing.T) {
		ctx := newStampTestContext(t, true)
		a1 := makeAdmin(t, ctx, "a1")
		makeAdmin(t, ctx, "a2")
		if err := ctx.DeleteUser(a1.ID); err != nil {
			t.Fatalf("delete one of two admins should succeed, got %v", err)
		}
	})
	t.Run("demote", func(t *testing.T) {
		ctx := newStampTestContext(t, true)
		a1 := makeAdmin(t, ctx, "a1")
		makeAdmin(t, ctx, "a2")
		if _, err := ctx.UpdateUser(a1.ID, FullUserUpdate(&UserInput{Username: "a1", Role: models.RoleEditor})); err != nil {
			t.Fatalf("demote one of two admins should succeed, got %v", err)
		}
	})
	t.Run("disable", func(t *testing.T) {
		ctx := newStampTestContext(t, true)
		a1 := makeAdmin(t, ctx, "a1")
		makeAdmin(t, ctx, "a2")
		if _, err := ctx.UpdateUser(a1.ID, FullUserUpdate(&UserInput{Username: "a1", Role: models.RoleAdmin, Disabled: true})); err != nil {
			t.Fatalf("disable one of two admins should succeed, got %v", err)
		}
	})
}

// TestLastAdmin_ConcurrentDeleteDifferentAdmins: two goroutines each delete a
// different one of two admins. Exactly one succeeds, the other gets ErrLastAdmin,
// and ≥1 enabled admin remains. SQLite serializes writers; Postgres coverage of
// the same invariant lives in the API test suite.
func TestLastAdmin_ConcurrentDeleteDifferentAdmins(t *testing.T) {
	ctx := newSharedFileContext(t)
	a1 := makeAdmin(t, ctx, "a1")
	a2 := makeAdmin(t, ctx, "a2")

	var wg sync.WaitGroup
	errs := make([]error, 2)
	targets := []uint{a1.ID, a2.ID}
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(idx uint) {
			defer wg.Done()
			errs[idx] = ctx.DeleteUser(targets[idx])
		}(uint(i))
	}
	wg.Wait()

	successes, lastAdmin := 0, 0
	for _, e := range errs {
		switch {
		case e == nil:
			successes++
		case errors.Is(e, ErrLastAdmin):
			lastAdmin++
		default:
			t.Fatalf("unexpected error: %v", e)
		}
	}
	if successes != 1 || lastAdmin != 1 {
		t.Fatalf("want exactly one success and one ErrLastAdmin, got successes=%d lastAdmin=%d", successes, lastAdmin)
	}
	n, err := ctx.CountEnabledAdmins()
	if err != nil {
		t.Fatalf("CountEnabledAdmins: %v", err)
	}
	if n < 1 {
		t.Fatalf("at least one enabled admin must remain, got %d", n)
	}
}

// TestRootAdminCache_ConcurrentMutationsConverge stresses the refreshRootAdmin
// resolve+store serialization: after several concurrent admin deletions settle,
// the no-auth default actor must equal the current oldest enabled admin — never a
// deleted one. Without the mutex, a stale read could win a later store and pin
// the cache to a removed admin (a lost update). Looped over fresh DBs to surface
// the race.
func TestRootAdminCache_ConcurrentMutationsConverge(t *testing.T) {
	for iter := 0; iter < 15; iter++ {
		ctx := newSharedFileContext(t)
		// a1 (oldest) .. a4. a4 is never deleted, so deleting a1..a3 always leaves
		// a4 as the sole remaining admin the cache must converge to.
		admins := make([]*models.User, 4)
		for i := 0; i < 4; i++ {
			u, err := ctx.CreateUser(&UserInput{Username: fmt.Sprintf("a%d", i), Password: "password1", Role: models.RoleAdmin})
			if err != nil {
				t.Fatalf("iter %d create a%d: %v", iter, i, err)
			}
			admins[i] = u
		}
		keep := admins[3]

		var wg sync.WaitGroup
		errs := make([]error, 3)
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				errs[idx] = ctx.DeleteUser(admins[idx].ID)
			}(i)
		}
		wg.Wait()
		for i, e := range errs {
			if e != nil {
				t.Fatalf("iter %d: delete a%d should succeed (a4 remains), got %v", iter, i, e)
			}
		}

		// The cache must have converged to the oldest remaining admin (a4).
		if got := ctx.defaultActorID(); got != keep.ID {
			t.Fatalf("iter %d: defaultActorID=%d, want surviving admin a4=%d (stale/lost-update cache)", iter, got, keep.ID)
		}
	}
}

// Phase 5: content stamped by a deleted user survives with a NULL creator.
func TestDeleteUser_NullsCreatorReferences(t *testing.T) {
	ctx := newStampTestContext(t, true)
	makeAdmin(t, ctx, "keeper") // keeps an admin so deleting U is allowed
	u, err := ctx.CreateUser(&UserInput{Username: "u", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	scoped := ctx.WithPrincipal(auth.FromUser(u))
	res := &models.Resource{Name: "owned"}
	if err := scoped.db.Create(res).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}
	if res.CreatedByUserId == nil || *res.CreatedByUserId != u.ID {
		t.Fatalf("resource should be stamped by U, got %v", res.CreatedByUserId)
	}

	if err := ctx.DeleteUser(u.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	var got models.Resource
	if err := ctx.db.First(&got, res.ID).Error; err != nil {
		t.Fatalf("resource should survive user deletion: %v", err)
	}
	if got.CreatedByUserId != nil {
		t.Fatalf("creator reference must be NULL after user deletion, got %v", *got.CreatedByUserId)
	}
}

// A Job preference belongs to the viewer rather than to the Job, so deleting the
// viewer removes it. Two reasons make the sweep load-bearing rather than tidy: a
// surviving pin exempts the Job's history from retention for everybody, forever,
// and a surviving dismissal would be inherited by whichever account later holds
// that id.
func TestDeleteUserRemovesTheirJobPreferences(t *testing.T) {
	ctx := newStampTestContext(t, true)
	makeAdmin(t, ctx, "keeper")
	viewer, err := ctx.CreateUser(&UserInput{Username: "viewer", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	svc := jobs.NewService()
	snap, err := svc.Accept(jobs.Deps{DB: ctx.db}, jobs.Acceptance{
		Kind: "remote-download", KindVersion: 1, State: jobs.StateQueued, Origin: "api",
		OwnerUserID: &viewer.ID, Title: "the download this viewer asked for",
		Replay: jobs.ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("accept job: %v", err)
	}
	admin := jobs.Access{Administrator: true}
	if err := svc.SetPreference(jobs.Deps{DB: ctx.db}, jobs.Access{UserID: viewer.ID}, jobs.PreferenceRequest{
		JobID: snap.ID, Pinned: boolPointer(true), Dismissed: boolPointer(true),
	}); err != nil {
		t.Fatalf("set preference: %v", err)
	}
	var before int64
	if err := ctx.db.Model(&models.JobPreference{}).Where("user_id = ?", viewer.ID).Count(&before).Error; err != nil {
		t.Fatalf("count preferences: %v", err)
	}
	if before != 1 {
		t.Fatalf("recorded %d preferences, want 1", before)
	}

	if err := ctx.DeleteUser(viewer.ID); err != nil {
		t.Fatalf("delete viewer: %v", err)
	}

	var after int64
	if err := ctx.db.Model(&models.JobPreference{}).Where("user_id = ?", viewer.ID).Count(&after).Error; err != nil {
		t.Fatalf("count preferences: %v", err)
	}
	if after != 0 {
		t.Fatalf("a deleted viewer's preferences survived: %d", after)
	}
	// The Job itself is untouched — the sweep removes a view, not history — and
	// it is now ownerless, which is what keeps a deleted account from leaving a
	// pin behind that exempts it from retention for everybody.
	if _, err := svc.Get(jobs.Deps{DB: ctx.db}, admin, snap.ID); err != nil {
		t.Fatalf("the job must survive: %v", err)
	}
	var job models.Job
	if err := ctx.db.Where("id = ?", snap.ID).First(&job).Error; err != nil {
		t.Fatalf("reload job: %v", err)
	}
	if job.OwnerUserID != nil {
		t.Fatalf("the deleted viewer still owns the job: %v", *job.OwnerUserID)
	}
}

func boolPointer(v bool) *bool { return &v }

// jobPreferenceRows counts one viewer's preference rows, which is what "did this
// admission land" is asked as.
func jobPreferenceRows(t *testing.T, ctx *MahresourcesContext, userID uint) int64 {
	t.Helper()
	var rows int64
	if err := ctx.db.Model(&models.JobPreference{}).Where("user_id = ?", userID).Count(&rows).Error; err != nil {
		t.Fatalf("count preferences for %d: %v", userID, err)
	}
	return rows
}

// The sweep above removes the preferences of a viewer who is deleted. This is the
// same rule read as a race, at the seam a request reaches it through: the
// administrator's context is in flight — authenticated before the account was
// removed, writing after it — and nothing else in the preference path notices.
//
// The Job belongs to somebody else and stays visible to the captured identity,
// because the visibility predicate is asked as the *captured* access, so the
// recheck inside the transaction passes and the row is viewer-keyed. A surviving
// pin would then exempt that Job's metadata and events from retention for
// everybody, forever, with no viewer left who could ever unpin it.
func TestJobPreferenceIsRefusedAfterTheViewerIsDeleted(t *testing.T) {
	ctx := newStampTestContext(t, true)
	ctx.SetJobService(jobs.NewService())
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

	// The administrator's request context, authenticated and in flight.
	inFlight := ctx.WithPrincipal(&auth.Principal{UserID: doomed.ID, Username: doomed.Username, Role: models.RoleAdmin})

	if err := ctx.DeleteUser(doomed.ID); err != nil {
		t.Fatalf("delete the administrator: %v", err)
	}

	// Nothing about the Job refuses this write: it is another user's Job, and the
	// access the check inside the transaction is asked with is a captured
	// administrator's.
	if _, err := inFlight.GetJob(job.ID); err != nil {
		t.Fatalf("the captured administrator cannot see the Job: %v", err)
	}

	err = inFlight.SetJobPreference(jobs.PreferenceRequest{JobID: job.ID, Pinned: boolPointer(true)})
	if rows := jobPreferenceRows(t, ctx, doomed.ID); err == nil || rows != 0 {
		t.Fatalf("a preference for the deleted administrator was written (err = %v, %d rows): a surviving pin exempts the Job from retention for everybody, forever", err, rows)
	}
	if !errors.Is(err, jobs.ErrViewerDeleted) {
		t.Fatalf("the refusal for a deleted viewer = %v, want ErrViewerDeleted", err)
	}

	// The refusal is durable rather than a snapshot of a row that happens to be
	// gone by now, so it holds however long after the deletion the write lands.
	var guard models.JobPinGuard
	if err := ctx.db.Where("user_id = ?", doomed.ID).First(&guard).Error; err != nil {
		t.Fatalf("the deleted viewer left no preference fence: %v", err)
	}
	if guard.DeletedAt == nil {
		t.Fatal("the deleted viewer's preference fence carries no tombstone, so a later admission would be admitted")
	}
}

// Phase 5b: a Job carries two live user references, and deleting its owner
// nulls both. The Job survives as admin-only history — its outcome and sanitized
// summary are facts about what happened — while the deleted identity keeps no
// grant over it and no ordinary user gains one.
func TestDeleteUser_NullsJobOwnershipAndActor(t *testing.T) {
	ctx := newStampTestContext(t, true)
	makeAdmin(t, ctx, "keeper")
	u, err := ctx.CreateUser(&UserInput{Username: "jobowner", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	svc := jobs.NewService()
	snap, err := svc.Accept(jobs.Deps{DB: ctx.db}, jobs.Acceptance{
		Kind: "remote-download", KindVersion: 1, State: jobs.StateQueued, Origin: "api",
		OwnerUserID: &u.ID, ActorUserID: &u.ID,
		Title:  "the download this user asked for",
		Replay: jobs.ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("accept job: %v", err)
	}

	if err := ctx.DeleteUser(u.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	var job models.Job
	if err := ctx.db.Where("id = ?", snap.ID).First(&job).Error; err != nil {
		t.Fatalf("the job must survive its owner's deletion: %v", err)
	}
	if job.OwnerUserID != nil || job.ActorUserID != nil {
		t.Fatalf("job references = owner %v, actor %v; both must be NULL", job.OwnerUserID, job.ActorUserID)
	}
	if job.Origin != "api" || job.State != string(jobs.StateQueued) || job.Title != "the download this user asked for" {
		t.Fatalf("the job's own history changed: %+v", job)
	}

	if _, err := svc.Get(jobs.Deps{DB: ctx.db}, jobs.Access{Administrator: true}, snap.ID); err != nil {
		t.Fatalf("an administrator must still see an ownerless job: %v", err)
	}
	if _, err := svc.Get(jobs.Deps{DB: ctx.db}, jobs.Access{UserID: u.ID}, snap.ID); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("a deleted owner's id must grant nothing, got %v", err)
	}
}

// Phase 5c: deleting the actor of a Job must not hand its execution to whoever
// remains. Owner and actor are separate provenance facts, and the durable
// principal class is what tells "an actor was recorded and is gone" apart from
// "this work was never anyone's" — the live column alone cannot, because user
// deletion nulls it.
//
// So the Job whose actor is deleted is refused at dispatch and blocked, rather
// than running as the surviving owner (which would transfer authority) or as the
// host (which would grant the work an identity nobody recorded).
func TestDeleteUser_RefusesDispatchOfAJobWhoseActorIsGone(t *testing.T) {
	ctx := newStampTestContext(t, true)
	makeAdmin(t, ctx, "keeper")
	owner, err := ctx.CreateUser(&UserInput{Username: "jobowner", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	actor, err := ctx.CreateUser(&UserInput{Username: "jobactor", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}

	svc := jobs.NewService()
	adapter := newRuntimeTestAdapter()
	if err := svc.RegisterAdapter(adapter); err != nil {
		t.Fatalf("register adapter: %v", err)
	}
	deps := jobs.Deps{DB: ctx.db}
	snap, err := svc.Accept(deps, jobs.Acceptance{
		Kind: runtimeTestKind, KindVersion: 1, State: jobs.StateQueued, Origin: "api",
		OwnerUserID: &owner.ID, ActorUserID: &actor.ID,
		Title:  "work the actor asked for",
		Replay: jobs.ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("accept job: %v", err)
	}

	if err := ctx.DeleteUser(actor.ID); err != nil {
		t.Fatalf("delete actor: %v", err)
	}

	execution, claimed, err := svc.Claim(context.Background(), deps, jobs.ClaimRequest{
		Kind: runtimeTestKind, KindVersion: 1, Claimant: "runtime-a",
	})
	if claimed {
		t.Fatalf("a Job whose actor was deleted was claimed as %+v", execution)
	}
	if err == nil {
		t.Fatal("claiming a Job whose actor was deleted reported no reason")
	}
	if dispatched := adapter.dispatched(); len(dispatched) != 0 {
		t.Fatalf("the work ran anyway: %+v", dispatched)
	}

	var job models.Job
	if err := ctx.db.Where("id = ?", snap.ID).First(&job).Error; err != nil {
		t.Fatalf("reload job: %v", err)
	}
	if job.State != string(jobs.StateBlocked) {
		t.Fatalf("state = %s, want blocked", job.State)
	}
	if job.OwnerUserID == nil || *job.OwnerUserID != owner.ID {
		t.Fatalf("the owner must survive the actor's deletion: %v", job.OwnerUserID)
	}
	if job.ActorUserID != nil {
		t.Fatalf("the deleted actor's reference survived: %v", *job.ActorUserID)
	}
	if job.ExecutionToken != "" {
		t.Fatalf("a blocked Job must not keep an execution token: %q", job.ExecutionToken)
	}
}
