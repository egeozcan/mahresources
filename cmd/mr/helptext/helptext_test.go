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

func TestLoadExampleHasNoLeadingNewline(t *testing.T) {
	h := Load(testFS, "testdata/valid.md")
	if strings.HasPrefix(h.Example, "\n") {
		t.Errorf("Example has leading newline: %q", h.Example)
	}
	if strings.HasPrefix(h.Example, " \n") {
		t.Errorf("Example has leading space+newline: %q", h.Example)
	}
	trimmed := strings.TrimLeft(h.Example, " \t")
	if !strings.HasPrefix(trimmed, "#") {
		t.Errorf("Example first non-whitespace char should be #, got: %q", h.Example)
	}
}

func TestLoadFrontMatterAllowsBlankLines(t *testing.T) {
	h := Load(testFS, "testdata/blank_line_in_front_matter.md")
	if got, want := h.Annotations["outputShape"], "Resource object"; got != want {
		t.Errorf("Annotations[outputShape] = %q, want %q", got, want)
	}
	if got, want := h.Annotations["exitCodes"], "0 on success; 1 on any error"; got != want {
		t.Errorf("Annotations[exitCodes] = %q, want %q", got, want)
	}
}
