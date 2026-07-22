package mrqlbench

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"mahresources/constants"
	"mahresources/models"
	"mahresources/mrql"
)

func TestRunnerRejectsUndersampledCanonicalArtifact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "canonical.db")
	db, _, err := models.CreateDatabaseConnection(constants.DbTypeSqlite, path, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() }()
	manifest, err := PrepareFixture(context.Background(), db, PrepareOptions{Profile: TinyProfile(), Dialect: constants.DbTypeSqlite, BatchSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewRunner(db, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Run(context.Background(), RunOptions{Scenarios: []string{"scalar-selective"}, Samples: 1, Status: "canonical"}); err == nil {
		t.Fatal("expected undersampled canonical run to fail")
	}
}

func TestRunnerMeasuresRealSQLAndProducesNativePlans(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runner.db")
	db, _, err := models.CreateDatabaseConnection(constants.DbTypeSqlite, path, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() }()
	manifest, err := PrepareFixture(context.Background(), db, PrepareOptions{Profile: TinyProfile(), Dialect: constants.DbTypeSqlite, BatchSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewRunner(db, manifest)
	if err != nil {
		t.Fatal(err)
	}
	groupedQuery, err := mrql.Parse(`type = "resource" GROUP BY contentType LIMIT 10`)
	if err != nil {
		t.Fatal(err)
	}
	if err := mrql.Validate(groupedQuery); err != nil {
		t.Fatal(err)
	}
	groupedQuery.EntityType = mrql.ExtractEntityType(groupedQuery)
	runner.collector.Reset("manual")
	grouped, err := runner.app.ExecuteMRQLGrouped(WithSample(context.Background(), "manual"), groupedQuery)
	if err != nil {
		t.Fatal(err)
	}
	if len(grouped.Groups) == 0 {
		t.Fatal("prepared fixture produced no content-type buckets")
	}
	if resources, ok := grouped.Groups[0].Items.([]models.Resource); !ok || len(resources) == 0 {
		t.Fatalf("first bucket items = %#v", grouped.Groups[0].Items)
	}
	if observations := runner.collector.Snapshot("manual"); len(observations) < 2 {
		t.Fatalf("manual bucket execution recorded only %#v", observations)
	}
	artifact, err := runner.Run(context.Background(), RunOptions{Scenarios: []string{"scalar-selective", "bucket-content-type"}, Samples: 2, Timeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Results) != 2 {
		t.Fatalf("results = %d", len(artifact.Results))
	}
	for _, result := range artifact.Results {
		if len(result.Samples) != 2 || result.Samples[0].SQLStatements == 0 {
			t.Fatalf("scenario %s has no measured SQL: %#v", result.ScenarioID, result)
		}
		if result.QueryFingerprint == "" || len(result.Plans) == 0 {
			t.Fatalf("scenario %s lacks fingerprint/plans: %#v", result.ScenarioID, result)
		}
		if result.Latency.P99 != nil {
			t.Fatalf("scenario %s reported p99 from two samples", result.ScenarioID)
		}
	}

	concurrent, err := runner.Run(context.Background(), RunOptions{Scenarios: []string{"scalar-selective"}, Samples: 4, Concurrency: 2, Timeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if len(concurrent.Results) != 1 || len(concurrent.Results[0].Samples) != 4 || concurrent.Environment.Concurrency != 2 {
		t.Fatalf("concurrent run = %#v", concurrent)
	}
}
