package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The accessors allowed to reach a process-wide host surface directly. Each one
// consults the open transaction first and falls back to the singleton.
var transactionAwareAccessors = map[string]bool{
	"kvStoreFor":          true,
	"loggerFor":           true,
	"loggerForInvocation": true,
}

// The process-wide accessors they wrap.
var singletonHostSurfaces = map[string]string{
	"getKVStore":      "mah.kv",
	"getPluginLogger": "mah.log",
}

// TestPluginHostSurfacesRespectAnOpenTransaction is the transaction sibling of
// TestPluginDbAccessGoesThroughTheBinder, and it exists because the failure it
// prevents is invisible rather than loud.
//
// getKVStore and getPluginLogger return the adapter captured at wiring time,
// whose *gorm.DB is the singleton's. Inside mah.db.transaction that is a
// different connection from the one the transaction holds — so the write does
// not join the transaction, and on SQLite it first blocks on the writer lock
// that transaction is holding until busy_timeout gives up. Neither outcome
// raises anything a plugin author would see: a stored cursor simply survives a
// rollback, or a log line simply does not appear.
//
// The next host-backed surface somebody adds to the plugin API will be copied
// from mah.kv, which is exactly the shape that used to be wrong.
func TestPluginHostSurfacesRespectAnOpenTransaction(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "plugin_system")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read plugin_system: %v", err)
	}

	var offenders []string
	allowed := map[string]int{}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			enclosing := fn.Name.Name
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				surface, watched := singletonHostSurfaces[sel.Sel.Name]
				if !watched {
					return true
				}
				if transactionAwareAccessors[enclosing] {
					allowed[sel.Sel.Name]++
					return true
				}
				offenders = append(offenders,
					name+": "+enclosing+" calls "+sel.Sel.Name+"() ("+surface+")")
				return true
			})
		}
	}

	sort.Strings(offenders)
	for _, o := range offenders {
		t.Errorf("plugin_system/%s. A host surface that writes to the database must be "+
			"reached through its transaction-aware accessor (kvStoreFor/loggerFor), or a call "+
			"made inside mah.db.transaction writes on a second connection: it escapes the "+
			"rollback and, on SQLite, blocks on the writer lock the transaction holds.", o)
	}

	// Guard against the rule quietly checking nothing.
	for accessor, surface := range singletonHostSurfaces {
		if allowed[accessor] == 0 {
			t.Fatalf("no %s call found inside a transaction-aware accessor: this test has "+
				"stopped checking %s", accessor, surface)
		}
	}
}
