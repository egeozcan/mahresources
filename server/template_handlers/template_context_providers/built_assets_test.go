package template_context_providers

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMissingBuiltAssets(t *testing.T) {
	root := t.TempDir()
	want := []string{"public/tailwind.css", "public/dist/main.js"}
	if got := MissingBuiltAssets(root); !reflect.DeepEqual(got, want) {
		t.Fatalf("empty checkout: got %v, want %v", got, want)
	}

	if err := os.MkdirAll(filepath.Join(root, "public", "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "public", "tailwind.css"), []byte("a{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := MissingBuiltAssets(root); !reflect.DeepEqual(got, []string{"public/dist/main.js"}) {
		t.Fatalf("css only: got %v", got)
	}

	if err := os.WriteFile(filepath.Join(root, "public", "dist", "main.js"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := MissingBuiltAssets(root); len(got) != 0 {
		t.Fatalf("built checkout: got %v, want none", got)
	}
}
