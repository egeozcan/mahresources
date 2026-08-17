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
// is the whole of it — nothing below server/ consults a role — so an operation
// reached any other way (a plugin hook fired by ordinary CRUD, a plugin action,
// a plugin page) arrives with no capability check at all. Scope cannot stand in
// for it: the tables these operations write carry no owner, so there is no
// subtree to confine the write to.
//
// application_context therefore guards the taxonomy and editor-level operations
// itself, and this test is what keeps a new one from being added without that
// decision being made. It is a source-level check on purpose: what it prevents
// is an operation nobody has written yet, which no behavioural test can cover.
//
// The rule it encodes is "an exported mutation named after one of these
// entities calls a capability guard". It deliberately does NOT say "every write
// to these tables", because that rule is false: a plain user's upload
// find-or-creates a Category, and group import creates and renames them in
// bulk. Both are legitimate at capWrite, and neither goes through an operation
// this test matches.
func TestTaxonomyOperationsCarryARoleGuard(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "application_context")

	// Entities whose write operations the HTTP layer gates above capWrite.
	// Relation *edges* are deliberately absent. /v1/relation is capEditor at the
	// HTTP layer, but an edge is subtree-checkable data about two groups, and
	// relationInScope already confines it — enforcing editor below server/ would
	// delete the confined-principal edge editing that guard was built for, since
	// no stored account is both scoped and an editor. Relation *types* are here:
	// they are global, they carry no owner, and deleting one cascades to every
	// edge of that type database-wide.
	entities := []string{"Category", "NoteType", "RelationType", "RelationshipType", "TemplatePartial"}
	// Verbs that make a method a mutation rather than a read.
	verbs := []string{"Create", "Update", "Delete", "Add", "Edit", "Patch", "Merge"}
	guards := []string{"requireTaxonomyRole(", "requireEditorRole("}

	// Named exemptions go here, each with the reason it is not a capability
	// decision. Empty today: every operation the matcher finds is one.
	exempt := map[string]string{}

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
		file, parseErr := parser.ParseFile(fset, path, nil, 0) // comments dropped
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil || !fn.Name.IsExported() {
				continue
			}
			if !roleGuardReceiverIsContext(fn) {
				continue
			}
			if !roleGuardHasPrefixAny(fn.Name.Name, verbs) || !roleGuardContainsAny(fn.Name.Name, entities) {
				continue
			}
			if _, ok := exempt[fn.Name.Name]; ok {
				continue
			}

			checked++
			body := roleGuardNodeSource(t, fset, path, fn)
			if !roleGuardContainsAny(body, guards) {
				t.Errorf("%s (%s) mutates taxonomy but calls no capability guard.\n"+
					"Add requireTaxonomyRole (admin: categories, resource categories, template partials) or "+
					"requireEditorRole (editor: note types, relation types, relation edges), or exempt it here with a reason.",
					fn.Name.Name, name)
			}
		}
	}

	// A rule that silently stops matching anything is not a rule. The count is
	// the current set of guarded operations; it exists so that renaming the
	// whole family, or moving the files, fails loudly instead of passing.
	if checked < 13 {
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

// nodeSource returns the raw source of a declaration, so the guard is looked for
// in the function that must carry it rather than anywhere in the file.
func roleGuardNodeSource(t *testing.T, fset *token.FileSet, path string, node ast.Node) string {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	start := fset.Position(node.Pos()).Offset
	end := fset.Position(node.End()).Offset
	if start < 0 || end > len(src) || start >= end {
		t.Fatalf("could not slice %s out of %s", path, path)
	}
	return string(src[start:end])
}
