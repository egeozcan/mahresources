package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// TestRetiredReplayReadersDoNotUseLegacyPlaintext pins the Task 12 boundary:
// after the writer epoch says plaintext is retired, retries and execution
// hydration must take their values from the canonical replay envelope.
func TestPlaintextRetirementReadersDoNotUseLegacyReplay(t *testing.T) {
	root := moduleRoot(t)
	checkDownloadReader(t, root, "download_history_context.go", "DownloadHistoryPayload", "entry", "legacyJobInputsRetired")
	checkDownloadReader(t, root, "scheduled_download_context.go", "ScheduledDownloadPayload", "row", "legacyJobInputsRetired")
	checkPluginCommandReader(t, root, "plugin_command_store.go", "hydratePluginCommandRun", []string{"ParamsJSON", "InputsJSON"})
	checkPluginCommandReader(t, root, "plugin_command_store.go", "hydratePluginCommandImport", []string{"FieldsJSON"})
	checkPluginImportRetryReader(t, root)
}

func parseProductionFunction(t *testing.T, root, fileName, functionName string) (*token.FileSet, *ast.FuncDecl) {
	t.Helper()
	fset := token.NewFileSet()
	path := filepath.Join(root, "application_context", fileName)
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == functionName {
			return fset, function
		}
	}
	t.Fatalf("%s has no production function %s", fileName, functionName)
	return nil, nil
}

func checkDownloadReader(t *testing.T, root, fileName, functionName, receiver, guardName string) {
	t.Helper()
	fset, function := parseProductionFunction(t, root, fileName, functionName)
	var gate *ast.IfStmt
	var guardPosition, errorReturnPosition, gatePosition token.Pos
	var canonicalResolve, canonicalOpen bool
	var earliestLegacyRead token.Pos
	ast.Inspect(function.Body, func(node ast.Node) bool {
		switch current := node.(type) {
		case *ast.CallExpr:
			if callName(current.Fun) == guardName && guardPosition == token.NoPos {
				guardPosition = current.Pos()
			}
			canonicalResolve = canonicalResolve || selectorCallNamed(current, "ResolveLegacyHandle")
			canonicalOpen = canonicalOpen || selectorCallNamed(current, "OpenReplay")
		case *ast.IfStmt:
			if errorReturnPosition == token.NoPos && isErrNotNil(current.Cond) && containsReturn(current.Body) {
				errorReturnPosition = current.Pos()
			}
			retired, ok := current.Cond.(*ast.Ident)
			if gate == nil && ok && retired.Name == "retired" && callsNamed(current.Body, "ResolveLegacyHandle") && callsNamed(current.Body, "OpenReplay") && containsReturn(current.Body) {
				gate, gatePosition = current, current.Pos()
			}
		case *ast.SelectorExpr:
			if current.Sel.Name != "Payload" && current.Sel.Name != "URL" {
				break
			}
			base, ok := current.X.(*ast.Ident)
			if !ok || base.Name != receiver {
				break
			}
			if earliestLegacyRead == token.NoPos || current.Pos() < earliestLegacyRead {
				earliestLegacyRead = current.Pos()
			}
		}
		return true
	})
	if gate == nil || !canonicalResolve || !canonicalOpen || guardPosition == token.NoPos || errorReturnPosition <= guardPosition || gatePosition <= errorReturnPosition {
		t.Fatalf("%s must fail on writer-epoch read errors and return from the retired-epoch branch only after canonical handle resolution and envelope open", functionName)
	}
	if earliestLegacyRead <= gatePosition {
		t.Fatalf("%s reads legacy Payload/URL before the retired-epoch branch", functionName)
	}
	if fset.Position(gatePosition).Line >= fset.Position(earliestLegacyRead).Line {
		t.Fatalf("%s legacy replay access is not lexically after its retirement gate", functionName)
	}
}

func isErrNotNil(expression ast.Expr) bool {
	binary, ok := expression.(*ast.BinaryExpr)
	if !ok || binary.Op != token.NEQ {
		return false
	}
	left, leftOK := binary.X.(*ast.Ident)
	right, rightOK := binary.Y.(*ast.Ident)
	return leftOK && rightOK && left.Name == "err" && right.Name == "nil"
}

func checkPluginCommandReader(t *testing.T, root, fileName, functionName string, sensitiveFields []string) {
	t.Helper()
	_, function := parseProductionFunction(t, root, fileName, functionName)
	var readsFence, guardedReturn, opensCanonical bool
	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		readsFence = readsFence || (ok && callName(call.Fun) == "pluginCommandInputsRetired")
		return true
	})
	var openPosition token.Pos
	for _, statement := range function.Body.List {
		branch, ok := statement.(*ast.IfStmt)
		if !ok {
			continue
		}
		unary, ok := branch.Cond.(*ast.UnaryExpr)
		if ok && unary.Op == token.NOT {
			identifier, isIdentifier := unary.X.(*ast.Ident)
			if isIdentifier && identifier.Name == "retired" && containsReturn(branch.Body) {
				guardedReturn = true
			}
		}
	}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if ok && selectorCallNamed(call, "OpenReplay") {
			opensCanonical = true
			openPosition = call.Pos()
		}
		return true
	})
	if !readsFence || !guardedReturn || !opensCanonical {
		t.Fatalf("%s must fence legacy input behind the writer epoch and open canonical replay", functionName)
	}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || !containsString(sensitiveFields, selector.Sel.Name) {
			return true
		}
		base, ok := selector.X.(*ast.Ident)
		if !ok || base.Name != "row" {
			return true
		}
		assignment, assigned := selectorParentAssignment(function.Body, selector)
		if !assigned || assignment.Tok != token.ASSIGN || selector.Pos() <= openPosition {
			t.Errorf("%s reads legacy replay field %s before canonical input is opened", functionName, selector.Sel.Name)
		}
		return true
	})
}

func checkPluginImportRetryReader(t *testing.T, root string) {
	t.Helper()
	_, function := parseProductionFunction(t, root, "plugin_command_import_command_facts.go", "pluginCommandImportFieldsJSON")
	var fencePosition, envelopePosition token.Pos
	var legacyFallback bool
	for _, statement := range function.Body.List {
		call, ok := statement.(*ast.AssignStmt)
		if !ok {
			continue
		}
		for _, value := range call.Rhs {
			if expressionCallsNamed(value, "pluginCommandInputsRetired") {
				fencePosition = value.Pos()
			}
		}
	}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if ok && selectorCallNamed(call, "OpenReplay") {
			envelopePosition = call.Pos()
		}
		branch, ok := node.(*ast.IfStmt)
		if !ok || !isPreRetirementCondition(branch.Cond, "retired") || !containsReturn(branch.Body) {
			return true
		}
		legacyFallback = legacyFallback || hasSelector(branch.Body, "source", "FieldsJSON")
		return true
	})
	if fencePosition == token.NoPos || envelopePosition <= fencePosition || !legacyFallback {
		t.Fatal("pluginCommandImportFieldsJSON must test the writer epoch before using a legacy field and open canonical replay in the retired branch")
	}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "FieldsJSON" {
			return true
		}
		base, ok := selector.X.(*ast.Ident)
		if !ok || base.Name != "source" {
			return true
		}
		if !selectorWithinNegatedBranch(function.Body, selector, "retired") {
			t.Errorf("pluginCommandImportFieldsJSON reads legacy FieldsJSON outside the pre-retirement branch")
		}
		if selector.Pos() <= fencePosition {
			t.Errorf("pluginCommandImportFieldsJSON reads legacy FieldsJSON before the writer epoch")
		}
		return true
	})
}

func selectorParentAssignment(root ast.Node, target *ast.SelectorExpr) (*ast.AssignStmt, bool) {
	var found *ast.AssignStmt
	ast.Inspect(root, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assignment.Lhs {
			if lhs == target {
				found = assignment
				return false
			}
		}
		return true
	})
	return found, found != nil
}

func callsNamed(node ast.Node, name string) bool {
	found := false
	ast.Inspect(node, func(current ast.Node) bool {
		call, ok := current.(*ast.CallExpr)
		if ok && callName(call.Fun) == name {
			found = true
			return false
		}
		return true
	})
	return found
}

func selectorCallNamed(call *ast.CallExpr, name string) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == name
}

func callName(expression ast.Expr) string {
	switch current := expression.(type) {
	case *ast.Ident:
		return current.Name
	case *ast.SelectorExpr:
		return current.Sel.Name
	default:
		return ""
	}
}

func containsReturn(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(current ast.Node) bool {
		if _, ok := current.(*ast.ReturnStmt); ok {
			found = true
			return false
		}
		return true
	})
	return found
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func expressionCallsNamed(expression ast.Expr, name string) bool {
	return callsNamed(expression, name)
}

func isNegatedIdentifier(expression ast.Expr, name string) bool {
	unary, ok := expression.(*ast.UnaryExpr)
	if !ok || unary.Op != token.NOT {
		return false
	}
	identifier, ok := unary.X.(*ast.Ident)
	return ok && identifier.Name == name
}

func hasSelector(root ast.Node, receiver, field string) bool {
	found := false
	ast.Inspect(root, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != field {
			return true
		}
		base, ok := selector.X.(*ast.Ident)
		found = found || ok && base.Name == receiver
		return true
	})
	return found
}

func selectorWithinNegatedBranch(root ast.Node, target *ast.SelectorExpr, name string) bool {
	contained := false
	ast.Inspect(root, func(node ast.Node) bool {
		branch, ok := node.(*ast.IfStmt)
		if !ok || !isPreRetirementCondition(branch.Cond, name) {
			return true
		}
		// In an && condition, the right side is evaluated only after !retired.
		conditionRight := false
		if conjunction, ok := branch.Cond.(*ast.BinaryExpr); ok && conjunction.Op == token.LAND {
			conditionRight = nodeContains(conjunction.Y, target)
		}
		if nodeContains(branch.Body, target) || conditionRight {
			contained = true
			return false
		}
		return true
	})
	return contained
}

func isPreRetirementCondition(expression ast.Expr, name string) bool {
	if isNegatedIdentifier(expression, name) {
		return true
	}
	conjunction, ok := expression.(*ast.BinaryExpr)
	return ok && conjunction.Op == token.LAND && isNegatedIdentifier(conjunction.X, name)
}

func nodeContains(root ast.Node, target ast.Node) bool {
	contained := false
	ast.Inspect(root, func(node ast.Node) bool {
		if node == target {
			contained = true
			return false
		}
		return true
	})
	return contained
}
