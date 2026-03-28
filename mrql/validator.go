package mrql

import "fmt"

// ValidationError is returned when semantic validation of a Query fails.
type ValidationError struct {
	Message string
	Pos     int
	Length  int
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error at position %d: %s", e.Pos, e.Message)
}

// Validate performs semantic validation of a parsed Query AST.
//
// It proceeds in two passes:
//  1. Extract the entity type from any `type = "resource|note|group"` comparison
//     found in the WHERE clause (if Query.EntityType is unset).
//  2. Walk the WHERE and ORDER BY fields, verifying that each referenced field
//     is valid for the resolved entity type.
//
// Returns *ValidationError on the first invalid field found, or nil if valid.
func Validate(q *Query) error {
	// Determine the effective entity type.
	entityType := q.EntityType
	if entityType == EntityUnspecified && q.Where != nil {
		entityType = extractEntityTypeFromNode(q.Where)
	}

	// Validate WHERE clause fields.
	if q.Where != nil {
		if err := validateNode(q.Where, entityType); err != nil {
			return err
		}
	}

	// Validate ORDER BY fields.
	for _, ob := range q.OrderBy {
		if err := validateFieldExpr(ob.Field, entityType); err != nil {
			return err
		}
	}

	return nil
}

// ExtractEntityType is a public wrapper that extracts the entity type from the
// query's WHERE clause without performing full validation.
func ExtractEntityType(q *Query) EntityType {
	if q.EntityType != EntityUnspecified {
		return q.EntityType
	}
	if q.Where == nil {
		return EntityUnspecified
	}
	return extractEntityTypeFromNode(q.Where)
}

// extractEntityTypeFromNode walks the AST looking for `type = "<value>"` comparisons.
// Returns EntityUnspecified if no such comparison is found.
func extractEntityTypeFromNode(node Node) EntityType {
	switch n := node.(type) {
	case *BinaryExpr:
		if et := extractEntityTypeFromNode(n.Left); et != EntityUnspecified {
			return et
		}
		return extractEntityTypeFromNode(n.Right)

	case *NotExpr:
		return extractEntityTypeFromNode(n.Expr)

	case *ComparisonExpr:
		if isTypeField(n.Field) && n.Operator.Type == TokenEq {
			if sl, ok := n.Value.(*StringLiteral); ok {
				if et, valid := ValidEntityTypes[sl.Value]; valid {
					return et
				}
			}
		}
	}
	return EntityUnspecified
}

// isTypeField returns true if the FieldExpr refers to the "type" pseudo-field.
func isTypeField(f *FieldExpr) bool {
	return len(f.Parts) == 1 && f.Parts[0].Value == "type"
}

// validateNode recursively validates all field references within a node.
// It also validates `type = "..."` values for invalid entity type strings.
func validateNode(node Node, entityType EntityType) error {
	switch n := node.(type) {
	case *BinaryExpr:
		if err := validateNode(n.Left, entityType); err != nil {
			return err
		}
		return validateNode(n.Right, entityType)

	case *NotExpr:
		return validateNode(n.Expr, entityType)

	case *ComparisonExpr:
		if err := validateFieldExpr(n.Field, entityType); err != nil {
			return err
		}
		// Validate entity type value in `type = "..."` comparisons
		if isTypeField(n.Field) && n.Operator.Type == TokenEq {
			if sl, ok := n.Value.(*StringLiteral); ok {
				if _, valid := ValidEntityTypes[sl.Value]; !valid {
					return &ValidationError{
						Message: fmt.Sprintf("invalid entity type value %q: must be one of resource, note, group", sl.Value),
						Pos:     sl.Pos(),
						Length:  len(sl.Value),
					}
				}
			}
		}
		return nil

	case *InExpr:
		return validateFieldExpr(n.Field, entityType)

	case *IsExpr:
		return validateFieldExpr(n.Field, entityType)

	case *TextSearchExpr:
		// TEXT ~ "..." has no field reference to validate
		return nil
	}
	return nil
}

// validateFieldExpr checks that the referenced field (or traversal) is valid for
// the given entity type.
func validateFieldExpr(f *FieldExpr, entityType EntityType) error {
	if len(f.Parts) == 0 {
		return nil
	}

	firstName := f.Parts[0].Value

	// "type" is always a valid pseudo-field for entity type filtering.
	if firstName == "type" && len(f.Parts) == 1 {
		return nil
	}

	// Handle dotted traversal: parent.field, children.field, or meta.key
	if len(f.Parts) == 2 {
		prefix := firstName
		switch prefix {
		case "meta":
			// meta.* is always valid
			return nil
		case "parent", "children":
			// Traversal only allowed on group entities
			if entityType != EntityGroup {
				return &ValidationError{
					Message: fmt.Sprintf("field %q: parent/children traversal is only valid for group entities (got %s)", f.Name(), entityType),
					Pos:     f.Pos(),
					Length:  len(f.Name()),
				}
			}
			return nil
		default:
			return &ValidationError{
				Message: fmt.Sprintf("unknown field prefix %q in %q", prefix, f.Name()),
				Pos:     f.Pos(),
				Length:  len(f.Name()),
			}
		}
	}

	// Single-part field name lookup
	fieldName := firstName
	_, ok := LookupField(entityType, fieldName)
	if !ok {
		return &ValidationError{
			Message: fmt.Sprintf("unknown or invalid field %q for entity type %s", fieldName, entityType),
			Pos:     f.Pos(),
			Length:  len(fieldName),
		}
	}
	return nil
}
