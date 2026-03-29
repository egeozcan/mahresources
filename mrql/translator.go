package mrql

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
)

// TranslateError is returned when the translator encounters an unrecoverable issue.
type TranslateError struct {
	Message string
	Pos     int
}

func (e *TranslateError) Error() string {
	return fmt.Sprintf("translate error at position %d: %s", e.Pos, e.Message)
}

// TranslateOptions provides configurable options for query translation.
type TranslateOptions struct {
}

// Translate converts a validated MRQL Query AST into a GORM DB query.
// The entity type must be determinable (either set on the Query or extractable
// from a `type = "..."` comparison in the WHERE clause).
func Translate(q *Query, db *gorm.DB) (*gorm.DB, error) {
	return TranslateWithOptions(q, db, TranslateOptions{})
}

// TranslateWithOptions is like Translate but accepts configuration options.
func TranslateWithOptions(q *Query, db *gorm.DB, opts TranslateOptions) (*gorm.DB, error) {
	// Resolve entity type
	entityType := q.EntityType
	if entityType == EntityUnspecified {
		entityType = ExtractEntityType(q)
	}
	if entityType == EntityUnspecified {
		return nil, &TranslateError{
			Message: "entity type is required: use type = \"resource|note|group\" in the query or set Query.EntityType",
			Pos:     0,
		}
	}

	// Build the translator context
	tc := &translateContext{
		db:         db,
		entityType: entityType,
		tableName:  entityTableName(entityType),
	}

	// Start with the correct table
	result := tc.db.Table(tc.tableName)

	// Translate WHERE clause
	if q.Where != nil {
		var err error
		result, err = tc.translateNode(result, q.Where)
		if err != nil {
			return nil, err
		}
	}

	// Translate ORDER BY clauses
	for _, ob := range q.OrderBy {
		col := tc.resolveOrderByColumn(ob.Field)
		direction := "ASC"
		if !ob.Ascending {
			direction = "DESC"
		}
		result = result.Order(col + " " + direction)
	}

	// Apply LIMIT
	if q.Limit >= 0 {
		result = result.Limit(q.Limit)
	}

	// Apply OFFSET
	if q.Offset >= 0 {
		result = result.Offset(q.Offset)
	}

	return result, nil
}

// translateContext holds shared state during AST translation.
type translateContext struct {
	db         *gorm.DB
	entityType EntityType
	tableName  string
}

// isPostgres returns true if the underlying database is PostgreSQL.
func (tc *translateContext) isPostgres() bool {
	return tc.db.Config.Dialector.Name() == "postgres"
}

// entityTableName returns the database table name for an entity type.
func entityTableName(et EntityType) string {
	switch et {
	case EntityResource:
		return "resources"
	case EntityNote:
		return "notes"
	case EntityGroup:
		return "groups"
	default:
		return ""
	}
}

// translateNode recursively translates an AST node into GORM Where/Or clauses.
func (tc *translateContext) translateNode(db *gorm.DB, node Node) (*gorm.DB, error) {
	switch n := node.(type) {
	case *BinaryExpr:
		return tc.translateBinaryExpr(db, n)
	case *NotExpr:
		return tc.translateNotExpr(db, n)
	case *ComparisonExpr:
		return tc.translateComparisonExpr(db, n)
	case *InExpr:
		return tc.translateInExpr(db, n)
	case *IsExpr:
		return tc.translateIsExpr(db, n)
	case *TextSearchExpr:
		return tc.translateTextSearch(db, n)
	default:
		return nil, &TranslateError{
			Message: fmt.Sprintf("unsupported AST node type %T", node),
			Pos:     node.Pos(),
		}
	}
}

// translateBinaryExpr handles AND and OR expressions.
func (tc *translateContext) translateBinaryExpr(db *gorm.DB, expr *BinaryExpr) (*gorm.DB, error) {
	if expr.Operator.Type == TokenAnd {
		var err error
		db, err = tc.translateNode(db, expr.Left)
		if err != nil {
			return nil, err
		}
		return tc.translateNode(db, expr.Right)
	}

	// OR: build each branch using a fresh session with only the Where clauses
	// from that branch, then combine with GORM's Or to produce
	// "WHERE (left conditions) OR (right conditions)".
	leftDB := tc.db.Session(&gorm.Session{NewDB: true})
	leftDB, err := tc.translateNode(leftDB, expr.Left)
	if err != nil {
		return nil, err
	}

	rightDB := tc.db.Session(&gorm.Session{NewDB: true})
	rightDB, err = tc.translateNode(rightDB, expr.Right)
	if err != nil {
		return nil, err
	}

	// Combine branches: db.Where(left).Or(right) produces "(left) OR (right)"
	db = db.Where(leftDB).Or(rightDB)

	return db, nil
}

// translateNotExpr handles NOT expressions.
func (tc *translateContext) translateNotExpr(db *gorm.DB, expr *NotExpr) (*gorm.DB, error) {
	// Build the inner expression in a fresh session, then negate it with Not.
	innerDB := tc.db.Session(&gorm.Session{NewDB: true})
	innerDB, err := tc.translateNode(innerDB, expr.Expr)
	if err != nil {
		return nil, err
	}

	db = db.Not(innerDB)

	return db, nil
}

// translateComparisonExpr handles field op value comparisons.
func (tc *translateContext) translateComparisonExpr(db *gorm.DB, expr *ComparisonExpr) (*gorm.DB, error) {
	fieldName := expr.Field.Name()

	// Handle type comparisons. These must produce explicit TRUE/FALSE clauses
	// so they compose correctly inside OR and NOT expressions. Returning db
	// unchanged (no clause) would break boolean composition — GORM treats an
	// empty subquery differently from a "WHERE 1=1" subquery in OR/NOT contexts.
	if fieldName == "type" {
		if sl, ok := expr.Value.(*StringLiteral); ok {
			requestedType, valid := ValidEntityTypes[strings.ToLower(sl.Value)]
			if valid {
				matches := requestedType == tc.entityType
				if expr.Operator.Type == TokenNeq {
					matches = !matches
				}
				if matches {
					db = db.Where("1 = 1") // explicit TRUE — composes in OR/NOT
				} else {
					db = db.Where("1 = 0") // explicit FALSE
				}
			}
		}
		return db, nil
	}

	// Handle parent.X and children.X traversal (groups only)
	if len(expr.Field.Parts) == 2 {
		root := expr.Field.Parts[0].Value
		if root == "parent" || root == "children" {
			return tc.translateTraversalComparison(db, expr, root)
		}
	}

	fd, ok := LookupField(tc.entityType, fieldName)
	if !ok {
		// If the field is valid on another entity type, this is a cross-entity
		// field mismatch (e.g., contentType on notes). Inject 1=0 instead of
		// erroring, so OR branches with type guards work correctly.
		if isFieldOnAnyEntity(fieldName) {
			db = db.Where("1 = 0")
			return db, nil
		}
		return nil, &TranslateError{
			Message: fmt.Sprintf("unknown field %q for entity type %s", fieldName, tc.entityType),
			Pos:     expr.Pos(),
		}
	}

	// Resolve the comparison value to a Go value
	val, err := tc.resolveValue(expr.Value, fd)
	if err != nil {
		return nil, err
	}

	// Handle relation fields (tags, groups) with equality and LIKE operators
	if fd.Type == FieldRelation {
		return tc.translateRelationComparison(db, fd, expr.Operator, val)
	}

	// Handle meta fields
	if fd.Type == FieldMeta {
		return tc.translateMetaComparison(db, fd, expr.Operator, val)
	}

	// Regular scalar field comparison
	column := tc.qualifiedColumn(fd.Column)
	op := tc.sqlOperator(expr.Operator)

	if expr.Operator.Type == TokenLike || expr.Operator.Type == TokenNotLike {
		return tc.translateLikeComparison(db, column, expr.Operator, val)
	}

	// For string equality, use case-insensitive comparison
	if fd.Type == FieldString && (expr.Operator.Type == TokenEq || expr.Operator.Type == TokenNeq) {
		db = db.Where("LOWER("+column+") "+op+" LOWER(?)", val)
		return db, nil
	}

	db = db.Where(column+" "+op+" ?", val)
	return db, nil
}

// translateRelationComparison handles tags = "name" and groups = "name" etc.
func (tc *translateContext) translateRelationComparison(db *gorm.DB, fd FieldDef, op Token, val interface{}) (*gorm.DB, error) {
	switch fd.Column {
	case "tags":
		return tc.translateTagComparison(db, op, val)
	case "groups":
		return tc.translateGroupComparison(db, op, val)
	case "parent_id":
		// parent.X traversal is handled in translateComparisonExpr for dotted fields
		// Here it's direct parent comparison (parent = "name")
		return tc.translateParentComparison(db, op, val)
	case "children":
		return tc.translateChildrenComparison(db, op, val)
	default:
		return nil, &TranslateError{
			Message: fmt.Sprintf("unsupported relation field %q", fd.Name),
			Pos:     0,
		}
	}
}

// translateTagComparison generates a subquery on the tag junction table.
func (tc *translateContext) translateTagComparison(db *gorm.DB, op Token, val interface{}) (*gorm.DB, error) {
	var junctionTable, entityCol string
	switch tc.entityType {
	case EntityResource:
		junctionTable = "resource_tags"
		entityCol = "resource_id"
	case EntityNote:
		junctionTable = "note_tags"
		entityCol = "note_id"
	case EntityGroup:
		junctionTable = "group_tags"
		entityCol = "group_id"
	default:
		return nil, &TranslateError{Message: "tags not supported for this entity type"}
	}

	isLike := op.Type == TokenLike || op.Type == TokenNotLike
	isNegated := op.Type == TokenNeq || op.Type == TokenNotLike

	var tagMatchClause string
	var tagMatchVal interface{}

	if isLike {
		likePattern := convertMRQLWildcards(fmt.Sprint(val))
		likeOp := tc.likeOperator()
		tagMatchClause = "LOWER(t.name) " + likeOp + " LOWER(?) ESCAPE '\\'"
		tagMatchVal = likePattern
	} else {
		tagMatchClause = "LOWER(t.name) = LOWER(?)"
		tagMatchVal = val
	}

	subquery := fmt.Sprintf(
		"%s.id IN (SELECT jt.%s FROM %s jt JOIN tags t ON t.id = jt.tag_id WHERE %s)",
		tc.tableName, entityCol, junctionTable, tagMatchClause,
	)

	if isNegated {
		subquery = fmt.Sprintf(
			"%s.id NOT IN (SELECT jt.%s FROM %s jt JOIN tags t ON t.id = jt.tag_id WHERE %s)",
			tc.tableName, entityCol, junctionTable, tagMatchClause,
		)
	}

	db = db.Where(subquery, tagMatchVal)
	return db, nil
}

// translateGroupComparison generates a subquery on the group junction table.
func (tc *translateContext) translateGroupComparison(db *gorm.DB, op Token, val interface{}) (*gorm.DB, error) {
	var junctionTable, entityCol string
	switch tc.entityType {
	case EntityResource:
		junctionTable = "groups_related_resources"
		entityCol = "resource_id"
	case EntityNote:
		junctionTable = "groups_related_notes"
		entityCol = "note_id"
	default:
		return nil, &TranslateError{Message: "group filtering not supported for this entity type"}
	}

	isLike := op.Type == TokenLike || op.Type == TokenNotLike
	isNegated := op.Type == TokenNeq || op.Type == TokenNotLike

	var groupMatchClause string
	var groupMatchVal interface{}

	if isLike {
		likePattern := convertMRQLWildcards(fmt.Sprint(val))
		likeOp := tc.likeOperator()
		groupMatchClause = "LOWER(g.name) " + likeOp + " LOWER(?) ESCAPE '\\'"
		groupMatchVal = likePattern
	} else {
		groupMatchClause = "LOWER(g.name) = LOWER(?)"
		groupMatchVal = val
	}

	subquery := fmt.Sprintf(
		"%s.id IN (SELECT jt.%s FROM %s jt JOIN groups g ON g.id = jt.group_id WHERE %s)",
		tc.tableName, entityCol, junctionTable, groupMatchClause,
	)

	if isNegated {
		subquery = fmt.Sprintf(
			"%s.id NOT IN (SELECT jt.%s FROM %s jt JOIN groups g ON g.id = jt.group_id WHERE %s)",
			tc.tableName, entityCol, junctionTable, groupMatchClause,
		)
	}

	db = db.Where(subquery, groupMatchVal)
	return db, nil
}

// translateParentComparison handles parent = "name" for groups.
func (tc *translateContext) translateParentComparison(db *gorm.DB, op Token, val interface{}) (*gorm.DB, error) {
	isNegated := op.Type == TokenNeq || op.Type == TokenNotLike
	isLike := op.Type == TokenLike || op.Type == TokenNotLike

	var matchClause string
	var matchVal interface{}

	if isLike {
		likePattern := convertMRQLWildcards(fmt.Sprint(val))
		likeOp := tc.likeOperator()
		matchClause = "LOWER(p.name) " + likeOp + " LOWER(?) ESCAPE '\\'"
		matchVal = likePattern
	} else {
		matchClause = "LOWER(p.name) = LOWER(?)"
		matchVal = val
	}

	if isNegated {
		// NOT IN + include root groups (no parent)
		subquery := fmt.Sprintf(
			"(groups.owner_id NOT IN (SELECT p.id FROM groups p WHERE %s) OR groups.owner_id IS NULL)",
			matchClause,
		)
		db = db.Where(subquery, matchVal)
	} else {
		subquery := fmt.Sprintf(
			"groups.owner_id IN (SELECT p.id FROM groups p WHERE %s)",
			matchClause,
		)
		db = db.Where(subquery, matchVal)
	}

	return db, nil
}

// translateChildrenComparison handles children = "name" for groups.
func (tc *translateContext) translateChildrenComparison(db *gorm.DB, op Token, val interface{}) (*gorm.DB, error) {
	isNegated := op.Type == TokenNeq || op.Type == TokenNotLike
	isLike := op.Type == TokenLike || op.Type == TokenNotLike

	var matchClause string
	var matchVal interface{}

	if isLike {
		likePattern := convertMRQLWildcards(fmt.Sprint(val))
		likeOp := tc.likeOperator()
		matchClause = "LOWER(c.name) " + likeOp + " LOWER(?) ESCAPE '\\'"
		matchVal = likePattern
	} else {
		matchClause = "LOWER(c.name) = LOWER(?)"
		matchVal = val
	}

	subquery := fmt.Sprintf(
		"groups.id IN (SELECT c.owner_id FROM groups c WHERE %s)",
		matchClause,
	)

	if isNegated {
		subquery = fmt.Sprintf(
			"groups.id NOT IN (SELECT c.owner_id FROM groups c WHERE %s)",
			matchClause,
		)
	}

	db = db.Where(subquery, matchVal)
	return db, nil
}

// translateTraversalComparison handles parent.field and children.field for groups.
// It generates subqueries that join through the owner_id relationship.
func (tc *translateContext) translateTraversalComparison(db *gorm.DB, expr *ComparisonExpr, root string) (*gorm.DB, error) {
	subField := expr.Field.Parts[1].Value

	// Resolve the value
	// Look up the sub-field on the group entity (since parent/children are always groups)
	subFd, ok := LookupField(EntityGroup, subField)
	if !ok && !IsCommonField(subField) {
		// Check if it's a meta field
		if subField != "meta" {
			return nil, &TranslateError{
				Message: fmt.Sprintf("unknown field %q for group traversal", subField),
				Pos:     expr.Field.Parts[1].Pos,
			}
		}
	}
	if IsCommonField(subField) {
		subFd, _ = LookupField(EntityGroup, subField)
	}

	val, err := tc.resolveValue(expr.Value, subFd)
	if err != nil {
		return nil, err
	}

	// Handle meta sub-fields: parent.meta.key
	if subField == "meta" {
		// This would be a 3-part field which is rejected by the parser
		return nil, &TranslateError{Message: "parent.meta requires a key (e.g., parent.meta.key) — use the full dotted path", Pos: expr.Pos()}
	}

	// Handle relation sub-fields: parent.tags, children.tags
	if subFd.Type == FieldRelation && subFd.Column == "tags" {
		return tc.translateTraversalTagComparison(db, expr.Operator, val, root)
	}

	// Scalar sub-field on parent/children
	col := subFd.Column
	op := tc.sqlOperator(expr.Operator)
	isNegated := expr.Operator.Type == TokenNeq || expr.Operator.Type == TokenNotLike

	if root == "parent" {
		// For negated operators, include groups with no parent (owner_id IS NULL)
		nullClause := ""
		if isNegated {
			nullClause = " OR groups.owner_id IS NULL"
		}

		if expr.Operator.Type == TokenLike || expr.Operator.Type == TokenNotLike {
			likePattern := convertMRQLWildcards(fmt.Sprint(val))
			likeOp := tc.likeOperator()
			if expr.Operator.Type == TokenNotLike {
				likeOp = "NOT " + likeOp
			}
			db = db.Where(
				fmt.Sprintf("(groups.owner_id IN (SELECT p.id FROM groups p WHERE p.%s %s ? ESCAPE '\\')%s)", col, likeOp, nullClause),
				likePattern,
			)
		} else if subFd.Type == FieldString && (expr.Operator.Type == TokenEq || expr.Operator.Type == TokenNeq) {
			db = db.Where(
				fmt.Sprintf("(groups.owner_id IN (SELECT p.id FROM groups p WHERE LOWER(p.%s) %s LOWER(?))%s)", col, op, nullClause),
				val,
			)
		} else {
			db = db.Where(
				fmt.Sprintf("(groups.owner_id IN (SELECT p.id FROM groups p WHERE p.%s %s ?)%s)", col, op, nullClause),
				val,
			)
		}
	} else {
		// children traversal
		// For negated operators, use NOT EXISTS semantics: "has no child matching X"
		// rather than "has some child not matching X" (which would incorrectly include
		// mixed-child groups). Also include leaf groups (no children at all).
		if isNegated {
			leafClause := " OR groups.id NOT IN (SELECT owner_id FROM groups WHERE owner_id IS NOT NULL)"
			if expr.Operator.Type == TokenNotLike {
				likePattern := convertMRQLWildcards(fmt.Sprint(val))
				likeOp := tc.likeOperator()
				// NOT LIKE → "no child matches pattern" → NOT IN (children matching pattern)
				db = db.Where(
					fmt.Sprintf("(groups.id NOT IN (SELECT c.owner_id FROM groups c WHERE c.%s %s ? ESCAPE '\\')%s)", col, likeOp, leafClause),
					likePattern,
				)
			} else if subFd.Type == FieldString && expr.Operator.Type == TokenNeq {
				// != → "no child equals X" → NOT IN (children equaling X)
				db = db.Where(
					fmt.Sprintf("(groups.id NOT IN (SELECT c.owner_id FROM groups c WHERE LOWER(c.%s) = LOWER(?))%s)", col, leafClause),
					val,
				)
			} else {
				// Numeric != → NOT IN (children matching value)
				db = db.Where(
					fmt.Sprintf("(groups.id NOT IN (SELECT c.owner_id FROM groups c WHERE c.%s = ?)%s)", col, leafClause),
					val,
				)
			}
		} else {
			// Positive operators: "has some child matching X"
			if expr.Operator.Type == TokenLike {
				likePattern := convertMRQLWildcards(fmt.Sprint(val))
				likeOp := tc.likeOperator()
				db = db.Where(
					fmt.Sprintf("groups.id IN (SELECT c.owner_id FROM groups c WHERE c.%s %s ? ESCAPE '\\')", col, likeOp),
					likePattern,
				)
			} else if subFd.Type == FieldString && expr.Operator.Type == TokenEq {
				db = db.Where(
					fmt.Sprintf("groups.id IN (SELECT c.owner_id FROM groups c WHERE LOWER(c.%s) = LOWER(?))", col),
					val,
				)
			} else {
				db = db.Where(
					fmt.Sprintf("groups.id IN (SELECT c.owner_id FROM groups c WHERE c.%s %s ?)", col, op),
					val,
				)
			}
		}
	}

	return db, nil
}

// translateTraversalTagComparison handles parent.tags = "X" and children.tags = "X".
func (tc *translateContext) translateTraversalTagComparison(db *gorm.DB, op Token, val interface{}, root string) (*gorm.DB, error) {
	isNegated := op.Type == TokenNeq || op.Type == TokenNotLike
	isLike := op.Type == TokenLike || op.Type == TokenNotLike

	var tagMatchClause string
	var tagMatchVal interface{}

	if isLike {
		likePattern := convertMRQLWildcards(fmt.Sprint(val))
		likeOp := tc.likeOperator()
		tagMatchClause = "LOWER(t.name) " + likeOp + " LOWER(?) ESCAPE '\\'"
		tagMatchVal = likePattern
	} else {
		tagMatchClause = "LOWER(t.name) = LOWER(?)"
		tagMatchVal = val
	}

	inOrNotIn := "IN"
	if isNegated {
		inOrNotIn = "NOT IN"
	}

	if root == "parent" {
		// Find groups whose parent has matching tags
		subquery := fmt.Sprintf(
			"groups.owner_id %s (SELECT gt.group_id FROM group_tags gt JOIN tags t ON t.id = gt.tag_id WHERE %s)",
			inOrNotIn, tagMatchClause,
		)
		if isNegated {
			// For negated operators, also include groups with no parent
			db = db.Where("("+subquery+" OR groups.owner_id IS NULL)", tagMatchVal)
		} else {
			db = db.Where(subquery, tagMatchVal)
		}
	} else {
		// Find groups that have children with matching tags
		subquery := fmt.Sprintf(
			"groups.id %s (SELECT c.owner_id FROM groups c JOIN group_tags gt ON gt.group_id = c.id JOIN tags t ON t.id = gt.tag_id WHERE c.owner_id IS NOT NULL AND %s)",
			inOrNotIn, tagMatchClause,
		)
		db = db.Where(subquery, tagMatchVal)
	}

	return db, nil
}

// translateTraversalIsNull handles parent.X IS [NOT] NULL and children.X IS [NOT] NULL.
func (tc *translateContext) translateTraversalIsNull(db *gorm.DB, expr *IsExpr, root string) (*gorm.DB, error) {
	subField := expr.Field.Parts[1].Value

	// Look up the sub-field on the group entity
	subFd, ok := LookupField(EntityGroup, subField)
	if !ok && !IsCommonField(subField) {
		return nil, &TranslateError{
			Message: fmt.Sprintf("unknown field %q for %s traversal", subField, root),
			Pos:     expr.Field.Parts[1].Pos,
		}
	}
	if IsCommonField(subField) {
		subFd, _ = LookupField(EntityGroup, subField)
	}

	col := subFd.Column

	if root == "parent" {
		// parent.X IS NULL → parent exists but parent.X is null, OR no parent at all
		// parent.X IS NOT NULL → parent exists and parent.X is not null
		if expr.Negated {
			db = db.Where(
				fmt.Sprintf("groups.owner_id IN (SELECT p.id FROM groups p WHERE p.%s IS NOT NULL)", col),
			)
		} else {
			db = db.Where(
				fmt.Sprintf("(groups.owner_id IN (SELECT p.id FROM groups p WHERE p.%s IS NULL) OR groups.owner_id IS NULL)", col),
			)
		}
	} else {
		// children.X IS NULL → has some child where X is null (or no children)
		// children.X IS NOT NULL → has some child where X is not null
		if expr.Negated {
			db = db.Where(
				fmt.Sprintf("groups.id IN (SELECT c.owner_id FROM groups c WHERE c.%s IS NOT NULL AND c.owner_id IS NOT NULL)", col),
			)
		} else {
			db = db.Where(
				fmt.Sprintf("(groups.id IN (SELECT c.owner_id FROM groups c WHERE c.%s IS NULL AND c.owner_id IS NOT NULL) OR groups.id NOT IN (SELECT owner_id FROM groups WHERE owner_id IS NOT NULL))", col),
			)
		}
	}

	return db, nil
}

// metaKeyPattern matches valid meta keys: only alphanumeric and underscores.
var metaKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// isValidMetaKey returns true if the key contains only safe characters for
// interpolation into json_extract paths.
func isValidMetaKey(key string) bool {
	return metaKeyPattern.MatchString(key)
}

// isNumericValue returns true if the value is a numeric Go type.
func isNumericValue(val interface{}) bool {
	switch val.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return true
	}
	return false
}

// translateMetaComparison handles meta.key comparisons using json_extract.
func (tc *translateContext) translateMetaComparison(db *gorm.DB, fd FieldDef, op Token, val interface{}) (*gorm.DB, error) {
	// Extract the key from "meta.key"
	key := strings.TrimPrefix(fd.Name, "meta.")

	if !isValidMetaKey(key) {
		return nil, &TranslateError{
			Message: fmt.Sprintf("invalid meta key %q: must contain only alphanumeric characters and underscores", key),
			Pos:     0,
		}
	}

	// Build the JSON extraction expression per database dialect.
	// On Postgres, ->>' returns text, so numeric comparisons/sorts need a cast.
	isNumericVal := isNumericValue(val)
	var jsonExpr string
	if tc.isPostgres() {
		if isNumericVal {
			// Cast to numeric for proper ordering and comparison
			jsonExpr = fmt.Sprintf("(%s.meta->>'%s')::numeric", tc.tableName, key)
		} else {
			jsonExpr = fmt.Sprintf("%s.meta->>'%s'", tc.tableName, key)
		}
	} else {
		// SQLite: json_extract returns the native JSON type (number, string, etc.)
		jsonExpr = fmt.Sprintf("json_extract(%s.meta, '$.%s')", tc.tableName, key)
	}

	sqlOp := tc.sqlOperator(op)

	if op.Type == TokenLike || op.Type == TokenNotLike {
		// LIKE always operates on text — use the text expression on Postgres
		textExpr := jsonExpr
		if tc.isPostgres() && isNumericVal {
			textExpr = fmt.Sprintf("%s.meta->>'%s'", tc.tableName, key)
		}
		likePattern := convertMRQLWildcards(fmt.Sprint(val))
		likeOp := tc.likeOperator()
		if op.Type == TokenNotLike {
			likeOp = "NOT " + likeOp
		}
		db = db.Where(textExpr+" "+likeOp+" ? ESCAPE '\\'", likePattern)
		return db, nil
	}

	// For string equality/inequality on meta fields, use case-insensitive comparison
	// to match the language's general case-insensitive rule.
	if !isNumericVal && (op.Type == TokenEq || op.Type == TokenNeq) {
		db = db.Where("LOWER("+jsonExpr+") "+sqlOp+" LOWER(?)", val)
		return db, nil
	}

	db = db.Where(jsonExpr+" "+sqlOp+" ?", val)
	return db, nil
}

// translateInExpr handles field IN (...) and field NOT IN (...).
func (tc *translateContext) translateInExpr(db *gorm.DB, expr *InExpr) (*gorm.DB, error) {
	fieldName := expr.Field.Name()
	fd, ok := LookupField(tc.entityType, fieldName)
	if !ok {
		if isFieldOnAnyEntity(fieldName) {
			db = db.Where("1 = 0")
			return db, nil
		}
		return nil, &TranslateError{
			Message: fmt.Sprintf("unknown field %q for entity type %s", fieldName, tc.entityType),
			Pos:     expr.Pos(),
		}
	}

	values := make([]interface{}, len(expr.Values))
	for i, v := range expr.Values {
		resolved, err := tc.resolveValue(v, fd)
		if err != nil {
			return nil, err
		}
		values[i] = resolved
	}

	// Handle relation fields (tags, groups) with IN using subqueries,
	// similar to translateRelationComparison but matching multiple values.
	if fd.Type == FieldRelation {
		return tc.translateRelationIn(db, fd, expr.Negated, values)
	}

	// Handle meta fields — need json_extract, not qualifiedColumn
	if fd.Type == FieldMeta {
		key := strings.TrimPrefix(fd.Name, "meta.")
		if !isValidMetaKey(key) {
			return nil, &TranslateError{Message: fmt.Sprintf("invalid meta key %q", key)}
		}
		var jsonExpr string
		if tc.isPostgres() {
			jsonExpr = fmt.Sprintf("%s.meta->>'%s'", tc.tableName, key)
		} else {
			jsonExpr = fmt.Sprintf("json_extract(%s.meta, '$.%s')", tc.tableName, key)
		}
		if expr.Negated {
			db = db.Where(jsonExpr+" NOT IN (?)", values)
		} else {
			db = db.Where(jsonExpr+" IN (?)", values)
		}
		return db, nil
	}

	column := tc.qualifiedColumn(fd.Column)

	if fd.Type == FieldString {
		// Case-insensitive IN for strings
		lowerValues := make([]interface{}, len(values))
		for i, v := range values {
			lowerValues[i] = strings.ToLower(fmt.Sprint(v))
		}
		if expr.Negated {
			db = db.Where("LOWER("+column+") NOT IN (?)", lowerValues)
		} else {
			db = db.Where("LOWER("+column+") IN (?)", lowerValues)
		}
	} else {
		if expr.Negated {
			db = db.Where(column+" NOT IN (?)", values)
		} else {
			db = db.Where(column+" IN (?)", values)
		}
	}

	return db, nil
}

// translateRelationIn handles field IN (...) for relation fields (tags, groups).
func (tc *translateContext) translateRelationIn(db *gorm.DB, fd FieldDef, negated bool, values []interface{}) (*gorm.DB, error) {
	// Lowercase all values for case-insensitive matching
	lowerValues := make([]interface{}, len(values))
	for i, v := range values {
		lowerValues[i] = strings.ToLower(fmt.Sprint(v))
	}

	inOrNotIn := "IN"
	if negated {
		inOrNotIn = "NOT IN"
	}

	switch fd.Column {
	case "tags":
		var junctionTable, entityCol string
		switch tc.entityType {
		case EntityResource:
			junctionTable = "resource_tags"
			entityCol = "resource_id"
		case EntityNote:
			junctionTable = "note_tags"
			entityCol = "note_id"
		case EntityGroup:
			junctionTable = "group_tags"
			entityCol = "group_id"
		default:
			return nil, &TranslateError{Message: "tags IN not supported for this entity type"}
		}

		subquery := fmt.Sprintf(
			"%s.id %s (SELECT jt.%s FROM %s jt JOIN tags t ON t.id = jt.tag_id WHERE LOWER(t.name) IN (?))",
			tc.tableName, inOrNotIn, entityCol, junctionTable,
		)
		db = db.Where(subquery, lowerValues)
		return db, nil

	case "groups":
		var junctionTable, entityCol string
		switch tc.entityType {
		case EntityResource:
			junctionTable = "groups_related_resources"
			entityCol = "resource_id"
		case EntityNote:
			junctionTable = "groups_related_notes"
			entityCol = "note_id"
		default:
			return nil, &TranslateError{Message: "groups IN not supported for this entity type"}
		}

		subquery := fmt.Sprintf(
			"%s.id %s (SELECT jt.%s FROM %s jt JOIN groups g ON g.id = jt.group_id WHERE LOWER(g.name) IN (?))",
			tc.tableName, inOrNotIn, entityCol, junctionTable,
		)
		db = db.Where(subquery, lowerValues)
		return db, nil

	default:
		return nil, &TranslateError{
			Message: fmt.Sprintf("IN not supported for relation field %q", fd.Name),
		}
	}
}

// translateIsExpr handles IS EMPTY, IS NOT EMPTY, IS NULL, IS NOT NULL.
func (tc *translateContext) translateIsExpr(db *gorm.DB, expr *IsExpr) (*gorm.DB, error) {
	fieldName := expr.Field.Name()

	// Handle parent.X / children.X IS NULL / IS NOT NULL via traversal subquery
	if len(expr.Field.Parts) == 2 && expr.IsNull {
		root := expr.Field.Parts[0].Value
		if root == "parent" || root == "children" {
			return tc.translateTraversalIsNull(db, expr, root)
		}
	}

	fd, ok := LookupField(tc.entityType, fieldName)
	if !ok {
		if isFieldOnAnyEntity(fieldName) {
			db = db.Where("1 = 0")
			return db, nil
		}
		return nil, &TranslateError{
			Message: fmt.Sprintf("unknown field %q for entity type %s", fieldName, tc.entityType),
			Pos:     expr.Pos(),
		}
	}

	if expr.IsNull {
		// For relation fields, IS NULL is equivalent to IS EMPTY — redirect
		if fd.Type == FieldRelation {
			return tc.translateRelationIsEmpty(db, fd, expr.Negated)
		}

		// IS NULL / IS NOT NULL for scalar and meta fields
		var column string
		if fd.Type == FieldMeta {
			key := strings.TrimPrefix(fd.Name, "meta.")
			if tc.isPostgres() {
				column = fmt.Sprintf("%s.meta->>'%s'", tc.tableName, key)
			} else {
				column = fmt.Sprintf("json_extract(%s.meta, '$.%s')", tc.tableName, key)
			}
		} else {
			column = tc.qualifiedColumn(fd.Column)
		}
		if expr.Negated {
			db = db.Where(column + " IS NOT NULL")
		} else {
			db = db.Where(column + " IS NULL")
		}
		return db, nil
	}

	// IS EMPTY / IS NOT EMPTY — for relation fields, check junction table
	if fd.Type == FieldRelation {
		return tc.translateRelationIsEmpty(db, fd, expr.Negated)
	}

	// IS EMPTY / IS NOT EMPTY for scalar fields: treat as NULL/empty check.
	// Meta fields need json_extract, not a direct column reference.
	var column string
	if fd.Type == FieldMeta {
		key := strings.TrimPrefix(fd.Name, "meta.")
		if tc.isPostgres() {
			column = fmt.Sprintf("%s.meta->>'%s'", tc.tableName, key)
		} else {
			column = fmt.Sprintf("json_extract(%s.meta, '$.%s')", tc.tableName, key)
		}
	} else {
		column = tc.qualifiedColumn(fd.Column)
	}
	if expr.Negated {
		db = db.Where(column + " IS NOT NULL AND " + column + " != ''")
	} else {
		db = db.Where(column + " IS NULL OR " + column + " = ''")
	}

	return db, nil
}

// translateRelationIsEmpty handles IS EMPTY / IS NOT EMPTY for relation fields.
func (tc *translateContext) translateRelationIsEmpty(db *gorm.DB, fd FieldDef, negated bool) (*gorm.DB, error) {
	existsOp := "NOT EXISTS"
	if negated {
		existsOp = "EXISTS"
	}

	switch fd.Column {
	case "tags":
		var junctionTable, entityCol string
		switch tc.entityType {
		case EntityResource:
			junctionTable = "resource_tags"
			entityCol = "resource_id"
		case EntityNote:
			junctionTable = "note_tags"
			entityCol = "note_id"
		case EntityGroup:
			junctionTable = "group_tags"
			entityCol = "group_id"
		}
		subquery := fmt.Sprintf(
			"%s (SELECT 1 FROM %s jt WHERE jt.%s = %s.id)",
			existsOp, junctionTable, entityCol, tc.tableName,
		)
		db = db.Where(subquery)

	case "groups":
		var junctionTable, entityCol string
		switch tc.entityType {
		case EntityResource:
			junctionTable = "groups_related_resources"
			entityCol = "resource_id"
		case EntityNote:
			junctionTable = "groups_related_notes"
			entityCol = "note_id"
		}
		subquery := fmt.Sprintf(
			"%s (SELECT 1 FROM %s jt WHERE jt.%s = %s.id)",
			existsOp, junctionTable, entityCol, tc.tableName,
		)
		db = db.Where(subquery)

	case "parent_id":
		// parent IS EMPTY → owner_id IS NULL
		if negated {
			db = db.Where(tc.tableName + ".owner_id IS NOT NULL")
		} else {
			db = db.Where(tc.tableName + ".owner_id IS NULL")
		}

	case "children":
		// children IS EMPTY → no rows in groups where owner_id = this group's id
		subquery := fmt.Sprintf(
			"%s (SELECT 1 FROM groups c WHERE c.owner_id = %s.id)",
			existsOp, tc.tableName,
		)
		db = db.Where(subquery)
	}

	return db, nil
}

// translateTextSearch handles TEXT ~ "query" for full-text search.
func (tc *translateContext) translateTextSearch(db *gorm.DB, expr *TextSearchExpr) (*gorm.DB, error) {
	searchTerm := strings.TrimSpace(expr.Value.Value)
	if searchTerm == "" {
		return db, nil
	}

	if tc.isPostgres() {
		// PostgreSQL: use the search_vector column with plainto_tsquery
		subquery := fmt.Sprintf(
			"%s.search_vector @@ plainto_tsquery('english', ?)",
			tc.tableName,
		)
		db = db.Where(subquery, searchTerm)
	} else {
		// SQLite: try FTS5 MATCH, fall back to LIKE if FTS tables don't exist
		sanitized := sanitizeFTS5(searchTerm)
		if sanitized == "" {
			return db, nil
		}
		ftsTable := tc.tableName + "_fts"

		// Check if the FTS table exists (handles -skip-fts and FTS init failures)
		var tableExists int
		tc.db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", ftsTable).Scan(&tableExists)

		if tableExists > 0 {
			subquery := fmt.Sprintf(
				"%s.id IN (SELECT rowid FROM %s WHERE %s MATCH ?)",
				tc.tableName, ftsTable, ftsTable,
			)
			db = db.Where(subquery, sanitized)
		} else {
			// Fallback: LIKE search on name and description
			likePattern := "%" + searchTerm + "%"
			db = db.Where(
				fmt.Sprintf("(%s.name LIKE ? ESCAPE '\\' OR %s.description LIKE ? ESCAPE '\\')",
					tc.tableName, tc.tableName),
				likePattern, likePattern,
			)
		}
	}

	return db, nil
}

// resolveValue converts an AST value node to a Go value suitable for SQL parameters.
func (tc *translateContext) resolveValue(node Node, fd FieldDef) (interface{}, error) {
	switch v := node.(type) {
	case *StringLiteral:
		return v.Value, nil
	case *NumberLiteral:
		// Use Raw for file size fields (already converted to bytes)
		if fd.Column == "file_size" && v.Unit != "" {
			return v.Raw, nil
		}
		// Return as float64 for general numeric comparisons
		if v.Value == float64(int64(v.Value)) {
			return int64(v.Value), nil
		}
		return v.Value, nil
	case *RelDateLiteral:
		return resolveRelativeDate(v), nil
	case *FuncCall:
		return resolveFunction(v)
	default:
		return nil, &TranslateError{
			Message: fmt.Sprintf("unsupported value type %T", node),
			Pos:     node.Pos(),
		}
	}
}

// resolveRelativeDate converts a relative date literal to a time.Time.
func resolveRelativeDate(rd *RelDateLiteral) time.Time {
	now := time.Now()
	switch rd.Unit {
	case "d":
		return now.AddDate(0, 0, -rd.Amount)
	case "w":
		return now.AddDate(0, 0, -rd.Amount*7)
	case "m":
		return now.AddDate(0, -rd.Amount, 0)
	case "y":
		return now.AddDate(-rd.Amount, 0, 0)
	default:
		return now
	}
}

// resolveFunction resolves a function call (NOW, START_OF_DAY, etc.) to a time.Time.
func resolveFunction(fc *FuncCall) (time.Time, error) {
	now := time.Now()
	// The name from the lexer includes "()" (e.g. "NOW()"), strip it
	name := strings.ToUpper(strings.TrimSuffix(fc.Name, "()"))

	switch name {
	case "NOW":
		return now, nil
	case "START_OF_DAY":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), nil
	case "START_OF_WEEK":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		monday := now.AddDate(0, 0, -(weekday - 1))
		return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, now.Location()), nil
	case "START_OF_MONTH":
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()), nil
	case "START_OF_YEAR":
		return time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location()), nil
	default:
		return time.Time{}, &TranslateError{
			Message: fmt.Sprintf("unknown function %q", fc.Name),
		}
	}
}

// sqlOperator maps a token operator to its SQL equivalent.
func (tc *translateContext) sqlOperator(op Token) string {
	switch op.Type {
	case TokenEq:
		return "="
	case TokenNeq:
		return "!="
	case TokenGt:
		return ">"
	case TokenGte:
		return ">="
	case TokenLt:
		return "<"
	case TokenLte:
		return "<="
	case TokenLike:
		return tc.likeOperator()
	case TokenNotLike:
		return "NOT " + tc.likeOperator()
	default:
		return "="
	}
}

// likeOperator returns "ILIKE" for Postgres, "LIKE" for everything else.
func (tc *translateContext) likeOperator() string {
	if tc.db.Config.Dialector.Name() == "postgres" {
		return "ILIKE"
	}
	return "LIKE"
}

// qualifiedColumn returns the fully qualified column name (table.column).
func (tc *translateContext) qualifiedColumn(column string) string {
	return tc.tableName + "." + column
}

// resolveOrderByColumn converts a FieldExpr to a qualified column name for ORDER BY.
func (tc *translateContext) resolveOrderByColumn(f *FieldExpr) string {
	fieldName := f.Name()

	// Handle meta.key → JSON extraction per dialect.
	// ORDER BY cannot know the runtime type of meta values, so we use
	// text ordering on Postgres (meta->>'key') and native JSON type on
	// SQLite (json_extract preserves numbers). This matches the existing
	// codebase's meta sort behavior in database_scopes/db_utils.go.
	if strings.HasPrefix(fieldName, "meta.") {
		key := strings.TrimPrefix(fieldName, "meta.")
		if !isValidMetaKey(key) {
			return tc.tableName + ".meta"
		}
		if tc.isPostgres() {
			return fmt.Sprintf("%s.meta->>'%s'", tc.tableName, key)
		}
		return fmt.Sprintf("json_extract(%s.meta, '$.%s')", tc.tableName, key)
	}

	fd, ok := LookupField(tc.entityType, fieldName)
	if !ok {
		// Fallback: use the field name as-is (validation should have caught this)
		return tc.tableName + "." + fieldName
	}

	return tc.qualifiedColumn(fd.Column)
}

// translateLikeComparison handles ~ and !~ operators with MRQL wildcard conversion.
func (tc *translateContext) translateLikeComparison(db *gorm.DB, column string, op Token, val interface{}) (*gorm.DB, error) {
	pattern := convertMRQLWildcards(fmt.Sprint(val))
	likeOp := tc.likeOperator()
	escapeClause := " ESCAPE '\\'"

	if op.Type == TokenNotLike {
		db = db.Where("LOWER("+column+") NOT "+likeOp+" LOWER(?)"+escapeClause, pattern)
	} else {
		db = db.Where("LOWER("+column+") "+likeOp+" LOWER(?)"+escapeClause, pattern)
	}

	return db, nil
}

// convertMRQLWildcards converts MRQL wildcards (* → %, ? → _) after escaping
// existing SQL wildcards in the value.
func convertMRQLWildcards(s string) string {
	// First escape existing SQL wildcards
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	// Then convert MRQL wildcards to SQL wildcards
	s = strings.ReplaceAll(s, "*", "%")
	s = strings.ReplaceAll(s, "?", "_")
	return s
}

// sanitizeFTS5 strips FTS5 operators and special characters from a search string,
// leaving only safe tokens.
func sanitizeFTS5(input string) string {
	// Remove FTS5 special operators: AND, OR, NOT, NEAR, +, -, *, ^, :, (, ), "
	// Keep only alphanumeric characters, spaces, and basic punctuation
	var sb strings.Builder
	for _, r := range input {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r == ' ' || r == '\t':
			sb.WriteRune(' ')
		case r == '.' || r == ',':
			sb.WriteRune(r)
		// Skip: *, +, ^, :, (, ), ", !, ~, etc.
		}
	}
	return strings.TrimSpace(sb.String())
}

