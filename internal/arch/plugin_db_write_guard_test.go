package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPluginDBAdapterWritersCarryAWriteRoleGuard pins the plugin seam. mah.db
// writes made by plugin Lua on a read surface never pass through the URL-path
// rule, so every mutating pluginDBAdapter method (and RunInTransaction) must
// ask requireWriteRole itself. Reads are enumerated by prefix, so an
// unrecognised name fails rather than passes. It is a sibling of
// TestTaxonomyOperationsCarryARoleGuard and reuses its helpers.
func TestPluginDBAdapterWritersCarryAWriteRoleGuard(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "application_context")

	readPrefixes := []string{"Get", "List", "Query", "Count", "Open", "KV", "Plugin", "Probe", "Frame", "Bind"}
	exempt := map[string]string{
		"TrimVideoClip":           "guarded in trimVideoGated, which it calls",
		"TrimVideoClipToResource": "guarded in TrimVideoToNewResource, which it calls",
	}

	fset := token.NewFileSet()
	var checked int
	sawTransaction := false

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read application_context: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		stripped := stripGoComments(readFileForGate(t, path))

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil || !fn.Name.IsExported() {
				continue
			}
			if len(fn.Recv.List) != 1 {
				continue
			}
			star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			if id, ok := star.X.(*ast.Ident); !ok || id.Name != "pluginDBAdapter" {
				continue
			}
			if roleGuardHasPrefixAny(fn.Name.Name, readPrefixes) {
				continue
			}
			if _, ok := exempt[fn.Name.Name]; ok {
				continue
			}
			checked++
			if fn.Name.Name == "RunInTransaction" {
				sawTransaction = true
			}
			body := roleGuardSlice(t, fset, stripped, fn)
			if !strings.Contains(body, "requireWriteRole(") {
				t.Errorf("pluginDBAdapter.%s (%s) is not a read but calls no requireWriteRole.\n"+
					"Add the guard as its first statement; if it is a read, name it so; "+
					"if it is guarded elsewhere, exempt it here with a reason.", fn.Name.Name, name)
			}
		}
	}

	if checked < 45 {
		t.Fatalf("only %d pluginDBAdapter writers were checked: the matcher has stopped finding them", checked)
	}
	if !sawTransaction {
		t.Fatal("RunInTransaction was not among the checked methods")
	}
}
