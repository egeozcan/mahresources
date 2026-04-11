package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

func TestWriter_WritesManifestAsFirstEntry(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewWriter(&buf, false)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	m := Manifest{
		SchemaVersion: SchemaVersion,
		CreatedAt:     time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC),
		CreatedBy:     "mahresources",
	}
	if err := w.WriteManifest(&m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	tr := tarReaderFromBytes(t, buf.Bytes())
	hdr, _, err := nextEntry(tr)
	if err != nil {
		t.Fatalf("read first entry: %v", err)
	}
	if hdr.Name != "manifest.json" {
		t.Fatalf("first entry = %q, want manifest.json", hdr.Name)
	}
}

func tarReaderFromBytes(t *testing.T, b []byte) *tar.Reader {
	t.Helper()
	return tar.NewReader(bytes.NewReader(b))
}

func nextEntry(tr *tar.Reader) (*tar.Header, []byte, error) {
	hdr, err := tr.Next()
	if err != nil {
		return nil, nil, err
	}
	body, err := io.ReadAll(tr)
	if err != nil {
		return hdr, nil, err
	}
	return hdr, body, nil
}

func TestWriter_WritesGroupAndDecodesManifest(t *testing.T) {
	var buf bytes.Buffer
	w, _ := NewWriter(&buf, false)

	m := Manifest{
		SchemaVersion: SchemaVersion,
		CreatedAt:     time.Now().UTC(),
		CreatedBy:     "mahresources",
		Roots:         []string{"g0001"},
		Counts:        Counts{Groups: 1},
		Entries: Entries{
			Groups: []GroupEntry{{ExportID: "g0001", Name: "Books", SourceID: 17, Path: "groups/g0001.json"}},
		},
	}
	if err := w.WriteManifest(&m); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	if err := w.WriteGroup(&GroupPayload{ExportID: "g0001", SourceID: 17, Name: "Books"}); err != nil {
		t.Fatalf("WriteGroup: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	tr := tarReaderFromBytes(t, buf.Bytes())
	mfHdr, mfBody, err := nextEntry(tr)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if mfHdr.Name != "manifest.json" {
		t.Fatalf("first entry = %q", mfHdr.Name)
	}
	var got Manifest
	if err := json.NewDecoder(bytes.NewReader(mfBody)).Decode(&got); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if got.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %d", got.SchemaVersion)
	}
	if len(got.Entries.Groups) != 1 || got.Entries.Groups[0].Name != "Books" {
		t.Fatalf("entries = %+v", got.Entries.Groups)
	}

	groupHdr, groupBody, err := nextEntry(tr)
	if err != nil {
		t.Fatalf("read group: %v", err)
	}
	if groupHdr.Name != "groups/g0001.json" {
		t.Fatalf("group entry = %q", groupHdr.Name)
	}
	if !strings.Contains(string(groupBody), `"name":"Books"`) {
		t.Fatalf("group body missing name: %s", groupBody)
	}
}

func TestWriter_BlobDeduplication(t *testing.T) {
	var buf bytes.Buffer
	w, _ := NewWriter(&buf, false)
	_ = w.WriteManifest(&Manifest{SchemaVersion: SchemaVersion})

	if err := w.WriteBlob("abc", strings.NewReader("hello"), 5); err != nil {
		t.Fatalf("first WriteBlob: %v", err)
	}
	// second call with same hash should be a no-op
	if err := w.WriteBlob("abc", strings.NewReader("hello"), 5); err != nil {
		t.Fatalf("second WriteBlob: %v", err)
	}
	_ = w.Close()

	tr := tarReaderFromBytes(t, buf.Bytes())
	count := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("iterate: %v", err)
		}
		if hdr.Name == "blobs/abc" {
			count++
		}
		_, _ = io.Copy(io.Discard, tr)
	}
	if count != 1 {
		t.Fatalf("blob written %d times, want 1", count)
	}
}

func TestWriter_BytesWrittenAdvancesWithEntries(t *testing.T) {
	var buf bytes.Buffer
	w, _ := NewWriter(&buf, false)
	start := w.BytesWritten()
	if start != 0 {
		t.Fatalf("initial BytesWritten = %d, want 0", start)
	}
	_ = w.WriteManifest(&Manifest{SchemaVersion: SchemaVersion, CreatedBy: "mahresources"})
	afterManifest := w.BytesWritten()
	if afterManifest <= start {
		t.Fatalf("BytesWritten did not advance after manifest: %d", afterManifest)
	}
	_ = w.WriteBlob("h1", strings.NewReader("PNGDATA"), 7)
	afterBlob := w.BytesWritten()
	if afterBlob <= afterManifest {
		t.Fatalf("BytesWritten did not advance after blob: %d", afterBlob)
	}
	_ = w.Close()
}

func TestWriter_GzipRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w, _ := NewWriter(&buf, true)
	_ = w.WriteManifest(&Manifest{SchemaVersion: SchemaVersion, CreatedBy: "mahresources"})
	_ = w.Close()

	gz, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("first entry: %v", err)
	}
	if hdr.Name != "manifest.json" {
		t.Fatalf("first entry = %q", hdr.Name)
	}
}
