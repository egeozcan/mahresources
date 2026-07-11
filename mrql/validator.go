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

// traversalRoots maps field names that can start a traversal chain to the entity
// types on which they are valid roots. "parent" and "children" are group-only;
// "owner" is valid on resources and notes.
var traversalRoots = map[string][]EntityType{
	"parent":   {EntityGroup},
	"children": {EntityGroup},
	"owner":    {EntityResource, EntityNote},
}

// traversalIntermediates are field names allowed in the middle of a traversal chain.
// Only parent and children can appear as intermediate steps — owner cannot because
// it references a group from a non-group entity and doesn't chain.
var traversalIntermediates = map[string]bool{
	"parent":   true,
	"children": true,
}

// countableRelation returns the FieldDef for fieldName if it is a relation on
// entityType that supports the .count pseudo-field: junction-backed relations
// (tags, groups/group, notes, resources) plus children on group.
func countableRelation(entityType EntityType, fieldName string) (FieldDef, bool) {
	fd, ok := LookupField(entityType, fieldName)
	if !ok || fd.Type != FieldRelation {
		return FieldDef{}, false
	}
	if _, isJunction := lookupJunction(entityType, fd.Column); isJunction {
		return fd, true
	}
	if fd.Column == "children" {
		return fd, true
	}
	return FieldDef{}, false
}

// isCountField returns true if f is a valid <relation>.count pseudo-field for
// the given entity type.
func isCountField(f *FieldExpr, entityType EntityType) bool {
	if len(f.Parts) != 2 || f.Parts[1].Value != "count" {
		return false
	}
	_, ok := countableRelation(entityType, f.Parts[0].Value)
	return ok
}

// dateBucketSuffixes are the valid GROUP BY bucket suffixes on datetime fields.
var dateBucketSuffixes = map[string]bool{
	"day": true, "week": true, "month": true, "year": true,
}

// isDateBucketField returns true if f is a <datetime>.<bucket> pseudo-field
// (e.g. created.month) for the given entity type.
func isDateBucketField(f *FieldExpr, entityType EntityType) bool {
	if len(f.Parts) != 2 || !dateBucketSuffixes[f.Parts[1].Value] {
		return false
	}
	fd, ok := LookupField(entityType, f.Parts[0].Value)
	return ok && fd.Type == FieldDateTime
}

// dateBucketWhereError is the error for date bucket pseudo-fields used outside
// GROUP BY.
func dateBucketWhereError(f *FieldExpr) *ValidationError {
	return &ValidationError{
		Message: `date bucket fields are only valid in GROUP BY; use a date range in WHERE (created >= "2026-07-01" AND created < "2026-08-01")`,
		Pos:     f.Pos(),
		Length:  len(f.Name()),
	}
}

// countOperatorError builds the standard error for unsupported operations on
// a count pseudo-field.
func countOperatorError(f *FieldExpr) *ValidationError {
	return &ValidationError{
		Message: fmt.Sprintf("%s only supports comparison operators (=, !=, >, >=, <, <=)", f.Name()),
		Pos:     f.Pos(),
		Length:  len(f.Name()),
	}
}

// isTraversalRoot returns true if fieldName is a valid traversal root for the
// given entity type.
func isTraversalRoot(fieldName string, entityType EntityType) bool {
	allowedTypes, ok := traversalRoots[fieldName]
	if !ok {
		return false
	}
	for _, et := range allowedTypes {
		if et == entityType {
			return true
		}
	}
	return false
}

// rejectFilterConstructs walks a filter-bar expression rejecting the two
// constructs ParseFilter forbids beyond the clause keywords the parser already
// catches: the `type` pseudo-field (implied by the page) and `$name` parameter
// placeholders (bar queries must be self-contained). Positions match the input.
func rejectFilterConstructs(node Node) error {
	switch n := node.(type) {
	case *BinaryExpr:
		if err := rejectFilterConstructs(n.Left); err != nil {
			return err
		}
		return rejectFilterConstructs(n.Right)
	case *NotExpr:
		return rejectFilterConstructs(n.Expr)
	case *ComparisonExpr:
		if isTypeField(n.Field) {
			return filterTypeFieldError(n.Field)
		}
		return rejectParamValue(n.Value)
	case *InExpr:
		if isTypeField(n.Field) {
			return filterTypeFieldError(n.Field)
		}
		for _, v := range n.Values {
			if err := rejectParamValue(v); err != nil {
				return err
			}
		}
	case *IsExpr:
		if isTypeField(n.Field) {
			return filterTypeFieldError(n.Field)
		}
	}
	return nil
}

// filterTypeFieldError builds the positioned error for using the `type`
// pseudo-field inside a filter-bar expression.
func filterTypeFieldError(f *FieldExpr) *ValidationError {
	return &ValidationError{
		Message: "the type field is implied by the page and cannot be used in a filter expression",
		Pos:     f.Pos(),
		Length:  len(f.Name()),
	}
}

// rejectParamValue rejects a $name parameter placeholder appearing as a value.
func rejectParamValue(value Node) error {
	if pr, ok := value.(*ParamRef); ok {
		return &ValidationError{
			Message: fmt.Sprintf("parameter placeholder $%s is not allowed in a filter expression", pr.Name),
			Pos:     pr.Pos(),
			Length:  len(pr.Name) + 1,
		}
	}
	return nil
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
	// In aggregated GROUP BY mode, ORDER BY is validated by validateGroupBy instead.
	isAggregatedGroupBy := q.GroupBy != nil && len(q.GroupBy.Aggregates) > 0
	for _, ob := range q.OrderBy {
		// ORDER BY RANDOM(): no field to validate. Rejected with GROUP BY in
		// both modes (meaningless for aggregate buckets; clashes with the
		// alias-based ORDER BY of aggregated mode).
		if ob.Random {
			if q.GroupBy != nil {
				return &ValidationError{
					Message: "ORDER BY RANDOM() is not supported with GROUP BY",
					Pos:     0,
					Length:  0,
				}
			}
			continue
		}
		// "rank" is the full-text relevance sort key (context-sensitive, like
		// distance). No entity has a real rank column, so it always means the
		// relevance key here; validateRankOrderKey enforces the TEXT ~ predicate
		// and rejects GROUP BY with a specific message (checked before the
		// aggregated-mode ORDER BY path so the message is stable in both modes).
		if len(ob.Field.Parts) == 1 && strings.EqualFold(ob.Field.Parts[0].Value, "rank") {
			if err := validateRankOrderKey(q, entityType, ob.Field); err != nil {
				return err
			}
			continue
		}
		if !isAggregatedGroupBy {
			// "distance" is the SIMILAR TO sort key, not a real column.
			if len(ob.Field.Parts) == 1 && ob.Field.Parts[0].Value == "distance" {
				if err := validateDistanceOrderKey(q, entityType, ob.Field); err != nil {
					return err
				}
				continue
			}
			// In bucketed GROUP BY mode, a date bucket key that is also a
			// GROUP BY field is a valid sort key (constant within each bucket,
			// used to order the bucket keys).
			if q.GroupBy != nil && isDateBucketField(ob.Field, entityType) && groupByHasField(q.GroupBy, ob.Field.Name()) {
				continue
			}
			if err := validateFieldExpr(ob.Field, entityType); err != nil {
				return err
			}
			if err := validateSortable(ob.Field, entityType); err != nil {
				return err
			}
		}
	}

	// Validate GROUP BY clause
	if q.GroupBy != nil {
		if err := validateGroupBy(q.GroupBy, entityType, q.OrderBy); err != nil {
			return err
		}
	}

	return nil
}

// validateDistanceOrderKey validates ORDER BY distance: resource entity only,
// no GROUP BY, and exactly one SIMILAR TO predicate in the WHERE clause (its
// target defines the distance).
func validateDistanceOrderKey(q *Query, entityType EntityType, f *FieldExpr) error {
	if entityType != EntityResource {
		return &ValidationError{
			Message: "ORDER BY distance requires type = \"resource\" with a SIMILAR TO predicate",
			Pos:     f.Pos(),
			Length:  len("distance"),
		}
	}
	if q.GroupBy != nil {
		return &ValidationError{
			Message: "ORDER BY distance is not supported with GROUP BY",
			Pos:     f.Pos(),
			Length:  len("distance"),
		}
	}
	switch len(collectSimilarToExprs(q.Where)) {
	case 0:
		return &ValidationError{
			Message: "ORDER BY distance requires a SIMILAR TO predicate in the query",
			Pos:     f.Pos(),
			Length:  len("distance"),
		}
	case 1:
		return nil
	default:
		return &ValidationError{
			Message: "ORDER BY distance is ambiguous with multiple SIMILAR TO predicates; use exactly one",
			Pos:     f.Pos(),
			Length:  len("distance"),
		}
	}
}

// validateRankOrderKey validates ORDER BY RANK: a determined single entity type,
// no GROUP BY, and exactly one TEXT ~ predicate in the WHERE clause (its term
// defines the relevance). Mirrors validateDistanceOrderKey. Cross-entity is
// rejected because bm25/ts_rank scores are not comparable across corpora.
func validateRankOrderKey(q *Query, entityType EntityType, f *FieldExpr) error {
	rankLen := len(f.Name())
	if entityType == EntityUnspecified {
		return &ValidationError{
			Message: "ORDER BY RANK requires a single entity type (add a type = ... filter)",
			Pos:     f.Pos(),
			Length:  rankLen,
		}
	}
	if q.GroupBy != nil {
		return &ValidationError{
			Message: "ORDER BY RANK is not supported with GROUP BY",
			Pos:     f.Pos(),
			Length:  rankLen,
		}
	}
	switch len(collectTextSearchExprs(q.Where)) {
	case 0:
		return &ValidationError{
			Message: "ORDER BY RANK requires a TEXT ~ predicate in the query",
			Pos:     f.Pos(),
			Length:  rankLen,
		}
	case 1:
		return nil
	default:
		return &ValidationError{
			Message: "ORDER BY RANK is ambiguous with multiple TEXT predicates; use exactly one",
			Pos:     f.Pos(),
			Length:  rankLen,
		}
	}
}

// validateRegexTraversalLeaf rejects ~*/!~* on traversal (owner./parent./
// children.) and recursive (ancestors./descendants.) chain leaves that are not
// string-typed. All chains resolve their leaf on the group entity; tags leaves
// and numeric/datetime leaves do not support regex — without this check
// `owner.tags ~* "x"` would silently translate to an equality match on the
// pattern string. Meta subpaths (meta.<key>, owner.meta.<key>) are dynamically
// typed and stay allowed: "meta" anywhere before the leaf marks the rest of the
// chain as a JSON subpath.
func validateRegexTraversalLeaf(n *ComparisonExpr) error {
	parts := n.Field.Parts
	if len(parts) < 2 {
		return nil
	}
	for _, p := range parts[:len(parts)-1] {
		if p.Value == "meta" {
			return nil
		}
	}
	root := parts[0].Value
	if !recursiveRoots[root] {
		if _, ok := traversalRoots[root]; !ok {
			return nil
		}
	}
	leaf := parts[len(parts)-1].Value
	fd, ok := LookupField(EntityGroup, leaf)
	if !ok {
		return nil // unknown leaves are caught by the chain validators
	}
	if fd.Type == FieldRelation || fd.Type == FieldNumber || fd.Type == FieldDateTime {
		return &ValidationError{
			Message: fmt.Sprintf("field %q does not support regex match", n.Field.Name()),
			Pos:     n.Operator.Pos,
			Length:  n.Operator.Length,
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
		// Re-resolve entity type inside NOT — NOT (type = "note" AND noteType = 1)
		// should validate noteType against the note entity type, not the parent's.
		innerType := entityType
		if entityType == EntityUnspecified {
			if it := extractEntityTypeFromNode(n.Expr); it != EntityUnspecified {
				innerType = it
			}
		}
		return validateNode(n.Expr, innerType)

	case *ComparisonExpr:
		if err := validateFieldExpr(n.Field, entityType); err != nil {
			return err
		}
		// Count pseudo-fields: only comparison operators against a non-negative integer.
		if isCountField(n.Field, entityType) {
			switch n.Operator.Type {
			case TokenEq, TokenNeq, TokenGt, TokenGte, TokenLt, TokenLte:
				// supported
			default:
				return countOperatorError(n.Field)
			}
			// Unbound parameter: defer the non-negative-integer check to bind time.
			if _, isParam := n.Value.(*ParamRef); isParam {
				return nil
			}
			nl, ok := n.Value.(*NumberLiteral)
			if !ok || nl.Unit != "" || nl.Value < 0 || nl.Value != float64(int64(nl.Value)) {
				return &ValidationError{
					Message: fmt.Sprintf("%s must be compared to a non-negative integer", n.Field.Name()),
					Pos:     n.Value.Pos(),
					Length:  0,
				}
			}
			return nil
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
			sl, ok := n.Value.(*StringLiteral)
			if !ok {
				return &ValidationError{
					Message: "type field requires resource, note, or group",
					Pos: n.Value.Pos(),
				}
			}
			if _, valid := ValidEntityTypes[strings.ToLower(sl.Value)]; !valid {
				return &ValidationError{
					Message: fmt.Sprintf("invalid entity type value %q: must be one of resource, note, group", sl.Value),
					Pos:     sl.Pos(),
					Length:  len(sl.Value),
				}
			}
		}
		// Validate operators on relation fields: only =, !=, ~, !~ are supported.
		// Only apply to single-part fields — multi-part traversals validate their
		// own leaf field types in validateTraversalChain.
		if !isTypeField(n.Field) && len(n.Field.Parts) == 1 {
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
		// Regex match (~*/!~*): PostgreSQL-only (dialect enforced at translation),
		// requires a string pattern, and is not allowed on numeric/datetime fields
		// or relation fields. Single-part relation fields are already rejected
		// above (regex is not in their allowed set); traversal/recursive leaves
		// are checked here against the group entity so `owner.tags ~* "x"` fails
		// loudly instead of silently equality-matching the pattern string.
		// String fields, meta keys (any subpath), and traversal string leaves
		// are allowed.
		if isRegexOperator(n.Operator) {
			switch n.Value.(type) {
			case *StringLiteral, *ParamRef:
				// ok — ParamRef re-checked after BindParams substitutes a literal.
			default:
				return &ValidationError{
					Message: fmt.Sprintf("regex match requires a string pattern for field %q", n.Field.Name()),
					Pos:     n.Value.Pos(),
					Length:  0,
				}
			}
			if !isTypeField(n.Field) && len(n.Field.Parts) == 1 {
				if fd, ok := LookupField(entityType, n.Field.Parts[0].Value); ok {
					if fd.Type == FieldNumber || fd.Type == FieldDateTime {
						return &ValidationError{
							Message: fmt.Sprintf("field %q does not support regex match", n.Field.Parts[0].Value),
							Pos:     n.Operator.Pos,
							Length:  n.Operator.Length,
						}
					}
				}
			}
			if err := validateRegexTraversalLeaf(n); err != nil {
				return err
			}
		}
		// Validate value type compatibility with field type
		if !isTypeField(n.Field) && n.Value != nil {
			if err := validateValueType(n.Field, n.Value, entityType); err != nil {
				return err
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
		// Count pseudo-fields do not support IN.
		if isCountField(n.Field, entityType) {
			return countOperatorError(n.Field)
		}
		// Reject traversal IN (multi-part chains like parent.name, owner.tags, etc.)
		if len(n.Field.Parts) >= 2 {
			prefix := n.Field.Parts[0].Value
			if recursiveRoots[prefix] {
				return &ValidationError{
					Message: fmt.Sprintf("%s does not support IN operator; use = or != instead", n.Field.Name()),
					Pos:     n.Field.Pos(),
					Length:  len(n.Field.Name()),
				}
			}
			if _, isRoot := traversalRoots[prefix]; isRoot {
				return &ValidationError{
					Message: fmt.Sprintf("%s does not support IN operator; use = or != instead", n.Field.Name()),
					Pos:     n.Field.Pos(),
					Length:  len(n.Field.Name()),
				}
			}
			if traversalIntermediates[prefix] {
				return &ValidationError{
					Message: fmt.Sprintf("%s does not support IN operator; use = or != instead", n.Field.Name()),
					Pos:     n.Field.Pos(),
					Length:  len(n.Field.Name()),
				}
			}
		}
		// Reject bare parent/children/owner IN — translator only supports tags/groups IN
		if len(n.Field.Parts) == 1 {
			fieldName := n.Field.Parts[0].Value
			if fieldName == "parent" || fieldName == "children" || fieldName == "owner" {
				return &ValidationError{
					Message: fmt.Sprintf("%s does not support IN operator; use %s = \"...\" or %s IS EMPTY instead", fieldName, fieldName, fieldName),
					Pos:     n.Field.Pos(),
					Length:  len(fieldName),
				}
			}
		}
		return validateFieldExpr(n.Field, entityType)

	case *IsExpr:
		// Count pseudo-fields do not support IS EMPTY / IS NULL.
		if isCountField(n.Field, entityType) {
			return countOperatorError(n.Field)
		}
		// Recursive roots (ancestors/descendants) support neither IS EMPTY nor
		// IS NULL — they are existential predicates over a group set.
		if len(n.Field.Parts) >= 2 && recursiveRoots[n.Field.Parts[0].Value] {
			return &ValidationError{
				Message: fmt.Sprintf("%s does not support IS EMPTY/IS NULL; compare a group field instead (e.g. %s = \"...\")", n.Field.Name(), n.Field.Name()),
				Pos:     n.Field.Pos(),
				Length:  len(n.Field.Name()),
			}
		}
		// Reject traversal IS EMPTY (not translatable as a subfield check),
		// but allow traversal IS NULL / IS NOT NULL (translatable via subquery).
		if len(n.Field.Parts) >= 2 {
			prefix := n.Field.Parts[0].Value
			isRoot := isTraversalRoot(prefix, entityType)
			isIntermediate := traversalIntermediates[prefix]
			if isRoot || isIntermediate {
				if !n.IsNull {
					return &ValidationError{
						Message: fmt.Sprintf("%s does not support IS EMPTY; use the base traversal field IS EMPTY or %s = \"...\" instead", n.Field.Name(), n.Field.Name()),
						Pos:     n.Field.Pos(),
						Length:  len(n.Field.Name()),
					}
				}
				// IS NULL on chains longer than 2 parts is not supported
				if len(n.Field.Parts) > 2 {
					return &ValidationError{
						Message: fmt.Sprintf("%s IS NULL is not supported for multi-level traversals", n.Field.Name()),
						Pos:     n.Field.Pos(),
						Length:  len(n.Field.Name()),
					}
				}
				// IS NULL on junction-table relations (tags) is not supported.
				// FK-based relations (parent, owner, children) are fine — they
				// translate to checking the FK column IS NULL on the joined group.
				if len(n.Field.Parts) == 2 {
					subField := n.Field.Parts[1].Value
					if subField == "tags" {
						return &ValidationError{
							Message: fmt.Sprintf("%s.tags IS NULL is not supported; use %s.tags = \"...\" for tag comparisons", prefix, prefix),
							Pos:     n.Field.Pos(),
							Length:  len(n.Field.Name()),
						}
					}
				}
			}
		}
		// Reject IS NULL on relation fields (tags, groups) — use IS EMPTY instead.
		// parent/children/owner IS NULL are handled by the IS EMPTY path.
		if n.IsNull && len(n.Field.Parts) == 1 {
			fieldName := n.Field.Parts[0].Value
			fd, ok := LookupField(entityType, fieldName)
			if ok && fd.Type == FieldRelation && fieldName != "parent" && fieldName != "children" && fieldName != "owner" {
				return &ValidationError{
					Message: fmt.Sprintf("use \"%s IS EMPTY\" instead of \"%s IS NULL\" for relation fields", fieldName, fieldName),
					Pos:     n.Field.Pos(),
					Length:  len(fieldName),
				}
			}
		}
		// For traversal IS NULL on FK relation leaves (parent, owner, children),
		// skip validateFieldExpr — it would reject them as unsupported leaf fields,
		// but they're valid for IS NULL checks (checking FK column IS NULL).
		if n.IsNull && len(n.Field.Parts) == 2 {
			subField := n.Field.Parts[1].Value
			if subField == "parent" || subField == "owner" || subField == "children" {
				prefix := n.Field.Parts[0].Value
				if isTraversalRoot(prefix, entityType) || traversalIntermediates[prefix] {
					return nil
				}
			}
		}
		return validateFieldExpr(n.Field, entityType)

	case *TextSearchExpr:
		// TEXT ~ "..." has no field reference to validate
		return nil

	case *SimilarToExpr:
		// SIMILAR TO reads resource perceptual-hash pairs — resource-only.
		// In type-guarded OR branches the BinaryExpr case above re-derives
		// the entity type per branch, so this sees the branch's type.
		if entityType != EntityResource {
			return &ValidationError{
				Message: "SIMILAR TO requires type = \"resource\" — only resources have perceptual hashes",
				Pos:     n.Pos(),
				Length:  len("SIMILAR TO"),
			}
		}
		if n.TargetID <= 0 {
			return &ValidationError{
				Message: "SIMILAR TO requires a positive resource ID",
				Pos:     n.Pos(),
				Length:  len("SIMILAR TO"),
			}
		}
		if n.Within > MaxSimilarityDistance {
			return &ValidationError{
				Message: fmt.Sprintf("WITHIN %d exceeds the maximum similarity distance %d — pairs are only stored up to distance %d", n.Within, MaxSimilarityDistance, MaxSimilarityDistance),
				Pos:     n.Pos(),
				Length:  len("SIMILAR TO"),
			}
		}
		return nil
	}
	return nil
}

// validateSortable rejects ORDER BY on fields that don't map to scalar columns
// (relation fields like tags/groups, and traversal paths like parent.name).
func validateSortable(f *FieldExpr, entityType EntityType) error {
	// Multi-part fields: allow meta.X, reject all traversal ORDER BY
	if len(f.Parts) >= 2 {
		prefix := f.Parts[0].Value
		if prefix == "meta" {
			// meta.X is sortable
			return nil
		}
		// <relation>.count is sortable (correlated scalar subquery)
		if isCountField(f, entityType) {
			return nil
		}
		// Any traversal field (parent.X, children.X, owner.X, ancestors.X, etc.) is not sortable
		if recursiveRoots[prefix] {
			return &ValidationError{
				Message: fmt.Sprintf("cannot ORDER BY %s: traversal fields are not sortable", f.Name()),
				Pos:     f.Pos(),
				Length:  len(f.Name()),
			}
		}
		if _, isRoot := traversalRoots[prefix]; isRoot {
			return &ValidationError{
				Message: fmt.Sprintf("cannot ORDER BY %s: traversal fields are not sortable", f.Name()),
				Pos:     f.Pos(),
				Length:  len(f.Name()),
			}
		}
		if traversalIntermediates[prefix] {
			return &ValidationError{
				Message: fmt.Sprintf("cannot ORDER BY %s: traversal fields are not sortable", f.Name()),
				Pos:     f.Pos(),
				Length:  len(f.Name()),
			}
		}
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

// validateValueType checks that the comparison value is compatible with the field type.
// Number fields reject string values; datetime fields reject plain numbers;
// meta fields and string fields accept anything (meta is dynamically typed).
func validateValueType(field *FieldExpr, value Node, entityType EntityType) error {
	fieldName := field.Parts[0].Value

	// Unbound parameter placeholder: accepted against any field type here; the
	// concrete value's type is re-checked after BindParams substitutes a literal.
	if _, ok := value.(*ParamRef); ok {
		return nil
	}

	// Meta fields are dynamically typed — any value is fine
	if len(field.Parts) >= 2 && field.Parts[0].Value == "meta" {
		return nil
	}
	// Traversal subfields — validated by the translator
	if len(field.Parts) >= 2 {
		prefix := field.Parts[0].Value
		if recursiveRoots[prefix] {
			return nil
		}
		if _, isRoot := traversalRoots[prefix]; isRoot {
			return nil
		}
		if traversalIntermediates[prefix] {
			return nil
		}
	}

	fd, ok := LookupField(entityType, fieldName)
	if !ok {
		return nil // field lookup errors caught elsewhere
	}

	switch fd.Type {
	case FieldNumber:
		// Number fields only accept numeric values.
		// FK fields (category, noteType) are typed as FieldNumber but users
		// may pass string values (which just won't match — no crash). Only
		// enforce for non-FK numeric fields.
		isFKField := strings.HasSuffix(fd.Column, "_id")
		if !isFKField {
			switch value.(type) {
			case *NumberLiteral:
				// ok
			default:
				return &ValidationError{
					Message: fmt.Sprintf("field %q is numeric but got a non-numeric value", fieldName),
					Pos:     value.Pos(),
					Length:  0,
				}
			}
		}
	case FieldDateTime:
		// DateTime fields accept strings (date literals), relative dates, and functions
		switch value.(type) {
		case *StringLiteral, *RelDateLiteral, *FuncCall:
			// ok
		default:
			return &ValidationError{
				Message: fmt.Sprintf("field %q is a datetime; use a date string, relative date (-7d), or function (NOW())", fieldName),
				Pos:     value.Pos(),
				Length:  0,
			}
		}
	}
	// FieldString, FieldRelation, FieldMeta — accept any value type
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

	// Multi-part fields: meta.key, count/bucket pseudo-fields, or traversal chains
	if len(f.Parts) >= 2 {
		prefix := firstName

		// meta.* is always valid (2-part only)
		if prefix == "meta" {
			return nil
		}

		// Date bucket pseudo-fields are only valid in GROUP BY (handled by
		// validateGroupBy before it calls this function) and as aggregated-mode
		// ORDER BY keys (validated against the key allowlist instead).
		if isDateBucketField(f, entityType) {
			return dateBucketWhereError(f)
		}

		// <relation>.count pseudo-field
		if len(f.Parts) == 2 && f.Parts[1].Value == "count" {
			if _, ok := countableRelation(entityType, prefix); ok {
				return nil
			}
			if fd, ok := LookupField(entityType, prefix); ok && fd.Type == FieldRelation {
				// Single-reference relations (owner, parent) get a targeted error.
				if fd.Column == "owner_id" || fd.Column == "parent_id" {
					return &ValidationError{
						Message: fmt.Sprintf("%s is a single reference and cannot be counted; use %s IS NULL / IS NOT NULL", prefix, prefix),
						Pos:     f.Pos(),
						Length:  len(f.Name()),
					}
				}
				// Countable relations need a concrete entity to resolve the
				// junction table — cross-entity queries cannot use .count.
				if entityType == EntityUnspecified {
					return &ValidationError{
						Message: fmt.Sprintf("%s requires an explicit entity type (e.g. type = \"resource\")", f.Name()),
						Pos:     f.Pos(),
						Length:  len(f.Name()),
					}
				}
			}
		}

		// Recursive hierarchy traversal: ancestors.X / descendants.X.
		if recursiveRoots[prefix] {
			return validateRecursiveChain(f, entityType)
		}

		// Traversal chain: root.intermediate...leaf
		return validateTraversalChain(f, entityType)
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

// validateTraversalChain validates a multi-part field expression as a traversal
// chain. The chain is classified as: root . [intermediate...] . leaf
//
// Rules:
//   - Root must be a valid traversal root for the entity type (parent/children
//     for groups, owner for resources/notes).
//   - Intermediates must be parent or children only (owner is not valid as an
//     intermediate because it references a group from a non-group entity).
//   - Leaf must be a valid group field: scalar fields or tags. Meta and other
//     relation fields (parent, children, groups) are not supported as leaves.
//   - For 2-part chains (root.leaf), the leaf is validated directly.
//   - For 3+ part chains, all parts between root and leaf are intermediates.
func validateTraversalChain(f *FieldExpr, entityType EntityType) error {
	root := f.Parts[0].Value

	// Validate root is a known traversal field for this entity type
	if !isTraversalRoot(root, entityType) {
		// Check if it's a known traversal root on some other entity type
		if _, anyRoot := traversalRoots[root]; anyRoot {
			return &ValidationError{
				Message: fmt.Sprintf("field %q: %s traversal is not valid for entity type %s", f.Name(), root, entityType),
				Pos:     f.Pos(),
				Length:  len(f.Name()),
			}
		}
		return &ValidationError{
			Message: fmt.Sprintf("unknown field prefix %q in %q; not a traversal field", root, f.Name()),
			Pos:     f.Pos(),
			Length:  len(f.Name()),
		}
	}

	// For chains with 3+ parts, validate intermediates (all parts except first and last).
	// Special case: "meta" followed by a key (e.g., owner.meta.abc) is a meta leaf,
	// not an intermediate — stop validating intermediates at that point.
	for i := 1; i < len(f.Parts)-1; i++ {
		part := f.Parts[i].Value

		// meta subpath leaf: once we see "meta", everything after it is the JSON
		// subpath — stop validating intermediates. Handles owner.meta.a.b.c etc.
		if part == "meta" && i < len(f.Parts)-1 {
			return nil
		}

		if !traversalIntermediates[part] {
			// Check if it's a known traversal root but not valid as intermediate
			if _, anyRoot := traversalRoots[part]; anyRoot {
				return &ValidationError{
					Message: fmt.Sprintf("%q is not valid as intermediate in traversal chain %q; only parent/children can appear in the middle", part, f.Name()),
					Pos:     f.Parts[i].Pos,
					Length:  len(part),
				}
			}
			return &ValidationError{
				Message: fmt.Sprintf("%q is not a traversal field; cannot appear in traversal chain %q", part, f.Name()),
				Pos:     f.Parts[i].Pos,
				Length:  len(part),
			}
		}
	}

	// Validate the leaf (last part) — must be a valid group field
	leaf := f.Parts[len(f.Parts)-1].Value

	// Bare "meta" as leaf requires a key — suggest the correct syntax.
	if leaf == "meta" {
		chainPrefix := f.Parts[0].Value
		for i := 1; i < len(f.Parts)-1; i++ {
			chainPrefix += "." + f.Parts[i].Value
		}
		return &ValidationError{
			Message: fmt.Sprintf("%s.meta requires a key (e.g. %s.meta.mykey)", chainPrefix, chainPrefix),
			Pos:     f.Pos(),
			Length:  len(f.Name()),
		}
	}

	// Look up the leaf field on group entity (all traversals resolve to groups)
	subFd, ok := LookupField(EntityGroup, leaf)
	if !ok && !IsCommonField(leaf) {
		return &ValidationError{
			Message: fmt.Sprintf("unknown field %q for traversal; valid fields: name, description, tags, category, url, id, created, updated", leaf),
			Pos:     f.Parts[len(f.Parts)-1].Pos,
			Length:  len(leaf),
		}
	}

	// Resolve the field def if it came from common fields
	if !ok {
		subFd, _ = LookupField(EntityGroup, leaf)
	}

	// Only tags is supported as a relation leaf field for comparisons.
	// Parent/owner/children as leaves are handled by IS NULL validation separately.
	if subFd.Type == FieldRelation && leaf != "tags" {
		return &ValidationError{
			Message: fmt.Sprintf("%s is not supported; only scalar fields and tags are valid as traversal leaf fields", f.Name()),
			Pos:     f.Pos(),
			Length:  len(f.Name()),
		}
	}

	return nil
}

// validateRecursiveChain validates an ancestors.X / descendants.X field
// expression. Recursive roots are valid on all entity types and take exactly one
// group-field leaf: a scalar field, tags, or a meta subpath. Further chaining
// (ancestors.parent.name) is not supported.
func validateRecursiveChain(f *FieldExpr, entityType EntityType) error {
	root := f.Parts[0].Value

	// Recursive roots resolve against the group hierarchy and need a concrete
	// entity table (groups.id vs <table>.owner_id). Cross-entity mode, which
	// only permits common fields, cannot express them.
	if entityType == EntityUnspecified {
		return &ValidationError{
			Message: fmt.Sprintf("%s requires an explicit entity type (e.g. type = \"group\")", f.Name()),
			Pos:     f.Pos(),
			Length:  len(f.Name()),
		}
	}

	if len(f.Parts) < 2 {
		return &ValidationError{
			Message: fmt.Sprintf("%s requires a group field (e.g. %s.category, %s.tags, %s.meta.key)", root, root, root, root),
			Pos:     f.Pos(),
			Length:  len(f.Name()),
		}
	}

	// Meta subpath leaf: root.meta.key[.key...]
	if f.Parts[1].Value == "meta" {
		if len(f.Parts) < 3 {
			return &ValidationError{
				Message: fmt.Sprintf("%s.meta requires a key (e.g. %s.meta.mykey)", root, root),
				Pos:     f.Pos(),
				Length:  len(f.Name()),
			}
		}
		return nil // meta segments validated by the translator
	}

	// Otherwise exactly root.leaf — no intermediate chaining.
	if len(f.Parts) != 2 {
		return &ValidationError{
			Message: fmt.Sprintf("%s does not support multi-level chains; use %s.<group field> or %s.meta.<key>", root, root, root),
			Pos:     f.Pos(),
			Length:  len(f.Name()),
		}
	}

	leaf := f.Parts[1].Value
	// LookupField checks common fields first, so a miss covers those too.
	subFd, ok := LookupField(EntityGroup, leaf)
	if !ok {
		return &ValidationError{
			Message: fmt.Sprintf("unknown field %q for %s; valid fields: name, description, tags, category, url, id, created, updated, meta.<key>", leaf, root),
			Pos:     f.Parts[1].Pos,
			Length:  len(leaf),
		}
	}
	// Only tags is supported as a relation leaf; parent/children/resources/notes are not.
	if subFd.Type == FieldRelation && leaf != "tags" {
		return &ValidationError{
			Message: fmt.Sprintf("%s is not supported; only scalar fields, tags, and meta are valid as %s leaf fields", f.Name(), root),
			Pos:     f.Pos(),
			Length:  len(f.Name()),
		}
	}
	return nil
}

// validateGroupBy validates the GROUP BY clause: entity type required, field
// types, no traversals, aggregate field type constraints, ORDER BY interaction.
func validateGroupBy(gb *GroupByClause, entityType EntityType, orderBy []OrderByClause) error {
	if entityType == EntityUnspecified {
		pos := 0
		if len(gb.Fields) > 0 {
			pos = gb.Fields[0].Pos()
		}
		return &ValidationError{
			Message: "GROUP BY requires an explicit entity type (e.g. type = \"resource\")",
			Pos:     pos,
			Length:  0,
		}
	}

	// Validate each GROUP BY field
	// Validate, normalize, and deduplicate GROUP BY fields.
	// Aliases like "group" and "groups" resolve to the same column —
	// keep the first occurrence and silently drop duplicates.
	// All original names (including dropped aliases) remain valid for ORDER BY.
	seenColumns := make(map[string]bool)
	allFieldNames := make(map[string]bool) // all names seen, including aliases
	deduped := make([]*FieldExpr, 0, len(gb.Fields))
	for _, f := range gb.Fields {
		// Reject the "type" pseudo-field — it's a filter, not a real column.
		if len(f.Parts) == 1 && f.Parts[0].Value == "type" {
			return &ValidationError{
				Message: "cannot GROUP BY type: it is a filter pseudo-field, not a data column",
				Pos:     f.Pos(),
				Length:  len("type"),
			}
		}

		// Reject recursive traversal roots (ancestors/descendants) — they are
		// existential filter predicates, not groupable columns.
		if len(f.Parts) >= 2 && recursiveRoots[f.Parts[0].Value] {
			return &ValidationError{
				Message: fmt.Sprintf("cannot GROUP BY %s: ancestors/descendants are filter-only and not groupable", f.Name()),
				Pos:     f.Pos(),
				Length:  len(f.Name()),
			}
		}

		// Date bucket pseudo-fields (created.month etc.) are valid GROUP BY keys;
		// validateFieldExpr would reject them as WHERE-only.
		if !isDateBucketField(f, entityType) {
			// Validate field exists for entity type (handles traversals, meta, scalars)
			if err := validateFieldExpr(f, entityType); err != nil {
				return err
			}
		}

		allFieldNames[f.Name()] = true

		// Resolve to canonical column for deduplication
		resolvedCol := f.Name()
		if len(f.Parts) == 1 {
			if fd, ok := LookupField(entityType, f.Parts[0].Value); ok {
				resolvedCol = fd.Column
			}
		}
		if seenColumns[resolvedCol] {
			continue // silently drop duplicate
		}
		seenColumns[resolvedCol] = true
		deduped = append(deduped, f)
	}
	gb.Fields = deduped
	gb.AllFieldNames = allFieldNames

	// Validate aggregate functions
	for _, agg := range gb.Aggregates {
		if err := validateAggregateFunc(agg, entityType); err != nil {
			return err
		}
	}

	// Validate HAVING expression
	if gb.Having != nil {
		if len(gb.Aggregates) == 0 {
			return &ValidationError{
				Message: "HAVING requires at least one aggregate function in GROUP BY (e.g. GROUP BY hash COUNT() HAVING COUNT() > 1)",
				Pos:     gb.Having.Pos(),
				Length:  0,
			}
		}
		if err := validateHavingNode(gb.Having, entityType); err != nil {
			return err
		}
	}

	// Validate ORDER BY interaction in aggregated mode
	if len(gb.Aggregates) > 0 && len(orderBy) > 0 {
		validOrderKeys := buildAggregateOrderKeys(gb)
		for _, ob := range orderBy {
			obName := ob.Field.Name()
			if !validOrderKeys[obName] {
				return &ValidationError{
					Message: fmt.Sprintf("ORDER BY %q is not valid in aggregated GROUP BY; use a group-by field or aggregate key (e.g. count, sum_fileSize)", obName),
					Pos:     ob.Field.Pos(),
					Length:  len(obName),
				}
			}
		}
	}

	return nil
}

// validateAggregateFunc checks one aggregate function's field against the
// entity type. Shared between the GROUP BY aggregate list and HAVING leaves.
func validateAggregateFunc(agg AggregateFunc, entityType EntityType) error {
	if agg.Field == nil {
		return nil // COUNT() takes no field
	}

	// Validate the field exists
	if err := validateFieldExpr(agg.Field, entityType); err != nil {
		return err
	}

	fieldName := agg.Field.Name()
	fd, ok := LookupField(entityType, fieldName)
	if !ok {
		// Meta fields are always ok
		if !strings.HasPrefix(fieldName, "meta.") {
			return &ValidationError{
				Message: fmt.Sprintf("unknown field %q for aggregate %s", fieldName, agg.Name),
				Pos:     agg.Field.Pos(),
				Length:  len(fieldName),
			}
		}
		return nil
	}

	// SUM/AVG require numeric fields
	if agg.Name == "SUM" || agg.Name == "AVG" {
		if fd.Type != FieldNumber && fd.Type != FieldMeta {
			return &ValidationError{
				Message: fmt.Sprintf("%s requires a numeric field, but %q is %s", agg.Name, fieldName, fieldTypeName(fd.Type)),
				Pos:     agg.Field.Pos(),
				Length:  len(fieldName),
			}
		}
	}
	// MIN/MAX allow numeric and datetime
	if agg.Name == "MIN" || agg.Name == "MAX" {
		if fd.Type != FieldNumber && fd.Type != FieldDateTime && fd.Type != FieldMeta {
			return &ValidationError{
				Message: fmt.Sprintf("%s requires a numeric or datetime field, but %q is %s", agg.Name, fieldName, fieldTypeName(fd.Type)),
				Pos:     agg.Field.Pos(),
				Length:  len(fieldName),
			}
		}
	}
	return nil
}

// validateHavingNode recursively validates a HAVING expression tree.
func validateHavingNode(node Node, entityType EntityType) error {
	switch n := node.(type) {
	case *BinaryExpr:
		if err := validateHavingNode(n.Left, entityType); err != nil {
			return err
		}
		return validateHavingNode(n.Right, entityType)
	case *NotExpr:
		return validateHavingNode(n.Expr, entityType)
	case *HavingComparison:
		if err := validateAggregateFunc(n.Agg, entityType); err != nil {
			return err
		}
		return validateHavingValue(n, entityType)
	default:
		return &ValidationError{
			Message: fmt.Sprintf("unsupported expression %T in HAVING", node),
			Pos:     node.Pos(),
			Length:  0,
		}
	}
}

// validateHavingValue checks that the comparison value matches the aggregate:
// numeric for COUNT/SUM/AVG and MIN/MAX on numeric fields; date values
// (string, relative date, function) additionally allowed for MIN/MAX on
// datetime fields, and any value for MIN/MAX on dynamically-typed meta fields.
func validateHavingValue(hc *HavingComparison, entityType EntityType) error {
	// Unbound parameter: defer the numeric-value check to bind time.
	if _, ok := hc.Value.(*ParamRef); ok {
		return nil
	}

	isMinMax := hc.Agg.Name == "MIN" || hc.Agg.Name == "MAX"
	allowDateValue := false
	if isMinMax && hc.Agg.Field != nil {
		fieldName := hc.Agg.Field.Name()
		if strings.HasPrefix(fieldName, "meta.") {
			allowDateValue = true
		} else if fd, ok := LookupField(entityType, fieldName); ok && fd.Type == FieldDateTime {
			allowDateValue = true
		}
	}

	switch hc.Value.(type) {
	case *NumberLiteral:
		return nil
	case *StringLiteral, *RelDateLiteral, *FuncCall:
		if allowDateValue {
			return nil
		}
	}

	aggLabel := hc.Agg.Name + "()"
	if hc.Agg.Field != nil {
		aggLabel = hc.Agg.Name + "(" + hc.Agg.Field.Name() + ")"
	}
	return &ValidationError{
		Message: fmt.Sprintf("HAVING %s requires a numeric value", aggLabel),
		Pos:     hc.Value.Pos(),
		Length:  0,
	}
}

// buildAggregateOrderKeys returns the set of valid ORDER BY keys for an
// aggregated GROUP BY query: group field names + aggregate output keys.
func buildAggregateOrderKeys(gb *GroupByClause) map[string]bool {
	keys := make(map[string]bool)
	// Accept any original field name (including dropped aliases) for ORDER BY
	for name := range gb.AllFieldNames {
		keys[name] = true
	}
	// Also add deduplicated field names (in case AllFieldNames is not set)
	for _, f := range gb.Fields {
		keys[f.Name()] = true
	}
	for _, agg := range gb.Aggregates {
		if agg.Field == nil {
			keys["count"] = true
		} else {
			keys[strings.ToLower(agg.Name)+"_"+agg.Field.Name()] = true
		}
	}
	return keys
}

// groupByHasField returns true if the GROUP BY clause contains the field name.
func groupByHasField(gb *GroupByClause, name string) bool {
	for _, f := range gb.Fields {
		if f.Name() == name {
			return true
		}
	}
	return false
}

// fieldTypeName returns a human-readable name for a FieldType.
func fieldTypeName(ft FieldType) string {
	switch ft {
	case FieldString:
		return "a string field"
	case FieldNumber:
		return "a numeric field"
	case FieldDateTime:
		return "a datetime field"
	case FieldRelation:
		return "a relation field"
	case FieldMeta:
		return "a meta field"
	default:
		return "unknown"
	}
}
