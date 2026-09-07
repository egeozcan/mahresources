//go:build postgres

package mrql

import (
	"encoding/json"
	"testing"

	"gorm.io/gorm"
)

// Check database work rather than elapsed time: even a tiny fixture must keep
// payload columns out of the random sort, regardless of the test host's speed.
func TestPG_BoundedRandomSortWidth(t *testing.T) {
	db := setupPostgresTestDB(t)
	built := parseAndTranslate(t, `type = resource AND meta.rating = 5 ORDER BY RANDOM() LIMIT 50`, EntityResource, db)
	stmt := built.Session(&gorm.Session{DryRun: true}).Find(&[]testResource{}).Statement
	var raw string
	if err := db.Raw("EXPLAIN (FORMAT JSON) "+stmt.SQL.String(), stmt.Vars...).Scan(&raw).Error; err != nil {
		t.Fatal(err)
	}
	var plans []struct{ Plan map[string]any }
	if err := json.Unmarshal([]byte(raw), &plans); err != nil {
		t.Fatal(err)
	}
	found := false
	var visit func(map[string]any, bool)
	visit = func(node map[string]any, belowLimit bool) {
		// PostgreSQL may sort the at-most-50 hydrated rows again after the
		// join. Only the population sort beneath the sampling LIMIT needs to
		// stay narrow; rejecting the final bounded sort overfits one plan.
		if node["Node Type"] == "Sort" && belowLimit {
			found = true
			if width := node["Plan Width"].(float64); width > 16 {
				t.Errorf("random sort carries full rows (%v bytes); want only ID and random key (16 bytes)", width)
			}
		}
		if children, ok := node["Plans"].([]any); ok {
			for _, child := range children {
				visit(child.(map[string]any), belowLimit || node["Node Type"] == "Limit")
			}
		}
	}
	visit(plans[0].Plan, false)
	if !found {
		t.Fatal("expected random sort in query plan")
	}
}

func TestPG_BoundedRandomPreservesSelection(t *testing.T) {
	db := setupPostgresTestDB(t)
	for _, table := range []string{"resources", "notes", "groups"} {
		// Include numeric strings, malformed values, and an out-of-scope owner.
		sql := `INSERT INTO ` + table + ` (id, name, meta, owner_id) SELECT 100+n, 'sample-' || n,
   CASE n % 4 WHEN 0 THEN '{"score":10}'::json WHEN 1 THEN '{"score":"010.0"}'::json
   WHEN 2 THEN '{"score":"oops"}'::json ELSE '{"score":20}'::json END,
   CASE WHEN n % 3 = 0 THEN 3 ELSE 1 END FROM generate_series(1, 80) n`
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name, source, table, where string
		entity                     EntityType
		scope                      uint
	}{
		{"resources", `type = resource AND meta.score = 10 ORDER BY RANDOM() LIMIT 7 OFFSET 2`, "resources", "", EntityResource, 0},
		{"notes", `type = note AND meta.score = 10 ORDER BY RANDOM() LIMIT 7 OFFSET 2`, "notes", "", EntityNote, 0},
		{"groups", `type = group AND meta.score = 10 ORDER BY RANDOM() LIMIT 7 OFFSET 2`, "groups", "", EntityGroup, 0},
		{"scope", `type = resource AND meta.score = 10 ORDER BY RANDOM() LIMIT 7 OFFSET 2`, "resources", " AND owner_id = 1", EntityResource, 1},
		{"relation", `type = resource AND meta.score = 10 AND owner = "Vacation" ORDER BY RANDOM() LIMIT 7 OFFSET 2`, "resources", " AND owner_id = 1", EntityResource, 0},
		{"empty", `type = resource AND meta.score = 99 ORDER BY RANDOM() LIMIT 7`, "resources", " AND 1 = 0", EntityResource, 0},
		{"zero limit", `type = resource AND meta.score = 10 ORDER BY RANDOM() LIMIT 0`, "resources", "", EntityResource, 0},
		{"past end", `type = resource AND meta.score = 10 ORDER BY RANDOM() LIMIT 7 OFFSET 1000`, "resources", "", EntityResource, 0},
		{"larger than population", `type = resource AND meta.score = 10 ORDER BY RANDOM() LIMIT 100`, "resources", "", EntityResource, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			q := mustParse(t, test.source)
			q.EntityType = test.entity
			if err := Validate(q); err != nil {
				t.Fatal(err)
			}
			// Pin a connection and the RNG so the ordinary SQL and translated query
			// must choose exactly the same rows, in exactly the same order.
			err := db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Exec("SET LOCAL max_parallel_workers_per_gather = 0").Error; err != nil {
					return err
				}
				if err := tx.Exec("SELECT setseed(0.42)").Error; err != nil {
					return err
				}
				expected := tx.Table(test.table).Select("id").Where(`CASE WHEN meta->>'score' ~ '^-{0,1}[0-9]+(\.[0-9]+){0,1}$' THEN (meta->>'score')::numeric ELSE NULL END = 10` + test.where).Order("RANDOM()").Limit(q.Limit).Offset(q.Offset)
				var want []testResource
				if err := expected.Find(&want).Error; err != nil {
					return err
				}
				if err := tx.Exec("SELECT setseed(0.42)").Error; err != nil {
					return err
				}
				built, err := TranslateWithOptions(q, tx, TranslateOptions{ScopeGroupID: test.scope})
				if err != nil {
					return err
				}
				// All test entities share these fields. Explicit Table in the translation
				// must still win over the destination model's resource table.
				var got []testResource
				if err := built.Find(&got).Error; err != nil {
					return err
				}
				if len(got) != len(want) {
					t.Fatalf("got %d rows, want %d", len(got), len(want))
				}
				for i := range want {
					if got[i].ID != want[i].ID {
						t.Fatalf("row %d: got %d, want %d (random ordering or selection changed)", i, got[i].ID, want[i].ID)
					}
					if got[i].Name == "" || got[i].Meta == "" {
						t.Fatalf("entity was not fully hydrated: %+v", got[i])
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
