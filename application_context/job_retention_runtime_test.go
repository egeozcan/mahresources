package application_context

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"mahresources/constants"
	"mahresources/jobs"
	"mahresources/models"

	"github.com/jmoiron/sqlx"
	"gorm.io/driver/sqlite"
)

const retentionRuntimeTestKind = "retention-runtime-test"

func TestJobRetentionRuntimeStartsAndContinuesItsBoundedCursor(t *testing.T) {
	ctx := newJobContext(t)
	if err := ctx.db.AutoMigrate(&models.JobRuntimeFence{}); err != nil {
		t.Fatalf("migrate runtime fence: %v", err)
	}

	base := time.Now().UTC().Add(-48 * time.Hour)
	pinnedID := makeExpiredRetentionRuntimeJob(t, ctx, "pinned", base)
	dueID := makeExpiredRetentionRuntimeJob(t, ctx, "behind pinned", base.Add(time.Minute))
	pinnedAt := time.Now().UTC()
	if err := ctx.db.Create(&models.JobPreference{JobID: pinnedID, UserID: 42, PinnedAt: &pinnedAt}).Error; err != nil {
		t.Fatalf("pin first expired Job: %v", err)
	}

	runtime := NewJobRetentionRuntime(ctx, ctx.JobService(), JobRetentionRuntimeConfig{
		Interval: time.Hour, ContinuationInterval: 10 * time.Millisecond,
		LeaseDuration: time.Second, LeaseRefreshInterval: 250 * time.Millisecond,
		BatchSize: 1,
	})
	runtime.Start()
	waitForRetentionJobState(t, ctx, dueID, false)
	runtime.Stop()

	if !retentionJobExists(t, ctx, pinnedID) {
		t.Fatal("the sweep pruned pinned history")
	}
	if retentionJobExists(t, ctx, dueID) {
		t.Fatal("the cursor did not continue past the pinned first candidate")
	}

	// A fresh runtime has no in-memory cursor. Its startup batch starts from the
	// oldest due work again and still reaches new history behind the pinned row.
	restartedID := makeExpiredRetentionRuntimeJob(t, ctx, "after restart", base.Add(2*time.Minute))
	restarted := NewJobRetentionRuntime(ctx, ctx.JobService(), JobRetentionRuntimeConfig{
		Interval: time.Hour, ContinuationInterval: 10 * time.Millisecond,
		LeaseDuration: time.Second, LeaseRefreshInterval: 250 * time.Millisecond,
		BatchSize: 1,
	})
	restarted.Start()
	waitForRetentionJobState(t, ctx, restartedID, false)
	restarted.Stop()
	if retentionJobExists(t, ctx, restartedID) {
		t.Fatal("a restarted runtime failed to start a new cursor cycle")
	}
}

func TestJobRetentionRuntimeStopsArtifactCleanupWithItsLifecycle(t *testing.T) {
	ctx := newJobContext(t)
	if err := ctx.db.AutoMigrate(&models.JobRuntimeFence{}); err != nil {
		t.Fatalf("migrate runtime fence: %v", err)
	}
	service := ctx.JobService()
	adapter := newRuntimeTestAdapter()
	adapter.def.Kind = retentionRuntimeTestKind
	if err := service.RegisterAdapter(adapter); err != nil {
		t.Fatalf("register retention test Kind: %v", err)
	}
	entered := make(chan struct{})
	canceled := make(chan struct{})
	adapter.cleanup = func(callCtx context.Context, _ jobs.ArtifactCleanupRequest) (jobs.ArtifactCleanupResult, error) {
		close(entered)
		<-callCtx.Done()
		close(canceled)
		return jobs.ArtifactCleanupResult{}, callCtx.Err()
	}

	createExpiredArtifactJob(t, ctx)

	runtime := NewJobRetentionRuntime(ctx, service, JobRetentionRuntimeConfig{
		Interval: time.Hour, ContinuationInterval: 10 * time.Millisecond,
		LeaseDuration: time.Second, LeaseRefreshInterval: 250 * time.Millisecond,
		QuiesceTimeout: time.Second, BatchSize: 1,
	})
	runtime.Start()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		runtime.Stop()
		t.Fatal("startup sweep did not reach the expired artifact cleanup")
	}

	stopped := make(chan struct{})
	go func() {
		runtime.Stop()
		close(stopped)
	}()
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not cancel the artifact cleanup context")
	}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not wait for the canceled bounded sweep")
	}
}

func TestJobRetentionLeaseSerializesDatabaseHandles(t *testing.T) {
	first := newJobContext(t)
	if err := first.db.AutoMigrate(&models.JobRuntimeFence{}); err != nil {
		t.Fatalf("migrate runtime fence: %v", err)
	}
	second := newSecondRetentionRuntimeContext(t, first)

	firstToken, err := newJobRetentionLeaseToken()
	if err != nil {
		t.Fatalf("create first lease token: %v", err)
	}
	secondToken, err := newJobRetentionLeaseToken()
	if err != nil {
		t.Fatalf("create second lease token: %v", err)
	}
	lease := time.Minute
	if acquired, err := acquireJobRetentionLease(context.Background(), first.db, firstToken, lease); err != nil || !acquired {
		t.Fatalf("first handle acquire: acquired=%v err=%v", acquired, err)
	}
	if acquired, err := acquireJobRetentionLease(context.Background(), second.db, secondToken, lease); err != nil || acquired {
		t.Fatalf("second handle entered an active lease: acquired=%v err=%v", acquired, err)
	}
	if err := releaseJobRetentionLease(first.db, firstToken); err != nil {
		t.Fatalf("release first lease: %v", err)
	}
	if acquired, err := acquireJobRetentionLease(context.Background(), second.db, secondToken, lease); err != nil || !acquired {
		t.Fatalf("second handle acquire after release: acquired=%v err=%v", acquired, err)
	}
	if err := releaseJobRetentionLease(second.db, secondToken); err != nil {
		t.Fatalf("release second lease: %v", err)
	}
}

func TestJobRetentionLeaseSurvivesSQLiteArtifactCleanupContention(t *testing.T) {
	first := newJobContext(t)
	setSQLiteBusyTimeoutForAllConnections(t, first, 50*time.Millisecond)
	if err := first.db.AutoMigrate(&models.JobRuntimeFence{}); err != nil {
		t.Fatalf("migrate runtime fence: %v", err)
	}
	service := first.JobService()
	adapter := newRuntimeTestAdapter()
	adapter.def.Kind = retentionRuntimeTestKind
	if err := service.RegisterAdapter(adapter); err != nil {
		t.Fatalf("register retention test Kind: %v", err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	var cleanupOnce sync.Once
	adapter.cleanup = func(_ context.Context, request jobs.ArtifactCleanupRequest) (jobs.ArtifactCleanupResult, error) {
		cleanupOnce.Do(func() {
			close(entered)
			<-release
		})
		removed := make([]string, 0, len(request.Artifacts))
		for _, artifact := range request.Artifacts {
			removed = append(removed, artifact.Key)
		}
		return jobs.ArtifactCleanupResult{Removed: removed}, nil
	}
	jobID := createExpiredArtifactJob(t, first)
	second := newSecondRetentionRuntimeContext(t, first)
	setSQLiteBusyTimeoutForAllConnections(t, second, 50*time.Millisecond)

	firstRuntime := NewJobRetentionRuntime(first, service, JobRetentionRuntimeConfig{
		Interval: time.Hour, LeaseDuration: 2 * time.Second,
		LeaseRefreshInterval: 100 * time.Millisecond, BatchSize: 1,
	})
	secondRuntime := NewJobRetentionRuntime(second, service, JobRetentionRuntimeConfig{
		Interval: 150 * time.Millisecond, ContinuationInterval: 25 * time.Millisecond,
		LeaseDuration: 2 * time.Second, LeaseRefreshInterval: 100 * time.Millisecond,
		BatchSize: 1,
	})
	firstRuntime.Start()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		firstRuntime.Stop()
		t.Fatal("startup sweep did not reach the expired artifact cleanup")
	}
	secondRuntime.Start()
	time.Sleep(350 * time.Millisecond) // Cross several refresh attempts that hit SQLite's short busy timeout.
	adapter.mu.Lock()
	cleanupCalls := len(adapter.cleanups)
	adapter.mu.Unlock()
	if cleanupCalls != 1 {
		close(release)
		firstRuntime.Stop()
		secondRuntime.Stop()
		t.Fatalf("artifact cleanup ran %d times while the first transaction was held, want 1", cleanupCalls)
	}
	close(release)

	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var output models.JobOutput
		err := first.db.Where("job_id = ? AND key = ?", jobID, "old-artifact").First(&output).Error
		if err == nil && output.Availability == string(jobs.OutputRemoved) {
			break
		}
		select {
		case <-deadline.C:
			firstRuntime.Stop()
			secondRuntime.Stop()
			t.Fatalf("expired artifact was not durably accounted for: err=%v availability=%q", err, output.Availability)
		case <-ticker.C:
		}
	}
	firstRuntime.Stop()
	secondRuntime.Stop()
	adapter.mu.Lock()
	cleanupCalls = len(adapter.cleanups)
	adapter.mu.Unlock()
	if cleanupCalls != 1 {
		t.Fatalf("artifact cleanup ran %d times, want one call across both runtimes", cleanupCalls)
	}
}

func makeExpiredRetentionRuntimeJob(t *testing.T, ctx *MahresourcesContext, title string, finished time.Time) string {
	t.Helper()
	snap := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: retentionRuntimeTestKind, KindVersion: 1, State: jobs.StateQueued,
		Origin: "test", Title: title, Replay: jobs.ReplayInput{NonReplayable: true},
	})
	finishedJob := finishJobFor(t, ctx, snap, jobs.StateSucceeded)
	expires := finished.Add(-time.Minute)
	if err := ctx.db.Model(&models.Job{}).Where("id = ?", finishedJob.ID).
		Updates(map[string]any{"finished_at": finished, "expires_at": expires}).Error; err != nil {
		t.Fatalf("age Job %s: %v", finishedJob.ID, err)
	}
	return finishedJob.ID
}

func createExpiredArtifactJob(t *testing.T, ctx *MahresourcesContext) string {
	t.Helper()
	service := ctx.JobService()
	if _, ok := service.AdapterFor(retentionRuntimeTestKind, 1); !ok {
		adapter := newRuntimeTestAdapter()
		adapter.def.Kind = retentionRuntimeTestKind
		if err := service.RegisterAdapter(adapter); err != nil {
			t.Fatalf("register retention test Kind: %v", err)
		}
	}
	snap := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: retentionRuntimeTestKind, KindVersion: 1, State: jobs.StateQueued,
		Origin: "test", Replay: jobs.ReplayInput{NonReplayable: true},
	})
	execution, ok, err := service.Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: retentionRuntimeTestKind, KindVersion: 1, Claimant: "retention-test",
	})
	if err != nil || !ok {
		t.Fatalf("claim artifact Job: ok=%v err=%v", ok, err)
	}
	expires := time.Now().UTC().Add(-time.Hour)
	if _, err := service.PublishOutput(ctx.jobDeps(), jobs.ExecutionRef{JobID: snap.ID, ExecutionToken: execution.ExecutionToken}, jobs.OutputInput{
		Key: "old-artifact", Type: string(jobs.OutputTypeArtifact), Label: "old artifact",
		Reference: json.RawMessage(`{"path":"old-artifact"}`), ExpiresAt: &expires,
	}); err != nil {
		t.Fatalf("publish expired artifact: %v", err)
	}
	if _, err := service.Finish(ctx.jobDeps(), jobs.FinishRequest{
		ExecutionRef:    jobs.ExecutionRef{JobID: snap.ID, ExecutionToken: execution.ExecutionToken},
		ExpectedVersion: execution.Version, Outcome: jobs.StateSucceeded,
	}); err != nil {
		t.Fatalf("finish artifact Job: %v", err)
	}
	return snap.ID
}

func newSecondRetentionRuntimeContext(t *testing.T, first *MahresourcesContext) *MahresourcesContext {
	t.Helper()
	dialector, ok := first.db.Dialector.(*sqlite.Dialector)
	if !ok {
		t.Fatalf("test database dialector is %T, want SQLite", first.db.Dialector)
	}
	db, _, err := models.CreateDatabaseConnection(constants.DbTypeSqlite, dialector.DSN, "", 0)
	if err != nil {
		t.Fatalf("open second database handle: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get second SQL database: %v", err)
	}
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })
	ctx := NewMahresourcesContext(first.GetDefaultFs(), db, sqlx.NewDb(sqlDB, "sqlite3"), first.Config)
	ctx.SetJobService(first.JobService())
	return ctx
}

func setSQLiteBusyTimeoutForAllConnections(t *testing.T, ctx *MahresourcesContext, timeout time.Duration) {
	t.Helper()
	db, err := ctx.db.DB()
	if err != nil {
		t.Fatalf("get SQLite connection pool: %v", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	connections := make([]*sql.Conn, 0, 4)
	for i := 0; i < cap(connections); i++ {
		connection, err := db.Conn(context.Background())
		if err != nil {
			for _, open := range connections {
				_ = open.Close()
			}
			t.Fatalf("open SQLite pool connection: %v", err)
		}
		connections = append(connections, connection)
		if _, err := connection.ExecContext(context.Background(), fmt.Sprintf("PRAGMA busy_timeout = %d", timeout.Milliseconds())); err != nil {
			for _, open := range connections {
				_ = open.Close()
			}
			t.Fatalf("set SQLite busy timeout: %v", err)
		}
	}
	for _, connection := range connections {
		if err := connection.Close(); err != nil {
			t.Fatalf("return SQLite pool connection: %v", err)
		}
	}
}

func retentionJobExists(t *testing.T, ctx *MahresourcesContext, jobID string) bool {
	t.Helper()
	var count int64
	if err := ctx.db.Model(&models.Job{}).Where("id = ?", jobID).Count(&count).Error; err != nil {
		t.Fatalf("count Job %s: %v", jobID, err)
	}
	return count != 0
}

func waitForRetentionJobState(t *testing.T, ctx *MahresourcesContext, jobID string, wantPresent bool) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if retentionJobExists(t, ctx, jobID) == wantPresent {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("Job %s did not reach present=%v", jobID, wantPresent)
		case <-ticker.C:
		}
	}
}
