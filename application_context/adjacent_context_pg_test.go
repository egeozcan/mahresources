//go:build postgres && json1 && fts5

package application_context

import (
	"fmt"
	"strings"
	"testing"

	"gorm.io/gorm"

	"mahresources/models"
)

// Postgres parity for adjacent_context_test.go: ties on timestamptz, the
// IS NULL partitions and the scope, and an EXPLAIN of every seek.

func newAdjacentPGContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	ctx := newMassEditPGContext(t)
	// The default resource category is inserted with an explicit id, which
	// leaves the sequence behind it.
	if err := ctx.db.Exec("SELECT setval(pg_get_serial_sequence('resource_categories', 'id'), (SELECT MAX(id) FROM resource_categories))").Error; err != nil {
		t.Fatalf("advance the resource category sequence: %v", err)
	}
	if err := models.EnsureResourceListIndexes(ctx.db); err != nil {
		t.Fatalf("resource list indexes: %v", err)
	}
	if err := models.EnsureSiblingNavIndexes(ctx.db); err != nil {
		t.Fatalf("sibling navigation indexes: %v", err)
	}
	return ctx
}

func TestAdjacentOrderingTiesAndPartitions_Postgres(t *testing.T) {
	runAdjacentOrderingTiesAndPartitions(t, newAdjacentPGContext(t))
}

func TestAdjacentSeriesMode_Postgres(t *testing.T) {
	runAdjacentSeriesMode(t, newAdjacentPGContext(t))
}

func TestAdjacentScope_Postgres(t *testing.T) {
	runAdjacentScope(t, newAdjacentPGContext(t))
}

// About 20k children under one owner, then ANALYZE: each seek must read its
// sibling index in order, with no Sort node, in the owned and the ownerless
// partition alike.
func TestAdjacentSeeksUseTheSiblingIndexes_Postgres(t *testing.T) {
	ctx := newAdjacentPGContext(t)
	db := ctx.db
	owner := &models.Group{Name: "owner"}
	if err := db.Create(owner).Error; err != nil {
		t.Fatal(err)
	}
	const n = 20000
	seeds := []string{
		fmt.Sprintf(`INSERT INTO resources (name, owner_id, resource_category_id, created_at, updated_at, hash)
			SELECT 'r' || g, CASE WHEN g %% 2 = 0 THEN %d END, 1, now() - (g || ' seconds')::interval, now(), 'h' || g FROM generate_series(1, %d) g`, owner.ID, n),
		fmt.Sprintf(`INSERT INTO notes (name, owner_id, created_at, updated_at)
			SELECT 'n' || g, CASE WHEN g %% 2 = 0 THEN %d END, now() - (g || ' seconds')::interval, now() FROM generate_series(1, %d) g`, owner.ID, n),
		fmt.Sprintf(`INSERT INTO groups (name, owner_id, created_at, updated_at)
			SELECT 'g' || g, CASE WHEN g %% 2 = 0 THEN %d END, now() - (g || ' seconds')::interval, now() FROM generate_series(1, %d) g`, owner.ID, n),
		"ANALYZE resources", "ANALYZE notes", "ANALYZE groups",
	}
	for _, statement := range seeds {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	for entityType, spec := range adjacentSpecs {
		for _, owned := range []bool{true, false} {
			for _, next := range []bool{true, false} {
				cur := adjacentRow{ID: 101}
				if entityType == "resource" {
					cur.Category = uptr(1)
				}
				if owned {
					cur.OwnerId = &owner.ID
				}
				var dest []adjacentLinkRow
				sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
					return adjacentSeek(tx, spec, cur, false, next).Find(&dest)
				})
				var lines []string
				if err := db.Raw("EXPLAIN " + sql).Scan(&lines).Error; err != nil {
					t.Fatalf("explain: %v", err)
				}
				plan := strings.Join(lines, "\n")
				want := "idx_" + spec.table + "_adjacent_created"
				if !strings.Contains(plan, "Index Scan") && !strings.Contains(plan, "Index Only Scan") || !strings.Contains(plan, want) {
					t.Errorf("%s owned=%v next=%v: seek does not scan %s:\n%s", entityType, owned, next, want, plan)
				}
				if strings.Contains(plan, "Sort") {
					t.Errorf("%s owned=%v next=%v: seek sorts:\n%s", entityType, owned, next, plan)
				}
			}
		}
	}
}
