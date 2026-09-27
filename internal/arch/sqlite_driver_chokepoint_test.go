package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// sqliteOpeners may open a SQLite handle themselves, each for the reason given.
var sqliteOpeners = map[string]string{
	"models/database.go": "the chokepoint: it opens every production handle with models.SQLiteDriverName",
	"server/api_tests/api_test_utils.go": "a test harness in a package nothing else imports; its " +
		"handlers run on go-sqlite3's own driver",
}

// TestProductionSQLiteHandlesUseTheServerDriver keeps every SQLite handle the server
// opens on models.SQLiteDriverName.
//
// That driver is what makes a transaction that reads before it writes safe on
// SQLite: it begins every transaction by taking the writer lock, unless the
// transaction is declared read-only. Under go-sqlite3's own driver the same code
// takes a WAL snapshot at its first read and fails with "database is locked" as soon
// as another connection commits before its first write, and nothing in the code
// shows the difference. A handle opened anywhere but models would silently bring
// that back for every transaction on it.
func TestProductionSQLiteHandlesUseTheServerDriver(t *testing.T) {
	root := moduleRoot(t)
	var offenders []string
	scanned := 0

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if rel == "." {
				return nil
			}
			name := entry.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "e2e" || name == "testdata" {
				return filepath.SkipDir
			}
			// A nested checkout (another branch's worktree) is not this module's code.
			if _, statErr := os.Stat(filepath.Join(path, ".git")); statErr == nil {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		scanned++
		if _, allowed := sqliteOpeners[rel]; allowed {
			return nil
		}
		for _, site := range sqliteOpenSites(t, path) {
			offenders = append(offenders, rel+":"+site)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk module: %v", err)
	}
	if scanned < 100 {
		t.Fatalf("scanned only %d Go files; the walk is broken, not the code", scanned)
	}
	sort.Strings(offenders)
	if len(offenders) > 0 {
		t.Fatalf("SQLite opened outside models; use models.CreateDatabaseConnection or "+
			"models.CreateReadOnlyDatabaseConnection:\n  %s", strings.Join(offenders, "\n  "))
	}
}

// sqliteOpenSites reports the places in one file that open SQLite without the
// server's driver: any use of gorm's SQLite dialector, and database/sql or sqlx
// opening the "sqlite3" driver by name.
func sqliteOpenSites(t *testing.T, path string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	gormSQLite := ""
	for _, spec := range file.Imports {
		importPath, _ := strconv.Unquote(spec.Path.Value)
		if importPath != "gorm.io/driver/sqlite" {
			continue
		}
		gormSQLite = "sqlite"
		if spec.Name != nil {
			gormSQLite = spec.Name.Name
		}
	}

	var sites []string
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if gormSQLite != "" && gormSQLite != "_" && pkg.Name == gormSQLite {
			sites = append(sites, strconv.Itoa(fset.Position(sel.Pos()).Line)+" "+pkg.Name+"."+sel.Sel.Name)
		}
		return true
	})
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "Open" && sel.Sel.Name != "Connect") {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || (pkg.Name != "sql" && pkg.Name != "sqlx") {
			return true
		}
		if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Value == `"sqlite3"` {
			sites = append(sites, strconv.Itoa(fset.Position(call.Pos()).Line)+" "+pkg.Name+"."+sel.Sel.Name+`("sqlite3")`)
		}
		return true
	})
	return sites
}
