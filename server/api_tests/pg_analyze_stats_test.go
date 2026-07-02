//go:build postgres

package api_tests

import (
	"testing"

	"mahresources/models"
)

// TestAnalyzePerceptualHashTables_RefreshesPlannerStats: after AutoMigrate adds
// the v2 hash columns, Postgres has no statistics for them until autoanalyze
// triggers at ~10% row churn — and without stats the planner seq-scans the whole
// image_hashes table for every chunk-index candidate query (observed 334ms vs
// 0.135ms per query on a 2.1M-row deployment). The post-migrate ANALYZE closes
// that gap. pg_class.reltuples is -1 for a never-analyzed table and >= 0 once
// ANALYZE has run, which makes it a deterministic, synchronous probe.
func TestAnalyzePerceptualHashTables_RefreshesPlannerStats(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	relTuples := func(table string) float64 {
		var v float64
		if err := tc.DB.Raw("SELECT reltuples FROM pg_class WHERE relname = ?", table).Scan(&v).Error; err != nil {
			t.Fatalf("reltuples(%s): %v", table, err)
		}
		return v
	}

	// Fresh table: normally -1 (never analyzed). Logged, not asserted — a
	// racing autoanalyze would make a hard precondition flaky.
	t.Logf("pre-ANALYZE reltuples: image_hashes=%v resource_similarities=%v",
		relTuples("image_hashes"), relTuples("resource_similarities"))

	if err := models.AnalyzePerceptualHashTables(tc.DB); err != nil {
		t.Fatalf("AnalyzePerceptualHashTables: %v", err)
	}

	for _, table := range []string{"image_hashes", "resource_similarities"} {
		if got := relTuples(table); got < 0 {
			t.Errorf("%s: reltuples = %v, want >= 0 (statistics present)", table, got)
		}
	}
}
