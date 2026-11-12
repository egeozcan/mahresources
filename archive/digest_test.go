package archive

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

const testDigest = "0123456789abcdef0123456789abcdef01234567"

func TestValidDigest(t *testing.T) {
	cases := []struct {
		hashType, hash string
		want           bool
	}{
		{"SHA1", testDigest, true},
		{"", testDigest, true},
		{"SHA1", strings.ToUpper(testDigest), false},
		{"SHA1", testDigest[:39], false},
		{"SHA1", testDigest + "0", false},
		{"SHA1", "../_job_replay_key", false},
		{"SHA1", "", false},
		{"MD5", testDigest, false},
	}
	for _, c := range cases {
		if got := ValidDigest(c.hashType, c.hash); got != c.want {
			t.Errorf("ValidDigest(%q, %q) = %v, want %v", c.hashType, c.hash, got, c.want)
		}
	}
}

// walkOne writes an archive whose resource and blob carry the given hashes and
// walks it with a collector, returning the collector and Walk's error.
func walkOne(t *testing.T, resHash, blobName string) (*testCollector, error) {
	t.Helper()
	var buf bytes.Buffer
	w, _ := NewWriter(&buf, false)
	_ = w.WriteManifest(&Manifest{SchemaVersion: SchemaVersion, CreatedBy: "mahresources"})
	_ = w.WriteResource(&ResourcePayload{ExportID: "r0001", Hash: resHash, HashType: "SHA1", BlobRef: resHash})
	_ = w.WriteBlob(blobName, strings.NewReader("x"), 1)
	_ = w.Close()

	r, err := NewReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, err := r.ReadManifest(); err != nil {
		t.Fatal(err)
	}
	c := newTestCollector()
	return c, r.Walk(c)
}

// Issue #94: the importer builds a storage path from the blob entry's name and
// the resource's hash, so the reader must never hand a visitor one that is not
// a digest.
func TestReader_RefusesHashesThatAreNotDigests(t *testing.T) {
	for _, bad := range []string{"../_job_replay_key", "../_imports/x.plan.json", strings.ToUpper(testDigest), testDigest[:39]} {
		c, err := walkOne(t, testDigest, bad)
		var format *FormatError
		if !errors.As(err, &format) {
			t.Errorf("blob entry %q: Walk = %v, want a FormatError", bad, err)
		}
		for h := range c.blobs {
			if !IsDigest(h) {
				t.Errorf("OnBlob saw %q", h)
			}
		}

		c, err = walkOne(t, bad, testDigest)
		if !errors.As(err, &format) {
			t.Errorf("resource hash %q: Walk = %v, want a FormatError", bad, err)
		}
		if len(c.resources) != 0 {
			t.Errorf("resource hash %q reached OnResource", bad)
		}
	}

	c, err := walkOne(t, testDigest, testDigest)
	if err != nil {
		t.Fatalf("valid archive: %v", err)
	}
	if len(c.resources) != 1 || len(c.blobs) != 1 {
		t.Fatalf("valid archive: %d resources, %d blobs", len(c.resources), len(c.blobs))
	}
}
