package groupio

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"mahresources/archive"
	"mahresources/models"
)

// blobHashArchive builds an archive holding one group and one resource whose
// payload hash, blob entry name and (optionally) single version hash are
// whatever the caller says, the way a crafted upload would carry them.
func blobHashArchive(t *testing.T, resHash, hashType, blobName, versionHash string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := archive.NewWriter(&buf, false)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	now := time.Now().UTC()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(w.WriteManifest(&archive.Manifest{
		SchemaVersion: archive.SchemaVersion,
		CreatedAt:     now,
		CreatedBy:     "test",
		Roots:         []string{"g0001"},
		Counts:        archive.Counts{Groups: 1, Resources: 1, Blobs: 1},
		ExportOptions: archive.ExportOptions{Fidelity: archive.ExportFidelity{ResourceBlobs: true}},
		Entries: archive.Entries{
			Groups:    []archive.GroupEntry{{ExportID: "g0001", Name: "Root", SourceID: 1, Path: "groups/g0001.json"}},
			Resources: []archive.ResourceEntry{{ExportID: "r0001", Name: "f.txt", SourceID: 1, Owner: "g0001", Hash: resHash, Path: "resources/r0001.json"}},
		},
	}))
	must(w.WriteGroup(&archive.GroupPayload{ExportID: "g0001", SourceID: 1, Name: "Root", CreatedAt: now, UpdatedAt: now}))
	rp := &archive.ResourcePayload{
		ExportID: "r0001", SourceID: 1, Name: "f.txt", Hash: resHash, HashType: hashType, BlobRef: resHash,
		OwnerRef: "g0001", CreatedAt: now, UpdatedAt: now,
	}
	if versionHash != "" {
		rp.Versions = []archive.ResourceVersionPayload{{
			VersionExportID: "v0001", VersionNumber: 1, Hash: versionHash, HashType: hashType,
			BlobRef: versionHash, CreatedAt: now,
		}}
	}
	must(w.WriteResource(rp))
	must(w.WriteBlob(blobName, bytes.NewReader(body), int64(len(body))))
	if versionHash != "" && versionHash != blobName {
		must(w.WriteBlob(versionHash, bytes.NewReader(body), int64(len(body))))
	}
	must(w.Close())
	return buf.Bytes()
}

func sha1Hex(b []byte) string {
	s := sha1.Sum(b)
	return hex.EncodeToString(s[:])
}

// snapshotFs maps every regular file in fs to its content.
func snapshotFs(t *testing.T, fs afero.Fs) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := afero.Walk(fs, "", func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		b, err := afero.ReadFile(fs, p)
		if err != nil {
			return err
		}
		out[strings.TrimPrefix(p, "/")] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("walk fs: %v", err)
	}
	return out
}

func assertNoLocationEscapes(t *testing.T, ctx *opCtx) {
	t.Helper()
	var resources []models.Resource
	ctx.db.Find(&resources)
	for _, r := range resources {
		if !strings.HasPrefix(r.Location, "resources/") {
			t.Errorf("resource %d has Location %q outside resources/", r.ID, r.Location)
		}
	}
	var versions []models.ResourceVersion
	ctx.db.Find(&versions)
	for _, v := range versions {
		if !strings.HasPrefix(v.Location, "resources/") {
			t.Errorf("version %d has Location %q outside resources/", v.ID, v.Location)
		}
	}
}

// Issue #94: a crafted archive's blob hash became a storage path, so
// "../_job_replay_key" bound a resource to the signing secret and
// "../_imports/x.plan.json" planted a file in a reserved directory.
func TestImport_RejectsBlobHashThatIsNotADigest(t *testing.T) {
	body := []byte("attacker bytes")
	valid := sha1Hex(body)
	cases := []struct {
		name, hash, hashType, versionHash string
	}{
		{name: "traversal to job replay key", hash: "../_job_replay_key", hashType: "SHA1"},
		{name: "traversal into imports", hash: "../_imports/x.plan.json", hashType: "SHA1"},
		{name: "uppercase hex", hash: strings.ToUpper(valid), hashType: "SHA1"},
		{name: "wrong length hex", hash: valid[:39], hashType: "SHA1"},
		{name: "sha256 length labelled SHA1", hash: valid + strings.Repeat("0", 24), hashType: "SHA1"},
		{name: "unknown hash type", hash: valid, hashType: "MD5"},
		{name: "traversal in version hash", hash: valid, hashType: "SHA1", versionHash: "../_imports/v.plan.json"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := createGUIDIsolatedContext(t, "blobhash"+string(rune('a'+i)))
			_ = ctx.fs.MkdirAll("_exports", 0755)
			_ = ctx.fs.MkdirAll("_imports", 0755)
			if err := afero.WriteFile(ctx.fs, "_job_replay_key", []byte("SECRET"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := afero.WriteFile(ctx.fs, "_exports/other.tar", []byte("OTHER"), 0600); err != nil {
				t.Fatal(err)
			}
			jobID := "blobhash-job"
			tarPath := "_imports/" + jobID + ".tar"
			evil := blobHashArchive(t, tc.hash, tc.hashType, tc.hash, tc.versionHash, body)

			// The parse pass refuses the archive outright.
			if err := afero.WriteFile(ctx.fs, tarPath, evil, 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := ctx.ParseImport(context.Background(), jobID, tarPath); err == nil {
				t.Error("ParseImport accepted an archive whose blob hash is not a digest")
			}

			// The apply pass must refuse it on its own too: parse a benign
			// archive for the plan, then swap the staged tar underneath it.
			benign := blobHashArchive(t, valid, "SHA1", valid, "", body)
			if err := afero.WriteFile(ctx.fs, tarPath, benign, 0644); err != nil {
				t.Fatal(err)
			}
			plan, err := ctx.ParseImport(context.Background(), jobID, tarPath)
			if err != nil {
				t.Fatalf("parse benign: %v", err)
			}
			if err := afero.WriteFile(ctx.fs, tarPath, evil, 0644); err != nil {
				t.Fatal(err)
			}
			before := snapshotFs(t, ctx.fs)

			if _, err := ctx.ApplyImport(context.Background(), jobID, importPlanPath(jobID), buildDefaultDecisions(plan), noopSink{}); err == nil {
				t.Error("ApplyImport accepted an archive whose blob hash is not a digest")
			}

			after := snapshotFs(t, ctx.fs)
			for p, c := range after {
				if old, ok := before[p]; !ok || old != c {
					t.Errorf("ApplyImport wrote %q", p)
				}
			}
			assertNoLocationEscapes(t, ctx)
		})
	}
}

// Positive control: a well-formed archive still imports into resources/.
func TestImport_ValidDigestBlobStillImports(t *testing.T) {
	ctx := createGUIDIsolatedContext(t, "blobhashvalid")
	body := []byte("legitimate bytes")
	hash := sha1Hex(body)
	jobID := "blobhash-valid"
	tarPath := "_imports/" + jobID + ".tar"
	_ = ctx.fs.MkdirAll("_imports", 0755)
	if err := afero.WriteFile(ctx.fs, tarPath, blobHashArchive(t, hash, "SHA1", hash, hash, body), 0644); err != nil {
		t.Fatal(err)
	}
	plan, err := ctx.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	result, err := ctx.ApplyImport(context.Background(), jobID, importPlanPath(jobID), buildDefaultDecisions(plan), noopSink{})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.CreatedResources != 1 {
		t.Fatalf("CreatedResources = %d, want 1", result.CreatedResources)
	}
	var r models.Resource
	if err := ctx.db.First(&r).Error; err != nil {
		t.Fatal(err)
	}
	if r.Location != "resources/"+hash {
		t.Errorf("Location = %q, want resources/%s", r.Location, hash)
	}
	got, err := afero.ReadFile(ctx.fs, r.Location)
	if err != nil || !bytes.Equal(got, body) {
		t.Errorf("blob at %q = %q, %v; want %q", r.Location, got, err, body)
	}
	assertNoLocationEscapes(t, ctx)
}

// The path builder refuses a non-digest on its own, behind the reader's check.
func TestImportBlobLocation_RefusesNonDigest(t *testing.T) {
	for _, h := range []string{"../_job_replay_key", "../../etc/passwd", "ABCDEF0123456789ABCDEF0123456789ABCDEF01"} {
		if loc, err := importBlobLocation(h, "text/plain"); err == nil {
			t.Errorf("importBlobLocation(%q) = %q, want an error", h, loc)
		}
	}
	h := sha1Hex([]byte("x"))
	if loc, err := importBlobLocation(h, ""); err != nil || loc != "resources/"+h {
		t.Errorf("importBlobLocation(%q) = %q, %v", h, loc, err)
	}
}
