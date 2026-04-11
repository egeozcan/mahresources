package download_queue

import (
	"testing"
	"time"

	"github.com/spf13/afero"
)

func TestSweepOrphanedExports_RemovesFilesOlderThanRetention(t *testing.T) {
	fs := afero.NewMemMapFs()
	_ = fs.MkdirAll("/data/_exports", 0755)

	freshPath := "/data/_exports/fresh.tar"
	expiredPath := "/data/_exports/expired.tar"

	if err := afero.WriteFile(fs, freshPath, []byte("fresh"), 0644); err != nil {
		t.Fatalf("write fresh: %v", err)
	}
	if err := afero.WriteFile(fs, expiredPath, []byte("expired"), 0644); err != nil {
		t.Fatalf("write expired: %v", err)
	}

	if err := fs.Chtimes(expiredPath, time.Now().Add(-48*time.Hour), time.Now().Add(-48*time.Hour)); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	removed, err := SweepOrphanedExports(fs, "/data/_exports", 24*time.Hour)
	if err != nil {
		t.Fatalf("SweepOrphanedExports: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}

	if exists, _ := afero.Exists(fs, freshPath); !exists {
		t.Fatalf("fresh file was removed")
	}
	if exists, _ := afero.Exists(fs, expiredPath); exists {
		t.Fatalf("expired file still present")
	}
}

func TestSweepOrphanedExports_NoExportsDirIsFine(t *testing.T) {
	fs := afero.NewMemMapFs()
	removed, err := SweepOrphanedExports(fs, "/missing", 24*time.Hour)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if removed != 0 {
		t.Fatalf("removed = %d, want 0", removed)
	}
}

func TestSweepOrphanedExports_UnderBasePathFs(t *testing.T) {
	// Production FS is always a BasePathFs wrapper. This test ensures the
	// sweep walks the wrapped path correctly even when the caller passes a
	// root-relative dir ("_exports") and the wrapper re-prepends its base.
	base := afero.NewMemMapFs()
	bpfs := afero.NewBasePathFs(base, "/data")

	// Write a fresh file and an expired file through the wrapper.
	if err := bpfs.MkdirAll("_exports", 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := afero.WriteFile(bpfs, "_exports/fresh.tar", []byte("fresh"), 0644); err != nil {
		t.Fatalf("write fresh: %v", err)
	}
	if err := afero.WriteFile(bpfs, "_exports/expired.tar", []byte("expired"), 0644); err != nil {
		t.Fatalf("write expired: %v", err)
	}
	// Backdate the expired file through the underlying FS (BasePathFs
	// forwards Chtimes but the rewrite happens under the hood).
	if err := bpfs.Chtimes("_exports/expired.tar", time.Now().Add(-48*time.Hour), time.Now().Add(-48*time.Hour)); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	removed, err := SweepOrphanedExports(bpfs, "_exports", 24*time.Hour)
	if err != nil {
		t.Fatalf("SweepOrphanedExports: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}

	if exists, _ := afero.Exists(bpfs, "_exports/fresh.tar"); !exists {
		t.Fatalf("fresh file was removed through BasePathFs")
	}
	if exists, _ := afero.Exists(bpfs, "_exports/expired.tar"); exists {
		t.Fatalf("expired file still present under BasePathFs")
	}
}
