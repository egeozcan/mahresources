package archive

import (
	"archive/tar"
	"bytes"
	"strings"
	"testing"
)

// Finding 106 of docs/ui-bug-hunt-2026-07-29.md: uploading a file that is not a
// tar answered "read manifest: archive: read first entry: unexpected EOF" — an
// internal call chain that names three layers and no remedy.
//
// The message belongs here rather than in the import job, because this is where
// the file is first recognised as not being an archive, and both the admin page
// and the `mr` CLI surface whatever this returns.
func TestReadManifest_JunkFileExplainsWhatWasExpected(t *testing.T) {
	cases := []struct {
		name    string
		payload []byte
	}{
		{"nonsense text", []byte("this is not a tar archive at all, just nonsense text\n")},
		{"short garbage", []byte("garbage2")},
		{"empty file", []byte{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, err := NewReader(bytes.NewReader(c.payload))
			if err != nil {
				// A gzip-sniffing reader may reject here instead; either way the
				// message is what this test is about.
				assertArchiveMessage(t, err.Error())
				return
			}
			defer r.Close()

			_, err = r.ReadManifest()
			if err == nil {
				t.Fatal("expected junk bytes to be rejected")
			}
			assertArchiveMessage(t, err.Error())
		})
	}
}

// TestReadManifest_WrongFirstEntryStillExplains covers a real tar that simply is
// not one of ours — the message must not regress into naming tar internals.
func TestReadManifest_WrongFirstEntryStillExplains(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	payload := []byte(`{"hello":"world"}`)
	if err := tw.WriteHeader(&tar.Header{Name: "not-manifest.json", Mode: 0o600, Size: int64(len(payload))}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	r, err := NewReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	defer r.Close()

	_, err = r.ReadManifest()
	if err == nil {
		t.Fatal("expected a tar whose first entry is not manifest.json to be rejected")
	}
	assertArchiveMessage(t, err.Error())
}

func assertArchiveMessage(t *testing.T, msg string) {
	t.Helper()
	for _, internal := range []string{"read first entry", "unexpected EOF", "tar:"} {
		if strings.Contains(msg, internal) {
			t.Errorf("message exposes internals (%q): %s", internal, msg)
		}
	}
	if !strings.Contains(strings.ToLower(msg), "export archive") {
		t.Errorf("message should say what the file was expected to be, got: %s", msg)
	}
	if !strings.Contains(msg, "manifest.json") {
		t.Errorf("message should name what a valid archive starts with, got: %s", msg)
	}
}
