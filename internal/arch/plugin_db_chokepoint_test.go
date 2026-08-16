package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"testing"
)

// The two functions allowed to reach the unbound provider. Everything else must
// go through querierFor/writerFor, which bind the call to its triggering
// principal.
var chokepointBinders = map[string]bool{
	"querierFor": true,
	"writerFor":  true,
}

// TestPluginDbAccessGoesThroughTheBinder pins every mah.db call site to the
// actor-bound adapter.
//
// getDbProvider/getDbWriter return the adapter captured at wiring time, whose
// context carries no principal — so anything a plugin creates through it gets a
// NULL CreatedByUserId under -auth, and any job it starts is nobody's. That was
// the state of the whole surface before per-invocation binding, and the fix was
// mechanical across 19 call sites.
//
// Mechanical is exactly the problem: the next mah.db function somebody adds will
// be copied from a neighbour, and if that neighbour predates this rule the new
// function silently reaches for the unbound provider. Nothing else fails —
// the call works, it just writes NULL. This test is the thing that notices.
func TestPluginDbAccessGoesThroughTheBinder(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "plugin_system", "db_api.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse db_api.go: %v", err)
	}

	// enclosing tracks the innermost named function around each position, so a
	// call inside a closure is attributed to the function that declares it.
	var offenders []string
	var allowed int

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		name := fn.Name.Name
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name != "getDbProvider" && sel.Sel.Name != "getDbWriter" {
				return true
			}
			if chokepointBinders[name] {
				allowed++
				return true
			}
			offenders = append(offenders, name+" calls "+sel.Sel.Name+"()")
			return true
		})
	}

	sort.Strings(offenders)
	for _, o := range offenders {
		t.Errorf("plugin_system/db_api.go: %s. Plugin DB access must go through "+
			"querierFor(L)/writerFor(L) so the call runs as the principal that triggered "+
			"it; the unbound provider stamps a NULL creator.", o)
	}

	// Guard against the rule quietly checking nothing — if the binders stop
	// calling the fallback, this test would pass on an empty file.
	if allowed == 0 {
		t.Fatal("no getDbProvider/getDbWriter call found inside querierFor/writerFor: " +
			"this test has stopped checking anything")
	}
}
