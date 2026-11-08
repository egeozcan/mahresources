package template_filters

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// Privacy mode builds the preview's meta context from a masked copy of the
// entity and does no second masking pass (plan P14). That holds only while
// populatePresentationNames reads the preloaded graph of the entity it is given
// and never the database. This pin fails if it grows a resolver parameter or
// starts calling one.
func TestPopulatePresentationNamesNeverReadsTheDatabase(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "shortcode_tag.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var fn *ast.FuncDecl
	for _, d := range file.Decls {
		if f, ok := d.(*ast.FuncDecl); ok && f.Name.Name == "populatePresentationNames" {
			fn = f
		}
	}
	if fn == nil {
		t.Fatal("populatePresentationNames moved; update this pin and re-check plan P14")
	}
	if n := len(fn.Type.Params.List); n != 2 {
		t.Fatalf("populatePresentationNames takes %d parameters; it must take only the context being filled and the entity value", n)
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Ident:
			if x.Name == "appCtx" || x.Name == "db" || x.Name == "DB" {
				t.Errorf("populatePresentationNames references %q: it must not read the database", x.Name)
			}
		case *ast.CallExpr:
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
				switch sel.Sel.Name {
				case "Find", "First", "Take", "Where", "Resolve", "ResolveRootScopeID", "ResolveParentScopeID", "GetGroup":
					t.Errorf("populatePresentationNames calls %s", sel.Sel.Name)
				}
			}
		}
		return true
	})
}
