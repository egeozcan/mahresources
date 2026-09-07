package mrql

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func TestParseMetadataIndexes(t *testing.T) {
	valid := `[{"entity":"resource","key":"score","kind":"numeric"},{"entity":"note","key":"camera.iso","kind":"text"}]`
	indexes, err := ParseMetadataIndexes(valid)
	if err != nil || len(indexes) != 2 {
		t.Fatalf("parse: %v %v", indexes, err)
	}
	for _, raw := range []string{
		"", "null", `{}`, `[] []`, `[{"entity":"resource","key":"score","kind":"numeric","typo":true}]`,
		`[{"entity":"resources","key":"score","kind":"numeric"}]`,
		`[{"entity":"resource","key":"score","kind":"unknown"}]`,
		`[{"entity":"resource","key":"score'); DROP TABLE resources;--","kind":"numeric"}]`,
		`[{"entity":"resource","key":"camera..iso","kind":"numeric"}]`,
		`[{"entity":"resource","key":"score","kind":"numeric"},{"entity":"resource","key":"score","kind":"numeric"}]`,
	} {
		if _, err := ParseMetadataIndexes(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
	var tooMany []MetadataIndex
	for i := 0; i <= MaxMetadataIndexes; i++ {
		tooMany = append(tooMany, MetadataIndex{"resource", fmt.Sprintf("key%d", i), "numeric"})
	}
	raw, _ := json.Marshal(tooMany)
	if _, err := ParseMetadataIndexes(string(raw)); err == nil {
		t.Fatal("accepted too many indexes")
	}
	if _, err := ParseMetadataIndexes("[]"); err != nil {
		t.Fatal(err)
	}
}

func TestMetadataIndexIdentityAndValidation(t *testing.T) {
	base := MetadataIndex{"resource", "score", "numeric"}
	if base.Name() != (MetadataIndex{"resource", "score", "numeric"}).Name() {
		t.Fatal("name is unstable")
	}
	for _, other := range []MetadataIndex{{"note", "score", "numeric"}, {"resource", "Score", "numeric"}, {"resource", "score", "text"}} {
		if base.Name() == other.Name() {
			t.Fatal("colliding index identities")
		}
	}
	if _, err := base.CreateSQL("mysql"); err == nil {
		t.Fatal("accepted unsupported database")
	}
	for _, key := range []string{"", strings.Repeat("a", 129), "a.b.c.d.e.f.g.h.i", "a'", "a[0]"} {
		bad := base
		bad.Key = key
		if _, err := bad.CreateSQL("postgres"); err == nil {
			t.Fatalf("accepted unsafe key %q", key)
		}
	}
}

func TestMetadataIndexesSQLite(t *testing.T) { checkMetadataIndexQueries(t, setupTestDB(t)) }

// Verify both result semantics and optimizer use at the actual MRQL seam.
func checkMetadataIndexQueries(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, table := range []string{"resources", "notes", "groups"} {
		for i := 0; i < 100; i++ {
			meta := fmt.Sprintf(`{"score":%d,"camera":{"iso":%d},"label":"value-%d"}`, i, i, i)
			if i == 10 {
				meta = `{"score":10,"camera":{"iso":10},"label":"MiXeD"}`
			}
			if i == 11 {
				meta = `{"score":"010.0","label":"mixed"}`
			}
			if i == 12 {
				meta = `{"score":"bad","label":null}`
			}
			if i == 13 {
				meta = `{"score":null}`
			}
			if err := db.Exec("INSERT INTO "+table+" (id, name, meta) VALUES (?, ?, ?)", 100+i, fmt.Sprintf("indexed-%d", i), meta).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, test := range []struct {
		index  MetadataIndex
		filter string
	}{
		{MetadataIndex{"resource", "score", "numeric"}, `meta.score = 10`},
		{MetadataIndex{"note", "camera.iso", "numeric"}, `meta.camera.iso >= 95`},
		{MetadataIndex{"group", "label", "text"}, `meta.label = "MIXED"`},
	} {
		t.Run(test.index.Entity+"_"+test.index.Key, func(t *testing.T) {
			query := `type = ` + test.index.Entity + ` AND ` + test.filter + ` ORDER BY id LIMIT 50`
			var opts TranslateOptions
			run := func() []testResource {
				var rows []testResource
				if err := parseAndTranslate(t, query, ValidEntityTypes[test.index.Entity], db, opts).Find(&rows).Error; err != nil {
					t.Fatal(err)
				}
				return rows
			}
			before := run()
			if len(before) == 0 {
				t.Fatal("fixture produced no matches")
			}
			indexes, err := test.index.Indexes(db.Dialector.Name())
			if err != nil {
				t.Fatal(err)
			}
			for _, index := range indexes {
				if err := db.Exec(index.SQL).Error; err != nil {
					t.Fatal(err)
				}
			}
			opts.ReadyMetadataIndexes = []MetadataIndex{test.index}
			after := run()
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("index changed results: before=%v after=%v", before, after)
			}
			// No ORDER BY/LIMIT when inspecting usability: otherwise a tiny fixture's
			// primary-key ordering may be cheaper than the metadata index.
			built := parseAndTranslate(t, `type = `+test.index.Entity+` AND `+test.filter, ValidEntityTypes[test.index.Entity], db, opts)
			stmt := built.Session(&gorm.Session{DryRun: true}).Find(&[]testResource{}).Statement
			var plan string
			if db.Dialector.Name() == "postgres" {
				err = db.Connection(func(conn *gorm.DB) error {
					if err := conn.Exec("SET enable_seqscan = off").Error; err != nil {
						return err
					}
					defer conn.Exec("RESET enable_seqscan")
					return conn.Raw("EXPLAIN (FORMAT JSON) "+stmt.SQL.String(), stmt.Vars...).Scan(&plan).Error
				})
			} else {
				var lines []struct{ Detail string }
				err = db.Raw("EXPLAIN QUERY PLAN "+stmt.SQL.String(), stmt.Vars...).Scan(&lines).Error
				for _, line := range lines {
					plan += line.Detail
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, index := range indexes {
				if !strings.Contains(plan, index.Name) {
					t.Fatalf("MRQL did not use metadata index %s: %s", index.Name, plan)
				}
			}
			if err := db.Exec("UPDATE "+test.index.Table()+" SET meta = ? WHERE id = ?", `{"score":10,"camera":{"iso":99},"label":"mixed"}`, 199).Error; err != nil {
				t.Fatal(err)
			}
			found := false
			for _, row := range run() {
				found = found || row.ID == 199
			}
			if !found {
				t.Fatal("index did not include updated metadata")
			}
			if err := db.Exec("DELETE FROM " + test.index.Table() + " WHERE id = 199").Error; err != nil {
				t.Fatal(err)
			}
			for _, row := range run() {
				if row.ID == 199 {
					t.Fatal("index retained deleted entity")
				}
			}
		})
	}
}
