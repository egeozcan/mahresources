//go:build postgres

package mrql

import (
	"os"
	"testing"
	"time"

	"gorm.io/gorm"
)

// Opt-in diagnostic benchmark: no large fixture in the regular suite.
func TestPG_RandomMetadataPerformance(t *testing.T) {
	if os.Getenv("MRQL_RANDOM_PERF") == "" {
		t.Skip("set MRQL_RANDOM_PERF=1")
	}
	db := setupPostgresTestDB(t)
	for _, sql := range []string{
		`TRUNCATE resources`,
		`ALTER TABLE resources ALTER COLUMN meta TYPE jsonb USING meta::jsonb`,
		`INSERT INTO resources (id, name, description, meta) SELECT n, 'resource-' || n, repeat(md5(n::text), 40), jsonb_build_object('score', n % 20, 'payload', repeat(md5(n::text), 40)) FROM generate_series(1, 200000) n`,
		`ANALYZE resources`,
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	var opts TranslateOptions
	if os.Getenv("MRQL_RANDOM_INDEX") != "" {
		if err := db.Exec(`CREATE INDEX resources_score_numeric ON resources ((CASE WHEN meta->>'score' ~ '^-{0,1}[0-9]+(\.[0-9]+){0,1}$' THEN (meta->>'score')::numeric ELSE NULL END)) INCLUDE (id)`).Error; err != nil {
			t.Fatal(err)
		}
		indexes, err := (MetadataIndex{"resource", "score", "numeric"}).Indexes("postgres")
		if err != nil {
			t.Fatal(err)
		}
		for _, index := range indexes {
			if err := db.Exec(index.SQL).Error; err != nil {
				t.Fatal(err)
			}
		}
		opts.ReadyMetadataIndexes = []MetadataIndex{{"resource", "score", "numeric"}}
		if err := db.Exec(`VACUUM ANALYZE resources`).Error; err != nil {
			t.Fatal(err)
		}
	}
	query := parseAndTranslate(t, `type = resource AND meta.score = 10 ORDER BY RANDOM() LIMIT 50`, EntityResource, db, opts)
	stmt := query.Session(&gorm.Session{DryRun: true}).Find(&[]testResource{}).Statement
	samples := []struct{ name, sql string }{
		{"original", `SELECT * FROM resources WHERE 1 = 1 AND CASE WHEN resources.meta->>'score' ~ '^-{0,1}[0-9]+(\.[0-9]+){0,1}$' THEN (resources.meta->>'score')::numeric ELSE NULL END = 10 ORDER BY RANDOM() LIMIT 50`},
		{"translated", db.Dialector.Explain(stmt.SQL.String(), stmt.Vars...)},
	}
	// Interleave old/new queries, warming both before recording five samples.
	for i := 0; i < 8; i++ {
		for _, sample := range samples {
			start := time.Now()
			var rows []testResource
			if err := db.Raw(sample.sql).Scan(&rows).Error; err != nil {
				t.Fatal(err)
			}
			if len(rows) != 50 {
				t.Fatalf("%s: %d rows", sample.name, len(rows))
			}
			if i >= 3 {
				t.Logf("%s: %s", sample.name, time.Since(start))
			}
		}
	}
	for _, sample := range samples {
		t.Logf("%s plan:", sample.name)
		var plan []struct {
			Plan string `gorm:"column:QUERY PLAN"`
		}
		if err := db.Raw("EXPLAIN (ANALYZE, BUFFERS) " + sample.sql).Scan(&plan).Error; err != nil {
			t.Fatal(err)
		}
		for _, line := range plan {
			t.Log(line.Plan)
		}
	}
}
