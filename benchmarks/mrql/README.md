# MRQL performance harness

This harness measures production MRQL parsing, translation, execution, hydration, HTTP encoding, and shortcode rendering against deterministic fixtures. It supports SQLite and PostgreSQL while keeping generated databases and sensitive values out of Git.

## Terms

- **Dataset profile**: deterministic entity and relation distributions led by resource cardinality.
- **Scenario**: one versioned query/render workload from the curated catalog.
- **Reference baseline**: results captured on a documented but non-stable machine.
- **Canonical baseline**: results captured on a stable, documented benchmark host.
- **Native plan**: non-executing SQLite `EXPLAIN QUERY PLAN` or PostgreSQL `EXPLAIN (FORMAT JSON)` output, captured after timing.

## Discover profiles and scenarios

```bash
go run --tags 'json1 fts5' ./cmd/mrql-bench list
go run --tags 'json1 fts5' ./cmd/mrql-bench list --json
```

Standard profiles contain 100k, 1m, or 3m resources. Notes, groups, tags, hierarchy edges, metadata, FTS content, and similarity rows are deterministic proportions recorded in the fixture manifest. `tiny` exists for integration checks only.

## SQLite

```bash
mkdir -p benchmarks/mrql/fixtures benchmarks/mrql/results

go run --tags 'json1 fts5' ./cmd/mrql-bench prepare \
  --backend sqlite \
  --profile 100k \
  --dsn benchmarks/mrql/fixtures/100k.sqlite \
  --manifest benchmarks/mrql/fixtures/100k-sqlite.manifest.json

go run --tags 'json1 fts5' ./cmd/mrql-bench run \
  --backend sqlite \
  --dsn benchmarks/mrql/fixtures/100k.sqlite \
  --manifest benchmarks/mrql/fixtures/100k-sqlite.manifest.json \
  --warmups 5 --samples 100 \
  --output benchmarks/mrql/results/100k-sqlite.json
```

Prepared fixtures are immutable. Reuse requires a compatible manifest; pass `--force` to `prepare` only when intentionally replacing a SQLite fixture.

## PostgreSQL

A disposable PostgreSQL 16 container is the safe default for one-shot runs:

```bash
go run --tags 'json1 fts5' ./cmd/mrql-bench run \
  --backend postgres --profile 100k \
  --warmups 5 --samples 100 \
  --output benchmarks/mrql/results/100k-postgres.json
```

For reusable large fixtures, provide a PostgreSQL URL whose database name contains `benchmark`. Preparation requires `--allow-destructive` and refuses any non-empty database. The tool never silently reads `DB_DSN`, truncates an arbitrary database, or stores DSN credentials in artifacts.

## Compare runs

```bash
go run ./cmd/mrql-bench compare \
  benchmarks/mrql/baselines/100k-sqlite.json \
  benchmarks/mrql/results/100k-sqlite.json
```

Compatibility requires matching artifact/catalog versions, fixture checksum and dialect, database version, concurrency, scenario IDs, and query fingerprints. SQL-count/result-bound failures are deterministic errors. Timing alerts are advisory: the default is a 15% increase plus the scenario's minimum absolute delta. Every delta is reported even below the alert threshold.

## Standard Go benchmarks

```bash
MRQL_BENCH_DSN=benchmarks/mrql/fixtures/100k.sqlite \
MRQL_BENCH_MANIFEST=benchmarks/mrql/fixtures/100k-sqlite.manifest.json \
go test --tags 'json1 fts5' ./benchmarks/mrql -bench . -benchmem
```

Set `MRQL_BENCH_SCENARIOS=scalar-selective,bucket-content-type` to select cases. Ordinary `go test` skips these benchmarks when no prepared fixture is configured and never seeds a large database implicitly.

## Measurement policy

- Fixture creation, migrations, `ANALYZE`, FTS setup, warmups, and native-plan capture are outside measured samples.
- Warm-cache latency is the primary repeatable measurement. `firstRunNanos` is recorded separately but is not claimed to be a true cold-cache result.
- Canonical runs require at least 100 measured samples before p99 is reported. Smaller smoke runs mark p99 unavailable.
- Official baselines use one worker. `concurrency` remains part of artifact compatibility so exploratory load runs cannot be mixed with latency baselines.
- `outputBytes` is encoded JSON/HTML size, not database wire traffic. Go benchmarks separately report allocations.
- Phase labels are intentionally honest: parsing/binding/validation is separate, while translation + database execution + transfer + model decoding is one combined phase because GORM does not expose portable wire/decode boundaries. Shortcode rendering reports its full template/hydration phase.
- SQL counts include every statement during the measured production operation: scope resolution, auxiliary counts, hydration, bucket fan-out, and nested MRQL. Setup and post-run planning are excluded.
- PostgreSQL and SQLite plans remain dialect-native. Stable plan signatures summarize node/scan/index structure; raw estimated costs are not equality gates.
- Query and SQL fingerprints redact bound values. Artifacts contain no DSNs, generated payload dumps, or interpolated SQL.

## Scale runs

The completion smoke checks prepare 1m and 3m fixtures and run selected correctness scenarios once. Full 100-sample matrices are manual because they can consume substantial disk, WAL, CPU, and time:

```bash
# Run each command with PROFILE=1m and again with PROFILE=3m.
PROFILE=1m

go run --tags 'json1 fts5' ./cmd/mrql-bench prepare --backend sqlite --profile "$PROFILE" \
  --dsn "benchmarks/mrql/fixtures/$PROFILE.sqlite" \
  --manifest "benchmarks/mrql/fixtures/$PROFILE-sqlite.manifest.json"

go run --tags 'json1 fts5' ./cmd/mrql-bench run --backend sqlite \
  --dsn "benchmarks/mrql/fixtures/$PROFILE.sqlite" \
  --manifest "benchmarks/mrql/fixtures/$PROFILE-sqlite.manifest.json" \
  --scenario scalar-selective,relation-common-tag,page-deep,bucket-content-type \
  --samples 1 --warmups 0 --status raw \
  --output "benchmarks/mrql/results/$PROFILE-sqlite-smoke.json"

# PostgreSQL uses a disposable PostgreSQL 16 container for each profile.
go run --tags 'json1 fts5' ./cmd/mrql-bench run --backend postgres --profile "$PROFILE" \
  --scenario scalar-selective,relation-common-tag,page-deep,bucket-content-type \
  --samples 1 --warmups 0 --status raw \
  --output "benchmarks/mrql/results/$PROFILE-postgres-smoke.json"
```

Baseline promotion is a reviewed file operation, not an automatic command. Canonical runs require at least 100 samples, a clean known Git revision, and an explicit stable host identity via `--canonical-host`. The artifact records that host ID, hardware, OS, Go/database versions, pool/concurrency settings, fixture checksum, scenario selection, timeout, warmups, and sample count without storing the DSN.
