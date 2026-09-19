package application_context

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/constants"
	"mahresources/models"
	"mahresources/plugin_commands"
)

func newPluginCommandStoreTestContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	path := filepath.Join(t.TempDir(), "command-store.db")
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=10000&_synchronous=NORMAL", path)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	modelsToMigrate := append(stampedModels(),
		&models.PluginCommandRun{}, &models.PluginCommandRunOutput{},
		&models.PluginCommandImport{}, &models.PluginCommandImportMap{},
		&models.User{}, &models.Session{}, &models.ApiToken{}, &models.SavedSearch{}, &models.UserSetting{},
		&models.RuntimeSetting{}, &models.LogEntry{},
	)
	require.NoError(t, db.AutoMigrate(modelsToMigrate...))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	cfg := &MahresourcesConfig{DbType: constants.DbTypeSqlite, AuthEnabled: true}
	return NewMahresourcesContext(afero.NewMemMapFs(), db, sqlx.NewDb(sqlDB, "sqlite3"), cfg)
}

func uintPtr(v uint) *uint { return &v }

func testRun(id string, owner *uint, actorless bool, at time.Time) plugin_commands.RunRecord {
	return plugin_commands.RunRecord{
		ID: id, PluginName: "worker", CommandName: "download", ParamsJSON: `{}`,
		Status: plugin_commands.RunStatusQueued, CreatedByUserID: owner,
		ActorlessAtSubmission: actorless, CreatedAt: at,
	}
}

func testOutput(id string, at time.Time) plugin_commands.RunOutput {
	return plugin_commands.RunOutput{RunID: id, ArgvJSON: `["tool"]`, CreatedAt: at}
}

func TestPluginCommandStoreCreateRunAndOutputIsAtomic(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	now := time.Now().UTC()
	require.NoError(t, ctx.db.Create(&models.PluginCommandRunOutput{RunID: "atomic-create", ArgvJSON: `[]`, CreatedAt: now}).Error)
	err := ctx.CreateRun(testRun("atomic-create", uintPtr(3), false, now), testOutput("atomic-create", now))
	require.Error(t, err)
	var count int64
	require.NoError(t, ctx.db.Model(&models.PluginCommandRun{}).Where("id = ?", "atomic-create").Count(&count).Error)
	require.Zero(t, count, "an output insert failure must roll back the run row")
}

func TestPluginCommandStoreRunTransitionsAndOutputPruning(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	owner := uint(41)
	require.NoError(t, ctx.CreateRun(testRun("run-transitions", &owner, false, now), testOutput("run-transitions", now)))
	won, err := ctx.FinishRun("run-transitions", plugin_commands.RunFinish{
		Status: plugin_commands.RunStatusSucceeded, FinishedAt: now.Add(time.Second),
	})
	require.NoError(t, err)
	require.False(t, won, "a queued command cannot succeed without first running")

	runs, err := ctx.NonterminalRuns()
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, plugin_commands.RunStatusQueued, runs[0].Status)

	started := now.Add(time.Second)
	won, err = ctx.MarkRunRunning("run-transitions", started)
	require.NoError(t, err)
	require.True(t, won)
	won, err = ctx.MarkRunRunning("run-transitions", started.Add(time.Second))
	require.NoError(t, err)
	require.False(t, won, "a stale start writer must lose")
	require.NoError(t, ctx.SetRunProcessGroup("run-transitions", 4321))

	require.NoError(t, ctx.RequestRunCancel("run-transitions", "operator cancelled"))
	require.ErrorIs(t, ctx.RequestRunCancel("run-transitions", "again"), plugin_commands.ErrRunNotCancellable)
	require.ErrorIs(t, ctx.RequestRunCancel("missing", "operator cancelled"), plugin_commands.ErrRunNotFound)
	won, err = ctx.FinishRun("run-transitions", plugin_commands.RunFinish{
		Status: plugin_commands.RunStatusSucceeded, OutputTail: "ignored", FinishedAt: now.Add(2 * time.Second),
	})
	require.NoError(t, err)
	require.False(t, won, "a durable cancellation latch must beat a success writer")

	finished := now.Add(2 * time.Second)
	won, err = ctx.FinishRun("run-transitions", plugin_commands.RunFinish{
		Status: plugin_commands.RunStatusCancelled, Error: "operator cancelled", OutputTail: "tail", FinishedAt: finished,
	})
	require.NoError(t, err)
	require.True(t, won)
	won, err = ctx.FinishRun("run-transitions", plugin_commands.RunFinish{
		Status: plugin_commands.RunStatusSucceeded, OutputTail: "stale", FinishedAt: finished.Add(time.Second),
	})
	require.NoError(t, err)
	require.False(t, won, "a stale terminal writer must lose")

	run, output, err := ctx.Run("run-transitions")
	require.NoError(t, err)
	require.Equal(t, plugin_commands.RunStatusCancelled, run.Status)
	require.True(t, run.CancelRequested)
	require.ErrorIs(t, ctx.RequestRunCancel("run-transitions", "terminal"), plugin_commands.ErrRunNotCancellable)
	require.Equal(t, "tail", output.OutputTail)
	require.Equal(t, 4321, *run.ProcessGroupID)

	expired, err := ctx.ExpiredTerminalRuns(finished.Add(time.Second))
	require.NoError(t, err)
	require.Len(t, expired, 1)
	pruned, err := ctx.PruneRunOutputs(now.Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, int64(1), pruned)
	run, output, err = ctx.Run("run-transitions")
	require.NoError(t, err)
	require.Equal(t, "run-transitions", run.ID, "pruning output must preserve the run")
	require.Empty(t, output.RunID)
}

func TestPluginCommandStoreFinishRunIsAtomicWithOutputTail(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	now := time.Now().UTC()
	require.NoError(t, ctx.CreateRun(testRun("atomic-finish", uintPtr(7), false, now), testOutput("atomic-finish", now)))
	won, err := ctx.MarkRunRunning("atomic-finish", now)
	require.NoError(t, err)
	require.True(t, won)
	require.NoError(t, ctx.db.Exec(`CREATE TRIGGER refuse_command_finish BEFORE UPDATE OF status ON plugin_command_runs WHEN NEW.status = 'succeeded' BEGIN SELECT RAISE(ABORT, 'blocked'); END`).Error)

	won, err = ctx.FinishRun("atomic-finish", plugin_commands.RunFinish{
		Status: plugin_commands.RunStatusSucceeded, OutputTail: "must roll back", FinishedAt: now.Add(time.Second),
	})
	require.Error(t, err)
	require.False(t, won)
	run, output, readErr := ctx.Run("atomic-finish")
	require.NoError(t, readErr)
	require.Equal(t, plugin_commands.RunStatusRunning, run.Status)
	require.Empty(t, output.OutputTail)
}

func TestPluginCommandStoreActorlessCreateClearsNoAuthDefaultActor(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	ctx.Config.AuthEnabled = false
	admin, err := ctx.CreateUser(&UserInput{Username: "root", Password: "password1", Role: models.RoleAdmin})
	require.NoError(t, err)
	require.NotZero(t, admin.ID)
	now := time.Now().UTC()
	require.NoError(t, ctx.CreateRun(testRun("actorless-no-auth", nil, true, now), testOutput("actorless-no-auth", now)))
	run, _, err := ctx.Run("actorless-no-auth")
	require.NoError(t, err)
	require.Nil(t, run.CreatedByUserID)
	require.True(t, run.ActorlessAtSubmission)
}

func TestPluginCommandStoreDeletedActorFailsClosedButActorlessProvenanceSurvives(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	admin := &models.User{Username: "admin", PasswordHash: "x", Role: models.RoleAdmin}
	actor := &models.User{Username: "actor", PasswordHash: "x", Role: models.RoleUser}
	viewer := &models.User{Username: "viewer", PasswordHash: "x", Role: models.RoleUser}
	require.NoError(t, ctx.db.Create(admin).Error)
	require.NoError(t, ctx.db.Create(actor).Error)
	require.NoError(t, ctx.db.Create(viewer).Error)
	now := time.Now().UTC()
	require.NoError(t, ctx.CreateRun(testRun("owned-run", &actor.ID, false, now), testOutput("owned-run", now)))
	require.NoError(t, ctx.CreateRun(testRun("actorless-run", nil, true, now), testOutput("actorless-run", now)))
	claim, err := ctx.ClaimImport(plugin_commands.ImportClaimRequest{
		ImportID: "owned-import", RunID: "owned-run", FileName: "out.bin", PluginGeneration: 9,
		CreatedByUserID: &actor.ID, CreatedAt: now,
	})
	require.NoError(t, err)
	require.True(t, claim.Created)

	require.NoError(t, ctx.DeleteUser(actor.ID))
	owned, _, err := ctx.Run("owned-run")
	require.NoError(t, err)
	require.Nil(t, owned.CreatedByUserID)
	require.False(t, owned.ActorlessAtSubmission)
	actorless, _, err := ctx.Run("actorless-run")
	require.NoError(t, err)
	require.Nil(t, actorless.CreatedByUserID)
	require.True(t, actorless.ActorlessAtSubmission)

	visible, err := ctx.Runs(plugin_commands.Access{PluginName: "worker", ActorUserID: &viewer.ID})
	require.NoError(t, err)
	require.Len(t, visible, 1)
	require.Equal(t, "actorless-run", visible[0].ID)
	visible, err = ctx.Runs(plugin_commands.Access{PluginName: "worker"})
	require.NoError(t, err)
	require.Empty(t, visible, "actorless access still requires a current principal")

	won, err := ctx.MarkImportRunning("owned-import", now.Add(time.Second))
	require.NoError(t, err)
	require.False(t, won, "an import whose actor was deleted must not start")
}

func TestPluginCommandStoreImportClaimStateTable(t *testing.T) {
	tests := []struct {
		name             string
		existingStatus   string
		wantID           string
		wantStatus       string
		wantResourceID   *uint
		wantCreated      bool
		wantEnqueue      bool
		wantOldPreserved bool
	}{
		{name: "absent", wantID: "new-import", wantStatus: plugin_commands.ImportStatusPending, wantCreated: true, wantEnqueue: true},
		{name: "pending", existingStatus: plugin_commands.ImportStatusPending, wantID: "old-import", wantStatus: plugin_commands.ImportStatusPending},
		{name: "running", existingStatus: plugin_commands.ImportStatusRunning, wantID: "old-import", wantStatus: plugin_commands.ImportStatusRunning},
		{name: "succeeded", existingStatus: plugin_commands.ImportStatusSucceeded, wantID: "old-import", wantStatus: plugin_commands.ImportStatusSucceeded, wantResourceID: uintPtr(88)},
		{name: "interrupted", existingStatus: plugin_commands.ImportStatusInterrupted, wantID: "old-import", wantStatus: plugin_commands.ImportStatusPending, wantEnqueue: true},
		{name: "failed", existingStatus: plugin_commands.ImportStatusFailed, wantID: "new-import", wantStatus: plugin_commands.ImportStatusPending, wantCreated: true, wantEnqueue: true, wantOldPreserved: true},
		{name: "cancelled", existingStatus: plugin_commands.ImportStatusCancelled, wantID: "new-import", wantStatus: plugin_commands.ImportStatusPending, wantCreated: true, wantEnqueue: true, wantOldPreserved: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newPluginCommandStoreTestContext(t)
			now := time.Now().UTC()
			owner := uint(15)
			require.NoError(t, ctx.CreateRun(testRun("run", nil, true, now), testOutput("run", now)))
			if tc.existingStatus != "" {
				old := models.PluginCommandImport{ID: "old-import", RunID: "run", FileName: "file.bin", PluginGeneration: 1, CreatedByUserId: &owner, Status: tc.existingStatus, CreatedAt: now.Add(-time.Hour)}
				if tc.existingStatus == plugin_commands.ImportStatusSucceeded {
					old.FinishedAt = &now
				}
				require.NoError(t, ctx.db.Create(&old).Error)
				require.NoError(t, ctx.db.Create(&models.PluginCommandImportMap{RunID: "run", FileName: "file.bin", ImportID: old.ID, ResourceID: tc.wantResourceID, Status: tc.existingStatus}).Error)
			}

			got, err := ctx.ClaimImport(plugin_commands.ImportClaimRequest{
				ImportID: "new-import", RunID: "run", FileName: "file.bin", PluginGeneration: 22,
				CreatedByUserID: uintPtr(77), CreatedAt: now,
			})
			require.NoError(t, err)
			require.Equal(t, tc.wantID, got.ImportID)
			require.Equal(t, tc.wantStatus, got.Status)
			require.Equal(t, tc.wantResourceID, got.ResourceID)
			require.Equal(t, tc.wantCreated, got.Created)
			require.Equal(t, tc.wantEnqueue, got.Enqueue)

			if tc.existingStatus == plugin_commands.ImportStatusInterrupted {
				var refreshed models.PluginCommandImport
				require.NoError(t, ctx.db.First(&refreshed, "id = ?", "old-import").Error)
				require.Equal(t, plugin_commands.ImportStatusPending, refreshed.Status)
				require.Equal(t, uint64(22), refreshed.PluginGeneration)
				require.Equal(t, uint(77), *refreshed.CreatedByUserId)
				require.Nil(t, refreshed.StartedAt)
				require.Nil(t, refreshed.FinishedAt)
			}
			if tc.wantOldPreserved {
				var count int64
				require.NoError(t, ctx.db.Model(&models.PluginCommandImport{}).Where("id = ?", "old-import").Count(&count).Error)
				require.Equal(t, int64(1), count)
			}
		})
	}
}

func TestPluginCommandStoreConcurrentImportClaimHasOneWinner(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	now := time.Now().UTC()
	owner := uint(5)
	require.NoError(t, ctx.CreateRun(testRun("race-run", &owner, false, now), testOutput("race-run", now)))

	const callers = 8
	start := make(chan struct{})
	results := make([]plugin_commands.ImportClaimResult, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = ctx.ClaimImport(plugin_commands.ImportClaimRequest{
				ImportID: fmt.Sprintf("claim-%02d", i), RunID: "race-run", FileName: "same.bin",
				PluginGeneration: 1, CreatedByUserID: &owner, CreatedAt: now,
			})
		}(i)
	}
	close(start)
	wg.Wait()

	winnerID := ""
	created := 0
	for i := range results {
		require.NoError(t, errs[i])
		if results[i].Created {
			created++
			winnerID = results[i].ImportID
		}
	}
	require.Equal(t, 1, created)
	require.NotEmpty(t, winnerID)
	for _, result := range results {
		require.Equal(t, winnerID, result.ImportID)
	}
	var active, maps int64
	require.NoError(t, ctx.db.Model(&models.PluginCommandImport{}).Where("status IN ?", []string{plugin_commands.ImportStatusPending, plugin_commands.ImportStatusRunning}).Count(&active).Error)
	require.NoError(t, ctx.db.Model(&models.PluginCommandImportMap{}).Count(&maps).Error)
	require.Equal(t, int64(1), active)
	require.Equal(t, int64(1), maps)
}

func TestPluginCommandStoreImportTransitionsAndRecovery(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	now := time.Now().UTC()
	owner := uint(8)
	require.NoError(t, ctx.CreateRun(testRun("imports-run", &owner, false, now), testOutput("imports-run", now)))
	_, err := ctx.ClaimImport(plugin_commands.ImportClaimRequest{ImportID: "pending", RunID: "imports-run", FileName: "a", PluginGeneration: 1, CreatedByUserID: &owner, CreatedAt: now})
	require.NoError(t, err)
	_, err = ctx.ClaimImport(plugin_commands.ImportClaimRequest{ImportID: "running", RunID: "imports-run", FileName: "b", PluginGeneration: 1, CreatedByUserID: &owner, CreatedAt: now})
	require.NoError(t, err)
	won, err := ctx.MarkImportRunning("running", now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, won)
	won, err = ctx.MarkImportRunning("running", now.Add(2*time.Second))
	require.NoError(t, err)
	require.False(t, won)

	nonterminal, err := ctx.NonterminalImports()
	require.NoError(t, err)
	require.Len(t, nonterminal, 2)
	has, err := ctx.HasNonterminalImports("imports-run")
	require.NoError(t, err)
	require.True(t, has)

	require.NoError(t, ctx.InterruptNonterminalImports(now.Add(3*time.Second)))
	nonterminal, err = ctx.NonterminalImports()
	require.NoError(t, err)
	require.Empty(t, nonterminal)
	has, err = ctx.HasNonterminalImports("imports-run")
	require.NoError(t, err)
	require.False(t, has)

	claim, err := ctx.ClaimImport(plugin_commands.ImportClaimRequest{ImportID: "ignored", RunID: "imports-run", FileName: "a", PluginGeneration: 2, CreatedByUserID: &owner, CreatedAt: now.Add(4 * time.Second)})
	require.NoError(t, err)
	require.Equal(t, "pending", claim.ImportID)
	require.True(t, claim.Enqueue)
	resourceID := uint(101)
	won, err = ctx.FinishImport("pending", plugin_commands.ImportFinish{Status: plugin_commands.ImportStatusSucceeded, ResourceID: &resourceID, FinishedAt: now.Add(5 * time.Second)})
	require.NoError(t, err)
	require.False(t, won, "a pending import cannot succeed without first running")
	won, err = ctx.MarkImportRunning("pending", now.Add(5*time.Second))
	require.NoError(t, err)
	require.True(t, won)
	won, err = ctx.FinishImport("pending", plugin_commands.ImportFinish{Status: plugin_commands.ImportStatusSucceeded, ResourceID: &resourceID, FinishedAt: now.Add(6 * time.Second)})
	require.NoError(t, err)
	require.True(t, won)
	won, err = ctx.FinishImport("pending", plugin_commands.ImportFinish{Status: plugin_commands.ImportStatusFailed, Error: "stale", FinishedAt: now.Add(7 * time.Second)})
	require.NoError(t, err)
	require.False(t, won)
	entry, ok, err := ctx.ImportMap("imports-run", "a")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, plugin_commands.ImportStatusSucceeded, entry.Status)
	require.Equal(t, resourceID, *entry.ResourceID)
}

func TestPluginCommandStampedModelsInventoryIncludesOnlyActorOwnedRows(t *testing.T) {
	seen := map[string]bool{}
	for _, model := range stampedModels() {
		seen[fmt.Sprintf("%T", model)] = true
	}
	require.True(t, seen["*models.PluginCommandRun"])
	require.True(t, seen["*models.PluginCommandImport"])
	require.False(t, seen["*models.PluginCommandRunOutput"])
	require.False(t, seen["*models.PluginCommandImportMap"])
}

func TestPluginCommandStoreRejectsInvalidTerminalStatuses(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	now := time.Now().UTC()
	require.NoError(t, ctx.CreateRun(testRun("invalid-status", uintPtr(1), false, now), testOutput("invalid-status", now)))
	_, err := ctx.FinishRun("invalid-status", plugin_commands.RunFinish{Status: "complete", FinishedAt: now})
	require.Error(t, err)
	_, err = ctx.FinishImport("missing", plugin_commands.ImportFinish{Status: "complete", FinishedAt: now})
	require.Error(t, err)
	require.False(t, errors.Is(err, plugin_commands.ErrRunNotFound))
}
