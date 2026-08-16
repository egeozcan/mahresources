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

// authImportAllowedFile is the single file in plugin_system permitted to import
// mahresources/auth.
const authImportAllowedFile = "actor.go"

// TestPluginSystemAuthImportIsConfined pins the auth dependency of the plugin
// host to one file.
//
// plugin_system needs exactly one fact from auth: the id of the user that
// triggered a call, so that entities a plugin creates are attributed to that
// user instead of getting a NULL creator. plugin_system/actor.go reads it and
// returns a uint; nothing else about the principal crosses the edge.
//
// The reason to police this rather than trust it: the *next* thing a
// *auth.Principal makes possible in this package is deciding that a confined
// principal may run plugin code after all. That deny is fail-closed today
// precisely because the host cannot see roles or scope, and lifting it is a
// deliberate change that has to land behind per-plugin capability grants — not
// something that arrives because a second file found a Principal already in
// scope and read one more field off it.
//
// Package-level check, not a call-level one: a file that cannot import auth
// cannot reach a Principal at all, which is a bound the compiler enforces for
// free once this test holds.
//
// _test.go files are exempt, as they are in the render-gate test next door: a
// test needs auth.WithPrincipal to build the request context it exercises the
// host with, and test files are not compiled into the package a plugin runs
// against, so nothing they import can widen production behaviour.
func TestPluginSystemAuthImportIsConfined(t *testing.T) {
	root := moduleRoot(t)
	pluginDir := filepath.Join(root, "plugin_system")

	const authPkg = `"mahresources/auth"`

	var importers []string
	fset := token.NewFileSet()

	err := filepath.Walk(pluginDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		for _, imp := range f.Imports {
			if imp.Path.Value == authPkg {
				importers = append(importers, filepath.Base(path))
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking plugin_system: %v", err)
	}

	// The allowed file must actually be one of them: if the extraction is ever
	// moved or deleted, an empty result should fail rather than pass vacuously.
	found := false
	for _, name := range importers {
		if name == authImportAllowedFile {
			found = true
			continue
		}
		t.Errorf("plugin_system/%s imports mahresources/auth; only %s may. "+
			"Read the acting user through actorFromContext and pass a uint — "+
			"a *auth.Principal must not travel further into the plugin host.",
			name, authImportAllowedFile)
	}
	if !found {
		t.Fatalf("plugin_system/%s no longer imports mahresources/auth: this test has stopped "+
			"checking anything. If the actor extraction moved, point authImportAllowedFile at "+
			"its new home.", authImportAllowedFile)
	}

	assertAuthStaysInsideFunctionBodies(t, filepath.Join(pluginDir, authImportAllowedFile))
}

// assertAuthStaysInsideFunctionBodies keeps the allowed file from re-exporting
// what the import ban exists to keep out.
//
// Confining the import to one file is not enough on its own: a function there
// that *returned* a *auth.Principal would let every other file in the package
// reach one through type inference, without importing auth and without tripping
// the check above. The narrowing that matters is that auth appears only inside
// function bodies — the package's vocabulary stays uint.
func assertAuthStaysInsideFunctionBodies(t *testing.T, path string) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}

	mentionsAuth := func(n ast.Node) bool {
		found := false
		ast.Inspect(n, func(x ast.Node) bool {
			sel, ok := x.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "auth" {
				found = true
			}
			return true
		})
		return found
	}

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Type != nil && mentionsAuth(d.Type) {
				t.Errorf("%s: func %s has auth in its signature; the plugin host must "+
					"exchange a uint, not a principal — a returned *auth.Principal is reachable "+
					"from every other file in the package without importing auth",
					filepath.Base(path), d.Name.Name)
			}
		case *ast.GenDecl:
			if mentionsAuth(d) {
				t.Errorf("%s: a package-level declaration mentions auth; keep it inside a "+
					"function body", filepath.Base(path))
			}
		}
	}
}
