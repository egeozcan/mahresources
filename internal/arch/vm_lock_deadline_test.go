package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// TestLockVMIntroducesNoDeadline pins the one claim the cancellable VM lock is
// built on: waiting for a plugin's VM can now be *abandoned* by the caller, and
// nothing else about it changed. A caller whose context never ends waits exactly
// as long as it always did.
//
// It is a source guard rather than a timing test because the invariant is not
// testable by waiting. Proving "this never gives up" would mean outlasting every
// deadline someone might add, and the values lying around this package to be
// copied by accident — luaExecTimeout and hookLockWait, both 5s — are large
// enough that the test would cost more seconds than it is worth and still only
// rule out the caps shorter than whatever holder it picked. The property is
// instead a property of two lines, so it is checked as one.
//
// The realistic regression is not someone deciding to cap a lock wait. It is
// someone plumbing a request's context through and reaching for the timeout that
// is already in scope: every one of these call sites sits feet away from a
// context.WithTimeout(…, luaExecTimeout) for the Lua call itself. Capping the
// *wait* with the budget meant for the *execution* reads entirely natural and
// would silently start failing slow-but-healthy plugins under load.
func TestLockVMIntroducesNoDeadline(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "plugin_system", "manager.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse manager.go: %v", err)
	}

	bodies := map[string]*ast.BlockStmt{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		bodies[fn.Name.Name] = fn.Body
	}

	// LockVM is the unbounded entry point every non-request caller uses, and the
	// one every request surface used before this change. It must hand down a
	// context that cannot expire.
	lockVM, ok := bodies["LockVM"]
	if !ok {
		t.Fatal("LockVM is gone from plugin_system/manager.go; this guard no longer checks anything")
	}
	if deadline := deadlineConstructor(lockVM); deadline != "" {
		t.Fatalf("LockVM builds a deadline (%s). It is the unbounded wait: a caller that is "+
			"still there must wait for as long as the plugin takes. Bound the caller's own "+
			"context instead, which is what LockVMWithContext is for.", deadline)
	}
	if !passesBackgroundContext(lockVM) {
		t.Fatal("LockVM no longer passes context.Background() down. That call is what makes its " +
			"wait unbounded; a context with a deadline here gives every background caller a " +
			"timeout none of them asked for.")
	}

	// LockVMWithContext is where request surfaces enter. Its own wait argument
	// must stay zero: the caller's context is the only thing allowed to end it.
	withCtx, ok := bodies["LockVMWithContext"]
	if !ok {
		t.Fatal("LockVMWithContext is gone from plugin_system/manager.go; this guard no longer checks anything")
	}
	if deadline := deadlineConstructor(withCtx); deadline != "" {
		t.Fatalf("LockVMWithContext builds a deadline (%s). It must inherit the caller's, not "+
			"impose one: a plugin that is merely slow would start failing requests that were "+
			"willing to wait.", deadline)
	}
	if !locksWithinForever(withCtx) {
		t.Fatal("LockVMWithContext no longer calls LockWithin with a zero wait. A non-zero wait " +
			"here is a deadline on every request-serving surface at once.")
	}
}

// deadlineConstructor reports the first deadline-constructing call in a body,
// or "" if there is none.
func deadlineConstructor(body *ast.BlockStmt) string {
	found := ""
	ast.Inspect(body, func(n ast.Node) bool {
		if found != "" {
			return false
		}
		sel, ok := callSelector(n)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		switch {
		case pkg.Name == "context" && (sel.Sel.Name == "WithTimeout" || sel.Sel.Name == "WithDeadline"):
			found = pkg.Name + "." + sel.Sel.Name
		case pkg.Name == "time" && (sel.Sel.Name == "After" || sel.Sel.Name == "NewTimer" || sel.Sel.Name == "NewTicker"):
			found = pkg.Name + "." + sel.Sel.Name
		}
		return true
	})
	return found
}

// passesBackgroundContext reports whether the body contains a context.Background()
// call, which is how LockVM says "no deadline".
func passesBackgroundContext(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		sel, ok := callSelector(n)
		if !ok {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "context" && sel.Sel.Name == "Background" {
			found = true
		}
		return true
	})
	return found
}

// locksWithinForever reports whether every LockWithin call in the body passes a
// literal 0 as its wait. A named constant would pass a substring check and be a
// deadline all the same, so the literal is the thing checked.
func locksWithinForever(body *ast.BlockStmt) bool {
	calls := 0
	forever := 0
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "LockWithin" || len(call.Args) != 2 {
			return true
		}
		calls++
		if lit, ok := call.Args[1].(*ast.BasicLit); ok && lit.Kind == token.INT && lit.Value == "0" {
			forever++
		}
		return true
	})
	return calls > 0 && calls == forever
}

func callSelector(n ast.Node) (*ast.SelectorExpr, bool) {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return sel, ok
}
