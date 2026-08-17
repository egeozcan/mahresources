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

// Role capability is decided in server/authz_policy.go, against URL paths. That
// is the whole of it — nothing else in the tree consults a role — so an
// operation reached any other way (a plugin hook fired by ordinary CRUD, a
// plugin action, a plugin page) arrives with no capability check at all. Scope
// cannot stand in for it: the tables these operations write carry no owner, so
// there is no subtree to confine the write to.
//
// application_context therefore guards those operations itself, and this test is
// what keeps a new one from being added without that decision being made. It is
// a source-level check on purpose: what it prevents is an operation nobody has
// written yet, which no behavioural test can cover.
//
// The rule it encodes is "an exported method named after one of these entities
// is a mutation unless it is plainly a read, and a mutation carries a guard".
// The polarity matters and was not free: an earlier version listed the mutating
// verbs instead, so BulkDeleteCategories — a name matching no listed verb —
// would have been admitted in silence. Enumerating the *reads* means an
// unrecognised name fails rather than passes.
//
// It deliberately does NOT say "every write to these tables". That rule is
// false: a plain user's remote upload find-or-creates a Category, and group
// import creates and renames them in bulk. Both are legitimate at capWrite, and
// neither goes through an operation this test matches.
func TestTaxonomyOperationsCarryARoleGuard(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "application_context")

	// Entities whose write operations the HTTP layer gates above capWrite.
	// Stems, not whole words: BulkDeleteCategories is a mutation of the same
	// table and "Category" does not appear in it.
	entities := []string{"Categor", "NoteType", "Relation", "TemplatePartial"}
	// Prefixes that make a method a read. Anything else is treated as a
	// mutation and must carry a guard.
	readPrefixes := []string{"Get", "Is", "Has", "List", "Count", "Find", "Query", "Load", "Resolve", "Detect", "Search", "Suggest", "Export"}
	guards := []string{"requireTaxonomyRole(", "requireEditorRole("}

	// Named exemptions, each with the reason it is not a capability decision.
	exempt := map[string]string{
		// CRUD factories, not operations: each returns an EntityWriter built from
		// the context it is called on, and every one of them is built once at
		// startup from the unbound singleton (server/routes.go). That is a known
		// gap in its own right — such a writer never binds a principal, so no
		// guard placed here could fire. Guarding the operations is the answer.
		"CategoryCRUD":         "returns a writer; the singleton it is built from carries no principal",
		"NoteTypeCRUD":         "returns a writer; the singleton it is built from carries no principal",
		"ResourceCategoryCRUD": "returns a writer; the singleton it is built from carries no principal",
	}

	fset := token.NewFileSet()
	var checked int

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
		// Comments are blanked at file level, before any function is sliced out
		// of it. stripGoComments overwrites them in place with spaces, so every
		// offset the parser reported still points at the same token — and it
		// parses a FILE, which a lone function declaration is not: handing it a
		// fragment makes it fall back to returning the text unchanged, comments
		// and all. That is precisely the hole this closes, so a commented-out
		// guard, or the doc comment each of these carries naming the guard in
		// prose, cannot stand in for the call.
		stripped := stripGoComments(readFileForGate(t, path))

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil || !fn.Name.IsExported() {
				continue
			}
			if !roleGuardReceiverIsContext(fn) {
				continue
			}
			if !roleGuardContainsAny(fn.Name.Name, entities) {
				continue
			}
			if roleGuardHasPrefixAny(fn.Name.Name, readPrefixes) {
				continue
			}
			if _, ok := exempt[fn.Name.Name]; ok {
				continue
			}

			checked++
			body := roleGuardSlice(t, fset, stripped, fn)
			if !roleGuardContainsAny(body, guards) {
				t.Errorf("%s (%s) mutates taxonomy but calls no capability guard.\n"+
					"Add requireTaxonomyRole (admin: categories, resource categories, template partials) or "+
					"requireEditorRole (editor: note types, relation types, relation edges); "+
					"if it is a read, name it so; if it is neither, exempt it here with a reason.",
					fn.Name.Name, name)
			}
		}
	}

	// A rule that silently stops matching anything is not a rule. This floor
	// catches the whole family being renamed or moved, which would otherwise
	// leave the test passing over nothing.
	if checked < 16 {
		t.Fatalf("only %d taxonomy operations were checked, which is fewer than exist: the matcher has stopped finding them", checked)
	}
}

func roleGuardReceiverIsContext(fn *ast.FuncDecl) bool {
	if len(fn.Recv.List) != 1 {
		return false
	}
	star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	ident, ok := star.X.(*ast.Ident)
	return ok && ident.Name == "MahresourcesContext"
}

func roleGuardHasPrefixAny(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func roleGuardContainsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func readFileForGate(t *testing.T, path string) string {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(src)
}

// roleGuardSlice returns one declaration's text out of the comment-stripped
// file, so the guard is looked for in the function that must carry it rather
// than anywhere in the file.
func roleGuardSlice(t *testing.T, fset *token.FileSet, src string, node ast.Node) string {
	t.Helper()
	start := fset.Position(node.Pos()).Offset
	end := fset.Position(node.End()).Offset
	if start < 0 || end > len(src) || start >= end {
		t.Fatalf("could not slice a declaration out of %d bytes", len(src))
	}
	return src[start:end]
}
