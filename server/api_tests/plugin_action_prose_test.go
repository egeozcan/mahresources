package api_tests

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The helper that answers "may this request run that plugin's actions" is named
// far from the file that declares it: in CLAUDE.md, in the commit that
// introduced it, and in this package's tests. Prose compiles whatever it says,
// so a name that no declaration answers to costs nothing at build time and costs
// a maintainer a search through two packages for code that is not there. This
// guard is the only kind that can catch it.
//
// The pattern is anchored on the left, so the several identifiers that carry it
// only as a suffix (writeActionPlugin, enableSyncActionPlugin, LogActionPlugin)
// are not candidates. What counts as declared is read out of the parsed syntax
// tree rather than the file text, so a mention inside a comment cannot stand in
// for the declaration it names.
func TestPluginActionProseNamesOnlyDeclaredHelpers(t *testing.T) {
	root := repoRootDir(t)
	namePattern := regexp.MustCompile(`\b[aA]ctionPlugin[A-Za-z0-9_]*\b`)

	declared := map[string]bool{}
	handlerDir := filepath.Join(root, "server", "api_handlers")
	entries, err := os.ReadDir(handlerDir)
	if err != nil {
		t.Fatalf("read server/api_handlers: %v", err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		// Mode 0 leaves comments out of the tree, which is the point: only code
		// declares.
		parsed, parseErr := parser.ParseFile(fset, filepath.Join(handlerDir, entry.Name()), nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", entry.Name(), parseErr)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && namePattern.MatchString(id.Name) {
				declared[id.Name] = true
			}
			return true
		})
	}

	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if name := info.Name(); name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, ".md") {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, _ := filepath.Rel(root, path)
		for i, line := range strings.Split(string(src), "\n") {
			for _, name := range namePattern.FindAllString(line, -1) {
				if !declared[name] {
					t.Errorf("%s:%d names %q, which no declaration in server/api_handlers answers to", rel, i+1, name)
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk: %v", walkErr)
	}
}

// plugin_action_rationale_test.go guards against wording appearing in
// action_handlers.go and CLAUDE.md, and the comment above that guard says, in
// the present tense, what the wording would mean: that both files record the
// wrong reason for the nil-principal divergence. Neither file records it. The
// reason both give is the one that same comment calls correct, that the only
// callers reaching the carve-out are this package's bare handler mounts.
//
// The comment also quotes a sentence it says the helper's comment ends with,
// and makes half a directive turn on it. That sentence is in no file in the
// repository.
//
// A committed claim of a live defect is not free. A maintainer who reads it goes
// looking, finds the code already saying the right thing, and cannot tell
// whether the guard is stale, whether a fix was lost, or which half of the
// directive still applies. In a tree whose comments are expected to carry real
// arguments, that is a false statement about the code sitting in the same
// commit as the code.
//
// Either repair the comment itself offers clears this, and this guard does not
// choose between them: delete the guard along with its comment, or rewrite the
// comment to describe the divergence that is actually in the tree.
func TestPluginActionRationaleDescribesTheTreeItGuards(t *testing.T) {
	root := repoRootDir(t)
	rel := filepath.Join("server", "api_tests", "plugin_action_rationale_test.go")

	src, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		if os.IsNotExist(err) {
			// Both claims went with the file.
			return
		}
		t.Fatalf("read %s: %v", rel, err)
	}

	// Scoped to this one file on purpose. The claims are quoted here as string
	// literals, so a repository-wide scan would find this file and report itself.
	for _, c := range []struct{ claim, contradiction string }{
		{
			"The reason recorded for it is not",
			"asserts that action_handlers.go and CLAUDE.md record the wrong reason for the nil-principal divergence, and both record the one this comment calls correct",
		},
		{
			"moving that case first",
			"quotes a sentence as ending the helper's comment, and no file in the repository contains it",
		},
	} {
		if strings.Contains(string(src), c.claim) {
			t.Errorf("%s %s (claim: %q)", rel, c.contradiction, c.claim)
		}
	}
}
