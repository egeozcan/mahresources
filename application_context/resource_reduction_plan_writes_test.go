package application_context

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
	"mahresources/models/query_models"
)

// The plan is rows (ADR 0011), and every invariant over them (derived columns,
// lock order, fences, the deletion hook's re-derivation) is kept by the store's own
// writers. A write from anywhere else would bypass all of it without any test
// failing, so a statement that writes one of the plan tables, or touches one through
// its model, may appear only in the store files.
func TestPlanTablesAreWrittenOnlyByTheStore(t *testing.T) {
	tables := `(reduction_clusters|reduction_members|reduction_extent_resources|reduction_extent_groups|reduction_extent|reduction_seeds|reduction_deleted_ids|reduction_migration_rows|reduction_migrations|resource_cleanup_manifest)`
	sqlWrite := regexp.MustCompile(`(?is)\b(insert\s+into|update|delete\s+from)\s+` + "`?\"?" + tables + `\b`)
	models := map[string]bool{
		"ReductionClusterRow": true, "ReductionMemberRow": true, "ReductionExtentResource": true, "ReductionExtentGroup": true,
		"ReductionExtentRow": true, "ReductionSeed": true, "ReductionDeletedID": true, "ReductionMigrationRow": true,
		"ReductionMigration": true, "ResourceCleanupManifest": true,
	}
	store := regexp.MustCompile(`^(reduction_[a-z_]*store[a-z_]*|reduction_deletion|resource_delete_stage|reduction_test_seams|reduction_legacy_model)\.go$`)

	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	checked := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		checked++
		if store.MatchString(file) {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		require.NoError(t, err)
		ast.Inspect(parsed, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.BasicLit:
				if x.Kind == token.STRING {
					if value, err := strconv.Unquote(x.Value); err == nil && sqlWrite.MatchString(value) {
						t.Errorf("%s writes a plan table with SQL outside the store: %.80q", file, value)
					}
				}
			case *ast.SelectorExpr:
				if pkg, ok := x.X.(*ast.Ident); ok && pkg.Name == "models" && models[x.Sel.Name] {
					t.Errorf("%s names models.%s outside the store", file, x.Sel.Name)
				}
			}
			return true
		})
	}
	require.Greater(t, checked, 100, "the guard scanned the package")
}

// The legacy `plan` and `extent` documents are megabytes per row on a library that
// ran a whole-library Reduction, and a SELECT * ships them on every fence, lease
// renewal and page. So no ordinary entry point may select them: every statement an
// entry point issues against resource_reductions names reductionColumns, and the
// documents cross the wire only in the migration. The run is against a row whose
// legacy columns hold multi-megabyte documents, and records the SQL actually sent.
func TestNoOrdinaryQuerySelectsTheLegacyPlanOrExtent(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	require.NoError(t, models.EnsureSimilarityDistanceIndexes(ctx.db))
	big := []byte(`{"clusters":[],"pad":"` + strings.Repeat("x", 3<<20) + `"}`)
	legacy := &legacyReduction{ResourceReduction: models.ResourceReduction{Name: "legacy", Status: models.ReductionStatusReady, MatchingMode: "both"}, Plan: big, Extent: big}
	require.NoError(t, ctx.db.Create(legacy).Error)
	current := &models.ResourceReduction{Name: "current", Status: models.ReductionStatusReady, MatchingMode: "both", PlanSchema: 1}
	require.NoError(t, ctx.db.Create(current).Error)

	var mu sync.Mutex
	var statements []string
	record := func(tx *gorm.DB) {
		sql := tx.Statement.SQL.String()
		if strings.Contains(sql, "resource_reductions") {
			mu.Lock()
			statements = append(statements, sql)
			mu.Unlock()
		}
	}
	const name = "test:record_reduction_sql"
	require.NoError(t, ctx.db.Callback().Query().After("gorm:query").Register(name, record))
	require.NoError(t, ctx.db.Callback().Row().After("gorm:row").Register(name, record))
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(name); _ = ctx.db.Callback().Row().Remove(name) })

	rows := []*models.ResourceReduction{&legacy.ResourceReduction, current}
	for _, r := range rows {
		_, _ = ctx.GetResourceReduction(r.ID, nil, false)
		_, _ = ctx.GetReductionReview(r.ID, nil, false, 1, &query_models.ResourceReductionReviewQuery{})
		_, _ = ctx.GetReductionClusterMembers(r.ID, nil, false, "none", "")
		_, _ = ctx.OverrideReductionCluster(&query_models.ReductionOverride{ID: r.ID, Version: r.Version, ClusterID: "none", Action: ReductionActionSkip}, nil, false)
		_, _ = ctx.BulkOverrideReductionClusters(&query_models.ReductionBulkOverride{ID: r.ID, Version: r.Version, Action: ReductionActionCheck, ClusterIDs: []string{"none"}}, nil, false)
		_, _ = ctx.ApplyResourceReduction(&query_models.ReductionApply{ID: r.ID, Version: r.Version}, nil, false)
		_, _ = ctx.UpdateResourceReductionSettings(&query_models.ResourceReductionEditor{ID: r.ID, Version: r.Version, Name: "renamed"}, nil, false)
		_, _ = ctx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{ID: r.ID, ResourceIds: []uint{1}}, nil, false)
		_, _ = ctx.RequestReductionCompute(r.ID, r.Version, nil, false, nil)
	}
	_, _ = ctx.GetResourceReductions(0, 20, &query_models.ResourceReductionQuery{})
	_, _ = ctx.GetResourceReductionCount(&query_models.ResourceReductionQuery{})
	_, _, _, _ = ctx.ListResourceReductions(0, 20, &query_models.ResourceReductionQuery{})
	_, _ = ctx.GetReductionProgress([]models.ResourceReduction{legacy.ResourceReduction, *current})
	_, _, _ = ctx.copyReductionsBatch("", 20, time.Now())

	legacyColumn := regexp.MustCompile("(?i)[\\s,.`\"(](plan|extent)[\\s,`\"),]")
	require.NotEmpty(t, statements, "the entry points issued no statement against resource_reductions")
	for _, sql := range statements {
		assert.False(t, strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "SELECT *"), "an unprojected load of resource_reductions: %s", sql)
		assert.NotRegexp(t, legacyColumn, strings.SplitN(sql, " FROM ", 2)[0], "a statement selects the legacy plan or extent: %s", sql)
	}
	var plan, extent int
	require.NoError(t, ctx.db.Raw(`SELECT length(plan), length(extent) FROM resource_reductions WHERE id = ?`, legacy.ID).Row().Scan(&plan, &extent))
	assert.Greater(t, plan, 3<<20, "the legacy documents are still on the row: nothing ordinary rewrote or dropped them")
}

// A record still running past the ceiling is one whose process died or whose batch
// was abandoned, and the page says so.
func TestDescribeLastApplyReadsAnOldRunningRecordAsInterrupted(t *testing.T) {
	assert.Nil(t, describeLastApply(nil))

	fresh := describeLastApply(&models.ReductionLastApply{StartedAt: time.Now().Add(-time.Minute)})
	assert.True(t, fresh.Running)
	assert.False(t, fresh.Interrupted)

	old := describeLastApply(&models.ReductionLastApply{StartedAt: time.Now().Add(-ReductionApplyCeiling - time.Minute)})
	assert.True(t, old.Running)
	assert.True(t, old.Interrupted)

	done := time.Now()
	finished := describeLastApply(&models.ReductionLastApply{StartedAt: time.Now().Add(-3 * ReductionApplyCeiling), FinishedAt: &done})
	assert.False(t, finished.Running)
	assert.False(t, finished.Interrupted, "a finished apply is never interrupted, however old")
}
