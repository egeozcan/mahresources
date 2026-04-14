package helptext

import (
	"embed"
	"strings"
	"testing"
)

//go:embed testdata/*.md
var testFS embed.FS

func TestLoadValid(t *testing.T) {
	h := Load(testFS, "testdata/valid.md")
	if !strings.Contains(h.Long, "Get a resource by ID and print its metadata.") {
		t.Fatalf("Long missing expected content: %q", h.Long)
	}
	if !strings.Contains(h.Example, "mr resource get 42") {
		t.Fatalf("Example missing expected content: %q", h.Example)
	}
	want := map[string]string{
		"outputShape": "Resource object with id, name, tags, groups, meta",
		"exitCodes":   "0 on success; 1 on any error",
		"relatedCmds": "resource edit, resource versions, resource download",
	}
	for k, v := range want {
		if h.Annotations[k] != v {
			t.Errorf("Annotations[%q] = %q, want %q", k, h.Annotations[k], v)
		}
	}
}

func TestLoadMissingLongPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for missing # Long section")
		}
	}()
	Load(testFS, "testdata/missing_long.md")
}

func TestLoadMalformedFrontMatterPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for malformed front matter")
		}
	}()
	Load(testFS, "testdata/malformed_front_matter.md")
}
