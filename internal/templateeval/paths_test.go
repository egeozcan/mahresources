package templateeval

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestModuleRootWalksUpToGoMod(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "server", "api_tests")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ModuleRoot(deep)
	if err != nil || got != root {
		t.Fatalf("ModuleRoot = %q, %v; want %q", got, err, root)
	}
	if _, err := ModuleRoot(string(filepath.Separator)); err == nil {
		t.Fatal("no go.mod above / should be an error")
	}
}

// TestResolveOutPath: the test runs in server/api_tests, so a relative
// TEMPLATE_EVAL_OUT, and the default, must land under the module root, not
// under that working directory.
func TestResolveOutPath(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "repo")
	at := time.Date(2026, 10, 24, 15, 4, 5, 0, time.UTC)
	cases := map[string]string{
		"": filepath.Join(root, "benchmarks", "template-generation", "results", "eval-20261024T150405Z.json"),
		"benchmarks/template-generation/results/x.json":            filepath.Join(root, "benchmarks", "template-generation", "results", "x.json"),
		filepath.Join(string(filepath.Separator), "tmp", "y.json"): filepath.Join(string(filepath.Separator), "tmp", "y.json"),
	}
	for in, want := range cases {
		if got := ResolveOutPath(in, root, at); got != want {
			t.Errorf("ResolveOutPath(%q) = %q, want %q", in, got, want)
		}
	}
}
