package api_tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRootDir walks up from the package directory to the module root, so the
// guard below and the one in plugin_action_prose_test.go can read sources
// outside their own package.
func repoRootDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod above the test directory")
		}
		dir = parent
	}
}

// actionPluginAccess grants every plugin to a request that carries no
// principal, where auth.PluginActionAccessFor refuses one. Its comment says
// that carve-out costs a deployment nothing because the only callers reaching
// it are this package's bare handler mounts, and this is the measurement behind
// that claim: both production mounts are routes, and every route sits behind
// withAuthentication, which attaches a principal to every non-public path
// in either auth mode, and neither of these handlers is on a public path. A later
// mount that really does pass no principal cannot appear without this failing,
// which is the signal to reconsider the carve-out rather than to relax the
// guard.
//
// Qualified references only (api_handlers.X), which is what a mount outside the
// defining package looks like — the declarations and the prose that names these
// handlers in application_context and plugin_system are not mounts.
func TestActionHandlers_BareMountsAreTestsOnly(t *testing.T) {
	root := repoRootDir(t)
	mounts := []string{
		"api_handlers.GetPluginActionsHandler(",
		"api_handlers.GetActionRunHandler(",
	}

	var production, tests int
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if skipScanDir(path, root, info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		found := false
		for _, mount := range mounts {
			if strings.Contains(string(src), mount) {
				found = true
			}
		}
		if !found {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if strings.HasSuffix(path, "_test.go") {
			tests++
			return nil
		}
		production++
		if rel != filepath.Join("server", "routes.go") {
			t.Errorf("%s mounts a plugin-action handler outside the routed chain; the nil-principal case is no longer tests-only", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if production == 0 || tests == 0 {
		t.Fatalf("found %d production and %d test mounts, so this guard measured nothing", production, tests)
	}
}

// skipScanDir reports whether a walk over this checkout's source should refuse
// to descend into dir.
//
// Name-based skipping is not enough. A nested git worktree is an entire second
// checkout of this repository sitting inside it -- .worktrees/<branch> is the
// convention here -- and its copy of routes.go is a real file with real mounts
// that this repository's guards must not census. It is not this checkout's
// source, it belongs to another branch, and counting it makes a guard fail on a
// developer's machine while passing on CI's fresh clone, which is the worst
// place for a guard to disagree with itself.
//
// The test is what a checkout root actually looks like rather than what it is
// called: a directory that carries its own .git entry is another checkout, and
// for a worktree that entry is a file rather than a directory. Dot-directories
// go too, which is how the catalogue drift guard in internal/arch already reads
// the tree.
func skipScanDir(path, root, name string) bool {
	if path == root {
		return false
	}
	if strings.HasPrefix(name, ".") || name == "node_modules" {
		return true
	}
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return true
	}
	return false
}
