//go:build postgres

package mrql

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func TestMetadataIndexesPostgres(t *testing.T) { checkMetadataIndexQueries(t, setupPostgresTestDB(t)) }

func TestMetadataNumericTranslationRequiresReadyMatchingIndex(t *testing.T) {
	db := setupPostgresTestDB(t)
	for _, test := range []struct {
		query  string
		entity EntityType
		ready  MetadataIndex
		uses   bool
	}{
		{`meta.score = 10`, EntityResource, MetadataIndex{"resource", "score", "numeric"}, true},
		{`meta.score >= 10`, EntityResource, MetadataIndex{"resource", "score", "numeric"}, true},
		{`NOT (meta.score = 10)`, EntityResource, MetadataIndex{"resource", "score", "numeric"}, true},
		{`meta.score = 10`, EntityResource, MetadataIndex{"group", "score", "numeric"}, false},
		{`meta.score = 10`, EntityResource, MetadataIndex{"resource", "other", "numeric"}, false},
		{`meta.score = 10`, EntityResource, MetadataIndex{"resource", "score", "text"}, false},
		{`meta.camera.iso = 10`, EntityNote, MetadataIndex{"note", "camera.iso", "numeric"}, true},
		{`meta.camera.iso = 10`, EntityNote, MetadataIndex{"note", "iso", "numeric"}, false},
		{`owner.meta.score = 10`, EntityResource, MetadataIndex{"group", "score", "numeric"}, true},
		{`owner.meta.score = 10`, EntityResource, MetadataIndex{"resource", "score", "numeric"}, false},
		{`owner.parent.meta.score = 10`, EntityNote, MetadataIndex{"group", "score", "numeric"}, true},
		{`children.meta.score != 10`, EntityGroup, MetadataIndex{"group", "score", "numeric"}, true},
		{`ancestors.meta.score = 10`, EntityResource, MetadataIndex{"group", "score", "numeric"}, true},
		{`ancestors.meta.score = 10`, EntityResource, MetadataIndex{"resource", "score", "numeric"}, false},
		{`descendants.meta.score = 10`, EntityGroup, MetadataIndex{"group", "score", "numeric"}, true},
	} {
		t.Run(test.entity.String()+"_"+test.query+"_"+test.ready.Entity+"_"+test.ready.Key+"_"+test.ready.Kind, func(t *testing.T) {
			query := "type = " + test.entity.String() + " AND " + test.query
			buildSQL := func(opts TranslateOptions) string {
				built := parseAndTranslate(t, query, test.entity, db, opts)
				return built.Session(&gorm.Session{DryRun: true}).Find(&[]testResource{}).Statement.SQL.String()
			}
			plain := buildSQL(TranslateOptions{})
			if strings.Contains(plain, "length(") {
				t.Fatalf("unindexed query acquired indexed predicate overhead: %s", plain)
			}
			// This dry-run checks the caller-provided capability alone. Execution
			// tests above activate it only after creating the complete index set.
			withReady := buildSQL(TranslateOptions{ReadyMetadataIndexes: []MetadataIndex{test.ready}})
			if test.uses {
				if !strings.Contains(withReady, "length(") {
					t.Fatalf("matching ready index was ignored: %s", withReady)
				}
			} else if withReady != plain {
				t.Fatalf("unrelated ready index changed SQL:\nbefore %s\nafter %s", plain, withReady)
			}
		})
	}
}

func TestMetadataTextIndexPostgresLongValues(t *testing.T) {
	db := setupPostgresTestDB(t)
	index := MetadataIndex{"resource", "label", "text"}
	sql, err := index.CreateSQL("postgres")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(sql).Error; err != nil {
		t.Fatal(err)
	}
	// A hash index stores fixed-size hashes, so even incompressible metadata
	// exceeding B-tree tuple limits must remain writable.
	if err := db.Exec(`UPDATE resources SET meta = json_build_object('label', (SELECT string_agg(md5(n::text), '') FROM generate_series(1,1000) n)) WHERE id = 1`).Error; err != nil {
		t.Fatal(err)
	}
}

func TestMetadataNumericIndexPostgresLongValues(t *testing.T) {
	db := setupPostgresTestDB(t)
	values := []any{
		10, 9, 11, nil, "bad", "010.0",
		strings.Repeat("0", 2000) + "10",   // Long representation of an ordinary value.
		strings.Repeat("1234567890", 1000), // Exceeds the B-tree numeric tuple limit.
		"-" + strings.Repeat("1234567890", 1000),
		strings.Repeat("x", 2000),
		"10." + strings.Repeat("0", 999),
		strings.Repeat("0", 998) + "10", // Exact boundary.
		strings.Repeat("0", 999) + "10", // First value routed through the fallback.
	}
	writeValue := func(id int, value any) {
		t.Helper()
		meta, err := json.Marshal(map[string]any{"score": value, "camera": map[string]any{"iso": value}})
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(`INSERT INTO resources(id, name, meta) VALUES (?, 'long-index-value', ?) ON CONFLICT (id) DO UPDATE SET meta = excluded.meta`, id, string(meta)).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i, value := range values {
		writeValue(1000+i, value)
	}
	if err := db.Exec(`INSERT INTO resources(id, name, meta) VALUES (1099, 'missing-key', '{}')`).Error; err != nil {
		t.Fatal(err)
	}

	// Compare with the previous unbounded numeric expression, including NOT:
	// NULL and FALSE are equivalent at a top-level WHERE but differ under NOT.
	var opts TranslateOptions
	checkQueries := func() {
		t.Helper()
		for _, key := range []string{"score", "camera.iso"} {
			original := pgMetaNumericExpr("resources", strings.Split(key, "."))
			for _, test := range []struct{ mrql, sql string }{
				{"meta.%s = 10", "%s = 10"},
				{"meta.%s != 10", "%s != 10"},
				{"meta.%s > 10", "%s > 10"},
				{"meta.%s >= 10", "%s >= 10"},
				{"meta.%s < 10", "%s < 10"},
				{"meta.%s <= 10", "%s <= 10"},
				{"NOT (meta.%s = 10)", "NOT (%s = 10)"},
				{"NOT (NOT (meta.%s >= 10))", "NOT (NOT (%s >= 10))"},
			} {
				query := "type = resource AND id >= 1000 AND " + fmt.Sprintf(test.mrql, key) + " ORDER BY id"
				var got, want []testResource
				if err := parseAndTranslate(t, query, EntityResource, db, opts).Find(&got).Error; err != nil {
					t.Fatalf("%s: %v", query, err)
				}
				if err := db.Table("resources").Where("id >= 1000 AND (" + fmt.Sprintf(test.sql, original) + ")").Order("id").Find(&want).Error; err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("%s: translated query differs from original expression\ngot %v\nwant %v", query, got, want)
				}
			}
		}
	}
	checkQueries()
	// Such strings were valid metadata even though explicitly casting them in
	// a numeric query can overflow. Index creation and writes must not cast them.
	writeValue(1100, strings.Repeat("1", 140000))
	for _, key := range []string{"score", "camera.iso"} {
		indexes, err := (MetadataIndex{"resource", key, "numeric"}).Indexes("postgres")
		if err != nil {
			t.Fatal(err)
		}
		for _, index := range indexes {
			if err := db.Exec(index.SQL).Error; err != nil {
				t.Fatal(err)
			}
		}
		opts.ReadyMetadataIndexes = append(opts.ReadyMetadataIndexes, MetadataIndex{"resource", key, "numeric"})
	}
	writeValue(1100, strings.Repeat("2", 140000))
	writeValue(1101, strings.Repeat("3", 140000))
	if err := db.Exec("DELETE FROM resources WHERE id >= 1100").Error; err != nil {
		t.Fatal(err)
	}
	checkQueries()
	for i, value := range values {
		writeValue(1000+i, value)
		writeValue(1200+i, value)
	}
	checkQueries()
}
