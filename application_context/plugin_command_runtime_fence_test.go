package application_context

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/constants"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/plugin_commands"
	"mahresources/plugin_system"
)

func TestPluginCommandDatabaseFenceIsBoundToOneStagingRootAndToken(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	first, err := ctx.acquirePluginCommandDBFence("/staging/a")
	require.NoError(t, err)
	second, err := ctx.acquirePluginCommandDBFence("/staging/a")
	require.NoError(t, err, "same-root takeover is allowed after the caller acquired RuntimeLease")
	require.NotEqual(t, first, second)
	err = ctx.releasePluginCommandDBFence(first)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no longer owned")
	_, err = ctx.acquirePluginCommandDBFence("/staging/b")
	require.ErrorIs(t, err, errPluginCommandFenceBoundToOtherRoot)
	require.NoError(t, ctx.releasePluginCommandDBFence(second))
	_, err = ctx.acquirePluginCommandDBFence("/staging/b")
	require.ErrorIs(t, err, errPluginCommandFenceBoundToOtherRoot,
		"a graceful release must preserve the database-to-staging-root binding")
}

// TestPluginCommandFenceOnAPrivateRootEndsWithItsOwner pins the one case in
// which the database fence moves to another staging root. A durable root keeps
// the database's retained outputs and import sources, so its binding outlives
// every restart; a process-private root (the MemoryFS default) is deleted with
// its process, so its binding ends once that process has released the fence or
// is proved gone. Proof is what the recorded owner allows: a process in this
// process table that no longer exists. An owner still running, from another
// boot (which may be another machine with this hostname), or on a host this
// process cannot inspect keeps the binding.
func TestPluginCommandFenceOnAPrivateRootEndsWithItsOwner(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	current := plugin_system.CurrentRuntimeIdentity()
	if current.Host == "" || current.BootSession == "" {
		t.Skip("this host cannot prove a recorded runtime gone")
	}
	setOwner := func(owner string) {
		t.Helper()
		require.NoError(t, ctx.db.Model(&models.JobRuntimeFence{}).
			Where("key = ?", pluginCommandRuntimeFenceKey).Update("owner", owner).Error)
	}
	boundRoot := func() models.JobRuntimeFence {
		t.Helper()
		var row models.JobRuntimeFence
		require.NoError(t, ctx.db.Where("key = ?", pluginCommandRuntimeFenceKey).First(&row).Error)
		return row
	}

	first, err := ctx.acquirePluginCommandDBFenceFor("/private/a", true)
	require.NoError(t, err)
	_, err = ctx.acquirePluginCommandDBFenceFor("/private/b", true)
	require.ErrorIs(t, err, errPluginCommandFenceBoundToOtherRoot, "a live owner keeps its private root")

	require.NoError(t, ctx.releasePluginCommandDBFence(first))
	second, err := ctx.acquirePluginCommandDBFenceFor("/private/b", true)
	require.NoError(t, err, "a released private root ends with its owner")
	require.NotEmpty(t, second)
	require.Equal(t, "/private/b", boundRoot().StagingRoot)

	gone := current
	gone.PID = 1 << 30
	setOwner(gone.String())
	_, err = ctx.acquirePluginCommandDBFenceFor("/private/c", true)
	require.NoError(t, err, "an owner whose process no longer exists is gone, so its private root is too")
	require.Equal(t, "/private/c", boundRoot().StagingRoot)

	anotherBoot := current
	anotherBoot.BootSession = "00000000-0000-4000-8000-000000000000"
	setOwner(anotherBoot.String())
	_, err = ctx.acquirePluginCommandDBFenceFor("/private/d", true)
	require.ErrorIs(t, err, errPluginCommandFenceBoundToOtherRoot, "an owner from another boot may be another machine with this hostname")

	setOwner(current.Host + "-elsewhere/" + current.BootSession + "/4242/0badc0de")
	_, err = ctx.acquirePluginCommandDBFenceFor("/private/d", true)
	require.ErrorIs(t, err, errPluginCommandFenceBoundToOtherRoot, "an owner on another host cannot be proved gone")

	setOwner("")
	_, err = ctx.acquirePluginCommandDBFenceFor("/private/d", true)
	require.ErrorIs(t, err, errPluginCommandFenceBoundToOtherRoot, "an unrecorded owner cannot be proved gone")

	setOwner(gone.String())
	_, err = ctx.acquirePluginCommandDBFenceFor("/durable/e", false)
	require.NoError(t, err, "a durable root may follow a private one on the same terms")
	row := boundRoot()
	require.Equal(t, "/durable/e", row.StagingRoot)
	require.False(t, row.StagingTemporary)
	require.Equal(t, current.String(), row.Owner)
}

// TestPluginCommandFenceOnAPrivateRootIsReleasedByStartingOnIt is the way out
// when the owner of a private root cannot be proved gone, as after a host
// reboot that skipped the clean stop: one server started with that root as its
// staging path takes the fence over on the root's own lease, keeps the binding
// private, and on its clean stop leaves it free for the next private root.
func TestPluginCommandFenceOnAPrivateRootIsReleasedByStartingOnIt(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	_, err := ctx.acquirePluginCommandDBFenceFor("/private/before-reboot", true)
	require.NoError(t, err)
	anotherBoot := plugin_system.CurrentRuntimeIdentity()
	anotherBoot.BootSession = "00000000-0000-4000-8000-000000000000"
	require.NoError(t, ctx.db.Model(&models.JobRuntimeFence{}).
		Where("key = ?", pluginCommandRuntimeFenceKey).Update("owner", anotherBoot.String()).Error)
	_, err = ctx.acquirePluginCommandDBFenceFor("/private/after-reboot", true)
	require.ErrorIs(t, err, errPluginCommandFenceOwnerNotStopped)

	token, err := ctx.acquirePluginCommandDBFenceFor("/private/before-reboot", false)
	require.NoError(t, err, "a server started on the bound root takes it over on the root's own lease")
	var row models.JobRuntimeFence
	require.NoError(t, ctx.db.Where("key = ?", pluginCommandRuntimeFenceKey).First(&row).Error)
	require.True(t, row.StagingTemporary, "starting on a private root by name keeps it private")
	require.NoError(t, ctx.releasePluginCommandDBFence(token))
	_, err = ctx.acquirePluginCommandDBFenceFor("/private/after-reboot", true)
	require.NoError(t, err)
}

// TestPluginCommandFenceRecordedBeforeOwnersIsKeptAsDurable covers a binding
// written before the fence recorded its owner and whether its root is private:
// nothing proves that root expendable, so it is kept, and a server started on
// that root by name takes it over as the /logs refusal says.
func TestPluginCommandFenceRecordedBeforeOwnersIsKeptAsDurable(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	require.NoError(t, ctx.db.Create(&models.JobRuntimeFence{
		Key: pluginCommandRuntimeFenceKey, Token: "left-by-an-earlier-release",
		StagingRoot: "/tmp/mahresources-plugin-commands-181321493", AcquiredAt: time.Now().UTC(),
	}).Error)
	_, err := ctx.acquirePluginCommandDBFenceFor("/tmp/mahresources-plugin-commands-new", true)
	require.ErrorIs(t, err, errPluginCommandFenceRootIsDurable)
	require.Contains(t, err.Error(), "/tmp/mahresources-plugin-commands-181321493")
	_, err = ctx.acquirePluginCommandDBFenceFor("/tmp/mahresources-plugin-commands-181321493", false)
	require.NoError(t, err)
}

// TestPluginCommandFenceOnADurableRootNamesTheRootToRestartWith pins the
// refusal a durable binding gives: it names the bound root and the flag that
// selects it, because no retry can change a binding that outlives restarts.
func TestPluginCommandFenceOnADurableRootNamesTheRootToRestartWith(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	token, err := ctx.acquirePluginCommandDBFenceFor("/durable/a", false)
	require.NoError(t, err)
	require.NoError(t, ctx.releasePluginCommandDBFence(token))
	_, err = ctx.acquirePluginCommandDBFenceFor("/private/b", true)
	require.ErrorIs(t, err, errPluginCommandFenceBoundToOtherRoot)
	require.ErrorIs(t, err, errPluginCommandFenceRootIsDurable)
	require.Contains(t, err.Error(), `"/durable/a"`)
}

// TestPluginCommandRuntimeOnAPrivateRootStartsAfterItsPredecessorStopped is the
// MemoryFS deployment with a persistent database: every process gets a new
// private staging root, and the next process must be able to run commands.
func TestPluginCommandRuntimeOnAPrivateRootStartsAfterItsPredecessorStopped(t *testing.T) {
	first := newPluginCommandStoreTestContext(t)
	require.NoError(t, first.StartPluginCommands(context.Background(), testPluginCommandSettings{
		root: t.TempDir(), commandPath: t.TempDir(), temporary: true,
	}))
	_, err := first.pluginCommandActive()
	require.NoError(t, err)
	require.NoError(t, first.StopPluginCommands())

	second := newPluginCommandContextOnSameDatabase(t, first)
	require.NoError(t, second.StartPluginCommands(context.Background(), testPluginCommandSettings{
		root: t.TempDir(), commandPath: t.TempDir(), temporary: true,
	}))
	t.Cleanup(func() { _ = second.StopPluginCommands() })
	_, err = second.pluginCommandActive()
	require.NoError(t, err, "a process-private staging root ends with the process that owned it")
}

// TestPluginCommandRuntimeOnAMovedDurableRootSaysWhichRootToUse covers the
// refusal an operator sees after changing -plugin-command-staging-path. No retry
// can move a binding meant to outlive restarts, so the runtime does not retry:
// /logs names the configured and the bound root and the flag, callers are told
// commands are unavailable with no time to try again, and the fence is asked
// once.
func TestPluginCommandRuntimeOnAMovedDurableRootSaysWhichRootToUse(t *testing.T) {
	first := newPluginCommandStoreTestContext(t)
	boundRoot := t.TempDir()
	require.NoError(t, first.StartPluginCommands(context.Background(), testPluginCommandSettings{
		root: boundRoot, commandPath: t.TempDir(),
	}))
	require.NoError(t, first.StopPluginCommands())

	second := newPluginCommandContextOnSameDatabase(t, first)
	configuredRoot := t.TempDir()
	var fenceAttempts atomic.Int32
	cfg := defaultPluginCommandControllerConfig()
	cfg.acquireBackoff = []time.Duration{5 * time.Millisecond}
	cfg.acquireDBFence = func(root string) (string, error) {
		fenceAttempts.Add(1)
		return second.acquirePluginCommandDBFenceFor(root, false)
	}
	require.NoError(t, second.startPluginCommandsWithConfig(context.Background(), testPluginCommandSettings{
		root: configuredRoot, commandPath: t.TempDir(),
	}, cfg))
	t.Cleanup(func() { _ = second.StopPluginCommands() })
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, int32(1), fenceAttempts.Load(), "a durable binding to another root is not retried")

	_, err := second.pluginCommandActive()
	var quarantined *plugin_commands.RuntimeQuarantinedError
	require.ErrorAs(t, err, &quarantined)
	require.Zero(t, quarantined.RetryAfter(), "no retry is scheduled, so none is promised")

	var logs []models.LogEntry
	require.NoError(t, second.db.Where("entity_type = ? AND level = ?", "plugin_command", models.LogLevelWarning).
		Order("id asc").Find(&logs).Error)
	require.Len(t, logs, 1)
	message := logs[0].Message
	require.Contains(t, message, boundRoot)
	require.Contains(t, message, configuredRoot)
	require.Contains(t, message, "-plugin-command-staging-path")
	require.NotContains(t, message, "automatic retry is active")
}

func newPluginCommandContextOnSameDatabase(t *testing.T, first *MahresourcesContext) *MahresourcesContext {
	t.Helper()
	sqlDB, err := first.db.DB()
	require.NoError(t, err)
	ctx := NewMahresourcesContext(afero.NewMemMapFs(), first.db, sqlx.NewDb(sqlDB, "sqlite3"), first.Config)
	keyring, err := jobs.LoadReplayKeyring(jobs.ReplayKeyConfig{Dialect: constants.DbTypeSqlite, Ephemeral: true})
	require.NoError(t, err)
	ctx.SetJobReplayKeyring(keyring)
	return ctx
}

func TestPluginCommandControllerAcquiresStagingLeaseBeforeDatabaseFence(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	settings := testPluginCommandSettings{root: t.TempDir(), commandPath: t.TempDir()}
	config := defaultPluginCommandControllerConfig()
	var order []string
	acquireLease := config.acquireLease
	config.acquireLease = func(root string) (*plugin_commands.RuntimeLease, error) {
		order = append(order, "staging")
		return acquireLease(root)
	}
	config.acquireDBFence = func(root string) (string, error) {
		require.Equal(t, "staging", order[len(order)-1])
		order = append(order, "database")
		return ctx.acquirePluginCommandDBFence(root)
	}
	require.NoError(t, ctx.startPluginCommandsWithConfig(context.Background(), settings, config))
	require.Equal(t, []string{"staging", "database"}, order)
	require.NoError(t, ctx.StopPluginCommands())
}

func TestPluginCommandStaleFenceTokenCannotStartAcceptedRun(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	ctx.SetJobService(jobs.NewService())
	root := t.TempDir()
	require.NoError(t, ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{root: root, commandPath: t.TempDir()}))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
	now := time.Now().UTC()
	record := testRun("stale-command-fence", nil, true, now)
	require.NoError(t, ctx.CreateRun(record, testOutput(record.ID, now)))
	controller := ctx.pluginCommandController
	controller.mu.Lock()
	oldToken := controller.dbFence
	controller.mu.Unlock()
	require.NotEmpty(t, oldToken)
	require.NoError(t, ctx.db.Model(&models.JobRuntimeFence{}).
		Where("key = ?", pluginCommandRuntimeFenceKey).Update("token", "rotated-token").Error)
	won, err := ctx.MarkRunRunning(record.ID, now.Add(time.Second))
	require.Error(t, err)
	require.Contains(t, err.Error(), "stale")
	require.False(t, won)
	stored, _, err := ctx.Run(record.ID)
	require.NoError(t, err)
	require.Equal(t, plugin_commands.RunStatusQueued, stored.Status)
}

func TestPluginCommandCanonicalJobFollowsRunUnderFencedClaim(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	service := jobs.NewService()
	ctx.SetJobService(service)
	root := t.TempDir()
	require.NoError(t, ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{root: root, commandPath: t.TempDir()}))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
	now := time.Now().UTC()
	record := testRun("canonical-command-lifecycle", nil, true, now)
	require.NoError(t, ctx.CreateRun(record, testOutput(record.ID, now)))
	stored, _, err := ctx.Run(record.ID)
	require.NoError(t, err)
	execution, claimed, err := ctx.claimPluginCommandJob(stored.JobID, JobKindPluginCommand, record.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	won, err := ctx.MarkRunRunning(record.ID, now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, won)
	stored, _, err = ctx.Run(record.ID)
	require.NoError(t, err)
	require.Equal(t, execution.ExecutionToken, stored.JobExecutionToken)
	won, err = ctx.FinishRun(record.ID, plugin_commands.RunFinish{Status: plugin_commands.RunStatusSucceeded,
		FinishedAt: now.Add(2 * time.Second), OutputTail: "completed"})
	require.NoError(t, err)
	require.True(t, won)
	snapshot, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, stored.JobID)
	require.NoError(t, err)
	require.Equal(t, jobs.StateSucceeded, snapshot.State)
	outputs, err := service.Outputs(ctx.jobDeps(), jobs.Access{Administrator: true}, stored.JobID)
	require.NoError(t, err)
	require.Len(t, outputs, 1)
	require.Equal(t, "command-history", outputs[0].Key)
}

func TestPluginCommandImportIsChildJobAndFinishesWithResourceOutput(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	service := jobs.NewService()
	ctx.SetJobService(service)
	root := t.TempDir()
	require.NoError(t, ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{root: root, commandPath: t.TempDir()}))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
	now := time.Now().UTC()
	actor := uint(7)
	run := testRun("import-parent-run", &actor, false, now)
	require.NoError(t, ctx.CreateRun(run, testOutput(run.ID, now)))
	storedRun, _, err := ctx.Run(run.ID)
	require.NoError(t, err)
	claim, err := ctx.ClaimImport(plugin_commands.ImportClaimRequest{ImportID: "import-child", RunID: run.ID,
		FileName: "asset.png", PluginGeneration: 1, CreatedByUserID: &actor, CreatedAt: now, FieldsJSON: `{}`})
	require.NoError(t, err)
	require.True(t, claim.Enqueue)
	require.NotEmpty(t, claim.JobID)
	var link models.JobLink
	require.NoError(t, ctx.db.Where("from_job_id = ? AND to_job_id = ?", storedRun.JobID, claim.JobID).First(&link).Error)
	require.Equal(t, string(jobs.LinkParentChild), link.Type)
	execution, claimed, err := ctx.claimPluginCommandJob(claim.JobID, JobKindPluginCommandImport, claim.ImportID)
	require.NoError(t, err)
	require.True(t, claimed)
	won, err := ctx.MarkImportRunning(claim.ImportID, now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, won)
	var storedImport models.PluginCommandImport
	require.NoError(t, ctx.db.First(&storedImport, "id = ?", claim.ImportID).Error)
	require.Equal(t, execution.ExecutionToken, storedImport.JobExecutionToken)
	resourceID := uint(123)
	won, err = ctx.FinishImport(claim.ImportID, plugin_commands.ImportFinish{Status: plugin_commands.ImportStatusSucceeded,
		ResourceID: &resourceID, FinishedAt: now.Add(2 * time.Second)})
	require.NoError(t, err)
	require.True(t, won)
	snapshot, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, claim.JobID)
	require.NoError(t, err)
	require.Equal(t, jobs.StateSucceeded, snapshot.State)
	outputs, err := service.Outputs(ctx.jobDeps(), jobs.Access{Administrator: true}, claim.JobID)
	require.NoError(t, err)
	require.Len(t, outputs, 1)
	require.Equal(t, "imported-resource", outputs[0].Key)
}

func TestPluginCommandImportClaimPersistsTokenAndRetryLineageAtomically(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	service := jobs.NewService()
	ctx.SetJobService(service)
	actor := uint(9)
	preparePluginCommandRetryAuthority(t, ctx, actor)
	root := t.TempDir()
	require.NoError(t, ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{root: root, commandPath: t.TempDir()}))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
	now := time.Now().UTC()
	run := testRun("import-retry-parent", &actor, false, now)
	require.NoError(t, ctx.CreateRun(run, testOutput(run.ID, now)))
	storedRun, _, err := ctx.Run(run.ID)
	require.NoError(t, err)
	_, claimedRun, err := ctx.claimPluginCommandJob(storedRun.JobID, JobKindPluginCommand, run.ID)
	require.NoError(t, err)
	require.True(t, claimedRun)
	won, err := ctx.MarkRunRunning(run.ID, now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, won)
	won, err = ctx.FinishRun(run.ID, plugin_commands.RunFinish{Status: plugin_commands.RunStatusSucceeded, FinishedAt: now.Add(2 * time.Second)})
	require.NoError(t, err)
	require.True(t, won)
	first, err := ctx.ClaimImport(plugin_commands.ImportClaimRequest{ImportID: "import-first", RunID: run.ID,
		FileName: "asset.bin", FieldsJSON: `{"name":"asset"}`, PluginGeneration: 1, CreatedByUserID: &actor, CreatedAt: now})
	require.NoError(t, err)
	firstExecution, claimed, err := ctx.claimPluginCommandJob(first.JobID, JobKindPluginCommandImport, first.ImportID)
	require.NoError(t, err)
	require.True(t, claimed)
	var beforeStart models.PluginCommandImport
	require.NoError(t, ctx.db.First(&beforeStart, "id = ?", first.ImportID).Error)
	require.Equal(t, plugin_commands.ImportStatusPending, beforeStart.Status)
	require.Equal(t, firstExecution.ExecutionToken, beforeStart.JobExecutionToken,
		"a crash before MarkImportRunning must leave a recoverable queued source with its canonical token")
	won, err = ctx.MarkImportRunning(first.ImportID, now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, won)
	won, err = ctx.FinishImport(first.ImportID, plugin_commands.ImportFinish{Status: plugin_commands.ImportStatusFailed,
		Error: "import failed", FinishedAt: now.Add(2 * time.Second)})
	require.NoError(t, err)
	require.True(t, won)
	exchangeFile := createPluginCommandRetryFile(t, root, run.PluginName, run.ID, "asset.bin")
	commands, err := service.AdvertisedCommands(context.Background(), ctx.jobDeps(), jobs.Access{Administrator: true}, first.JobID)
	require.NoError(t, err)
	keys := make(map[string]bool)
	for _, command := range commands {
		keys[command.Key] = true
	}
	require.True(t, keys["inspect"])
	require.True(t, keys["retry-import"], "retry requires the durable failed source, fields and successful parent run")
	require.NoError(t, os.Remove(exchangeFile))
	commands, err = service.AdvertisedCommands(context.Background(), ctx.jobDeps(), jobs.Access{Administrator: true}, first.JobID)
	require.NoError(t, err)
	keys = make(map[string]bool)
	for _, command := range commands {
		keys[command.Key] = true
	}
	require.True(t, keys["retry-import"], "interactive selectors use the durable fact without per-Job file probes")
	snapshot, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, first.JobID)
	require.NoError(t, err)
	_, err = service.ExecuteCommand(context.Background(), ctx.jobDeps(), jobs.CommandRequest{
		JobID: first.JobID, Key: "retry-import", IdempotencyKey: "missing-exchange-file",
		ExpectedVersion: snapshot.Version, Actor: jobs.Access{Administrator: true},
	})
	require.ErrorIs(t, err, jobs.ErrCommandNotAdvertised,
		"pre-execute revalidation must reject a physical file that disappeared outside the runtime lifecycle")
	commands, err = service.AdvertisedCommands(context.Background(), ctx.jobDeps(), jobs.Access{Administrator: true}, first.JobID)
	require.NoError(t, err)
	keys = make(map[string]bool)
	for _, command := range commands {
		keys[command.Key] = true
	}
	require.False(t, keys["retry-import"], "failed physical-file revalidation invalidates the durable availability fact")
	// Restore the file so the later import lineage operation is testing its own
	// transactional claim behaviour rather than a missing staging artifact.
	require.NoError(t, os.WriteFile(exchangeFile, []byte("source bytes"), 0o600))
	second, err := ctx.ClaimImport(plugin_commands.ImportClaimRequest{ImportID: "import-second", RunID: run.ID,
		FileName: "asset.bin", FieldsJSON: `{"name":"asset"}`, PluginGeneration: 2, CreatedByUserID: &actor, CreatedAt: now.Add(3 * time.Second)})
	require.NoError(t, err)
	require.True(t, second.Enqueue)
	require.NotEqual(t, first.JobID, second.JobID)
	var retry models.JobLink
	require.NoError(t, ctx.db.Where("from_job_id = ? AND to_job_id = ? AND type = ?", second.JobID, first.JobID, string(jobs.LinkRetryOf)).First(&retry).Error)
	var prior, successor models.PluginCommandImport
	require.NoError(t, ctx.db.First(&prior, "id = ?", first.ImportID).Error)
	require.NoError(t, ctx.db.First(&successor, "id = ?", second.ImportID).Error)
	require.Equal(t, first.JobID, prior.JobID)
	require.Equal(t, second.JobID, successor.JobID)
}

func TestPluginCommandImportRetryRecheckUsesOneConnection(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	service := jobs.NewService()
	ctx.SetJobService(service)
	actor := uint(9)
	preparePluginCommandRetryAuthority(t, ctx, actor)
	root := t.TempDir()
	require.NoError(t, ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{root: root, commandPath: t.TempDir()}))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
	now := time.Now().UTC()
	run := testRun("one-connection-import-retry", &actor, false, now)
	require.NoError(t, ctx.CreateRun(run, testOutput(run.ID, now)))
	storedRun, _, err := ctx.Run(run.ID)
	require.NoError(t, err)
	_, claimedRun, err := ctx.claimPluginCommandJob(storedRun.JobID, JobKindPluginCommand, run.ID)
	require.NoError(t, err)
	require.True(t, claimedRun)
	won, err := ctx.MarkRunRunning(run.ID, now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, won)
	won, err = ctx.FinishRun(run.ID, plugin_commands.RunFinish{Status: plugin_commands.RunStatusSucceeded, FinishedAt: now.Add(2 * time.Second)})
	require.NoError(t, err)
	require.True(t, won)
	claim, err := ctx.ClaimImport(plugin_commands.ImportClaimRequest{
		ImportID: "one-connection-import-retry", RunID: run.ID, FileName: "asset.bin",
		FieldsJSON: `{"name":"asset"}`, PluginGeneration: 1, CreatedByUserID: &actor, CreatedAt: now,
	})
	require.NoError(t, err)
	execution, claimed, err := ctx.claimPluginCommandJob(claim.JobID, JobKindPluginCommandImport, claim.ImportID)
	require.NoError(t, err)
	require.True(t, claimed)
	var source models.PluginCommandImport
	require.NoError(t, ctx.db.First(&source, "id = ?", claim.ImportID).Error)
	require.Equal(t, execution.ExecutionToken, source.JobExecutionToken)
	won, err = ctx.MarkImportRunning(claim.ImportID, now.Add(3*time.Second))
	require.NoError(t, err)
	require.True(t, won)
	won, err = ctx.FinishImport(claim.ImportID, plugin_commands.ImportFinish{
		Status: plugin_commands.ImportStatusFailed, Error: "transient import failure", FinishedAt: now.Add(4 * time.Second),
	})
	require.NoError(t, err)
	require.True(t, won)

	createPluginCommandRetryFile(t, root, run.PluginName, run.ID, "asset.bin")
	snapshot, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, claim.JobID)
	require.NoError(t, err)
	sqlDB, err := ctx.db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	require.NoError(t, ctx.db.Exec("PRAGMA busy_timeout = 100").Error)

	type result struct {
		command jobs.CommandResult
		err     error
	}
	done := make(chan result, 1)
	go func() {
		command, executeErr := service.ExecuteCommand(context.Background(), ctx.jobDeps(), jobs.CommandRequest{
			JobID: claim.JobID, Key: "retry-import", IdempotencyKey: "one-connection-retry",
			ExpectedVersion: snapshot.Version, Actor: jobs.Access{Administrator: true},
		})
		done <- result{command: command, err: executeErr}
	}()
	select {
	case got := <-done:
		if got.err != nil && !errors.Is(got.err, jobs.ErrCommandFailed) {
			t.Fatalf("ExecuteCommand returned %v (result %+v), want completion or a command execution failure", got.err, got.command)
		}
	case <-time.After(3 * time.Second):
		// Give a regression a chance to leave its one-connection wait and finish
		// after the assertion, so teardown does not strand a goroutine.
		sqlDB.SetMaxOpenConns(4)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
		t.Fatal("ExecuteCommand deadlocked while rechecking Retry with the only database connection held")
	}
}

func TestPluginCommandRunClaimPersistsTokenBeforeRunnerStart(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	ctx.SetJobService(jobs.NewService())
	require.NoError(t, ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{root: t.TempDir(), commandPath: t.TempDir()}))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
	now := time.Now().UTC()
	run := testRun("claim-before-fork", nil, true, now)
	require.NoError(t, ctx.CreateRun(run, testOutput(run.ID, now)))
	stored, _, err := ctx.Run(run.ID)
	require.NoError(t, err)
	execution, claimed, err := ctx.claimPluginCommandJob(stored.JobID, JobKindPluginCommand, run.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	stored, _, err = ctx.Run(run.ID)
	require.NoError(t, err)
	require.Equal(t, plugin_commands.RunStatusQueued, stored.Status)
	require.Equal(t, execution.ExecutionToken, stored.JobExecutionToken,
		"a crash before MarkRunRunning must leave a recoverable queued source with its canonical token")
	commands, err := ctx.JobService().AdvertisedCommands(context.Background(), ctx.jobDeps(), jobs.Access{Administrator: true}, stored.JobID)
	require.NoError(t, err)
	keys := make(map[string]bool)
	for _, command := range commands {
		keys[command.Key] = true
	}
	require.True(t, keys[jobs.CommandCancel])
	require.True(t, keys["inspect"])
	require.False(t, keys[jobs.CommandRetry])
	require.False(t, keys[jobs.CommandRepeat])
}

func TestPluginCommandQueuedCanonicalCancelAlsoCancelsSource(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	service := jobs.NewService()
	ctx.SetJobService(service)
	require.NoError(t, ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{root: t.TempDir(), commandPath: t.TempDir()}))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
	now := time.Now().UTC()
	run := testRun("canonical-queued-cancel", nil, true, now)
	require.NoError(t, ctx.CreateRun(run, testOutput(run.ID, now)))
	stored, _, err := ctx.Run(run.ID)
	require.NoError(t, err)
	snapshot, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, stored.JobID)
	require.NoError(t, err)
	_, err = service.ExecuteCommand(context.Background(), ctx.jobDeps(), jobs.CommandRequest{
		JobID: stored.JobID, Key: jobs.CommandCancel, IdempotencyKey: "cancel-queued-command",
		ExpectedVersion: uint64(snapshot.Version), Actor: jobs.Access{Administrator: true},
	})
	require.NoError(t, err)
	stored, _, err = ctx.Run(run.ID)
	require.NoError(t, err)
	require.Equal(t, plugin_commands.RunStatusCancelled, stored.Status)
	require.True(t, stored.CancelRequested)
	snapshot, err = service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, stored.JobID)
	require.NoError(t, err)
	require.Equal(t, jobs.StateCancelled, snapshot.State)
}

func TestPluginCommandKindsAreAdminOnlyAcrossJobReadSeams(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	service := jobs.NewService()
	ctx.SetJobService(service)
	now := time.Now().UTC()
	actor := uint(42)
	run := testRun("admin-only-command", &actor, false, now)
	require.NoError(t, ctx.CreateRun(run, testOutput(run.ID, now)))
	command, _, err := ctx.Run(run.ID)
	require.NoError(t, err)
	importClaim, err := ctx.ClaimImport(plugin_commands.ImportClaimRequest{ImportID: "admin-only-import", RunID: run.ID,
		FileName: "file.bin", FieldsJSON: `{"name":"file"}`, CreatedByUserID: &actor, CreatedAt: now})
	require.NoError(t, err)
	viewer := jobs.Access{UserID: actor}
	admin := jobs.Access{Administrator: true}
	for _, jobID := range []string{command.JobID, importClaim.JobID} {
		if _, err := service.Get(ctx.jobDeps(), viewer, jobID); !errors.Is(err, jobs.ErrNotFound) {
			t.Fatalf("viewer Get(%s) error = %v, want not found", jobID, err)
		}
		if _, err := service.Timeline(ctx.jobDeps(), viewer, jobID, 0, 20); !errors.Is(err, jobs.ErrNotFound) {
			t.Fatalf("viewer Timeline(%s) error = %v, want not found", jobID, err)
		}
		if _, err := service.Outputs(ctx.jobDeps(), viewer, jobID); !errors.Is(err, jobs.ErrNotFound) {
			t.Fatalf("viewer Outputs(%s) error = %v, want not found", jobID, err)
		}
		if _, err := service.Get(ctx.jobDeps(), admin, jobID); err != nil {
			t.Fatalf("administrator Get(%s): %v", jobID, err)
		}
	}
	page, err := service.List(ctx.jobDeps(), viewer, jobs.Filter{}, jobs.Cursor{}, 20)
	require.NoError(t, err)
	require.Empty(t, page.Jobs)
	adminPage, err := service.List(ctx.jobDeps(), admin, jobs.Filter{}, jobs.Cursor{}, 20)
	require.NoError(t, err)
	require.Len(t, adminPage.Jobs, 2)
	viewerSummary, err := service.Summary(ctx.jobDeps(), viewer, jobs.Filter{}, time.Hour)
	require.NoError(t, err)
	require.Zero(t, viewerSummary.Total)
	adminSummary, err := service.Summary(ctx.jobDeps(), admin, jobs.Filter{}, time.Hour)
	require.NoError(t, err)
	require.EqualValues(t, 2, adminSummary.Total)
}

func TestPluginCommandHeartbeatKeepsLongRunningCanonicalClaimAlive(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	ctx.SetJobService(jobs.NewService())
	require.NoError(t, ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{root: t.TempDir(), commandPath: t.TempDir()}))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
	now := time.Now().UTC()
	run := testRun("long-running-command", nil, true, now)
	require.NoError(t, ctx.CreateRun(run, testOutput(run.ID, now)))
	stored, _, err := ctx.Run(run.ID)
	require.NoError(t, err)
	execution, claimed, err := ctx.claimPluginCommandJob(stored.JobID, JobKindPluginCommand, run.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, ctx.db.Model(&models.JobClaim{}).Where("job_id = ?", execution.JobID).
		Update("lease_expires_at", time.Now().UTC().Add(-time.Minute)).Error)
	require.NoError(t, ctx.heartbeatPluginCommandJob(execution))
	var claim models.JobClaim
	require.NoError(t, ctx.db.First(&claim, "job_id = ?", execution.JobID).Error)
	require.Equal(t, models.JobClaimStateHeld, claim.State)
	require.True(t, claim.LeaseExpiresAt.After(time.Now().Add(time.Minute)), "lease expires at %s", claim.LeaseExpiresAt)
	// The same callback cannot keep a replacement controller's claim alive.
	controller := ctx.pluginCommandController
	controller.mu.Lock()
	oldToken := controller.dbFence
	controller.mu.Unlock()
	require.NoError(t, ctx.db.Transaction(func(tx *gorm.DB) error {
		return tx.Model(&models.JobRuntimeFence{}).Where("key = ?", pluginCommandRuntimeFenceKey).Update("token", "new-owner").Error
	}))
	require.Error(t, ctx.heartbeatPluginCommandJob(execution))
	require.NotEmpty(t, oldToken)
}

// TestPluginCommandStoreNamesAReleasedFenceAsLost pins the error the store's
// fenced writes give once this process no longer holds the fence: the import
// terminal writer stops retrying on exactly this, so it must survive wrapping.
func TestPluginCommandStoreNamesAReleasedFenceAsLost(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	require.NoError(t, ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{
		root: t.TempDir(), commandPath: t.TempDir(),
	}))
	require.NoError(t, ctx.StopPluginCommands())
	_, err := ctx.FinishImport("released-fence-import", plugin_commands.ImportFinish{
		Status: plugin_commands.ImportStatusFailed, FinishedAt: time.Now().UTC(),
	})
	require.ErrorIs(t, err, plugin_commands.ErrRuntimeFenceLost)
}

func TestPluginCommandSettingsReportAPrivateStagingRoot(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	require.False(t, pluginCommandStagingTemporary(ctx.PluginCommandSettings()))
	ctx.Config.PluginCommandStagingTemporary = true
	require.True(t, pluginCommandStagingTemporary(ctx.PluginCommandSettings()),
		"the MemoryFS default root must reach the fence as private to this process")
}
