package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
	"mahresources/constants"
	"mahresources/internal/mrqlbench"
	"mahresources/models"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cmd := newRootCommand(os.Stdout, os.Stderr)
	if err := cmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand(stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{Use: "mrql-bench", Short: "Prepare and run reproducible MRQL benchmarks", SilenceUsage: true, SilenceErrors: true}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.AddCommand(newListCommand(stdout), newPrepareCommand(stdout), newRunCommand(stdout), newCompareCommand(stdout))
	return root
}

func newListCommand(stdout io.Writer) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "list", Short: "List profiles and scenarios", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		payload := struct {
			Profiles  []mrqlbench.Profile  `json:"profiles"`
			Scenarios []mrqlbench.Scenario `json:"scenarios"`
		}{mrqlbench.Profiles(), mrqlbench.Scenarios()}
		if asJSON {
			encoder := json.NewEncoder(stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(payload)
		}
		fmt.Fprintln(stdout, "Profiles:")
		for _, profile := range payload.Profiles {
			fmt.Fprintf(stdout, "  %-5s %d resources\n", profile.ID, profile.Resources)
		}
		fmt.Fprintln(stdout, "Scenarios:")
		for _, scenario := range payload.Scenarios {
			fmt.Fprintf(stdout, "  %-28s %s\n", scenario.ID, scenario.Description)
		}
		return nil
	}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
	return cmd
}

func newPrepareCommand(stdout io.Writer) *cobra.Command {
	var backend, dsn, profileID, manifestPath, revision string
	var batchSize int
	var force, allowDestructive bool
	cmd := &cobra.Command{Use: "prepare", Short: "Create a deterministic benchmark fixture", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		profile, ok := profileFromFlag(profileID)
		if !ok {
			return fmt.Errorf("unknown profile %q", profileID)
		}
		if backend == "" {
			backend = "sqlite"
		}
		backend = normalizeBackend(backend)
		if dsn == "" {
			return errors.New("--dsn is required (PostgreSQL disposable fixtures are prepared by run)")
		}
		if manifestPath == "" {
			if backend == "postgres" {
				return errors.New("explicit PostgreSQL preparation requires a credential-free --manifest path")
			}
			manifestPath = dsn + ".manifest.json"
		}
		if backend == "sqlite" {
			if force {
				for _, suffix := range []string{"", "-wal", "-shm"} {
					_ = os.Remove(dsn + suffix)
				}
				_ = os.Remove(manifestPath)
			}
			if _, err := os.Stat(dsn); err == nil {
				return fmt.Errorf("SQLite fixture %q exists; use --force to rebuild", dsn)
			}
		} else if backend == "postgres" {
			if !allowDestructive {
				return errors.New("explicit PostgreSQL preparation requires --allow-destructive")
			}
			if err := requireBenchmarkPostgresDSN(dsn); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("unsupported backend %q", backend)
		}
		db, closeDB, err := openDatabase(backend, dsn, 2)
		if err != nil {
			return sanitizeError(err, dsn)
		}
		defer closeDB()
		if backend == "postgres" {
			count, err := mrqlbench.CountPostgresUserTables(db)
			if err != nil {
				return sanitizeError(err, dsn)
			}
			if count != 0 {
				return fmt.Errorf("refusing to prepare non-empty PostgreSQL database (%d user tables)", count)
			}
		}
		manifest, err := mrqlbench.PrepareFixture(cmd.Context(), db, mrqlbench.PrepareOptions{Profile: profile, Dialect: dialectConstant(backend), BatchSize: batchSize, Revision: revision})
		if err != nil {
			return sanitizeError(err, dsn)
		}
		if err := mrqlbench.WriteJSONAtomic(manifestPath, manifest); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "prepared %s %s fixture (%d resources); manifest: %s\n", backend, profile.ID, profile.Resources, manifestPath)
		return nil
	}}
	cmd.Flags().StringVar(&backend, "backend", "sqlite", "sqlite or postgres")
	cmd.Flags().StringVar(&dsn, "dsn", "", "SQLite path or PostgreSQL URL")
	cmd.Flags().StringVar(&profileID, "profile", "100k", "100k, 1m, 3m, or tiny")
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "manifest output path")
	cmd.Flags().StringVar(&revision, "revision", "", "schema/code revision label")
	cmd.Flags().IntVar(&batchSize, "batch-size", 500, "fixture insertion batch size")
	cmd.Flags().BoolVar(&force, "force", false, "replace an existing SQLite fixture")
	cmd.Flags().BoolVar(&allowDestructive, "allow-destructive", false, "acknowledge explicit PostgreSQL schema preparation")
	return cmd
}

func newRunCommand(stdout io.Writer) *cobra.Command {
	var backend, dsn, manifestPath, profileID, output, status, canonicalHost string
	var scenarios []string
	var warmups, samples, poolSize, concurrency int
	var timeout time.Duration
	cmd := &cobra.Command{Use: "run", Short: "Run benchmark scenarios", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if backend == "" {
			backend = "sqlite"
		}
		backend = normalizeBackend(backend)
		if backend != "sqlite" && backend != "postgres" {
			return fmt.Errorf("unsupported backend %q", backend)
		}
		var disposable *mrqlbench.DisposablePostgres
		var manifest mrqlbench.FixtureManifest
		if backend == "postgres" && dsn == "" && manifestPath == "" {
			profile, ok := profileFromFlag(profileID)
			if !ok {
				return fmt.Errorf("unknown profile %q", profileID)
			}
			var err error
			disposable, err = mrqlbench.StartDisposablePostgres(cmd.Context())
			if err != nil {
				return err
			}
			defer disposable.Close(context.Background())
			dsn = disposable.DSN
			db, closeDB, err := openDatabase(backend, dsn, poolSize)
			if err != nil {
				return sanitizeError(err, dsn)
			}
			defer closeDB()
			manifest, err = mrqlbench.PrepareFixture(cmd.Context(), db, mrqlbench.PrepareOptions{Profile: profile, Dialect: constants.DbTypePosgres, BatchSize: 500, Revision: gitRevision()})
			if err != nil {
				return sanitizeError(err, dsn)
			}
			return runAndWrite(cmd.Context(), stdout, db, manifest, output, status, canonicalHost, scenarios, warmups, samples, timeout, poolSize, concurrency)
		}
		if dsn == "" || manifestPath == "" {
			return errors.New("--dsn and --manifest are required for prepared fixtures")
		}
		loaded, err := mrqlbench.ReadManifest(manifestPath)
		if err != nil {
			return err
		}
		manifest = loaded
		if normalizeBackend(backend) != manifest.Dialect {
			return fmt.Errorf("backend %q does not match manifest dialect %q", backend, manifest.Dialect)
		}
		db, closeDB, err := openDatabase(backend, dsn, poolSize)
		if err != nil {
			return sanitizeError(err, dsn)
		}
		defer closeDB()
		return runAndWrite(cmd.Context(), stdout, db, manifest, output, status, canonicalHost, scenarios, warmups, samples, timeout, poolSize, concurrency)
	}}
	cmd.Flags().StringVar(&backend, "backend", "sqlite", "sqlite or postgres")
	cmd.Flags().StringVar(&dsn, "dsn", "", "prepared fixture DSN/path")
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "prepared fixture manifest")
	cmd.Flags().StringVar(&profileID, "profile", "100k", "profile for disposable PostgreSQL")
	cmd.Flags().StringVar(&output, "output", "", "result JSON path (required)")
	cmd.Flags().StringVar(&status, "status", "reference", "raw, reference, or canonical")
	cmd.Flags().StringVar(&canonicalHost, "canonical-host", "", "stable host identifier required for canonical status")
	cmd.Flags().StringSliceVar(&scenarios, "scenario", nil, "scenario ID (repeatable/comma-separated)")
	cmd.Flags().IntVar(&warmups, "warmups", 5, "untimed warmup executions")
	cmd.Flags().IntVar(&samples, "samples", 100, "measured samples")
	cmd.Flags().IntVar(&poolSize, "pool-size", 4, "database connection pool size")
	cmd.Flags().IntVar(&concurrency, "concurrency", 1, "parallel measured executions (official baselines use 1)")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "timeout per scenario")
	_ = cmd.MarkFlagRequired("output")
	return cmd
}

func runAndWrite(ctx context.Context, stdout io.Writer, db *gorm.DB, manifest mrqlbench.FixtureManifest, output, status, canonicalHost string, scenarios []string, warmups, samples int, timeout time.Duration, poolSize, concurrency int) error {
	runner, err := mrqlbench.NewRunner(db, manifest)
	if err != nil {
		return err
	}
	artifact, err := runner.Run(ctx, mrqlbench.RunOptions{Scenarios: scenarios, Warmups: warmups, Samples: samples, Timeout: timeout, Status: status, Revision: gitRevision(), CanonicalHost: canonicalHost, PoolSize: poolSize, Concurrency: concurrency})
	if err != nil {
		return err
	}
	if status == "reference" || status == "canonical" {
		artifact = mrqlbench.AggregateOnly(artifact)
	}
	if err := mrqlbench.WriteJSONAtomic(output, artifact); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %d scenario results to %s\n", len(artifact.Results), output)
	return nil
}

func newCompareCommand(stdout io.Writer) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "compare REFERENCE CANDIDATE", Short: "Compare compatible result artifacts", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		reference, err := mrqlbench.ReadRun(args[0])
		if err != nil {
			return err
		}
		candidate, err := mrqlbench.ReadRun(args[1])
		if err != nil {
			return err
		}
		comparison := mrqlbench.Compare(reference, candidate)
		if asJSON {
			encoder := json.NewEncoder(stdout)
			encoder.SetIndent("", "  ")
			if err := encoder.Encode(comparison); err != nil {
				return err
			}
		} else {
			for _, delta := range comparison.Deltas {
				fmt.Fprintf(stdout, "%-28s %+7.2f%% %+dns\n", delta.ScenarioID, delta.RelativePercent, delta.AbsoluteNanos)
			}
			for _, message := range comparison.Errors {
				fmt.Fprintf(stdout, "incompatible: %s\n", message)
			}
		}
		if !comparison.Compatible {
			return errors.New("benchmark artifacts are incompatible")
		}
		if len(comparison.Regressions) > 0 {
			return fmt.Errorf("%d benchmark regression(s) detected", len(comparison.Regressions))
		}
		return nil
	}}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
	return cmd
}

func openDatabase(backend, dsn string, poolSize int) (*gorm.DB, func(), error) {
	db, _, err := models.CreateDatabaseConnection(dialectConstant(backend), dsn, "", 0)
	if err != nil {
		return nil, func() {}, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, func() {}, err
	}
	if poolSize > 0 {
		sqlDB.SetMaxOpenConns(poolSize)
		sqlDB.SetMaxIdleConns(poolSize)
	}
	return db, func() { _ = sqlDB.Close() }, nil
}

func profileFromFlag(id string) (mrqlbench.Profile, bool) {
	if id == "tiny" {
		return mrqlbench.TinyProfile(), true
	}
	return mrqlbench.ProfileByID(id)
}
func dialectConstant(backend string) string {
	if normalizeBackend(backend) == "postgres" {
		return constants.DbTypePosgres
	}
	return constants.DbTypeSqlite
}
func normalizeBackend(backend string) string {
	backend = strings.ToLower(backend)
	if backend == "postgresql" {
		return "postgres"
	}
	return backend
}
func sanitizeError(err error, dsn string) error {
	if err == nil {
		return nil
	}
	return errors.New(strings.ReplaceAll(err.Error(), dsn, "<redacted-dsn>"))
}
func gitRevision() string {
	output, err := exec.Command("git", "rev-parse", "--short=12", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	revision := strings.TrimSpace(string(output))
	if status, statusErr := exec.Command("git", "status", "--porcelain").Output(); statusErr == nil && len(bytes.TrimSpace(status)) > 0 {
		revision += "-dirty"
	}
	return revision
}
func requireBenchmarkPostgresDSN(dsn string) error {
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return errors.New("invalid PostgreSQL DSN")
	}
	if !strings.Contains(strings.ToLower(config.Database), "benchmark") {
		return errors.New("effective PostgreSQL database name must contain 'benchmark'")
	}
	if searchPath := config.RuntimeParams["search_path"]; searchPath != "" {
		return errors.New("PostgreSQL benchmark DSN must not set search_path")
	}
	return nil
}
