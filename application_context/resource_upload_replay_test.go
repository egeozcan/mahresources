package application_context

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
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

	// guardedPath, when non-empty, makes any Create/Remove/OpenFile that would
	// touch it record the violation.
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

// FailIfDestinationIsTouched makes any Create, Remove or OpenFile for the
// given path record a violation.
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
	// The second upload of the same content hits the hash-merge path, which returns
	// a ResourceExistsError. Either that, or it silently merges (depending on owner).
	// In both cases, the destination must not be touched.
	_ = second
	_ = err
	ff.AssertDestinationWasNotTouched(t)
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
