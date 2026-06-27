package mrql

import "unicode"

const (
	MaxGeneratedLimit  = 500
	MaxGeneratedOffset = 10000
)

func LintGeneratedQuery(q *Query) []map[string]any {
	var errs []map[string]any
	add := func(pos int, msg string) {
		err := map[string]any{"message": msg}
		if pos >= 0 {
			err["pos"] = pos
			err["length"] = 1
		}
		errs = append(errs, err)
	}

	if q.Limit > MaxGeneratedLimit || q.Limit == 0 {
		add(-1, "LIMIT must be between 1 and 500 for generated MRQL")
	}
	if q.Offset > MaxGeneratedOffset {
		add(-1, "OFFSET must be between 0 and 10000 for generated MRQL")
	}

	walkGeneratedNode(q.Where, func(n Node) {
		switch expr := n.(type) {
		case *TextSearchExpr:
			if !containsAlphaNum(expr.Value.Value) {
				add(expr.Pos(), "TEXT search must contain at least one alphanumeric term")
			}
		case *ComparisonExpr:
			name := expr.Field.Name()
			if requiresGeneratedNumericIDField(expr.Field) {
				if _, ok := expr.Value.(*NumberLiteral); !ok {
					add(expr.Pos(), name+" requires a numeric ID in generated MRQL")
				}
			}
		case *InExpr:
			name := expr.Field.Name()
			if requiresGeneratedNumericIDField(expr.Field) {
				for _, v := range expr.Values {
					if _, ok := v.(*NumberLiteral); !ok {
						add(expr.Pos(), name+" requires a numeric ID in generated MRQL")
						break
					}
				}
			}
		}
	})

	return errs
}

func walkGeneratedNode(n Node, visit func(Node)) {
	if n == nil {
		return
	}
	visit(n)
	switch x := n.(type) {
	case *BinaryExpr:
		walkGeneratedNode(x.Left, visit)
		walkGeneratedNode(x.Right, visit)
	case *NotExpr:
		walkGeneratedNode(x.Expr, visit)
	}
}

func containsAlphaNum(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func requiresGeneratedNumericIDField(field *FieldExpr) bool {
	if field == nil || len(field.Parts) == 0 {
		return false
	}

	if len(field.Parts) == 1 {
		switch field.Parts[0].Value {
		case "category", "resourceCategory", "noteType":
			return true
		default:
			return false
		}
	}

	if hasGeneratedMetaPath(field) {
		return false
	}

	if field.Parts[len(field.Parts)-1].Value != "category" {
		return false
	}

	_, ok := traversalRoots[field.Parts[0].Value]
	return ok
}

func hasGeneratedMetaPath(field *FieldExpr) bool {
	for _, part := range field.Parts[:len(field.Parts)-1] {
		if part.Value == "meta" {
			return true
		}
	}
	return false
}
