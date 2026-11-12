package groupio

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/afero"
	"mahresources/archive"
	"mahresources/models"
)

// A resource row whose stored location points into private storage does not
// own that file, however the row came to exist. Export treats it as a missing
// blob: the bytes stay out of the archive and the manifest does not promise
// them.
func TestStreamExport_PrivateStorageLocationIsNeverArchived(t *testing.T) {
	ctx := createTestContext(t)
	root := mustCreateGroup(t, ctx, "Root", nil)
	secret := []byte("SECRET-REPLAY-KEY-BYTES")

	for _, location := range []string{"_job_replay_key", "/_exports/x.tar", "resources/../_imports/y.tar"} {
		t.Run(location, func(t *testing.T) {
			if err := afero.WriteFile(ctx.fs, "_job_replay_key", secret, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := afero.WriteFile(ctx.fs, "_exports/x.tar", secret, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := afero.WriteFile(ctx.fs, "_imports/y.tar", secret, 0o600); err != nil {
				t.Fatal(err)
			}
			r := models.Resource{
				Name: "planted", OwnerId: &root.ID, Hash: "0123456789abcdef0123456789abcdef01234567", HashType: "SHA1",
				FileSize: int64(len(secret)), Location: location, ResourceCategoryId: testDefaultResourceCategoryID,
			}
			if err := ctx.db.Create(&r).Error; err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { ctx.db.Unscoped().Delete(&models.Resource{}, r.ID) })

			var buf bytes.Buffer
			req := &ExportRequest{
				RootGroupIDs: []uint{root.ID},
				Scope:        archive.ExportScope{Subtree: true, OwnedResources: true},
				Fidelity:     archive.ExportFidelity{ResourceBlobs: true},
			}
			if err := ctx.StreamExport(context.Background(), req, &buf, nil); err != nil {
				t.Fatalf("StreamExport: %v", err)
			}
			if bytes.Contains(buf.Bytes(), secret) {
				t.Fatal("archive contains the private file's bytes")
			}
			rdr, err := archive.NewReader(&buf)
			if err != nil {
				t.Fatal(err)
			}
			defer rdr.Close()
			m, _ := rdr.ReadManifest()
			c := newExportCollector()
			if err := rdr.Walk(c); err != nil {
				t.Fatalf("Walk: %v", err)
			}
			if m.Counts.Blobs != 0 || len(c.blobs) != 0 {
				t.Errorf("blobs: manifest %d, archive %d; want 0", m.Counts.Blobs, len(c.blobs))
			}
			for _, rp := range c.resources {
				if !rp.BlobMissing || rp.BlobRef != "" {
					t.Errorf("resource payload BlobMissing=%v BlobRef=%q, want missing and empty", rp.BlobMissing, rp.BlobRef)
				}
			}
		})
	}
}
