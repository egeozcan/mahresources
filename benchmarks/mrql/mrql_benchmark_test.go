package mrql_benchmark_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"mahresources/constants"
	"mahresources/internal/mrqlbench"
	"mahresources/models"
)

func BenchmarkMRQL(b *testing.B) {
	dsn := os.Getenv("MRQL_BENCH_DSN")
	manifestPath := os.Getenv("MRQL_BENCH_MANIFEST")
	if dsn == "" || manifestPath == "" {
		b.Skip("set MRQL_BENCH_DSN and MRQL_BENCH_MANIFEST to a prepared fixture; see benchmarks/mrql/README.md")
	}
	manifest, err := mrqlbench.ReadManifest(manifestPath)
	if err != nil {
		b.Fatal(err)
	}
	dbType := constants.DbTypeSqlite
	if manifest.Dialect == "postgres" {
		dbType = constants.DbTypePosgres
	}
	db, _, err := models.CreateDatabaseConnection(dbType, dsn, "", 0)
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		b.Fatal(err)
	}
	defer sqlDB.Close()
	runner, err := mrqlbench.NewRunner(db, manifest)
	if err != nil {
		b.Fatal(err)
	}

	ids := []string{"scalar-selective", "relation-common-tag", "page-deep", "aggregate-content-type", "bucket-content-type", "cross-entity-top-n"}
	if configured := os.Getenv("MRQL_BENCH_SCENARIOS"); configured != "" {
		ids = strings.Split(configured, ",")
	}
	for _, id := range ids {
		id := strings.TrimSpace(id)
		b.Run(id, func(b *testing.B) {
			b.ReportAllocs()
			var sqlTotal, outputTotal int64
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				sample, err := runner.Measure(context.Background(), id, b.Name())
				if err != nil {
					b.Fatal(err)
				}
				sqlTotal += int64(sample.SQLStatements)
				outputTotal += sample.OutputBytes
			}
			b.StopTimer()
			if b.N > 0 {
				b.ReportMetric(float64(sqlTotal)/float64(b.N), "sql/op")
				b.ReportMetric(float64(outputTotal)/float64(b.N), "output-bytes/op")
			}
		})
	}
}
