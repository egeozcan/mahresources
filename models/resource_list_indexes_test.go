package models

import (
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func explainPlan(t *testing.T, db *gorm.DB, query string, args ...any) string {
	t.Helper()
	rows, err := db.Raw("EXPLAIN QUERY PLAN "+query, args...).Rows()
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		plan.WriteString(detail + "\n")
	}
	return plan.String()
}

// The resource list filtered by owner reads newest first. With only the
// created_at and owner_id indexes, the planner either walks created_at and
// discards every other owner's rows (on a 2.2M library, 639k of them before the
// first page of one large owner) or fetches every owned row and sorts them. The
// composite index reads exactly one page.
func TestOwnerFilteredResourcePageReadsTheOwnerIndexInOrder(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:resource_list_indexes?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// The shared in-memory database lives until its last connection closes.
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	for _, statement := range []string{
		"CREATE TABLE resources (id integer primary key, name text, owner_id integer, created_at datetime)",
		"CREATE INDEX idx_resources_created_at ON resources(created_at)",
		"CREATE INDEX idx_resources_owner_id ON resources(owner_id)",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create test table: %v", err)
		}
	}

	if err := EnsureResourceListIndexes(db); err != nil {
		t.Fatalf("create resource list indexes: %v", err)
	}
	// The statement GetResources issues for ?OwnerId=7.
	plan := explainPlan(t, db, "SELECT * FROM `resources` WHERE resources.owner_id = ? ORDER BY created_at desc LIMIT 50", 7)
	if !strings.Contains(plan, "idx_resources_owner_created") || strings.Contains(plan, "TEMP B-TREE") {
		t.Fatalf("the owner page does not read idx_resources_owner_created in order:\n%s", plan)
	}

	// A second pass leaves the index alone, and one under the name on other
	// columns is replaced.
	if err := EnsureResourceListIndexes(db); err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if err := db.Exec("DROP INDEX idx_resources_owner_created").Error; err != nil {
		t.Fatalf("drop: %v", err)
	}
	if err := db.Exec("CREATE INDEX idx_resources_owner_created ON resources(name)").Error; err != nil {
		t.Fatalf("create the wrong shape: %v", err)
	}
	if err := EnsureResourceListIndexes(db); err != nil {
		t.Fatalf("repair pass: %v", err)
	}
	var columns []string
	if err := db.Raw("SELECT name FROM pragma_index_info('idx_resources_owner_created') ORDER BY seqno").Scan(&columns).Error; err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if strings.Join(columns, ", ") != "owner_id, created_at" {
		t.Fatalf("idx_resources_owner_created covers %v, want owner_id, created_at", columns)
	}

	// Right columns, but partial or unique, is not the index either: a partial
	// one serves a single owner, and a unique one would refuse a second resource.
	for _, wrong := range []string{
		"CREATE INDEX idx_resources_owner_created ON resources(owner_id, created_at) WHERE owner_id = 7",
		"CREATE UNIQUE INDEX idx_resources_owner_created ON resources(owner_id, created_at)",
	} {
		if err := db.Exec("DROP INDEX idx_resources_owner_created").Error; err != nil {
			t.Fatalf("drop: %v", err)
		}
		if err := db.Exec(wrong).Error; err != nil {
			t.Fatalf("create %q: %v", wrong, err)
		}
		if err := EnsureResourceListIndexes(db); err != nil {
			t.Fatalf("repair pass: %v", err)
		}
		plan := explainPlan(t, db, "SELECT * FROM `resources` WHERE resources.owner_id = ? ORDER BY created_at desc LIMIT 50", 8)
		if !strings.Contains(plan, "idx_resources_owner_created") || strings.Contains(plan, "TEMP B-TREE") {
			t.Fatalf("after %q, owner 8's page does not read the index in order:\n%s", wrong, plan)
		}
		var flags struct{ Unique, Partial int }
		if err := db.Raw("SELECT \"unique\" AS \"unique\", partial FROM pragma_index_list('resources') WHERE name = 'idx_resources_owner_created'").Scan(&flags).Error; err != nil {
			t.Fatalf("inspect flags: %v", err)
		}
		if flags.Unique != 0 || flags.Partial != 0 {
			t.Fatalf("after %q the index is unique=%d partial=%d, want a plain index", wrong, flags.Unique, flags.Partial)
		}
	}
}
