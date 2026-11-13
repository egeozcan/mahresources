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

// TestContainmentWritersRederiveTheirScope pins the #113 rule: a write that
// changes which group owns a row, carries one entity's content onto another,
// or deletes a group, resource or note must re-derive the caller's subtree on
// its own transaction with rescopeTx (which also re-reads the account), not
// trust the allow-list bound when the request started. Seven review rounds
// each found one more writer that did not, so the list is checked here.
//
// Two checks:
//   - every pinned writer calls rescopeTx( itself;
//   - every function in application_context whose body carries an
//     owner-changing statement, moves versions between resources, or calls a
//     delete helper is either pinned or exempted below with a reason. A new
//     writer of that kind therefore fails here until someone decides.
//
// It is a text check over comment-stripped bodies, so it proves a call is
// present, not that it comes first; the stale-binding tests in
// application_context (group_reparent_scope_test.go) and
// server/api_tests/group_reparent_stale_scope_test.go prove the behaviour.
func TestContainmentWritersRederiveTheirScope(t *testing.T) {
	pinned := map[string]bool{
		"UpdateGroup":             true,
		"MergeGroups":             true,
		"massEdit":                true,
		"EditResource":            true,
		"CreateOrUpdateNote":      true,
		"MergeResourcesExpecting": true,
		"DeleteGroup":             true,
		"BulkDeleteGroups":        true,
		"DeleteResource":          true,
		"BulkDeleteResources":     true,
		"DeleteNote":              true,
		"BulkDeleteNotes":         true,
	}
	exempt := map[string]string{
		"deleteGroupInTransaction": "delete helper; its callers (DeleteGroup, BulkDeleteGroups, MergeGroups) rescope first",
		"deleteResourceDBOnly":     "delete helper; its callers (BulkDeleteResources, MergeResourcesExpecting) rescope first",
		"deleteNoteInTransaction":  "delete helper; its callers (DeleteNote, BulkDeleteNotes) rescope first",
	}
	// Each signature is a set of substrings that must all appear in one body.
	signatures := [][]string{
		{"SET owner_id"},
		{`Update("owner_id"`},
		{"ResourceVersion{}", `"resource_id":`}, // re-pointing versions at another resource
		{"deleteGroupInTransaction("},
		{"deleteResourceDBOnly("},
		{"deleteNoteInTransaction("},
	}

	root := moduleRoot(t)
	dir := filepath.Join(root, "application_context")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read application_context: %v", err)
	}

	fset := token.NewFileSet()
	found := map[string]bool{}
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
			if !ok || fn.Body == nil {
				continue
			}
			if !isContextMethod(fn) {
				continue
			}
			fname := fn.Name.Name
			body := roleGuardSlice(t, fset, stripped, fn)
			if pinned[fname] {
				found[fname] = true
				if !strings.Contains(body, "rescopeTx(") {
					t.Errorf("%s (%s) is a containment writer but does not call rescopeTx.\n"+
						"Re-derive the subtree on its transaction before it reads its targets "+
						"(see rescopeTx and docs/architecture/authentication-and-roles.md).", fname, name)
				}
				continue
			}
			if _, ok := exempt[fname]; ok {
				continue
			}
			for _, sig := range signatures {
				if containsAll(body, sig) {
					t.Errorf("%s (%s) contains %q, which changes ownership, moves content between "+
						"entities or deletes one, but is neither pinned as a rescopeTx writer nor exempted.\n"+
						"Call rescopeTx on its transaction and pin it here, or exempt it with a reason.", fname, name, sig)
					break
				}
			}
		}
	}

	var missing []string
	for fname := range pinned {
		if !found[fname] {
			missing = append(missing, fname)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("pinned containment writers not found in application_context (renamed? update this list): %v", missing)
	}
}

// isContextMethod reports whether fn is a method on *MahresourcesContext, the
// type every application writer hangs off.
func isContextMethod(fn *ast.FuncDecl) bool {
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		return false
	}
	star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	id, ok := star.X.(*ast.Ident)
	return ok && id.Name == "MahresourcesContext"
}

// TestUpdateGroupTakesTheTreeLockFirstAndUnconditionally pins part of the #113
// fix that no SQLite test can see, because lockGroupTreeMutation is a no-op
// there and CI runs no Postgres tests. UpdateGroup clears the owner when
// OwnerId is 0, and that is a re-parent to the root. If the lock is taken only
// when OwnerId != 0 (the upstream shape), a stale scoped re-parent on Postgres
// can queue on the row behind an admin's uncommitted owner clear, still match
// its constant allow-list once the clear commits, and put the group back
// under the scoped user's subtree. The lock must therefore be the first
// statement of UpdateGroup's transaction, outside any branch.
// TestUpdateGroup_OwnerClearTakesTheTreeLock_Postgres and
// TestUpdateGroup_ScopedReparentQueuedBehindAdminOwnerClear_Postgres in
// application_context check the behaviour.
func TestUpdateGroupTakesTheTreeLockFirstAndUnconditionally(t *testing.T) {
	path := filepath.Join(moduleRoot(t), "application_context", "group_crud_context.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	var update *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "UpdateGroup" && isContextMethod(fn) && fn.Body != nil {
			update = fn
		}
	}
	if update == nil {
		t.Fatal("UpdateGroup not found in application_context/group_crud_context.go (moved? update this test)")
	}

	var txBodies []*ast.BlockStmt
	ast.Inspect(update.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Transaction" || len(call.Args) == 0 {
			return true
		}
		if lit, ok := call.Args[0].(*ast.FuncLit); ok {
			txBodies = append(txBodies, lit.Body)
		}
		return true
	})
	if len(txBodies) != 1 {
		t.Fatalf("UpdateGroup opens %d transactions with a function literal, want 1 (restructured? update this test)", len(txBodies))
	}

	body := txBodies[0]
	if len(body.List) == 0 || !isTreeLockStmt(body.List[0]) {
		t.Fatal("UpdateGroup's transaction does not start with an unconditional " +
			"`if err := ctx.lockGroupTreeMutation(tx); err != nil { ... }`.\n" +
			"An owner clear (OwnerId 0) is a re-parent to the root and must take the tree lock " +
			"before any group row is read, or a parked scoped re-parent can undo it on Postgres (#113).")
	}
}

// isTreeLockStmt reports whether stmt is `if err := <x>.lockGroupTreeMutation(...); ...`.
func isTreeLockStmt(stmt ast.Stmt) bool {
	ifStmt, ok := stmt.(*ast.IfStmt)
	if !ok {
		return false
	}
	assign, ok := ifStmt.Init.(*ast.AssignStmt)
	if !ok || len(assign.Rhs) != 1 {
		return false
	}
	call, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "lockGroupTreeMutation"
}

func containsAll(s string, subs []string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
