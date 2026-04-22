package archive_test

import (
	"archive/tar"
	"bytes"
	"errors"
	"strings"
	"testing"

	"mahresources/archive"
)

// buildManifestTar packs a single manifest.json entry whose content is the
// supplied JSON bytes. Returns the tar stream so tests can feed it to
// archive.NewReader.
func buildManifestTar(t *testing.T, manifestJSON []byte) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o600, Size: int64(len(manifestJSON))}); err != nil {
		t.Fatalf("tw.WriteHeader: %v", err)
	}
	if _, err := tw.Write(manifestJSON); err != nil {
		t.Fatalf("tw.Write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tw.Close: %v", err)
	}
	return &buf
}

// BH-017: omitting schema_version entirely should produce a "missing required
// field" error, NOT the misleading "unsupported schema_version 0".
func TestReadManifest_MissingSchemaVersion(t *testing.T) {
	json := []byte(`{"created_at":"2026-04-22T00:00:00Z","created_by":"test"}`)
	buf := buildManifestTar(t, json)

	r, err := archive.NewReader(buf)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	_, err = r.ReadManifest()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var missing *archive.ErrMissingSchemaVersion
	if !errors.As(err, &missing) {
		t.Fatalf("expected ErrMissingSchemaVersion, got %T: %v", err, err)
	}
	msg := err.Error()
	for _, substr := range []string{"missing", "schema_version"} {
		if !strings.Contains(msg, substr) {
			t.Errorf("error message %q missing substring %q", msg, substr)
		}
	}
}

// TestReadManifest_UnsupportedVersion keeps coverage on the existing branch.
func TestReadManifest_UnsupportedVersion(t *testing.T) {
	json := []byte(`{"schema_version":9999,"created_at":"2026-04-22T00:00:00Z"}`)
	buf := buildManifestTar(t, json)

	r, err := archive.NewReader(buf)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	_, err = r.ReadManifest()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var unsup *archive.ErrUnsupportedSchemaVersion
	if !errors.As(err, &unsup) {
		t.Fatalf("expected ErrUnsupportedSchemaVersion, got %T: %v", err, err)
	}
}
