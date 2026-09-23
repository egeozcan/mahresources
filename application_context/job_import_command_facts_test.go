package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"gorm.io/gorm"
	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/types"
)

func seedImportJobsForCommandFactBackfill(t *testing.T, ctx *MahresourcesContext) map[string]string {
	t.Helper()
	fs := ctx.fs
	if err := fs.MkdirAll("_imports", 0o755); err != nil {
		t.Fatalf("create imports directory: %v", err)
	}
	for _, path := range []string{
		importArchivePathFor("backfill-parse-valid"),
		importArchivePathFor("backfill-apply-valid"),
		importPlanPathFor("backfill-apply-valid"),
		importArchivePathFor("backfill-apply-plan-missing"),
	} {
		if err := afero.WriteFile(fs, path, []byte("staged"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	seed := func(kind string, input any) string {
		t.Helper()
		encoded, err := json.Marshal(input)
		if err != nil {
			t.Fatalf("encode existing %s Job input: %v", kind, err)
		}
		snap := acceptJobFor(t, ctx, jobs.Acceptance{
			Kind: kind, KindVersion: jobImportKindVersion, State: jobs.StateQueued,
			Origin: "backfill-test", Title: "Pre-existing import", Replay: jobs.ReplayInput{Input: encoded},
		})
		if err := ctx.db.Model(&models.Job{}).Where("id = ?", snap.ID).Update("state", jobs.StateFailed).Error; err != nil {
			t.Fatalf("make existing %s Job failed: %v", kind, err)
		}
		return snap.ID
	}
	return map[string]string{
		"parse-valid": seed(JobKindGroupImportParse, importParseJobInput{
			Handle: "backfill-parse-valid", Archive: importArchivePathFor("backfill-parse-valid"),
		}),
		"parse-missing": seed(JobKindGroupImportParse, importParseJobInput{
			Handle: "backfill-parse-missing", Archive: importArchivePathFor("backfill-parse-missing"),
		}),
		"apply-valid": seed(JobKindGroupImportApply, importApplyJobInput{
			ParseHandle: "backfill-apply-valid", Plan: importPlanPathFor("backfill-apply-valid"),
			Decisions: ImportDecisions{MappingActions: map[string]MappingAction{}, DanglingActions: map[string]DanglingAction{}},
		}),
		"apply-plan-missing": seed(JobKindGroupImportApply, importApplyJobInput{
			ParseHandle: "backfill-apply-plan-missing", Plan: importPlanPathFor("backfill-apply-plan-missing"),
			Decisions: ImportDecisions{MappingActions: map[string]MappingAction{}, DanglingActions: map[string]DanglingAction{}},
		}),
	}
}

func startupReconcileImportFactsForTest(t *testing.T, ctx *MahresourcesContext) {
	t.Helper()
	ctx.DownloadManager().SetSettings(download_queue.NewStaticDownloadSettings(
		download_queue.TimeoutConfig{}, time.Hour))
	// Production runs this sweep before SetJobService. Import fact recovery belongs
	// to the next startup step, after the service has registered the import Kinds.
	ctx.RunStartupExportSweep()
	ctx.SetJobService(jobs.NewService())
	if err := ctx.ReconcileImportCommandAvailability(); err != nil {
		t.Fatalf("reconcile import command availability: %v", err)
	}
}

func assertImportJobRetry(t *testing.T, ctx *MahresourcesContext, id string, want bool) {
	t.Helper()
	commands, err := ctx.JobService().AdvertisedCommands(context.Background(), ctx.jobDeps(), jobs.Access{Administrator: true}, id)
	if err != nil {
		t.Fatalf("advertise import Job %s commands: %v", id, err)
	}
	got := false
	for _, command := range commands {
		if command.Key == jobs.CommandRetry {
			got = true
		}
	}
	if got != want {
		t.Errorf("Job %s Retry advertised = %v, want %v; commands=%+v", id, got, want, commands)
	}
}

func TestImportCommandAvailabilityBackfillsJobsAndReconcilesAfterRestart(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	jobIDs := seedImportJobsForCommandFactBackfill(t, ctx)
	var before int64
	if err := ctx.db.Model(&models.JobImportCommandFact{}).Count(&before).Error; err != nil {
		t.Fatalf("count import facts before simulated upgrade: %v", err)
	}
	if before != 0 {
		t.Fatalf("pre-upgrade fact rows = %d, want none", before)
	}

	// A fresh application context has the upgraded schema and the old Jobs/files,
	// but no installed service when the export sweep runs.
	restarted := NewMahresourcesContext(ctx.fs, ctx.db, ctx.readOnlyDB, ctx.Config)
	restarted.SetJobReplayKeyring(ctx.JobReplayKeyring())
	if restarted.PluginManager() != nil {
		t.Cleanup(restarted.PluginManager().Close)
	}
	startupReconcileImportFactsForTest(t, restarted)
	for name, want := range map[string]bool{
		"parse-valid": true, "parse-missing": false,
		"apply-valid": true, "apply-plan-missing": false,
	} {
		assertImportJobRetry(t, restarted, jobIDs[name], want)
	}
	assertAdapterSelectorMatchesCommands(t, restarted, jobs.Access{Administrator: true}, jobs.CommandRetry)
	var afterBackfill int64
	if err := ctx.db.Model(&models.JobImportCommandFact{}).Count(&afterBackfill).Error; err != nil {
		t.Fatalf("count backfilled import facts: %v", err)
	}
	if afterBackfill != 4 {
		t.Fatalf("backfilled fact rows = %d, want one per distinct parse handle", afterBackfill)
	}

	// Artifact loss during downtime is reconciled on the next startup before the
	// detail Commands and SQL selector are read.
	for _, path := range []string{
		importArchivePathFor("backfill-parse-valid"),
		importPlanPathFor("backfill-apply-valid"),
	} {
		if err := ctx.fs.Remove(path); err != nil {
			t.Fatalf("remove %s before second restart: %v", path, err)
		}
	}
	secondRestart := NewMahresourcesContext(ctx.fs, ctx.db, ctx.readOnlyDB, ctx.Config)
	secondRestart.SetJobReplayKeyring(ctx.JobReplayKeyring())
	if secondRestart.PluginManager() != nil {
		t.Cleanup(secondRestart.PluginManager().Close)
	}
	startupReconcileImportFactsForTest(t, secondRestart)
	assertImportJobRetry(t, secondRestart, jobIDs["parse-valid"], false)
	assertImportJobRetry(t, secondRestart, jobIDs["apply-valid"], false)
	assertImportJobRetry(t, secondRestart, jobIDs["parse-missing"], false)
	assertImportJobRetry(t, secondRestart, jobIDs["apply-plan-missing"], false)
	assertAdapterSelectorMatchesCommands(t, secondRestart, jobs.Access{Administrator: true}, jobs.CommandRetry)
}

func assertImportCommandJobBackfillIsBounded(t *testing.T, ctx *MahresourcesContext) {
	t.Helper()
	const jobCount = 1001
	rows := make([]models.Job, jobCount)
	now := time.Now().UTC()
	for i := range rows {
		handle := fmt.Sprintf("bounded-backfill-%04d", i)
		summary, err := json.Marshal(importParseSummary{Handle: handle, Archive: handle + ".tar"})
		if err != nil {
			t.Fatalf("encode summary: %v", err)
		}
		rows[i] = models.Job{
			ID:   fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1),
			Kind: JobKindGroupImportParse, KindVersion: jobImportKindVersion,
			State: string(jobs.StateFailed), Origin: "backfill-test", VisibilityClass: "owner",
			ExecutionPrincipal: "host", ReplayClass: "replayable", Version: 1,
			AcceptedAt: now.Add(time.Duration(i) * time.Nanosecond), Summary: types.JSON(summary),
		}
	}
	if err := ctx.db.CreateInBatches(&rows, 200).Error; err != nil {
		t.Fatalf("seed legacy import Jobs: %v", err)
	}

	type observedQuery struct {
		sql  string
		vars []interface{}
	}
	var pageSQL []observedQuery
	if err := ctx.db.Callback().Query().After("gorm:query").Register("selector-test:import-job-backfill-pages", func(tx *gorm.DB) {
		if tx.Statement.Table == "jobs" {
			pageSQL = append(pageSQL, observedQuery{
				sql: tx.Statement.SQL.String(), vars: append([]interface{}(nil), tx.Statement.Vars...),
			})
		}
	}); err != nil {
		t.Fatalf("register Job query observer: %v", err)
	}
	if err := ctx.ReconcileImportCommandAvailability(); err != nil {
		t.Fatalf("backfill import Jobs: %v", err)
	}
	if len(pageSQL) != 4 {
		t.Fatalf("Job backfill page query count = %d, want 4 bounded keyset pages", len(pageSQL))
	}
	keysetPages := 0
	for i, query := range pageSQL {
		lower := strings.ToLower(query.sql)
		bounded := strings.Contains(lower, "limit 500")
		for _, value := range query.vars {
			if limit, ok := value.(int); ok && limit == 500 {
				bounded = true
			}
		}
		if !bounded {
			t.Errorf("Job backfill query %d has no 500-row bound: %s vars=%v", i, query.sql, query.vars)
		}
		if strings.Contains(lower, "id >") || strings.Contains(lower, "`id` >") || strings.Contains(lower, `"id" >`) {
			keysetPages++
		}
	}
	if keysetPages != 3 {
		t.Errorf("Job keyset pages = %d, want 3 later pages with an id boundary", keysetPages)
	}
	var factCount int64
	if err := ctx.db.Model(&models.JobImportCommandFact{}).Count(&factCount).Error; err != nil {
		t.Fatalf("count backfilled facts: %v", err)
	}
	if factCount != jobCount {
		t.Fatalf("backfilled facts = %d, want %d", factCount, jobCount)
	}
}

func TestImportCommandJobBackfillUsesBoundedKeysetBatches(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	assertImportCommandJobBackfillIsBounded(t, ctx)
}
