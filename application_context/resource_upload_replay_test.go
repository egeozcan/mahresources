package application_context

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
)

// errInjectedDestinationCopy is the sentinel returned when the fault-injecting
// filesystem truncates a resource destination write.
var errInjectedDestinationCopy = errors.New("injected destination copy failure")

// ---------------------------------------------------------------------------
// Fault-injecting filesystem
// ---------------------------------------------------------------------------

// faultFs wraps an afero.Fs and lets the test inject a write failure at a
// precise byte offset for the next resource destination Create, and also assert
// that an existing destination is never touched.
type faultFs struct {
	afero.Fs
	mu sync.Mutex

	// failAfter > 0 means the next Create for a resource path will return a
	// file that fails after writing this many bytes.
	failAfter int64

	// guardedPath, when non-empty, makes any Create/Remove/Open/OpenFile that
	// would touch it record the violation.
	guardedPath string
	violated    bool
}

func newFaultFs(base afero.Fs) *faultFs {
	return &faultFs{Fs: base}
}

// FailNextResourceCopyAfter arranges for the next Create in the /resources/
// tree to return a file whose Write fails after n bytes.
func (f *faultFs) FailNextResourceCopyAfter(n int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failAfter = n
}

// FailIfDestinationIsTouched makes any Create, Remove, Open or OpenFile for
// the given path record a violation.
func (f *faultFs) FailIfDestinationIsTouched(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.guardedPath = path
	f.violated = false
}

// AssertDestinationWasNotTouched fails the test if a guarded path was touched.
func (f *faultFs) AssertDestinationWasNotTouched(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.violated {
		t.Fatal("a guarded destination was touched when it should not have been")
	}
}

func (f *faultFs) isGuarded(name string) bool {
	return f.guardedPath != "" && name == f.guardedPath
}

func (f *faultFs) Create(name string) (afero.File, error) {
	f.mu.Lock()
	if f.isGuarded(name) {
		f.violated = true
	}
	limit := f.failAfter
	if limit > 0 && isResourcePath(name) {
		f.failAfter = 0
		f.mu.Unlock()
		real, err := f.Fs.Create(name)
		if err != nil {
			return nil, err
		}
		return &truncatingFile{File: real, remaining: limit}, nil
	}
	f.mu.Unlock()
	return f.Fs.Create(name)
}

func (f *faultFs) Remove(name string) error {
	f.mu.Lock()
	if f.isGuarded(name) {
		f.violated = true
	}
	f.mu.Unlock()
	return f.Fs.Remove(name)
}

func (f *faultFs) Open(name string) (afero.File, error) {
	f.mu.Lock()
	if f.isGuarded(name) {
		f.violated = true
	}
	f.mu.Unlock()
	return f.Fs.Open(name)
}

func (f *faultFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	f.mu.Lock()
	if f.isGuarded(name) && (flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC)) != 0 {
		f.violated = true
	}
	f.mu.Unlock()
	return f.Fs.OpenFile(name, flag, perm)
}

func isResourcePath(name string) bool {
	return strings.HasPrefix(name, "/resources/") || strings.HasPrefix(name, "resources/")
}

// truncatingFile writes up to `remaining` bytes, then returns the sentinel.
type truncatingFile struct {
	afero.File
	remaining int64
}

func (f *truncatingFile) Write(p []byte) (int, error) {
	if f.remaining <= 0 {
		return 0, errInjectedDestinationCopy
	}
	if int64(len(p)) > f.remaining {
		n, err := f.File.Write(p[:f.remaining])
		f.remaining = 0
		if err != nil {
			return n, err
		}
		return n, errInjectedDestinationCopy
	}
	n, err := f.File.Write(p)
	f.remaining -= int64(n)
	return n, err
}

// ---------------------------------------------------------------------------
// Test context builder
// ---------------------------------------------------------------------------

// newReplayUploadContext builds a WAL-backed context whose primary filesystem
// is a faultFs, suitable for the replay and precedence tests.
func newReplayUploadContext(t *testing.T) (*MahresourcesContext, *faultFs) {
	t.Helper()
	ctx := newWALTestContext(t, 0)
	ff := newFaultFs(ctx.fs)
	ctx.fs = ff
	return ctx, ff
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestAddResource_ReplayReplacesATruncatedUncommittedDestination simulates an
// interrupted file copy: the first upload writes a truncated prefix to the
// resource destination and fails. The second upload of the same content must
// detect the size mismatch, replace the truncated file, and succeed.
func TestAddResource_ReplayReplacesATruncatedUncommittedDestination(t *testing.T) {
	payload := bytes.Repeat([]byte("complete-payload-"), 4096)
	ctx, ff := newReplayUploadContext(t)

	ff.FailNextResourceCopyAfter(1024)

	_, err := ctx.AddResource(newBytesFile(payload), "replay.bin", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "replay"},
	})
	require.Error(t, err, "first upload should fail due to injected copy failure")
	// The sentinel may be wrapped, so check Contains rather than ErrorIs.
	require.ErrorIs(t, err, errInjectedDestinationCopy)

	// The second upload with the same content should succeed.
	got, err := ctx.AddResource(newBytesFile(payload), "replay.bin", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "replay"},
	})
	require.NoError(t, err)
	require.NotNil(t, got)

	stored, err := afero.ReadFile(ctx.fs, got.Location)
	require.NoError(t, err)
	require.Equal(t, payload, stored, "the committed resource's backing file must be the complete payload, not the truncated prefix")
}

// TestAddResource_CommittedHashLookupPrecedesDestinationRepair verifies that
// when a committed resource already exists for the same hash, the duplicate
// upload is handled by the hash lookup (merge path) without ever touching the
// destination file on disk.
func TestAddResource_CommittedHashLookupPrecedesDestinationRepair(t *testing.T) {
	payload := []byte("already committed")
	ctx, ff := newReplayUploadContext(t)

	first, err := ctx.AddResource(newBytesFile(payload), "first.bin", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "first"},
	})
	require.NoError(t, err)

	ff.FailIfDestinationIsTouched(first.Location)
	second, err := ctx.AddResource(newBytesFile(payload), "second.bin", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "second"},
	})

	require.Nil(t, second)
	var existsErr *ResourceExistsError
	require.ErrorAs(t, err, &existsErr)
	require.Equal(t, first.ID, existsErr.ResourceID)
	ff.AssertDestinationWasNotTouched(t)
}

func TestAddResourceForJobRecoversCommitBeforeQueueAcknowledgement(t *testing.T) {
	payload := []byte("committed before the queue recorded its resource id")
	ctx := newJobHarnessContext(t, false)
	accepted := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: runtimeTestKind, KindVersion: 1, State: jobs.StateQueued, Origin: "api", Title: "receipt test",
		Replay: jobs.ReplayInput{NonReplayable: true},
	})
	jobID := accepted.ID
	creator := func() *query_models.ResourceCreator {
		return &query_models.ResourceCreator{ResourceQueryBase: query_models.ResourceQueryBase{Name: "replayed download"}}
	}

	// The returned ID stands for the process-local value lost when the process
	// exits after the resource transaction commits but before the queue stamps it.
	first, err := ctx.AddResourceForJob(jobID, nil, newBytesFile(payload), "download.bin", creator())
	require.NoError(t, err)
	require.NotNil(t, first)

	replayed, err := ctx.AddResourceForJob(jobID, nil, newBytesFile(payload), "download.bin", creator())
	require.NoError(t, err)
	require.Equal(t, first.ID, replayed.ID, "reconciliation must recover the committed resource receipt")

	var resources int64
	require.NoError(t, ctx.db.Model(&models.Resource{}).Where("hash = ?", first.Hash).Count(&resources).Error)
	require.EqualValues(t, 1, resources)
	var receipts int64
	require.NoError(t, ctx.db.Model(&models.JobResourceReceipt{}).Where("job_id = ?", jobID).Count(&receipts).Error)
	require.EqualValues(t, 1, receipts)

	// The receipt is specific to this canonical Job. A separate upload of the
	// same bytes keeps the ordinary duplicate refusal.
	_, err = ctx.AddResource(newBytesFile(payload), "independent.bin", creator())
	var exists *ResourceExistsError
	require.ErrorAs(t, err, &exists)
	require.Equal(t, first.ID, exists.ResourceID)
	registerClaimableKind(t, ctx, runtimeTestKind, 1)
	execution, claimed, err := ctx.JobService().Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: runtimeTestKind, KindVersion: 1, JobID: jobID, Claimant: "receipt-retention-test",
	})
	require.NoError(t, err)
	require.True(t, claimed)
	_, err = ctx.JobService().Finish(ctx.jobDeps(), jobs.FinishRequest{
		ExecutionRef:    jobs.ExecutionRef{JobID: jobID, ExecutionToken: execution.ExecutionToken},
		ExpectedVersion: execution.Version, Outcome: jobs.StateSucceeded,
	})
	require.NoError(t, err)
	require.NoError(t, ctx.db.Model(&models.Job{}).Where("id = ?", jobID).
		Update("expires_at", time.Now().UTC().Add(-time.Hour)).Error)
	// This harness opens SQLite directly, unlike the production connection
	// helper. Pin one connection and enable production's FK pragma so the
	// Job→receipt cascade is exercised by retention.
	sqlDB, err := ctx.db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, ctx.db.Exec("PRAGMA foreign_keys = ON").Error)
	sweep, err := ctx.JobService().Sweep(ctx.jobDeps(), jobs.RetentionPolicy{History: time.Second}, jobs.SweepCursor{}, 32)
	require.NoError(t, err)
	require.Equal(t, 1, sweep.Pruned, "the expired Job should be pruned with its receipt")
	require.NoError(t, ctx.db.Model(&models.JobResourceReceipt{}).Where("job_id = ?", jobID).Count(&receipts).Error)
	require.Zero(t, receipts, "the receipt must follow canonical Job retention")
}

func TestAddResourceForJobRecordsReceiptWhenAttachingSecondOwner(t *testing.T) {
	payload := []byte("shared bytes attached to a second owner")
	ctx := newJobHarnessContext(t, false)
	firstOwner := &models.Group{Name: "receipt-first-owner"}
	secondOwner := &models.Group{Name: "receipt-second-owner"}
	require.NoError(t, ctx.db.Create(firstOwner).Error)
	require.NoError(t, ctx.db.Create(secondOwner).Error)
	first, err := ctx.AddResource(newBytesFile(payload), "first.bin", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "first", OwnerId: firstOwner.ID},
	})
	require.NoError(t, err)
	accepted := acceptJobFor(t, ctx, jobs.Acceptance{
		Kind: runtimeTestKind, KindVersion: 1, State: jobs.StateQueued, Origin: "api", Title: "second owner receipt",
		Replay: jobs.ReplayInput{NonReplayable: true},
	})
	added, err := ctx.AddResourceForJob(accepted.ID, nil, newBytesFile(payload), "second.bin", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "second", OwnerId: secondOwner.ID},
	})
	require.NoError(t, err)
	require.Equal(t, first.ID, added.ID, "same bytes should attach the owner to the existing Resource")
	var receipt models.JobResourceReceipt
	require.NoError(t, ctx.db.First(&receipt, "job_id = ?", accepted.ID).Error)
	require.Equal(t, first.ID, receipt.ResourceID, "owner attachment and receipt must commit together")
	var owners int64
	require.NoError(t, ctx.db.Table("groups_related_resources").Where("resource_id = ? AND group_id = ?", first.ID, secondOwner.ID).Count(&owners).Error)
	require.EqualValues(t, 1, owners)
	sqlDB, err := ctx.db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, ctx.db.Exec("PRAGMA foreign_keys = ON").Error)
	require.NoError(t, ctx.db.Delete(&models.Resource{}, first.ID).Error, "Resource deletion should cascade to its receipt")
	var receipts int64
	require.NoError(t, ctx.db.Model(&models.JobResourceReceipt{}).Where("job_id = ?", accepted.ID).Count(&receipts).Error)
	require.Zero(t, receipts)
}

// TestAddResource_ConcurrentSameContentNeverUnlinksTheWinner verifies that
// two concurrent uploads of the same content produce one logical resource
// and a complete backing file.
func TestAddResource_ConcurrentSameContentNeverUnlinksTheWinner(t *testing.T) {
	payload := bytes.Repeat([]byte("same-content-"), 4096)
	ctx, _ := newReplayUploadContext(t)

	ownerGroup := &models.Group{Name: "concurrent-replay-owner"}
	require.NoError(t, ctx.db.Create(ownerGroup).Error)

	start := make(chan struct{})
	const n = 2
	results := make([]*models.Resource, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			<-start
			results[idx], errs[idx] = ctx.AddResource(
				newBytesFile(payload),
				"same.bin",
				&query_models.ResourceCreator{
					ResourceQueryBase: query_models.ResourceQueryBase{
						Name:    "same",
						OwnerId: ownerGroup.ID,
					},
				},
			)
		}(i)
	}
	close(start)
	wg.Wait()

	// Exactly one should succeed; the other is a duplicate.
	var created int
	var createdRes *models.Resource
	for i := 0; i < n; i++ {
		if errs[i] == nil {
			created++
			createdRes = results[i]
		}
	}
	require.Equal(t, 1, created, "exactly one upload of identical content should succeed")
	require.NotNil(t, createdRes)

	stored, err := afero.ReadFile(ctx.fs, createdRes.Location)
	require.NoError(t, err)
	require.Equal(t, payload, stored, "the backing file must be intact after concurrent uploads")
}

// TestAddResource_StatErrorFailsClosed verifies that a non-ErrNotExist Stat
// error on the destination path fails the upload rather than silently
// creating a new file.
func TestAddResource_StatErrorFailsClosed(t *testing.T) {
	ctx := newWALTestContext(t, 0)
	// Use an errFs that returns permission-denied on Stat for resource paths.
	efs := &statErrFs{Fs: ctx.fs}
	ctx.fs = efs

	payload := []byte("stat-error-test")
	_, err := ctx.AddResource(newBytesFile(payload), "staterr.bin", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "staterr"},
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, fs.ErrPermission) || strings.Contains(err.Error(), "stat resource destination"),
		"expected a stat error or wrapped stat error, got: %v", err)
}

// statErrFs wraps an afero.Fs and returns ErrPermission on Stat for resource paths,
// but allows MkdirAll and Create to work (so the file write paths remain accessible).
type statErrFs struct {
	afero.Fs
	once atomic.Bool
}

func (f *statErrFs) Stat(name string) (os.FileInfo, error) {
	if isResourcePath(name) && !f.once.Swap(true) {
		// First Stat on a resource path fails; subsequent ones pass through.
		// This is just enough to test the fail-closed branch.
		return nil, fs.ErrPermission
	}
	return f.Fs.Stat(name)
}
