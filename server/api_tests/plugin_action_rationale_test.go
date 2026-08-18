package api_tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRootDir walks up from the package directory to the module root, so the two
// guards below can read sources outside their own package.
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

// actionPluginAllowed diverges from auth.PluginAccessFor on one case: a context
// carrying no principal is unrestricted here and refused there. The divergence
// is right. The reason recorded for it is not, in the helper's own comment and
// in CLAUDE.md, which repeats it.
//
// Measured, by making the nil branch refuse: every TestPluginActions_* test
// still passes, TestPluginActions_AuthDisabledListsEveryAction among them, and
// that one goes through the real router with auth off. What breaks is fourteen
// TestActionRun_* tests, which mount the handler bare and post with no
// principal. No deployment loses a button to the refusal; this package's own
// bare mounts do, and nothing else does.
//
// The comment's preceding sentence already says why it cannot be otherwise:
// withAuthentication is the outermost middleware on the only router, and its
// auth-off branch attaches the root admin, falling back to
// auth.SystemPrincipal() — both non-nil. Nor are there "embeds":
// TestActionHandlers_BareMountsAreTestsOnly measures that below.
//
// This is load-bearing prose, not decoration. It ends "Do not 'unify' this onto
// PluginAccessFor without moving that case first", and a reader who checks the
// stated reason finds it false with no way to tell which half of the directive
// survives.
//
// Absence rather than the presence of particular replacement wording, so this
// passes under either repair: rewriting the reason to name this package's bare
// mounts, or attaching principals in those tests and unifying onto
// PluginAccessFor, which deletes the helper outright.
func TestActionPluginAllowed_RationaleDoesNotClaimAnAuthOffDeployment(t *testing.T) {
	root := repoRootDir(t)

	for _, f := range []struct {
		path   string
		claims []string
	}{
		{
			filepath.Join("server", "api_handlers", "action_handlers.go"),
			[]string{
				"auth-off deployment's action buttons away",
				"by tests and by embeds",
			},
		},
		{
			"CLAUDE.md",
			[]string{
				"auth-off deployment's action buttons away",
				"mounted bare by tests and embeds",
			},
		},
	} {
		src, err := os.ReadFile(filepath.Join(root, f.path))
		if err != nil {
			t.Fatalf("read %s: %v", f.path, err)
		}
		for _, claim := range f.claims {
			if strings.Contains(string(src), claim) {
				t.Errorf("%s still gives %q as the reason the nil-principal case diverges, which measurement contradicts", f.path, claim)
			}
		}
	}
}

// The true and only constraint behind the divergence, pinned so a later mount
// that really does pass no principal cannot appear without this being revisited.
// Both production mounts are routes, and every route sits behind
// withAuthentication, which attaches a principal in either auth mode.
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
			if name := info.Name(); name == ".git" || name == "node_modules" {
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
