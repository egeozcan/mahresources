//go:build postgres && json1 && fts5

package application_context

import (
	"context"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"mahresources/models"
	"mahresources/shortcodes"
)

// Postgres parity for relation_template_context_test.go: the joined edge
// statement, the type-name order and the scope, and an allow-list past
// Postgres's 65535 bind parameters.

func TestRelationsForTemplateListing_Postgres(t *testing.T) {
	runRelationsForTemplateListing(t, newMassEditPGContext(t))
}

func TestRelationsForTemplateScope_Postgres(t *testing.T) {
	runRelationsForTemplateScope(t, newMassEditPGContext(t))
}

func TestRelationsForTemplateLargeAllowList_Postgres(t *testing.T) {
	runRelationsForTemplateLargeAllowList(t, newMassEditPGContext(t), 70000)
}

func TestRelationsForTemplateHiddenEdgesCannotDisplaceVisibleEdges_Postgres(t *testing.T) {
	runRelationsForTemplateHiddenEdgesCannotDisplaceVisibleEdges(t, newMassEditPGContext(t))
}

func TestRelationIncomingIndexIsNotBuiltByPostgresStartupMigration(t *testing.T) {
	ctx := newMassEditPGContext(t)
	if err := models.EnsureSupplementalIndexes(ctx.db); err != nil {
		t.Fatal(err)
	}
	if ctx.db.Migrator().HasIndex(&models.GroupRelation{}, "idx__group_relations__to_group_id") {
		t.Error("the incoming relation index must not be built synchronously by EnsureSupplementalIndexes on Postgres")
	}
}

func TestRelationIncomingIndexBuildAllowsPostgresWrites(t *testing.T) {
	ctx, dsn := newMassEditPGContextWithDSN(t)
	w := newRelationsWorld(t, ctx)
	writer := ctx.db.Begin()
	if writer.Error != nil {
		t.Fatal(writer.Error)
	}
	defer writer.Rollback()
	// Keep a writer open so that the build cannot finish before we check
	// both its lock mode and whether another writer can still proceed.
	if err := writer.Exec("LOCK TABLE group_relations IN ROW EXCLUSIVE MODE").Error; err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- models.EnsureRelationIncomingIndexOnOwnConnection(dsn) }()
	deadline, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		var building bool
		if err := ctx.db.WithContext(deadline).Raw(`SELECT EXISTS (
			SELECT 1 FROM pg_locks WHERE relation = 'group_relations'::regclass
			AND mode = 'ShareUpdateExclusiveLock' AND granted
			AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
		)`).Scan(&building).Error; err != nil {
			t.Fatal(err)
		}
		if building {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("build finished before the older writer committed: %v", err)
		case <-deadline.Done():
			t.Fatal("build never acquired the concurrent index lock")
		case <-ticker.C:
		}
	}
	if err := ctx.db.WithContext(deadline).Create(&models.GroupRelation{
		FromGroupId: &w.auth.ID, ToGroupId: &w.queue.ID, RelationTypeId: &w.dependsSvc.ID,
	}).Error; err != nil {
		t.Fatalf("concurrent relation write: %v", err)
	}
	if err := writer.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("build did not finish after the writer committed")
	}
	var valid bool
	if err := ctx.db.Raw(`SELECT indisvalid AND NOT indisunique FROM pg_index
		WHERE indexrelid = 'idx__group_relations__to_group_id'::regclass`).Scan(&valid).Error; err != nil || !valid {
		t.Fatalf("index missing or invalid: %v (valid: %v)", err, valid)
	}
}

func TestRelationsForTemplateUsesAnIndexPerDirection_Postgres(t *testing.T) {
	ctx, dsn := newMassEditPGContextWithDSN(t)
	w := newRelationsWorld(t, ctx)
	// Give the planner an analyzed table where scanning all relations for
	// the carrier's handful of edges is not the best plan.
	if err := ctx.db.Exec(`WITH others AS (
		INSERT INTO groups (name) SELECT 'other-' || n FROM generate_series(1, 10000) AS n RETURNING id
	) INSERT INTO group_relations (from_group_id, to_group_id, relation_type_id)
	SELECT id, ?, ? FROM others`, w.release.ID, w.blocks.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := models.EnsureRelationIncomingIndexOnOwnConnection(dsn); err != nil {
		t.Fatal(err)
	}
	if err := ctx.db.Exec("ANALYZE group_relations").Error; err != nil {
		t.Fatal(err)
	}
	for dir, index := range map[string]string{"in": "idx__group_relations__to_group_id", "out": "unique_rel"} {
		req := shortcodes.RelationsRequest{GroupID: w.carrier.ID, Direction: dir, Limit: 20}
		for _, scope := range []*scopeFilter{nil, {allowed: []uint{w.carrier.ID, w.auth.ID, w.upstrm.ID}}} {
			var rows []relationTemplateRow
			stmt := relationTemplateQuery(ctx.db.Session(&gorm.Session{DryRun: true}), req, req.Limit, scope).Find(&rows).Statement
			var lines []string
			if err := ctx.db.Raw("EXPLAIN "+stmt.SQL.String(), stmt.Vars...).Scan(&lines).Error; err != nil {
				t.Fatal(err)
			}
			plan := strings.Join(lines, "\n")
			if !strings.Contains(plan, index) {
				t.Errorf("%s lookup (scoped=%v) does not use %s:\n%s", dir, scope != nil, index, plan)
			}
		}
	}
}
