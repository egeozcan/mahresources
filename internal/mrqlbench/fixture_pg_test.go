//go:build postgres

package mrqlbench

import (
	"context"
	"testing"
	"time"

	"mahresources/constants"
	"mahresources/internal/testpgutil"
)

func TestPostgresPrepareRunCompareParity(t *testing.T) {
	container, err := testpgutil.StartContainer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer container.Stop(context.Background())
	db := container.CreateTestDB(t)
	if err := db.Exec("CREATE SCHEMA existing; CREATE TABLE existing.data (id bigint)").Error; err != nil {
		t.Fatal(err)
	}
	if count, err := CountPostgresUserTables(db); err != nil || count != 1 {
		t.Fatalf("non-public user-table count = %d, err=%v", count, err)
	}
	if err := db.Exec("DROP SCHEMA existing CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	manifest, err := PrepareFixture(context.Background(), db, PrepareOptions{Profile: TinyProfile(), Dialect: constants.DbTypePosgres, BatchSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewRunner(db, manifest)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := runner.Run(context.Background(), RunOptions{Scenarios: []string{"scalar-selective", "bucket-content-type"}, Samples: 1, Status: "raw", Timeout: time.Minute, PoolSize: 4, Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	comparison := Compare(artifact, artifact)
	if !comparison.Compatible || len(comparison.Regressions) != 0 {
		t.Fatalf("self comparison = %#v", comparison)
	}

	nestedOptions := RunOptions{Scenarios: []string{"nested-mrql"}, Samples: 3, Status: "raw", Timeout: time.Minute, PoolSize: 4, Concurrency: 1}
	firstNested, err := runner.Run(context.Background(), nestedOptions)
	if err != nil {
		t.Fatal(err)
	}
	secondNested, err := runner.Run(context.Background(), nestedOptions)
	if err != nil {
		t.Fatal(err)
	}
	secondNested.Results[0].Latency = firstNested.Results[0].Latency
	if got := Compare(firstNested, secondNested); !got.Compatible {
		t.Fatalf("independent deterministic nested runs are incompatible: %#v", got)
	}
}
