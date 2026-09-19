package application_context

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/spf13/afero"
	"gorm.io/gorm"
	"mahresources/auth"
	"mahresources/models"
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
	fields := plugin_commands.ResourceFields{Name: "from command", Meta: map[string]any{"source": "command"}}
	if err := ctx.ValidateImport(plugin_commands.ImportValidation{PluginName: "commanded", PluginGeneration: generation, ActorUserID: &actor.ID, Fields: fields}); err != nil {
		t.Fatal(err)
	}
	id, err := ctx.ImportResource(context.Background(), plugin_commands.ImportSource{Path: sourcePath, FileName: "result.bin", RunID: "run-memory", ImportID: "claim-memory"}, fields, scratch)
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
			ids[i], errs[i] = ctx.ImportResource(context.Background(), plugin_commands.ImportSource{Path: paths[i], FileName: "result.bin", RunID: "run", ImportID: []string{"claim-one", "claim-two"}[i]}, plugin_commands.ResourceFields{Name: "same"}, root)
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
