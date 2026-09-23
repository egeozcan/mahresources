package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"gorm.io/gorm"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/plugin_system"
)

func TestRegisteredKindCommandSelectorsMatchCommands(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	prepareSelectorPlugin(t, ctx)
	scopeGroup := models.Group{Name: "selector-scope"}
	if err := ctx.db.Create(&scopeGroup).Error; err != nil {
		t.Fatalf("create scope group: %v", err)
	}
	owner := models.User{Username: "selector-owner", Role: models.RoleUser}
	otherOwner := models.User{Username: "selector-other-owner", Role: models.RoleUser}
	if err := ctx.db.Create(&owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if err := ctx.db.Create(&otherOwner).Error; err != nil {
		t.Fatalf("create other owner: %v", err)
	}

	seedSelectorJobs(t, ctx, &owner.ID, &otherOwner.ID)
	registrations := ctx.JobService().Registrations()
	if len(registrations) != 11 {
		t.Fatalf("registered Kind/version count = %d, want 11 (including downloads, plugin commands, and summary export)", len(registrations))
	}
	for _, registration := range registrations {
		if _, ok := registration.Adapter.(jobs.CommandFilterAdapter); !ok {
			t.Errorf("%s v%d has no command selector", registration.Definition.Kind, registration.Definition.KindVersion)
		}
	}

	admin := jobs.Access{Administrator: true}
	ownerAccess := jobs.Access{UserID: owner.ID}
	for _, key := range []string{jobs.CommandCancel, jobs.CommandResume, jobs.CommandRetry, jobs.CommandRepeat, "selector-unknown"} {
		assertAdapterSelectorMatchesCommands(t, ctx, admin, key)
		assertAdapterSelectorMatchesCommands(t, ctx, ownerAccess, key)
		assertCommandListSummaryMatchDetails(t, ctx, admin, key)
		assertCommandListSummaryMatchDetails(t, ctx, ownerAccess, key)
	}

	// A role change is read from the current user row. The same owned Job keeps
	// its history, but a guest loses every Kind command and the SQL selectors
	// must agree with detail advertisements.
	if err := ctx.db.Model(&models.User{}).Where("id = ?", owner.ID).Update("role", models.RoleGuest).Error; err != nil {
		t.Fatalf("demote owner: %v", err)
	}
	for _, key := range []string{jobs.CommandCancel, jobs.CommandResume, jobs.CommandRetry, jobs.CommandRepeat} {
		assertAdapterSelectorMatchesCommands(t, ctx, ownerAccess, key)
		assertCommandListSummaryMatchDetails(t, ctx, ownerAccess, key)
	}

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

func prepareSelectorPlugin(t *testing.T, ctx *MahresourcesContext) {
	t.Helper()
	if ctx.PluginManager() == nil {
		t.Fatal("selector test has no plugin manager")
	}
	if _, err := ctx.EnsurePluginStates(); err != nil {
		t.Fatalf("ensure plugin states: %v", err)
	}
	if err := ctx.PluginManager().EnablePlugin(pluginActionTestPlugin); err != nil {
		t.Fatalf("enable action plugin: %v", err)
	}
	if err := ctx.db.Model(&models.PluginState{}).Where("plugin_name = ?", pluginActionTestPlugin).
		Updates(map[string]any{"enabled": true, "allow_scoped_principals": true}).Error; err != nil {
		t.Fatalf("record plugin access: %v", err)
	}
}

func seedSelectorJobs(t *testing.T, ctx *MahresourcesContext, ownerID, otherOwnerID *uint) {
	t.Helper()
	seed := func(kind string, version uint, state jobs.State, owner *uint, input json.RawMessage) jobs.Snapshot {
		t.Helper()
		snap := acceptJobFor(t, ctx, jobs.Acceptance{
			Kind: kind, KindVersion: version, State: jobs.StateQueued, Origin: "selector-test",
			OwnerUserID: owner, ActorUserID: owner, Title: kind,
			Replay: jobs.ReplayInput{Input: input},
		})
		if state != jobs.StateQueued {
			if err := ctx.db.Model(&models.Job{}).Where("id = ?", snap.ID).Update("state", state).Error; err != nil {
				t.Fatalf("set %s state to %s: %v", kind, state, err)
			}
		}
		return snap
	}
	downloadInput, err := remoteDownloadInputJSON(&query_models.ResourceFromRemoteCreator{URL: "https://example.test/archive.bin"}, "")
	if err != nil {
		t.Fatalf("build download input: %v", err)
	}
	seed(JobKindRemoteDownload, jobDownloadKindVersion, jobs.StateFailed, ownerID, downloadInput)
	seed(JobKindRemoteDownload, jobDownloadKindVersion, jobs.StatePaused, ownerID, downloadInput)
	seed(JobKindRemoteDownload, jobDownloadKindVersion, jobs.StateQueued, ownerID, downloadInput)
	deferredInput, err := remoteDownloadInputJSON(&query_models.ResourceFromRemoteCreator{URL: "https://example.test/deferred.bin"}, pluginActionTestPlugin)
	if err != nil {
		t.Fatalf("build deferred input: %v", err)
	}
	seed(JobKindDeferredDownload, jobDownloadKindVersion, jobs.StateFailed, ownerID, deferredInput)

	groupID := createExportGroupForTest(t, ctx, "selector-export-group")
	exportInput, err := json.Marshal(exportJobInput{Request: *exportRequestForTest(groupID)})
	if err != nil {
		t.Fatalf("build export input: %v", err)
	}
	seed(JobKindGroupExport, jobExportKindVersion, jobs.StateFailed, ownerID, exportInput)
	seed(JobKindGroupExport, jobExportKindVersion, jobs.StateSucceeded, ownerID, exportInput)
	seed(JobKindGroupExport, jobExportKindVersion, jobs.StateQueued, ownerID, exportInput)
	seed(JobKindGroupExport, jobExportKindVersion, jobs.StateFailed, otherOwnerID, exportInput)

	for i := 0; i < 40; i++ {
		handle := fmt.Sprintf("selector-sparse-%03d", i)
		input, err := json.Marshal(importParseJobInput{Handle: handle, Archive: importArchivePathFor(handle)})
		if err != nil {
			t.Fatalf("build sparse parse input: %v", err)
		}
		seed(JobKindGroupImportParse, jobImportKindVersion, jobs.StateFailed, ownerID, input)
		if err := setImportCommandAvailability(ctx.db, handle, false, false); err != nil {
			t.Fatalf("record sparse parse facts: %v", err)
		}
	}
	parseHandle := "selector-parse-available"
	parseInput, err := json.Marshal(importParseJobInput{Handle: parseHandle, Archive: importArchivePathFor(parseHandle)})
	if err != nil {
		t.Fatalf("build parse input: %v", err)
	}
	seed(JobKindGroupImportParse, jobImportKindVersion, jobs.StateFailed, ownerID, parseInput)
	seed(JobKindGroupImportParse, jobImportKindVersion, jobs.StateQueued, ownerID, parseInput)
	if err := setImportCommandAvailability(ctx.db, parseHandle, true, false); err != nil {
		t.Fatalf("record parse facts: %v", err)
	}
	applyHandle := "selector-apply-available"
	applyInput, err := json.Marshal(importApplyJobInput{
		ParseHandle: applyHandle, Plan: importPlanPathFor(applyHandle),
		Decisions: ImportDecisions{MappingActions: map[string]MappingAction{}, DanglingActions: map[string]DanglingAction{}},
	})
	if err != nil {
		t.Fatalf("build apply input: %v", err)
	}
	seed(JobKindGroupImportApply, jobImportKindVersion, jobs.StateFailed, ownerID, applyInput)
	seed(JobKindGroupImportApply, jobImportKindVersion, jobs.StateQueued, ownerID, applyInput)
	if err := setImportCommandAvailability(ctx.db, applyHandle, true, true); err != nil {
		t.Fatalf("record apply facts: %v", err)
	}
	missingPlanHandle := "selector-apply-no-plan"
	missingPlanInput, err := json.Marshal(importApplyJobInput{
		ParseHandle: missingPlanHandle, Plan: importPlanPathFor(missingPlanHandle),
		Decisions: ImportDecisions{MappingActions: map[string]MappingAction{}, DanglingActions: map[string]DanglingAction{}},
	})
	if err != nil {
		t.Fatalf("build missing-plan apply input: %v", err)
	}
	seed(JobKindGroupImportApply, jobImportKindVersion, jobs.StateFailed, ownerID, missingPlanInput)
	if err := setImportCommandAvailability(ctx.db, missingPlanHandle, true, false); err != nil {
		t.Fatalf("record missing-plan facts: %v", err)
	}

	reduction := createReductionRowForTest(t, ctx, `{"clusters":[]}`, models.ReductionStatusFailed)
	reductionInput, err := json.Marshal(reductionComputeJobInput{ReductionID: reduction.ID, Version: reduction.Version})
	if err != nil {
		t.Fatalf("build Reduction input: %v", err)
	}
	seed(JobKindReductionCompute, jobReductionKindVersion, jobs.StateFailed, ownerID, reductionInput)
	seed(JobKindReductionCompute, jobReductionKindVersion, jobs.StateQueued, ownerID, reductionInput)
	seed(JobKindSimilarityRecompute, jobMaintenanceKindVersion, jobs.StateFailed, ownerID, maintenanceJobInputJSON())
	seed(JobKindSimilarityRecompute, jobMaintenanceKindVersion, jobs.StateQueued, ownerID, maintenanceJobInputJSON())

	seedPlugin := func(subtype, action, schedule string, owner *uint, replayable bool) jobs.Snapshot {
		t.Helper()
		input, err := json.Marshal(pluginActionJobInput{
			Subtype: subtype, Plugin: pluginActionTestPlugin, Action: action, ScheduleID: schedule,
			EntityType: "resource", Overlap: plugin_system.ScheduleOverlapSkip,
			Runtime: plugin_system.CurrentRuntimeIdentity().String(),
		})
		if err != nil {
			t.Fatalf("build plugin-action input: %v", err)
		}
		acceptance := jobs.Acceptance{
			Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, State: jobs.StateQueued,
			Origin: "selector-test", OwnerUserID: owner, ActorUserID: owner, Title: "Plugin work",
		}
		if replayable {
			acceptance.Replay = jobs.ReplayInput{Input: input}
		} else {
			acceptance.Replay = jobs.ReplayInput{NonReplayable: true}
			acceptance.Summary = pluginActionSummaryOf(input)
		}
		snap := acceptJobFor(t, ctx, acceptance)
		if err := ctx.db.Model(&models.Job{}).Where("id = ?", snap.ID).Update("state", jobs.StateFailed).Error; err != nil {
			t.Fatalf("set plugin action failed: %v", err)
		}
		return snap
	}
	seedPlugin(pluginActionSubtypeRegistered, "retryable-work", "", ownerID, true)
	seedPlugin(pluginActionSubtypeRegistered, "failing-work", "", ownerID, true)
	seedPlugin(pluginActionSubtypeScheduled, "", "retryable-tick", ownerID, true)
	seedPlugin(pluginActionSubtypeScheduled, "", "tick", ownerID, true)
	seedPlugin(pluginActionSubtypeClosure, "", "", ownerID, false)
}

func assertAdapterSelectorMatchesCommands(t *testing.T, ctx *MahresourcesContext, access jobs.Access, key string) {
	t.Helper()
	for _, registration := range ctx.JobService().Registrations() {
		selector := registration.Adapter.(jobs.CommandFilterAdapter)
		base := ctx.db.Model(&models.Job{}).
			Where("jobs.kind = ? AND jobs.kind_version = ?", registration.Definition.Kind, registration.Definition.KindVersion)
		if !access.Administrator {
			base = base.Where("jobs.visibility_class = ? AND jobs.owner_user_id = ?", jobs.VisibilityOwner, access.UserID)
		}
		selected, supported, err := selector.SelectCommandJobs(context.Background(), jobs.CommandFilterRequest{
			Deps: ctx.jobDeps(), Access: access, Key: key, Jobs: base,
		})
		if err != nil {
			t.Fatalf("%s v%d selector for %s: %v", registration.Definition.Kind, registration.Definition.KindVersion, key, err)
		}
		if !supported && selected != nil {
			t.Fatalf("%s v%d returned a query for unsupported key %s", registration.Definition.Kind, registration.Definition.KindVersion, key)
		}
		var selectedIDs []string
		if supported {
			if selected == nil {
				t.Fatalf("%s v%d supports %s but returned no query", registration.Definition.Kind, registration.Definition.KindVersion, key)
			}
			if err := selected.Pluck("jobs.id", &selectedIDs).Error; err != nil {
				t.Fatalf("run %s v%d selector for %s: %v", registration.Definition.Kind, registration.Definition.KindVersion, key, err)
			}
		}
		selectedSet := make(map[string]bool, len(selectedIDs))
		for _, id := range selectedIDs {
			selectedSet[id] = true
		}

		var rows []models.Job
		if err := base.Find(&rows).Error; err != nil {
			t.Fatalf("load %s v%d Jobs: %v", registration.Definition.Kind, registration.Definition.KindVersion, err)
		}
		for _, row := range rows {
			snapshot, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, row.ID)
			if err != nil {
				t.Fatalf("load detail for %s: %v", row.ID, err)
			}
			commands, err := registration.Adapter.Commands(context.Background(), jobs.CommandContext{
				Snapshot: snapshot, Access: access, Deps: ctx.jobDeps(),
			})
			if err != nil {
				t.Fatalf("%s v%d Commands for %s: %v", registration.Definition.Kind, registration.Definition.KindVersion, row.ID, err)
			}
			if got, want := selectedSet[row.ID], offersCommand(commands, key); got != want {
				t.Errorf("%s v%d selector for %s Job %s = %v, Commands advertises %v (state %s, access %+v)",
					registration.Definition.Kind, registration.Definition.KindVersion, key, row.ID, got, want, row.State, access)
			}
		}
	}
}

func assertCommandListSummaryMatchDetails(t *testing.T, ctx *MahresourcesContext, access jobs.Access, key string) {
	t.Helper()
	page, err := ctx.JobService().List(ctx.jobDeps(), access, jobs.Filter{Command: key}, jobs.Cursor{}, 200)
	if err != nil {
		t.Fatalf("list Jobs with %s for %+v: %v", key, access, err)
	}
	got := make(map[string]bool, len(page.Jobs))
	for _, job := range page.Jobs {
		got[job.ID] = true
	}
	var rows []models.Job
	base := ctx.db.Model(&models.Job{})
	if !access.Administrator {
		base = base.Where("visibility_class = ? AND owner_user_id = ?", jobs.VisibilityOwner, access.UserID)
	}
	if err := base.Find(&rows).Error; err != nil {
		t.Fatalf("load visible detail Jobs for %s: %v", key, err)
	}
	want := make(map[string]bool)
	for _, row := range rows {
		commands, err := ctx.JobService().AdvertisedCommands(context.Background(), ctx.jobDeps(), access, row.ID)
		if err != nil {
			t.Fatalf("detail Commands for %s: %v", row.ID, err)
		}
		if offersCommand(commands, key) {
			want[row.ID] = true
		}
	}
	if len(got) != len(want) {
		t.Errorf("List(%s) count = %d, detail Commands count = %d for %+v", key, len(got), len(want), access)
	}
	for id := range want {
		if !got[id] {
			t.Errorf("List(%s) omitted Job %s whose detail Commands advertise it", key, id)
		}
	}
	for id := range got {
		if !want[id] {
			t.Errorf("List(%s) included Job %s whose detail Commands do not advertise it", key, id)
		}
	}
	summary, err := ctx.JobService().Summary(ctx.jobDeps(), access, jobs.Filter{Command: key}, 24*time.Hour)
	if err != nil {
		t.Fatalf("summary Jobs with %s for %+v: %v", key, access, err)
	}
	if summary.Total != int64(len(want)) {
		t.Errorf("Summary(%s).Total = %d, detail Commands count = %d for %+v", key, summary.Total, len(want), access)
	}
}

func TestImportCommandSelectorIsSparseAndExecutionInvalidatesMissingFiles(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	handle := "selector-sparse-hit"
	for i := 0; i < 300; i++ {
		decoy := fmt.Sprintf("selector-sparse-decoy-%03d", i)
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
	parse := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindGroupImportParse, KindVersion: jobImportKindVersion, State: jobs.StateQueued,
		Origin: "selector-test", Title: "Parse", Replay: jobs.ReplayInput{Input: input},
	})
	if err := ctx.db.Model(&models.Job{}).Where("id = ?", parse.ID).Update("state", jobs.StateFailed).Error; err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(ctx.GetDefaultFs(), importArchivePathFor(handle), []byte("archive"), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}
	if err := setImportCommandAvailability(ctx.db, handle, true, false); err != nil {
		t.Fatal(err)
	}
	var importAdapter jobs.CommandFilterAdapter
	for _, registration := range ctx.JobService().Registrations() {
		if registration.Definition.Kind == JobKindGroupImportParse {
			importAdapter = registration.Adapter.(jobs.CommandFilterAdapter)
		}
	}
	if importAdapter == nil {
		t.Fatal("no import-parse selector registered")
	}
	query, supported, err := importAdapter.SelectCommandJobs(context.Background(), jobs.CommandFilterRequest{
		Deps: ctx.jobDeps(), Access: jobs.Access{Administrator: true}, Key: jobs.CommandRetry,
		Jobs: ctx.db.Model(&models.Job{}).Where("kind = ? AND kind_version = ?", JobKindGroupImportParse, jobImportKindVersion),
	})
	if err != nil || !supported {
		t.Fatalf("select import Retry candidates: supported=%v err=%v", supported, err)
	}
	var selected []string
	if err := query.Pluck("jobs.id", &selected).Error; err != nil {
		t.Fatalf("run sparse import selector: %v", err)
	}
	if len(selected) != 1 || selected[0] != parse.ID {
		t.Fatalf("sparse import selector returned %d IDs, want only hit %s", len(selected), parse.ID)
	}
	statement := query.Session(&gorm.Session{DryRun: true}).Find(&[]models.Job{}).Statement
	rows, err := ctx.db.Raw("EXPLAIN QUERY PLAN "+statement.SQL.String(), statement.Vars...).Rows()
	if err != nil {
		t.Fatalf("explain sparse selector: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatalf("read explain plan row: %v", err)
		}
		plan.WriteString(detail)
		plan.WriteByte('\n')
	}
	if !strings.Contains(plan.String(), "SEARCH f") {
		t.Fatalf("import fact selector did not use a sparse indexed lookup:\n%s", plan.String())
	}

	// A file can disappear outside the tracked filesystem while a page is open.
	// The persisted fact keeps detail and selector in agreement until execution;
	// execution then clears that fact before the transactional advertisement check.
	if err := ctx.fs.Remove(importArchivePathFor(handle)); err != nil {
		t.Fatalf("remove archive externally: %v", err)
	}
	if !offersCommand(advertisedForTest(t, ctx, parse.ID), jobs.CommandRetry) {
		t.Fatal("detail Commands did not use the persisted availability fact")
	}
	_, err = ctx.JobService().ExecuteCommand(context.Background(), ctx.jobDeps(), jobs.CommandRequest{
		JobID: parse.ID, Key: jobs.CommandRetry, IdempotencyKey: "missing-archive",
		ExpectedVersion: parse.Version, Actor: jobs.Access{Administrator: true},
	})
	if !errors.Is(err, jobs.ErrCommandNotAdvertised) {
		t.Fatalf("Retry with missing archive error = %v, want not-advertised", err)
	}
	archiveAvailable, _, err := importCommandAvailability(ctx.db, handle)
	if err != nil || archiveAvailable {
		t.Fatalf("missing archive fact = %v, err=%v; want false", archiveAvailable, err)
	}
	if offersCommand(advertisedForTest(t, ctx, parse.ID), jobs.CommandRetry) {
		t.Fatal("detail Commands kept Retry after execution invalidated the missing archive fact")
	}
	assertAdapterSelectorMatchesCommands(t, ctx, jobs.Access{Administrator: true}, jobs.CommandRetry)

	applyHandle := "selector-apply-external-delete"
	if err := ctx.GetDefaultFs().MkdirAll("_imports", 0o755); err != nil {
		t.Fatalf("create import staging directory: %v", err)
	}
	for _, path := range []string{importArchivePathFor(applyHandle), importPlanPathFor(applyHandle)} {
		if err := afero.WriteFile(ctx.GetDefaultFs(), path, []byte("staged"), 0o644); err != nil {
			t.Fatalf("write apply evidence %s: %v", path, err)
		}
	}
	applyInput, err := json.Marshal(importApplyJobInput{
		ParseHandle: applyHandle, Plan: importPlanPathFor(applyHandle),
		Decisions: ImportDecisions{MappingActions: map[string]MappingAction{}, DanglingActions: map[string]DanglingAction{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	apply := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: JobKindGroupImportApply, KindVersion: jobImportKindVersion, State: jobs.StateQueued,
		Origin: "selector-test", Title: "Apply", Replay: jobs.ReplayInput{Input: applyInput},
	})
	if err := ctx.db.Model(&models.Job{}).Where("id = ?", apply.ID).Update("state", jobs.StateFailed).Error; err != nil {
		t.Fatal(err)
	}
	if err := setImportCommandAvailability(ctx.db, applyHandle, true, true); err != nil {
		t.Fatal(err)
	}
	if !offersCommand(advertisedForTest(t, ctx, apply.ID), jobs.CommandRetry) {
		t.Fatal("apply detail Commands did not advertise from persisted file facts")
	}
	if err := ctx.fs.Remove(importPlanPathFor(applyHandle)); err != nil {
		t.Fatalf("remove apply plan externally: %v", err)
	}
	_, err = ctx.JobService().ExecuteCommand(context.Background(), ctx.jobDeps(), jobs.CommandRequest{
		JobID: apply.ID, Key: jobs.CommandRetry, IdempotencyKey: "missing-apply-plan",
		ExpectedVersion: apply.Version, Actor: jobs.Access{Administrator: true},
	})
	if !errors.Is(err, jobs.ErrCommandNotAdvertised) {
		t.Fatalf("apply Retry with missing plan error = %v, want not-advertised", err)
	}
	archiveAvailable, planAvailable, err := importCommandAvailability(ctx.db, applyHandle)
	if err != nil || !archiveAvailable || planAvailable {
		t.Fatalf("apply facts after missing plan = (%v,%v), err=%v; want (true,false)", archiveAvailable, planAvailable, err)
	}
	if offersCommand(advertisedForTest(t, ctx, apply.ID), jobs.CommandRetry) {
		t.Fatal("apply detail Commands kept Retry after execution invalidated its missing plan fact")
	}
	assertAdapterSelectorMatchesCommands(t, ctx, jobs.Access{Administrator: true}, jobs.CommandRetry)
}

func TestImportCommandFactReconciliationUsesBoundedKeysetBatches(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	const factCount = 1001
	facts := make([]models.JobImportCommandFact, factCount)
	for i := range facts {
		facts[i] = models.JobImportCommandFact{
			ParseHandle: fmt.Sprintf("reconcile-%05d", i), ArchiveAvailable: true, PlanAvailable: true,
		}
	}
	if err := ctx.db.CreateInBatches(&facts, 200).Error; err != nil {
		t.Fatalf("create import facts: %v", err)
	}
	var factScans []string
	if err := ctx.db.Callback().Query().After("gorm:query").Register("selector-test:import-fact-batch-check", func(tx *gorm.DB) {
		if tx.Statement.Table == "job_import_command_facts" {
			factScans = append(factScans, tx.Statement.SQL.String())
		}
	}); err != nil {
		t.Fatalf("register fact scan observer: %v", err)
	}
	if err := ctx.ReconcileImportCommandAvailability(); err != nil {
		t.Fatalf("reconcile import facts: %v", err)
	}
	if len(factScans) != 4 {
		t.Fatalf("fact scan query count = %d, want 4 bounded pages for %d rows", len(factScans), factCount)
	}
	keysetPages := 0
	for i, sql := range factScans {
		lower := strings.ToLower(sql)
		if !strings.Contains(lower, "limit 500") {
			t.Errorf("fact scan %d has no 500-row bound: %s", i, sql)
		}
		if strings.Contains(lower, "parse_handle"+" >") || strings.Contains(lower, "parse_handle` >") {
			keysetPages++
		}
	}
	if keysetPages != 3 {
		t.Errorf("keyset pages = %d, want 3 later pages with a last-handle predicate", keysetPages)
	}
	var available int64
	if err := ctx.db.Model(&models.JobImportCommandFact{}).
		Where("archive_available = ? OR plan_available = ?", true, true).Count(&available).Error; err != nil {
		t.Fatalf("count unreconciled facts: %v", err)
	}
	if available != 0 {
		t.Fatalf("available fact count after filesystem reconciliation = %d, want zero", available)
	}
}
