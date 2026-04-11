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
