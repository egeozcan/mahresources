package application_context

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/afero"
	"gorm.io/gorm"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/plugin_commands"
)

func commandImportContext(t *testing.T) (*MahresourcesContext, *models.User, *models.Group, uint64) {
	t.Helper()
	pluginDir := t.TempDir()
	writeConsentTestPlugin(t, pluginDir, "commanded", `plugin = {
  name="commanded", version="1", api_version=1,
  capabilities={"commands","db:write"},
  commands={{name="convert",argv={"tool","{{input}}"}}}
}
function init() end`)
	ctx := createTestContextWithPlugins(t, pluginDir)
	t.Cleanup(ctx.PluginManager().Close)
	if err := ctx.db.AutoMigrate(&models.PluginCommandRun{}, &models.PluginCommandRunOutput{}, &models.PluginCommandImport{}, &models.PluginCommandImportMap{}); err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.EnsurePluginStates(); err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetPluginEnabledWithOptions("commanded", true, PluginEnableOptions{ConfirmCommands: true}); err != nil {
		t.Fatal(err)
	}
	root := &models.Group{Name: "import-root"}
	if err := ctx.db.Create(root).Error; err != nil {
		t.Fatal(err)
	}
	user, err := ctx.CreateUser(&UserInput{Username: "importer", Password: "password1", Role: models.RoleUser, ScopeGroupId: &root.ID})
	if err != nil {
		t.Fatal(err)
	}
	plugins := ctx.PluginManager().Plugins()
	if len(plugins) != 1 {
		t.Fatalf("plugins = %+v", plugins)
	}
	return ctx, user, root, plugins[0].Generation
}

func createImportClaimForTest(t *testing.T, ctx *MahresourcesContext, actor *models.User, id, runID, name string, generation uint64) {
	t.Helper()
	bound := ctx.WithPrincipal(auth.FromUser(actor))
	claim := &models.PluginCommandImport{ID: id, RunID: runID, FileName: name, PluginGeneration: generation, CreatedByUserId: &actor.ID, Status: plugin_commands.ImportStatusRunning}
	if err := bound.db.Create(claim).Error; err != nil {
		t.Fatal(err)
	}
}

func TestPluginCommandImportValidatesActorGenerationCapabilitiesAndScope(t *testing.T) {
	ctx, actor, root, generation := commandImportContext(t)
	validation := plugin_commands.ImportValidation{PluginName: "commanded", PluginGeneration: generation, ActorUserID: &actor.ID, Fields: plugin_commands.ResourceFields{GroupIDs: []uint{root.ID}}}
	if err := ctx.ValidateImport(validation); err != nil {
		t.Fatalf("valid import refused: %v", err)
	}
	validation.PluginGeneration++
	if err := ctx.ValidateImport(validation); err == nil {
		t.Fatal("stale generation accepted")
	}
	validation.PluginGeneration = generation
	outside := &models.Group{Name: "outside"}
	if err := ctx.db.Create(outside).Error; err != nil {
		t.Fatal(err)
	}
	validation.Fields.GroupIDs = []uint{outside.ID}
	if err := ctx.ValidateImport(validation); err == nil {
		t.Fatal("out-of-scope group accepted")
	}
	missing := uint(999999)
	validation.ActorUserID = &missing
	validation.Fields.GroupIDs = nil
	if err := ctx.ValidateImport(validation); err == nil {
		t.Fatal("missing actor accepted")
	}
	guest, err := ctx.CreateUser(&UserInput{Username: "import-guest", Password: "password1", Role: models.RoleGuest, ScopeGroupId: &root.ID})
	if err != nil {
		t.Fatal(err)
	}
	validation.ActorUserID = &guest.ID
	validation.Fields.GroupIDs = []uint{root.ID}
	if err := ctx.ValidateImport(validation); err == nil {
		t.Fatal("guest actor accepted")
	}
	disabled, err := ctx.CreateUser(&UserInput{Username: "import-disabled", Password: "password1", Role: models.RoleEditor})
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.db.Model(&models.User{}).Where("id = ?", disabled.ID).Update("disabled", true).Error; err != nil {
		t.Fatal(err)
	}
	validation.ActorUserID = &disabled.ID
	validation.Fields.GroupIDs = nil
	if err := ctx.ValidateImport(validation); err == nil {
		t.Fatal("disabled actor accepted")
	}

	createImportClaimForTest(t, ctx, actor, "claim-deleted", "run-deleted", "result.bin", generation)
	if err := ctx.db.Model(&models.PluginCommandImport{}).Where("id = ?", "claim-deleted").Update("status", plugin_commands.ImportStatusPending).Error; err != nil {
		t.Fatal(err)
	}
	if err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.PluginCommandImport{}).Where("id = ?", "claim-deleted").Update("created_by_user_id", nil).Error; err != nil {
			return err
		}
		return tx.Delete(&models.User{}, actor.ID).Error
	}); err != nil {
		t.Fatal(err)
	}
	var claim models.PluginCommandImport
	if err := ctx.db.First(&claim, "id = ?", "claim-deleted").Error; err != nil {
		t.Fatal(err)
	}
	if claim.CreatedByUserId != nil {
		t.Fatalf("deleted actor was not nulled from import claim: %v", claim.CreatedByUserId)
	}
	if won, err := ctx.MarkImportRunning(claim.ID, time.Now().UTC()); err != nil || won {
		t.Fatalf("deleted actor claim started: won=%v err=%v", won, err)
	}
}

func testImportSource(t *testing.T, path, scratch, name, runID, importID string) (plugin_commands.ImportSource, func()) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	factory := func() (*os.File, func() error, error) {
		temp, err := os.CreateTemp(scratch, "upload-")
		if err != nil {
			return nil, nil, err
		}
		return temp, func() error { return os.Remove(temp.Name()) }, nil
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	return plugin_commands.ImportSource{
		File: file, SourceSize: info.Size(), CreateScratch: factory, FileName: name, RunID: runID, ImportID: importID,
	}, func() { _ = file.Close() }
}

func TestContextImportFileRequiresExactSourceSize(t *testing.T) {
	for _, tc := range []struct {
		name      string
		body      string
		expected  int64
		cancelled bool
		want      string
		wantError string
	}{
		{name: "exact", body: "payload", expected: 7, want: "payload"},
		{name: "short", body: "short", expected: 6, wantError: "changed during snapshot"},
		{name: "long", body: "too-long", expected: 3, wantError: "changed during snapshot"},
		{name: "cancelled", body: "payload", expected: 7, cancelled: true, wantError: context.Canceled.Error()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "source")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			file, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			ctx, cancel := context.WithCancel(context.Background())
			if tc.cancelled {
				cancel()
			} else {
				defer cancel()
			}
			body, err := io.ReadAll(newContextImportFile(file, ctx, tc.expected))
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("ReadAll error = %v, want %q", err, tc.wantError)
				}
				return
			}
			if err != nil || string(body) != tc.want {
				t.Fatalf("ReadAll = %q, %v", body, err)
			}
		})
	}
}

func TestPluginCommandImportAcceptsImplicitRootInAuthOffMode(t *testing.T) {
	ctx, _, _, generation := commandImportContext(t)
	ctx.Config.AuthEnabled = false
	root, err := ctx.EnsureRootAdmin()
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.ValidateImport(plugin_commands.ImportValidation{
		PluginName: "commanded", PluginGeneration: generation, ActorUserID: nil,
	}); err != nil {
		t.Fatalf("auth-off import refused: %v", err)
	}

	const importID = "claim-auth-off"
	claim := &models.PluginCommandImport{
		ID: importID, RunID: "run-auth-off", FileName: "result.bin",
		PluginGeneration: generation, Status: plugin_commands.ImportStatusRunning,
	}
	if err := ctx.db.Create(claim).Error; err != nil {
		t.Fatal(err)
	}
	if claim.CreatedByUserId == nil || *claim.CreatedByUserId != root.ID {
		t.Fatalf("claim creator = %v, want root %d", claim.CreatedByUserId, root.ID)
	}

	scratch := t.TempDir()
	sourcePath := filepath.Join(scratch, "outer-snapshot")
	if err := os.WriteFile(sourcePath, []byte("auth-off output"), 0o600); err != nil {
		t.Fatal(err)
	}
	source, closeSource := testImportSource(t, sourcePath, scratch, "result.bin", "run-auth-off", importID)
	defer closeSource()
	resourceID, err := ctx.ImportResource(context.Background(), source, plugin_commands.ResourceFields{Name: "auth-off import"}, scratch)
	if err != nil {
		t.Fatal(err)
	}
	var resource models.Resource
	if err := ctx.db.First(&resource, resourceID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.CreatedByUserId == nil || *resource.CreatedByUserId != root.ID {
		t.Fatalf("resource creator = %v, want root %d", resource.CreatedByUserId, root.ID)
	}
}

func TestPluginCommandImportMemoryFSStoresSnapshotBytes(t *testing.T) {
	ctx, _, _, generation := commandImportContext(t)
	actor, err := ctx.CreateUser(&UserInput{Username: "memory-importer", Password: "password1", Role: models.RoleEditor})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("memory-fs-import-bytes")
	scratch := t.TempDir()
	sourcePath := filepath.Join(scratch, "outer-snapshot")
	if err := os.WriteFile(sourcePath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	createImportClaimForTest(t, ctx, actor, "claim-memory", "run-memory", "result.bin", generation)
	series, err := ctx.CreateSeries(&query_models.SeriesCreator{Name: "Command Series", Slug: "command-series", Meta: `{"shared":true}`})
	if err != nil {
		t.Fatal(err)
	}
	fields := plugin_commands.ResourceFields{Name: "from command", Meta: map[string]any{"source": "command"}, SeriesID: series.ID}
	if err := ctx.ValidateImport(plugin_commands.ImportValidation{PluginName: "commanded", PluginGeneration: generation, ActorUserID: &actor.ID, Fields: fields}); err != nil {
		t.Fatal(err)
	}
	source, closeSource := testImportSource(t, sourcePath, scratch, "result.bin", "run-memory", "claim-memory")
	defer closeSource()
	id, err := ctx.ImportResource(context.Background(), source, fields, scratch)
	if err != nil {
		t.Fatal(err)
	}
	var resource models.Resource
	if err := ctx.db.First(&resource, id).Error; err != nil {
		t.Fatal(err)
	}
	stored, err := afero.ReadFile(ctx.fs, resource.Location)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != string(payload) {
		t.Fatalf("stored = %q", stored)
	}
	if resource.CreatedByUserId == nil || *resource.CreatedByUserId != actor.ID {
		t.Fatalf("creator = %v", resource.CreatedByUserId)
	}
	if resource.SeriesID == nil || *resource.SeriesID != series.ID {
		t.Fatalf("series = %v, want %d", resource.SeriesID, series.ID)
	}
	if string(resource.Meta) != `{"shared":true,"source":"command"}` && string(resource.Meta) != `{"source":"command","shared":true}` {
		t.Fatalf("effective meta = %s", resource.Meta)
	}
}

type commandImportTestSettings struct{ root string }

func (s commandImportTestSettings) StagingRoot() string              { return s.root }
func (s commandImportTestSettings) PendingPerPluginLimit() int       { return 100 }
func (s commandImportTestSettings) PerRunQuota() int64               { return 8 << 30 }
func (s commandImportTestSettings) GlobalStagingQuota() int64        { return 50 << 30 }
func (s commandImportTestSettings) ExchangeRetention() time.Duration { return time.Hour }
func (s commandImportTestSettings) OutputRetention() time.Duration   { return time.Hour }
func (s commandImportTestSettings) CommandPath() string              { return "/bin" }

type commandImportTestJobs struct {
	imports chan func(context.Context, plugin_commands.Progress) plugin_commands.Outcome
}

func (j *commandImportTestJobs) SubmitCommandJob(plugin_commands.RunJobSpec, func(string) error, func(context.Context, plugin_commands.Progress) plugin_commands.Outcome) (string, error) {
	return "", errors.New("unexpected command job")
}
func (j *commandImportTestJobs) SubmitImportJob(_ plugin_commands.ImportJobSpec, run func(context.Context, plugin_commands.Progress) plugin_commands.Outcome) (string, error) {
	j.imports <- run
	return "import-job", nil
}

type commandImportTestExecutor struct{}

func (commandImportTestExecutor) Execute(context.Context, plugin_commands.QueuedRun) plugin_commands.Outcome {
	return plugin_commands.Outcome{Status: plugin_commands.RunStatusFailed, Error: "unexpected command execution"}
}

func TestPluginCommandImportDispatcherPersistsMapAndScopedGroupAssociation(t *testing.T) {
	ctx, actor, scopeRoot, generation := commandImportContext(t)
	root := t.TempDir()
	runID := "composed-import-run"
	finished := time.Now().UTC()
	bound := ctx.WithPrincipal(auth.FromUser(actor))
	if err := bound.CreateRun(plugin_commands.RunRecord{
		ID: runID, PluginName: "commanded", CommandName: "convert", ParamsJSON: "{}",
		Status: plugin_commands.RunStatusQueued, CreatedByUserID: &actor.ID,
		CreatedAt: finished.Add(-time.Second),
	}, plugin_commands.RunOutput{RunID: runID, ArgvJSON: "[]", CreatedAt: finished.Add(-time.Second)}); err != nil {
		t.Fatal(err)
	}
	if won, err := bound.MarkRunRunning(runID, finished.Add(-500*time.Millisecond)); err != nil || !won {
		t.Fatalf("start run: won=%v err=%v", won, err)
	}
	if won, err := bound.FinishRun(runID, plugin_commands.RunFinish{Status: plugin_commands.RunStatusSucceeded, FinishedAt: finished}); err != nil || !won {
		t.Fatalf("finish run: won=%v err=%v", won, err)
	}
	runDir := filepath.Join(root, "plugin_exchange", "commanded", runID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	payload := []byte("dispatcher-to-add-resource")
	if err := os.WriteFile(filepath.Join(runDir, "result.bin"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	jobs := &commandImportTestJobs{imports: make(chan func(context.Context, plugin_commands.Progress) plugin_commands.Outcome, 1)}
	dispatcher := plugin_commands.NewDispatcher(plugin_commands.Dependencies{
		Store: ctx, Jobs: jobs, Executor: commandImportTestExecutor{}, Settings: commandImportTestSettings{root: root},
	})
	dispatcher.SetImporter(ctx)
	if err := dispatcher.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = dispatcher.Stop(stopCtx)
	})
	got, err := dispatcher.SubmitImport(plugin_commands.ImportSubmission{
		Access: plugin_commands.Access{PluginName: "commanded", ActorUserID: &actor.ID},
		RunID:  runID, Name: "result.bin", Fields: plugin_commands.ResourceFields{
			Name: "composed", GroupIDs: []uint{scopeRoot.ID},
		},
		PluginGeneration: generation, ActorUserID: &actor.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	var run func(context.Context, plugin_commands.Progress) plugin_commands.Outcome
	select {
	case run = <-jobs.imports:
	case <-time.After(time.Second):
		t.Fatal("import was not dispatched")
	}
	if outcome := run(context.Background(), nil); outcome.Status != plugin_commands.ImportStatusSucceeded {
		t.Fatalf("outcome = %+v", outcome)
	}
	mapped, found, err := ctx.ImportMap(runID, "result.bin")
	if err != nil || !found || mapped.ImportID != got.ImportID || mapped.ResourceID == nil || mapped.Status != plugin_commands.ImportStatusSucceeded {
		t.Fatalf("map = %+v found=%v err=%v", mapped, found, err)
	}
	var resource models.Resource
	if err := ctx.db.Preload("Groups").First(&resource, *mapped.ResourceID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.CreatedByUserId == nil || *resource.CreatedByUserId != actor.ID {
		t.Fatalf("creator = %v, want scoped actor %d", resource.CreatedByUserId, actor.ID)
	}
	if len(resource.Groups) != 1 || resource.Groups[0].ID != scopeRoot.ID {
		t.Fatalf("groups = %+v, want scoped group %d", resource.Groups, scopeRoot.ID)
	}
	stored, err := afero.ReadFile(ctx.fs, resource.Location)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != string(payload) {
		t.Fatalf("stored = %q", stored)
	}
}

func TestPluginCommandImportSameContentConcurrencyUsesOneValidBackingFile(t *testing.T) {
	ctx, _, _, generation := commandImportContext(t)
	actor, err := ctx.CreateUser(&UserInput{Username: "concurrent-importer", Password: "password1", Role: models.RoleEditor})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("same-content-from-two-imports")
	root := t.TempDir()
	paths := []string{filepath.Join(root, "one"), filepath.Join(root, "two")}
	for _, path := range paths {
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for i, id := range []string{"claim-one", "claim-two"} {
		createImportClaimForTest(t, ctx, actor, id, "run-"+id, "result.bin", generation)
		_ = i
	}
	start := make(chan struct{})
	ids := make([]uint, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range paths {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			file, err := os.Open(paths[i])
			if err != nil {
				errs[i] = err
				return
			}
			defer file.Close()
			factory := func() (*os.File, func() error, error) {
				temp, err := os.CreateTemp(root, "upload-")
				if err != nil {
					return nil, nil, err
				}
				return temp, func() error { return os.Remove(temp.Name()) }, nil
			}
			info, err := file.Stat()
			if err != nil {
				errs[i] = err
				return
			}
			ids[i], errs[i] = ctx.ImportResource(context.Background(), plugin_commands.ImportSource{
				File: file, SourceSize: info.Size(), CreateScratch: factory, FileName: "result.bin", RunID: "run", ImportID: []string{"claim-one", "claim-two"}[i],
			}, plugin_commands.ResourceFields{Name: "same"}, root)
		}(i)
	}
	close(start)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if ids[0] != ids[1] {
		t.Fatalf("resource ids = %v", ids)
	}
	var resource models.Resource
	if err := ctx.db.First(&resource, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	stored, err := afero.ReadFile(ctx.fs, resource.Location)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != string(payload) {
		t.Fatalf("stored=%q", stored)
	}
}
