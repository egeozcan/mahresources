package mrql

import (
	"fmt"
	"strings"
)

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

	// Validate ORDER BY fields — must be sortable (scalar or meta, not relation/traversal).
	for _, ob := range q.OrderBy {
		if err := validateFieldExpr(ob.Field, entityType); err != nil {
			return err
		}
		if err := validateSortable(ob.Field, entityType); err != nil {
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

// extractEntityTypeFromNode extracts a single entity type only if a
// `type = "..."` comparison appears in a top-level AND chain (i.e.,
// reachable purely through AND from the root). A type comparison under
// OR or NOT does not constrain the whole query to one entity type —
// those queries run as cross-entity.
func extractEntityTypeFromNode(node Node) EntityType {
	types := collectTopLevelTypes(node)
	if len(types) == 0 {
		return EntityUnspecified
	}
	// All top-level type comparisons must agree
	first := types[0]
	for _, et := range types[1:] {
		if et != first {
			return EntityUnspecified
		}
	}
	return first
}

// collectTopLevelTypes finds `type = "..."` comparisons reachable only through
// AND from the root. It does NOT descend into OR or NOT branches, because a
// type comparison inside those doesn't constrain the entire query.
func collectTopLevelTypes(node Node) []EntityType {
	switch n := node.(type) {
	case *BinaryExpr:
		if n.Operator.Type == TokenAnd {
			// AND: both sides constrain the query
			left := collectTopLevelTypes(n.Left)
			right := collectTopLevelTypes(n.Right)
			return append(left, right...)
		}
		// OR: type comparisons inside OR don't constrain the whole query
		return nil

	case *NotExpr:
		// NOT type = "resource" doesn't mean "only resources"
		return nil

	case *ComparisonExpr:
		if isTypeField(n.Field) && n.Operator.Type == TokenEq {
			if sl, ok := n.Value.(*StringLiteral); ok {
				if et, valid := ValidEntityTypes[strings.ToLower(sl.Value)]; valid {
					return []EntityType{et}
				}
			}
		}
	}
	return nil
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
		// For OR branches under an unspecified entity type, each branch may
		// declare its own type. Validate each against its own extracted type
		// so queries like (type=note AND noteType=1) OR (type=resource AND contentType~"image/*") work.
		leftType := entityType
		rightType := entityType
		if entityType == EntityUnspecified && n.Operator.Type == TokenOr {
			if lt := extractEntityTypeFromNode(n.Left); lt != EntityUnspecified {
				leftType = lt
			}
			if rt := extractEntityTypeFromNode(n.Right); rt != EntityUnspecified {
				rightType = rt
			}
		}
		if err := validateNode(n.Left, leftType); err != nil {
			return err
		}
		return validateNode(n.Right, rightType)

	case *NotExpr:
		return validateNode(n.Expr, entityType)

	case *ComparisonExpr:
		if err := validateFieldExpr(n.Field, entityType); err != nil {
			return err
		}
		// Validate type pseudo-field: only = and != with valid entity names allowed
		if isTypeField(n.Field) {
			if n.Operator.Type != TokenEq && n.Operator.Type != TokenNeq {
				return &ValidationError{
					Message: fmt.Sprintf("type field only supports = and != operators, got %q", n.Operator.Value),
					Pos:     n.Operator.Pos,
					Length:  n.Operator.Length,
				}
			}
			if sl, ok := n.Value.(*StringLiteral); ok {
				if _, valid := ValidEntityTypes[strings.ToLower(sl.Value)]; !valid {
					return &ValidationError{
						Message: fmt.Sprintf("invalid entity type value %q: must be one of resource, note, group", sl.Value),
						Pos:     sl.Pos(),
						Length:  len(sl.Value),
					}
				}
			}
		}
		// Validate operators on relation fields: only =, !=, ~, !~ are supported
		if !isTypeField(n.Field) {
			fieldName := n.Field.Parts[0].Value
			fd, ok := LookupField(entityType, fieldName)
			if ok && fd.Type == FieldRelation {
				switch n.Operator.Type {
				case TokenEq, TokenNeq, TokenLike, TokenNotLike:
					// supported
				default:
					return &ValidationError{
						Message: fmt.Sprintf("field %q is a relation and only supports =, !=, ~, !~ operators", fieldName),
						Pos:     n.Operator.Pos,
						Length:  n.Operator.Length,
					}
				}
			}
		}
		return nil

	case *InExpr:
		// Reject type IN (...) and traversal IN (...)
		if isTypeField(n.Field) {
			return &ValidationError{
				Message: "type field does not support IN operator; use type = \"...\" or type != \"...\"",
				Pos:     n.Field.Pos(),
				Length:  len(n.Field.Name()),
			}
		}
		if len(n.Field.Parts) == 2 {
			prefix := n.Field.Parts[0].Value
			if prefix == "parent" || prefix == "children" {
				return &ValidationError{
					Message: fmt.Sprintf("%s.%s does not support IN operator; use = or != instead", prefix, n.Field.Parts[1].Value),
					Pos:     n.Field.Pos(),
					Length:  len(n.Field.Name()),
				}
			}
		}
		// Reject bare parent/children IN — translator only supports tags/groups IN
		if len(n.Field.Parts) == 1 {
			fieldName := n.Field.Parts[0].Value
			if fieldName == "parent" || fieldName == "children" {
				return &ValidationError{
					Message: fmt.Sprintf("%s does not support IN operator; use %s = \"...\" or %s IS EMPTY instead", fieldName, fieldName, fieldName),
					Pos:     n.Field.Pos(),
					Length:  len(fieldName),
				}
			}
		}
		return validateFieldExpr(n.Field, entityType)

	case *IsExpr:
		// Reject traversal IS EMPTY (not translatable as a subfield check),
		// but allow traversal IS NULL / IS NOT NULL (translatable via subquery).
		if len(n.Field.Parts) == 2 {
			prefix := n.Field.Parts[0].Value
			if (prefix == "parent" || prefix == "children") && !n.IsNull {
				return &ValidationError{
					Message: fmt.Sprintf("%s.%s does not support IS EMPTY; use parent/children IS EMPTY or %s.%s = \"...\" instead", prefix, n.Field.Parts[1].Value, prefix, n.Field.Parts[1].Value),
					Pos:     n.Field.Pos(),
					Length:  len(n.Field.Name()),
				}
			}
		}
		// Reject IS NULL on relation fields (tags, groups) — use IS EMPTY instead.
		// parent IS NULL and children IS NULL are handled by the IS EMPTY path.
		if n.IsNull && len(n.Field.Parts) == 1 {
			fieldName := n.Field.Parts[0].Value
			fd, ok := LookupField(entityType, fieldName)
			if ok && fd.Type == FieldRelation && fieldName != "parent" && fieldName != "children" {
				return &ValidationError{
					Message: fmt.Sprintf("use \"%s IS EMPTY\" instead of \"%s IS NULL\" for relation fields", fieldName, fieldName),
					Pos:     n.Field.Pos(),
					Length:  len(fieldName),
				}
			}
		}
		return validateFieldExpr(n.Field, entityType)

	case *TextSearchExpr:
		// TEXT ~ "..." has no field reference to validate
		return nil
	}
	return nil
}

// validateSortable rejects ORDER BY on fields that don't map to scalar columns
// (relation fields like tags/groups, and traversal paths like parent.name).
func validateSortable(f *FieldExpr, entityType EntityType) error {
	// Traversal fields (parent.X, children.X) are not sortable
	if len(f.Parts) == 2 {
		prefix := f.Parts[0].Value
		if prefix == "parent" || prefix == "children" {
			return &ValidationError{
				Message: fmt.Sprintf("cannot ORDER BY %s: traversal fields are not sortable", f.Name()),
				Pos:     f.Pos(),
				Length:  len(f.Name()),
			}
		}
		// meta.X is sortable
		return nil
	}

	name := f.Parts[0].Value
	if name == "type" {
		return &ValidationError{
			Message: "cannot ORDER BY type",
			Pos:     f.Pos(),
			Length:  len(name),
		}
	}

	fd, ok := LookupField(entityType, name)
	if !ok {
		return nil // unknown field already caught by validateFieldExpr
	}
	if fd.Type == FieldRelation {
		return &ValidationError{
			Message: fmt.Sprintf("cannot ORDER BY %s: relation fields are not sortable", name),
			Pos:     f.Pos(),
			Length:  len(name),
		}
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
			// Validate the subfield against group fields
			subField := f.Parts[1].Value
			if subField == "meta" {
				return nil // meta.* always valid
			}
			if _, ok := LookupField(EntityGroup, subField); !ok && !IsCommonField(subField) {
				return &ValidationError{
					Message: fmt.Sprintf("unknown field %q for %s traversal; valid fields: name, description, tags, category, id, created, updated, meta.*", subField, prefix),
					Pos:     f.Parts[1].Pos,
					Length:  len(subField),
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
