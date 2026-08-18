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
