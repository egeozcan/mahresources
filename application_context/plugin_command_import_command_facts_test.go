package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/plugin_commands"
	"mahresources/plugin_system"
)

func preparePluginCommandRetryAuthority(t *testing.T, ctx *MahresourcesContext, actorID uint) {
	t.Helper()
	if old := ctx.PluginManager(); old != nil {
		old.Close()
	}
	pluginDir := t.TempDir()
	writeConsentTestPlugin(t, pluginDir, "worker", `plugin = {
  name = "worker", version = "1.0", api_version = 1,
  capabilities = {"commands", "db:write"}
}
function init() end
`)
	pm, err := plugin_system.NewPluginManager(pluginDir)
	require.NoError(t, err)
	ctx.pluginManager = pm
	pm.SetConsentStore(&pluginConsentStore{ctx: ctx})
	t.Cleanup(pm.Close)

	actor := models.User{ID: actorID, Username: "plugin-retry-actor", Role: models.RoleUser}
	require.NoError(t, ctx.db.Create(&actor).Error)
	_, err = ctx.EnsurePluginStates()
	require.NoError(t, err)
	require.NoError(t, ctx.SetPluginEnabledWithOptions("worker", true, PluginEnableOptions{}))
}

func createPluginCommandRetryFile(t *testing.T, root, pluginName, runID, name string) string {
	t.Helper()
	dir := filepath.Join(root, "plugin_exchange", pluginName, runID)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("source bytes"), 0o600))
	return path
}

func seedPluginCommandRetryCandidate(t *testing.T, ctx *MahresourcesContext, actorID uint, runID, importID, fileName string) (string, string) {
	return seedPluginCommandRetryCandidateWithFields(t, ctx, actorID, runID, importID, fileName, `{"name":"asset"}`)
}

func seedPluginCommandRetryCandidateWithFields(t *testing.T, ctx *MahresourcesContext, actorID uint, runID, importID, fileName, fieldsJSON string) (string, string) {
	t.Helper()
	input, err := json.Marshal(pluginCommandImportReplayInput{
		RunID: runID, FileName: fileName, FieldsJSON: fieldsJSON, PluginGeneration: 1,
	})
	require.NoError(t, err)
	snapshot, err := ctx.JobService().Accept(ctx.jobDeps(), jobs.Acceptance{
		Kind: JobKindPluginCommandImport, KindVersion: jobPluginCommandVersion, State: jobs.StateQueued,
		Origin: "plugin-command", ActorUserID: &actorID, Title: "Import " + fileName,
		Replay: jobs.ReplayInput{Input: input},
	})
	require.NoError(t, err)
	require.NoError(t, ctx.db.Model(&models.Job{}).Where("id = ?", snapshot.ID).Update("state", jobs.StateFailed).Error)
	now := time.Now().UTC()
	require.NoError(t, ctx.db.Create(&models.PluginCommandImport{
		ID: importID, JobID: snapshot.ID, RunID: runID, FileName: fileName, FieldsJSON: fieldsJSON,
		PluginGeneration: 1, CreatedByUserId: &actorID, Status: plugin_commands.ImportStatusFailed,
		CreatedAt: now,
	}).Error)
	require.NoError(t, ctx.db.Create(&models.PluginCommandImportMap{
		RunID: runID, FileName: fileName, ImportID: importID, Status: plugin_commands.ImportStatusFailed,
	}).Error)
	require.NoError(t, setPluginCommandImportFactTx(ctx.db, models.PluginCommandImport{
		ID: importID, JobID: snapshot.ID, RunID: runID, FileName: fileName,
	}, fieldsJSON, true, now))
	return snapshot.ID, importID
}

func seedPluginCommandRunJob(t *testing.T, ctx *MahresourcesContext, actorID uint, runID string) string {
	t.Helper()
	input, err := json.Marshal(pluginCommandRunReplayInput{PluginName: "worker", CommandName: "download", ParamsJSON: `{}`})
	require.NoError(t, err)
	snapshot, err := ctx.JobService().Accept(ctx.jobDeps(), jobs.Acceptance{
		Kind: JobKindPluginCommand, KindVersion: jobPluginCommandVersion, State: jobs.StateQueued,
		Origin: "plugin-command", ActorUserID: &actorID, Title: "Plugin command " + runID,
		Replay: jobs.ReplayInput{Input: input},
	})
	require.NoError(t, err)
	require.NoError(t, ctx.db.Model(&models.Job{}).Where("id = ?", snapshot.ID).Update("state", jobs.StateFailed).Error)
	require.NoError(t, ctx.db.Create(&models.PluginCommandRun{
		ID: runID, JobID: snapshot.ID, PluginName: "worker", CommandName: "download", ParamsJSON: `{}`,
		Status: plugin_commands.RunStatusFailed, CreatedByUserId: &actorID, CreatedAt: time.Now().UTC(),
	}).Error)
	return snapshot.ID
}

func startPluginCommandRetryTestRuntime(t *testing.T, ctx *MahresourcesContext, root, commandPath string) {
	t.Helper()
	require.NoError(t, ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{root: root, commandPath: commandPath}))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
}

func TestPluginCommandImportRetrySelectorParityAndCurrentAuthority(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	service := jobs.NewService()
	ctx.SetJobService(service)
	const actorID = uint(9)
	preparePluginCommandRetryAuthority(t, ctx, actorID)
	root, commandPath := t.TempDir(), t.TempDir()
	startPluginCommandRetryTestRuntime(t, ctx, root, commandPath)

	runID := "retry-selector-parent"
	require.NoError(t, ctx.db.Create(&models.PluginCommandRun{
		ID: runID, PluginName: "worker", CommandName: "download", ParamsJSON: `{}`,
		Status: plugin_commands.RunStatusSucceeded, CreatedByUserId: ptrToUser(actorID),
		CreatedAt: time.Now().UTC(),
	}).Error)
	eligibleID, importID := seedPluginCommandRetryCandidate(t, ctx, actorID, runID, "retry-selector-import", "asset.bin")
	createPluginCommandRetryFile(t, root, "worker", runID, "asset.bin")
	runJobID := seedPluginCommandRunJob(t, ctx, actorID, "retry-selector-run-job")

	admin := jobs.Access{Administrator: true}
	assertAdapterSelectorMatchesCommands(t, ctx, admin, "retry-import")
	assertCommandListSummaryMatchDetails(t, ctx, admin, "retry-import")
	assertAdapterSelectorMatchesCommands(t, ctx, admin, jobs.CommandCancel)
	assertAdapterSelectorMatchesCommands(t, ctx, admin, "inspect")
	require.NotEqual(t, eligibleID, runJobID)

	userAccess := jobs.Access{UserID: actorID}
	page, err := service.List(ctx.jobDeps(), userAccess, jobs.Filter{Command: "retry-import"}, jobs.Cursor{}, 200)
	require.NoError(t, err)
	require.Empty(t, page.Jobs, "plugin command Jobs remain hidden by the admin visibility class")

	// Changing the submitted actor to a read-only role revokes Retry from both
	// detail Commands and the SQL selector.
	require.NoError(t, ctx.db.Model(&models.User{}).Where("id = ?", actorID).Update("role", models.RoleGuest).Error)
	assertAdapterSelectorMatchesCommands(t, ctx, admin, "retry-import")
	assertCommandListSummaryMatchDetails(t, ctx, admin, "retry-import")
	require.NoError(t, ctx.db.Model(&models.User{}).Where("id = ?", actorID).Update("role", models.RoleUser).Error)

	// The live source, map, and parent run are joined on every selector read.
	require.NoError(t, ctx.db.Model(&models.PluginCommandImportMap{}).Where("import_id = ?", importID).
		Update("status", plugin_commands.ImportStatusSucceeded).Error)
	assertAdapterSelectorMatchesCommands(t, ctx, admin, "retry-import")
	require.NoError(t, ctx.db.Model(&models.PluginCommandImportMap{}).Where("import_id = ?", importID).
		Update("status", plugin_commands.ImportStatusFailed).Error)
	require.NoError(t, ctx.db.Model(&models.PluginCommandRun{}).Where("id = ?", runID).
		Update("output_unverified", true).Error)
	assertAdapterSelectorMatchesCommands(t, ctx, admin, "retry-import")
	require.NoError(t, ctx.db.Model(&models.PluginCommandRun{}).Where("id = ?", runID).
		Update("output_unverified", false).Error)

	// Plugin enablement is database authority in addition to the in-memory
	// registration and capability set.
	require.NoError(t, ctx.db.Model(&models.PluginState{}).Where("plugin_name = ?", "worker").Update("enabled", false).Error)
	assertAdapterSelectorMatchesCommands(t, ctx, admin, "retry-import")
	assertCommandListSummaryMatchDetails(t, ctx, admin, "retry-import")
	require.NoError(t, ctx.db.Model(&models.PluginState{}).Where("plugin_name = ?", "worker").Update("enabled", true).Error)
	// A locally active runtime whose persisted DB fence was replaced cannot
	// keep advertising facts written under its former token.
	controller := ctx.pluginCommandController
	controller.mu.Lock()
	originalToken := controller.dbFence
	controller.mu.Unlock()
	require.NoError(t, ctx.db.Model(&models.JobRuntimeFence{}).
		Where("key = ?", pluginCommandRuntimeFenceKey).Update("token", "rotated-selector-token").Error)
	assertAdapterSelectorMatchesCommands(t, ctx, admin, "retry-import")
	assertCommandListSummaryMatchDetails(t, ctx, admin, "retry-import")
	require.NoError(t, ctx.db.Model(&models.JobRuntimeFence{}).
		Where("key = ?", pluginCommandRuntimeFenceKey).Update("token", originalToken).Error)
	assertAdapterSelectorMatchesCommands(t, ctx, admin, "retry-import")

	// A missing fact cannot be reconstructed while serving a list or summary.
	require.NoError(t, ctx.db.Where("import_id = ?", importID).Delete(&models.PluginCommandImportCommandFactGroup{}).Error)
	require.NoError(t, ctx.db.Where("import_id = ?", importID).Delete(&models.PluginCommandImportCommandFact{}).Error)
	assertAdapterSelectorMatchesCommands(t, ctx, admin, "retry-import")
	assertCommandListSummaryMatchDetails(t, ctx, admin, "retry-import")

	require.NoError(t, ctx.StopPluginCommands())
	assertAdapterSelectorMatchesCommands(t, ctx, admin, "retry-import")
	assertCommandListSummaryMatchDetails(t, ctx, admin, "retry-import")
}

func TestPluginCommandImportRetryFactsReconcileRemovedFileAfterRestart(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	service := jobs.NewService()
	ctx.SetJobService(service)
	const actorID = uint(9)
	preparePluginCommandRetryAuthority(t, ctx, actorID)
	root, commandPath := t.TempDir(), t.TempDir()
	runID := "retry-restart-parent"
	require.NoError(t, ctx.db.Create(&models.PluginCommandRun{
		ID: runID, PluginName: "worker", CommandName: "download", ParamsJSON: `{}`,
		Status: plugin_commands.RunStatusSucceeded, CreatedByUserId: ptrToUser(actorID),
		CreatedAt: time.Now().UTC(),
	}).Error)
	jobID, _ := seedPluginCommandRetryCandidate(t, ctx, actorID, runID, "retry-restart-import", "restart.bin")
	path := createPluginCommandRetryFile(t, root, "worker", runID, "restart.bin")
	startPluginCommandRetryTestRuntime(t, ctx, root, commandPath)

	commands, err := service.AdvertisedCommands(context.Background(), ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
	require.NoError(t, err)
	require.True(t, offersCommand(commands, "retry-import"))
	require.NoError(t, os.Remove(path))
	require.NoError(t, ctx.StopPluginCommands())
	ctx.pluginCommandController = nil // a process restart creates a new controller over the same DB and staging root
	require.NoError(t, ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{root: root, commandPath: commandPath}))

	commands, err = service.AdvertisedCommands(context.Background(), ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
	require.NoError(t, err)
	require.False(t, offersCommand(commands, "retry-import"), "bounded startup reconciliation marks an externally removed exchange file unavailable")
	assertAdapterSelectorMatchesCommands(t, ctx, jobs.Access{Administrator: true}, "retry-import")
}

func TestPluginCommandImportRetryFactsInvalidateOnFileDiscardAndRunCleanup(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	service := jobs.NewService()
	ctx.SetJobService(service)
	const actorID = uint(9)
	preparePluginCommandRetryAuthority(t, ctx, actorID)
	root, commandPath := t.TempDir(), t.TempDir()
	runID := "retry-discard-parent"
	require.NoError(t, ctx.db.Create(&models.PluginCommandRun{
		ID: runID, PluginName: "worker", CommandName: "download", ParamsJSON: `{}`,
		Status: plugin_commands.RunStatusSucceeded, CreatedByUserId: ptrToUser(actorID), CreatedAt: time.Now().UTC(),
	}).Error)
	fileJobID, _ := seedPluginCommandRetryCandidate(t, ctx, actorID, runID, "retry-discard-file", "discard.bin")
	runJobID, _ := seedPluginCommandRetryCandidate(t, ctx, actorID, runID, "retry-discard-run", "discard-run.bin")
	createPluginCommandRetryFile(t, root, "worker", runID, "discard.bin")
	createPluginCommandRetryFile(t, root, "worker", runID, "discard-run.bin")
	startPluginCommandRetryTestRuntime(t, ctx, root, commandPath)
	admin := jobs.Access{Administrator: true}
	assertAdapterSelectorMatchesCommands(t, ctx, admin, "retry-import")

	access := plugin_commands.Access{Administrator: true, PluginName: "worker", ActorUserID: ptrToUser(actorID)}
	require.NoError(t, ctx.DiscardCommandFile(access, runID, "discard.bin"))
	fileCommands, err := service.AdvertisedCommands(context.Background(), ctx.jobDeps(), admin, fileJobID)
	require.NoError(t, err)
	require.False(t, offersCommand(fileCommands, "retry-import"), "file discard invalidates its exact import fact")
	runCommands, err := service.AdvertisedCommands(context.Background(), ctx.jobDeps(), admin, runJobID)
	require.NoError(t, err)
	require.True(t, offersCommand(runCommands, "retry-import"), "discarding one file leaves another admitted exchange fact intact")

	require.NoError(t, ctx.DiscardCommandRun(access, runID))
	require.NoError(t, ctx.MarkRunExchangeRemoved(runID, time.Now().UTC()))
	runCommands, err = service.AdvertisedCommands(context.Background(), ctx.jobDeps(), admin, runJobID)
	require.NoError(t, err)
	require.False(t, offersCommand(runCommands, "retry-import"), "run cleanup invalidates every import file fact for that run")
	assertAdapterSelectorMatchesCommands(t, ctx, admin, "retry-import")
}

func TestPluginCommandImportRetryRequiresAvailableCanonicalReplayEnvelope(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	service := jobs.NewService()
	ctx.SetJobService(service)
	const actorID = uint(9)
	preparePluginCommandRetryAuthority(t, ctx, actorID)
	root, commandPath := t.TempDir(), t.TempDir()
	runID := "retry-replay-parent"
	require.NoError(t, ctx.db.Create(&models.PluginCommandRun{
		ID: runID, PluginName: "worker", CommandName: "download", ParamsJSON: `{}`,
		Status: plugin_commands.RunStatusSucceeded, CreatedByUserId: ptrToUser(actorID), CreatedAt: time.Now().UTC(),
	}).Error)
	forgottenJobID, _ := seedPluginCommandRetryCandidate(t, ctx, actorID, runID, "retry-replay-forget", "forget.bin")
	expiredJobID, _ := seedPluginCommandRetryCandidate(t, ctx, actorID, runID, "retry-replay-expire", "expire.bin")
	createPluginCommandRetryFile(t, root, "worker", runID, "forget.bin")
	createPluginCommandRetryFile(t, root, "worker", runID, "expire.bin")
	startPluginCommandRetryTestRuntime(t, ctx, root, commandPath)
	admin := jobs.Access{Administrator: true}
	assertAdapterSelectorMatchesCommands(t, ctx, admin, "retry-import")
	assertCommandListSummaryMatchDetails(t, ctx, admin, "retry-import")

	_, err := service.ForgetReplay(ctx.jobDeps(), admin, forgottenJobID)
	require.NoError(t, err)
	assertAdapterSelectorMatchesCommands(t, ctx, admin, "retry-import")
	assertCommandListSummaryMatchDetails(t, ctx, admin, "retry-import")
	commands, err := service.AdvertisedCommands(context.Background(), ctx.jobDeps(), admin, forgottenJobID)
	require.NoError(t, err)
	require.False(t, offersCommand(commands, "retry-import"), "a forgotten canonical input suppresses Retry even while the fact row remains")

	require.NoError(t, ctx.db.Model(&models.JobReplayEnvelope{}).Where("job_id = ?", expiredJobID).
		Update("expires_at", time.Now().UTC().Add(-time.Second)).Error)
	assertAdapterSelectorMatchesCommands(t, ctx, admin, "retry-import")
	assertCommandListSummaryMatchDetails(t, ctx, admin, "retry-import")
	commands, err = service.AdvertisedCommands(context.Background(), ctx.jobDeps(), admin, expiredJobID)
	require.NoError(t, err)
	require.False(t, offersCommand(commands, "retry-import"), "an expired canonical input suppresses Retry before the purge sweep runs")
}

func TestPluginCommandImportRetryFactsFollowReplayPurgeAndRetiredEnvelope(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	service := jobs.NewService()
	ctx.SetJobService(service)
	const actorID = uint(9)
	preparePluginCommandRetryAuthority(t, ctx, actorID)
	root, commandPath := t.TempDir(), t.TempDir()
	runID := "retry-fact-retirement-parent"
	require.NoError(t, ctx.db.Create(&models.PluginCommandRun{
		ID: runID, PluginName: "worker", CommandName: "download", ParamsJSON: `{}`,
		Status: plugin_commands.RunStatusSucceeded, CreatedByUserId: ptrToUser(actorID), CreatedAt: time.Now().UTC(),
	}).Error)
	group := models.Group{Name: "retry-fact-retirement-group"}
	require.NoError(t, ctx.db.Create(&group).Error)
	acceptedFields, err := json.Marshal(plugin_commands.ResourceFields{
		SeriesID: 314, GroupIDs: []uint{group.ID},
	})
	require.NoError(t, err)
	forgottenJobID, forgottenImportID := seedPluginCommandRetryCandidateWithFields(
		t, ctx, actorID, runID, "retry-fact-retirement-forgotten", "forget.bin", string(acceptedFields),
	)
	expiredJobID, expiredImportID := seedPluginCommandRetryCandidateWithFields(
		t, ctx, actorID, runID, "retry-fact-retirement-expired", "expire.bin", string(acceptedFields),
	)
	retainedJobID, retainedImportID := seedPluginCommandRetryCandidateWithFields(
		t, ctx, actorID, runID, "retry-fact-retirement-retained", "retained.bin", string(acceptedFields),
	)
	createPluginCommandRetryFile(t, root, "worker", runID, "forget.bin")
	createPluginCommandRetryFile(t, root, "worker", runID, "expire.bin")
	createPluginCommandRetryFile(t, root, "worker", runID, "retained.bin")
	startPluginCommandRetryTestRuntime(t, ctx, root, commandPath)

	// The canonical envelope remains the only source after epoch two. A stale
	// compatibility projection must not replace its validated group or series.
	require.NoError(t, ctx.db.Model(&models.JobWriterEpoch{}).
		Where("id = ?", models.JobWriterEpochRowID).
		Update("minimum_epoch", models.JobWriterEpochRetiredPlaintext).Error)
	staleFields := `{"series_id":999,"group_ids":[]}`
	require.NoError(t, ctx.db.Model(&models.PluginCommandImport{}).
		Where("id IN ?", []string{forgottenImportID, expiredImportID, retainedImportID}).
		Update("fields_json", staleFields).Error)
	require.NoError(t, ctx.StopPluginCommands())
	ctx.pluginCommandController = nil // Reconcile as a new process over retained legacy rows.
	startPluginCommandRetryTestRuntime(t, ctx, root, commandPath)

	var canonicalFact models.PluginCommandImportCommandFact
	require.NoError(t, ctx.db.Where("job_id = ?", forgottenJobID).First(&canonicalFact).Error)
	require.Equal(t, uint(314), canonicalFact.SeriesID)
	require.Equal(t, 1, canonicalFact.GroupCount)
	var canonicalGroup models.PluginCommandImportCommandFactGroup
	require.NoError(t, ctx.db.Where("import_id = ?", forgottenImportID).First(&canonicalGroup).Error)
	require.Equal(t, group.ID, canonicalGroup.GroupID)
	var retainedFact models.PluginCommandImportCommandFact
	require.NoError(t, ctx.db.Where("job_id = ?", retainedJobID).First(&retainedFact).Error)
	require.Equal(t, uint(314), retainedFact.SeriesID)

	admin := jobs.Access{Administrator: true}
	_, err = service.ForgetReplay(ctx.jobDeps(), admin, forgottenJobID)
	require.NoError(t, err)
	assertPluginCommandImportFactsAbsent(t, ctx, forgottenJobID, forgottenImportID)

	// Expiry purges the same derived values in the transaction that empties the
	// envelope, rather than leaving GroupID and SeriesID behind indefinitely.
	require.NoError(t, ctx.db.Model(&models.JobReplayEnvelope{}).Where("job_id = ?", expiredJobID).
		Update("expires_at", time.Now().UTC().Add(-time.Second)).Error)
	purged, err := service.PurgeExpiredReplay(ctx.jobDeps(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, purged)
	assertPluginCommandImportFactsAbsent(t, ctx, expiredJobID, expiredImportID)

	// Ordinary history retention also removes the projection when it deletes a
	// Job whose replay deadline has not yet arrived.
	now := time.Now().UTC()
	finishedAt, jobDeadline, replayDeadline := now.Add(-48*time.Hour), now.Add(-24*time.Hour), now.Add(24*time.Hour)
	require.NoError(t, ctx.db.Model(&models.Job{}).Where("id = ?", retainedJobID).
		Updates(map[string]any{"finished_at": finishedAt, "expires_at": jobDeadline}).Error)
	require.NoError(t, ctx.db.Model(&models.JobReplayEnvelope{}).Where("job_id = ?", retainedJobID).
		Update("expires_at", replayDeadline).Error)
	sweep, err := service.Sweep(ctx.jobDeps(), jobs.RetentionPolicy{}, jobs.SweepCursor{}, 100)
	require.NoError(t, err)
	require.Equal(t, 1, sweep.Pruned)
	assertPluginCommandImportFactsAbsent(t, ctx, retainedJobID, retainedImportID)

	// Source rows can outlive Job history. Startup reconciliation must neither
	// reopen stale plaintext nor recreate derived facts from a purged envelope.
	require.NoError(t, ctx.StopPluginCommands())
	ctx.pluginCommandController = nil
	startPluginCommandRetryTestRuntime(t, ctx, root, commandPath)
	assertPluginCommandImportFactsAbsent(t, ctx, forgottenJobID, forgottenImportID)
	assertPluginCommandImportFactsAbsent(t, ctx, expiredJobID, expiredImportID)
	assertPluginCommandImportFactsAbsent(t, ctx, retainedJobID, retainedImportID)
	commands, err := service.AdvertisedCommands(context.Background(), ctx.jobDeps(), admin, forgottenJobID)
	require.NoError(t, err)
	require.False(t, offersCommand(commands, "retry-import"))
	commands, err = service.AdvertisedCommands(context.Background(), ctx.jobDeps(), admin, expiredJobID)
	require.NoError(t, err)
	require.False(t, offersCommand(commands, "retry-import"))
}

func assertPluginCommandImportFactsAbsent(t *testing.T, ctx *MahresourcesContext, jobID, importID string) {
	t.Helper()
	var facts, groups int64
	require.NoError(t, ctx.db.Model(&models.PluginCommandImportCommandFact{}).Where("job_id = ?", jobID).Count(&facts).Error)
	require.NoError(t, ctx.db.Model(&models.PluginCommandImportCommandFactGroup{}).Where("import_id = ?", importID).Count(&groups).Error)
	require.Zero(t, facts, "purged replay input must not retain a derived command fact")
	require.Zero(t, groups, "purged replay input or pruned Job must not retain normalized group facts")
}

func TestPluginCommandImportRetrySelectorHonorsCurrentActorGroupScope(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	ctx.SetJobService(jobs.NewService())
	const actorID = uint(9)
	preparePluginCommandRetryAuthority(t, ctx, actorID)
	scopeRoot := models.Group{Name: "plugin-retry-scope-root"}
	require.NoError(t, ctx.db.Create(&scopeRoot).Error)
	inside := models.Group{Name: "plugin-retry-scope-child", OwnerId: &scopeRoot.ID}
	outside := models.Group{Name: "plugin-retry-scope-outside"}
	require.NoError(t, ctx.db.Create(&inside).Error)
	require.NoError(t, ctx.db.Create(&outside).Error)
	require.NoError(t, ctx.db.Model(&models.User{}).Where("id = ?", actorID).Update("scope_group_id", scopeRoot.ID).Error)

	runID := "retry-scope-parent"
	require.NoError(t, ctx.db.Create(&models.PluginCommandRun{
		ID: runID, PluginName: "worker", CommandName: "download", ParamsJSON: `{}`,
		Status: plugin_commands.RunStatusSucceeded, CreatedByUserId: ptrToUser(actorID), CreatedAt: time.Now().UTC(),
	}).Error)
	insideFields, err := json.Marshal(plugin_commands.ResourceFields{GroupIDs: []uint{inside.ID}})
	require.NoError(t, err)
	outsideFields, err := json.Marshal(plugin_commands.ResourceFields{GroupIDs: []uint{outside.ID}})
	require.NoError(t, err)
	insideJobID, insideImportID := seedPluginCommandRetryCandidateWithFields(t, ctx, actorID, runID, "retry-scope-inside", "inside.bin", string(insideFields))
	outsideJobID, _ := seedPluginCommandRetryCandidateWithFields(t, ctx, actorID, runID, "retry-scope-outside", "outside.bin", string(outsideFields))
	root, commandPath := t.TempDir(), t.TempDir()
	createPluginCommandRetryFile(t, root, "worker", runID, "inside.bin")
	createPluginCommandRetryFile(t, root, "worker", runID, "outside.bin")
	startPluginCommandRetryTestRuntime(t, ctx, root, commandPath)

	admin := jobs.Access{Administrator: true}
	assertAdapterSelectorMatchesCommands(t, ctx, admin, "retry-import")
	insideCommands, err := ctx.JobService().AdvertisedCommands(context.Background(), ctx.jobDeps(), admin, insideJobID)
	require.NoError(t, err)
	require.True(t, offersCommand(insideCommands, "retry-import"), "groups within the actor's current scope remain valid")
	outsideCommands, err := ctx.JobService().AdvertisedCommands(context.Background(), ctx.jobDeps(), admin, outsideJobID)
	require.NoError(t, err)
	require.False(t, offersCommand(outsideCommands, "retry-import"), "an out-of-scope group invalidates the accepted fields for Retry")
	require.NoError(t, ctx.db.Where("import_id = ?", insideImportID).Delete(&models.PluginCommandImportCommandFactGroup{}).Error)
	insideCommands, err = ctx.JobService().AdvertisedCommands(context.Background(), ctx.jobDeps(), admin, insideJobID)
	require.NoError(t, err)
	require.False(t, offersCommand(insideCommands, "retry-import"), "an incomplete normalized group fact fails closed")
	assertAdapterSelectorMatchesCommands(t, ctx, admin, "retry-import")
}

func TestPluginCommandImportRetryListAndSummaryQueryCountDoesNotScaleWithJobs(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	service := jobs.NewService()
	ctx.SetJobService(service)
	const actorID = uint(9)
	preparePluginCommandRetryAuthority(t, ctx, actorID)
	root, commandPath := t.TempDir(), t.TempDir()
	startPluginCommandRetryTestRuntime(t, ctx, root, commandPath)
	runID := "retry-query-count-run"
	require.NoError(t, ctx.db.Create(&models.PluginCommandRun{
		ID: runID, PluginName: "worker", CommandName: "download", ParamsJSON: `{}`,
		Status: plugin_commands.RunStatusSucceeded, CreatedByUserId: ptrToUser(actorID),
		CreatedAt: time.Now().UTC(),
	}).Error)
	for i := 0; i < 80; i++ {
		fileName := fmt.Sprintf("candidate-%03d.bin", i)
		importID := fmt.Sprintf("retry-query-%03d", i)
		seedPluginCommandRetryCandidate(t, ctx, actorID, runID, importID, fileName)
	}

	var selectorQueries atomic.Int64
	callbackName := "test:plugin-command-import-retry-selector-count"
	err := ctx.db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "plugin_command_import_command_facts") {
			selectorQueries.Add(1)
		}
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(callbackName) })

	page, err := service.List(ctx.jobDeps(), jobs.Access{Administrator: true}, jobs.Filter{Command: "retry-import"}, jobs.Cursor{}, 200)
	require.NoError(t, err)
	require.Len(t, page.Jobs, 80)
	smallListQueries := selectorQueries.Load()

	selectorQueries.Store(0)
	summary, err := service.Summary(ctx.jobDeps(), jobs.Access{Administrator: true}, jobs.Filter{Command: "retry-import"}, 24*time.Hour)
	require.NoError(t, err)
	require.EqualValues(t, 80, summary.Total)
	smallSummaryQueries := selectorQueries.Load()

	for i := 80; i < 160; i++ {
		fileName := fmt.Sprintf("candidate-%03d.bin", i)
		importID := fmt.Sprintf("retry-query-%03d", i)
		seedPluginCommandRetryCandidate(t, ctx, actorID, runID, importID, fileName)
	}
	selectorQueries.Store(0)
	page, err = service.List(ctx.jobDeps(), jobs.Access{Administrator: true}, jobs.Filter{Command: "retry-import"}, jobs.Cursor{}, 200)
	require.NoError(t, err)
	require.Len(t, page.Jobs, 160)
	require.EqualValues(t, smallListQueries, selectorQueries.Load(), "page selector SQL work stays constant as eligible imports double")

	selectorQueries.Store(0)
	summary, err = service.Summary(ctx.jobDeps(), jobs.Access{Administrator: true}, jobs.Filter{Command: "retry-import"}, 24*time.Hour)
	require.NoError(t, err)
	require.EqualValues(t, 160, summary.Total)
	require.EqualValues(t, smallSummaryQueries, selectorQueries.Load(), "summary selector SQL work stays constant as eligible imports double")
}

func ptrToUser(id uint) *uint { return &id }
