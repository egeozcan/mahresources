//go:build postgres && json1 && fts5

package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/plugin_system"
)

func TestRegisteredKindCommandSelectorsMatchCommandsPostgres(t *testing.T) {
	ctx, _, _ := newPostgresOwnershipFixture(t, 2)
	if old := ctx.PluginManager(); old != nil {
		old.Close()
	}
	pluginDir := t.TempDir()
	pluginPath := filepath.Join(pluginDir, pluginActionTestPlugin)
	if err := os.MkdirAll(pluginPath, 0o755); err != nil {
		t.Fatalf("create plugin directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginPath, "plugin.lua"), []byte(pluginActionTestSource), 0o644); err != nil {
		t.Fatalf("write plugin source: %v", err)
	}
	pm, err := plugin_system.NewPluginManager(pluginDir)
	if err != nil {
		t.Fatalf("create plugin manager: %v", err)
	}
	ctx.pluginManager = pm
	if err := ctx.registerPluginActionJobKind(ctx.JobService()); err != nil {
		t.Fatalf("install plugin action host seam: %v", err)
	}
	t.Cleanup(pm.Close)
	prepareSelectorPlugin(t, ctx)
	var scopeGroup models.Group
	scopeGroup.Name = "postgres-selector-scope"
	if err := ctx.db.Create(&scopeGroup).Error; err != nil {
		t.Fatalf("create scope group: %v", err)
	}
	owner := models.User{Username: "pg-selector-owner", Role: models.RoleUser}
	otherOwner := models.User{Username: "pg-selector-other-owner", Role: models.RoleUser}
	if err := ctx.db.Create(&owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if err := ctx.db.Create(&otherOwner).Error; err != nil {
		t.Fatalf("create other owner: %v", err)
	}
	seedSelectorJobs(t, ctx, &owner.ID, &otherOwner.ID)
	if got := len(ctx.JobService().Registrations()); got != 8 {
		t.Fatalf("registered Kind/version count = %d, want 8", got)
	}
	admin := jobs.Access{Administrator: true}
	ownerAccess := jobs.Access{UserID: owner.ID}
	for _, key := range []string{jobs.CommandCancel, jobs.CommandResume, jobs.CommandRetry, jobs.CommandRepeat, "selector-unknown"} {
		assertAdapterSelectorMatchesCommands(t, ctx, admin, key)
		assertAdapterSelectorMatchesCommands(t, ctx, ownerAccess, key)
		assertCommandListSummaryMatchDetails(t, ctx, admin, key)
		assertCommandListSummaryMatchDetails(t, ctx, ownerAccess, key)
	}
	if err := ctx.db.Model(&models.User{}).Where("id = ?", owner.ID).Update("role", models.RoleGuest).Error; err != nil {
		t.Fatalf("demote owner: %v", err)
	}
	assertAdapterSelectorMatchesCommands(t, ctx, ownerAccess, jobs.CommandRetry)
	assertCommandListSummaryMatchDetails(t, ctx, ownerAccess, jobs.CommandRetry)
	if err := ctx.db.Model(&models.User{}).Where("id = ?", owner.ID).Updates(map[string]any{
		"role": models.RoleUser, "scope_group_id": scopeGroup.ID,
	}).Error; err != nil {
		t.Fatalf("scope owner: %v", err)
	}
	if err := ctx.SetPluginScopedAccess(pluginActionTestPlugin, true); err != nil {
		t.Fatalf("allow scoped plugin actions: %v", err)
	}
	assertAdapterSelectorMatchesCommands(t, ctx, ownerAccess, jobs.CommandRetry)
	assertCommandListSummaryMatchDetails(t, ctx, ownerAccess, jobs.CommandRetry)
	if err := ctx.SetPluginScopedAccess(pluginActionTestPlugin, false); err != nil {
		t.Fatalf("revoke scoped plugin actions: %v", err)
	}
	assertAdapterSelectorMatchesCommands(t, ctx, ownerAccess, jobs.CommandRetry)
	assertCommandListSummaryMatchDetails(t, ctx, ownerAccess, jobs.CommandRetry)
}

func TestPostgresImportCommandSelectorHasSparseDatabasePlan(t *testing.T) {
	ctx, _, _ := newPostgresOwnershipFixture(t, 2)
	handle := "pg-selector-sparse-hit"
	for i := 0; i < 500; i++ {
		decoy := fmt.Sprintf("pg-selector-sparse-decoy-%04d", i)
		input, err := json.Marshal(importParseJobInput{Handle: decoy, Archive: importArchivePathFor(decoy)})
		if err != nil {
			t.Fatal(err)
		}
		snap := acceptJobFor(t, ctx, jobs.Acceptance{
			Kind: JobKindGroupImportParse, KindVersion: jobImportKindVersion, State: jobs.StateQueued,
			Origin: "selector-test", Title: "Parse", Replay: jobs.ReplayInput{Input: input},
		})
		if err := ctx.db.Model(&models.Job{}).Where("id = ?", snap.ID).Update("state", jobs.StateFailed).Error; err != nil {
			t.Fatal(err)
		}
		if err := setImportCommandAvailability(ctx.db, decoy, false, false); err != nil {
			t.Fatal(err)
		}
	}
	input, err := json.Marshal(importParseJobInput{Handle: handle, Archive: importArchivePathFor(handle)})
	if err != nil {
		t.Fatal(err)
	}
	match := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindGroupImportParse, KindVersion: jobImportKindVersion, State: jobs.StateQueued,
		Origin: "selector-test", Title: "Parse", Replay: jobs.ReplayInput{Input: input},
	})
	if err := ctx.db.Model(&models.Job{}).Where("id = ?", match.ID).Update("state", jobs.StateFailed).Error; err != nil {
		t.Fatal(err)
	}
	if err := setImportCommandAvailability(ctx.db, handle, true, false); err != nil {
		t.Fatal(err)
	}
	var selector jobs.CommandFilterAdapter
	for _, registration := range ctx.JobService().Registrations() {
		if registration.Definition.Kind == JobKindGroupImportParse {
			selector = registration.Adapter.(jobs.CommandFilterAdapter)
		}
	}
	query, supported, err := selector.SelectCommandJobs(context.Background(), jobs.CommandFilterRequest{
		Deps: ctx.jobDeps(), Access: jobs.Access{Administrator: true}, Key: jobs.CommandRetry,
		Jobs: ctx.db.Model(&models.Job{}).Where("kind = ? AND kind_version = ?", JobKindGroupImportParse, jobImportKindVersion),
	})
	if err != nil || !supported {
		t.Fatalf("select import Retry candidates: supported=%v err=%v", supported, err)
	}
	var selected []string
	if err := query.Pluck("jobs.id", &selected).Error; err != nil {
		t.Fatalf("run sparse Postgres selector: %v", err)
	}
	if len(selected) != 1 || selected[0] != match.ID {
		t.Fatalf("sparse Postgres selector returned %d IDs, want only hit %s", len(selected), match.ID)
	}
	statement := query.Session(&gorm.Session{DryRun: true}).Find(&[]models.Job{}).Statement
	var planLines []string
	if err := ctx.db.Raw("EXPLAIN "+statement.SQL.String(), statement.Vars...).Scan(&planLines).Error; err != nil {
		t.Fatalf("explain Postgres import selector: %v", err)
	}
	plan := strings.Join(planLines, "\n")
	if !strings.Contains(plan, "job_import_command_facts") {
		t.Fatalf("Postgres selector plan omitted indexed import facts:\n%s", plan)
	}
}

func TestPostgresImportCommandAvailabilityBackfillsJobsAndReconcilesAfterRestart(t *testing.T) {
	first, other, _ := newPostgresOwnershipFixture(t, 2)
	if err := first.db.Migrator().DropTable(&models.JobImportCommandFact{}); err != nil {
		t.Fatalf("remove the not-yet-upgraded import fact table: %v", err)
	}
	jobIDs := seedImportJobsForCommandFactBackfill(t, first)
	if err := first.db.AutoMigrate(&models.JobImportCommandFact{}); err != nil {
		t.Fatalf("migrate import command facts during upgrade: %v", err)
	}
	var before int64
	if err := first.db.Model(&models.JobImportCommandFact{}).Count(&before).Error; err != nil {
		t.Fatalf("count import facts before simulated upgrade: %v", err)
	}
	if before != 0 {
		t.Fatalf("pre-upgrade fact rows = %d, want none", before)
	}
	other.SetJobService(nil)
	if other.PluginManager() != nil {
		other.PluginManager().Close()
		other.pluginManager = nil
	}
	startupReconcileImportFactsForTest(t, other)
	for name, want := range map[string]bool{
		"parse-valid": true, "parse-missing": false,
		"apply-valid": true, "apply-plan-missing": false,
	} {
		assertImportJobRetry(t, other, jobIDs[name], want)
	}
	assertAdapterSelectorMatchesCommands(t, other, jobs.Access{Administrator: true}, jobs.CommandRetry)
	var afterBackfill int64
	if err := first.db.Model(&models.JobImportCommandFact{}).Count(&afterBackfill).Error; err != nil {
		t.Fatalf("count backfilled import facts: %v", err)
	}
	if afterBackfill != 4 {
		t.Fatalf("backfilled fact rows = %d, want four parse handles", afterBackfill)
	}

	for _, path := range []string{
		importArchivePathFor("backfill-parse-valid"),
		importPlanPathFor("backfill-apply-valid"),
	} {
		if err := first.fs.Remove(path); err != nil {
			t.Fatalf("remove %s before second restart: %v", path, err)
		}
	}
	other.SetJobService(nil)
	startupReconcileImportFactsForTest(t, other)
	assertImportJobRetry(t, other, jobIDs["parse-valid"], false)
	assertImportJobRetry(t, other, jobIDs["apply-valid"], false)
	assertAdapterSelectorMatchesCommands(t, other, jobs.Access{Administrator: true}, jobs.CommandRetry)
}

func TestPostgresImportCommandJobBackfillUsesBoundedKeysetBatches(t *testing.T) {
	ctx, _, _ := newPostgresOwnershipFixture(t, 2)
	assertImportCommandJobBackfillIsBounded(t, ctx)
}
