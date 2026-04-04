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

// fkStep describes one level of a foreign-key traversal (owner, parent, or children).
type fkStep struct {
	fkExpr    string // source FK expression, e.g. "resources.owner_id" or "_t0.id"
	selectCol string // what to SELECT: "_t0.id" (forward) or "_t0.owner_id" (reverse)
	alias     string // subquery table alias
}

// traversalFieldNames are fields that can start or continue an FK traversal chain.
var traversalFieldNames = map[string]bool{
	"owner": true, "parent": true, "children": true,
}

// isPostgres returns true if the underlying database is PostgreSQL.
func (tc *translateContext) isPostgres() bool {
	return tc.db.Config.Dialector.Name() == "postgres"
}

// buildFKStep creates a single traversal step for the given field name.
// For "children" (reverse FK), the SELECT column is the child's owner_id so the
// outer query matches on the parent's id. For "owner" and "parent" (forward FK),
// the SELECT column is the group's id so the outer query matches on the FK column.
func buildFKStep(fieldName string, outerRef string, idx int) fkStep {
	alias := fmt.Sprintf("_t%d", idx)
	if fieldName == "children" {
		return fkStep{fkExpr: outerRef, selectCol: alias + ".owner_id", alias: alias}
	}
	// owner and parent: forward FK lookup — SELECT id
	return fkStep{fkExpr: outerRef, selectCol: alias + ".id", alias: alias}
}

// buildTraversalChain converts the traversal portion of a FieldExpr (all parts
// except the leaf) into a slice of fkStep values describing nested subqueries.
func (tc *translateContext) buildTraversalChain(parts []Token) []fkStep {
	var steps []fkStep
	for i := 0; i < len(parts)-1; i++ {
		fieldName := parts[i].Value
		var outerRef string
		if i == 0 {
			// First step: reference from the entity table
			if fieldName == "children" {
				outerRef = tc.tableName + ".id"
			} else {
				outerRef = tc.tableName + ".owner_id"
			}
		} else {
			// Subsequent steps: reference from the previous subquery alias
			prevAlias := steps[i-1].alias
			if fieldName == "children" {
				outerRef = prevAlias + ".id"
			} else {
				outerRef = prevAlias + ".owner_id"
			}
		}
		steps = append(steps, buildFKStep(fieldName, outerRef, i))
	}
	return steps
}

// wrapChainSubqueries wraps the innermost WHERE clause in nested IN subqueries,
// building from the inside out. Each step becomes:
//
//	fkExpr IN (SELECT selectCol FROM groups alias WHERE ...)
func (tc *translateContext) wrapChainSubqueries(steps []fkStep, innerWhere string, innerVals []interface{}) (string, []interface{}) {
	currentWhere := innerWhere
	currentVals := innerVals
	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		childFilter := ""
		// For reverse (children) traversal, the SELECT is owner_id which can be NULL.
		// Filter out NULLs to prevent NOT IN from returning empty sets.
		if strings.HasSuffix(step.selectCol, ".owner_id") {
			childFilter = step.alias + ".owner_id IS NOT NULL AND "
		}
		currentWhere = fmt.Sprintf("%s IN (SELECT %s FROM groups %s WHERE %s%s)",
			step.fkExpr, step.selectCol, step.alias, childFilter, currentWhere)
	}
	return currentWhere, currentVals
}

// buildScalarClause builds a single-column comparison clause with appropriate
// case-insensitivity and LIKE handling.
func (tc *translateContext) buildScalarClause(qualifiedCol string, op Token, val interface{}, fd FieldDef) (string, interface{}) {
	if op.Type == TokenLike || op.Type == TokenNotLike {
		likePattern := convertMRQLWildcards(fmt.Sprint(val))
		likeOp := tc.likeOperator()
		if op.Type == TokenNotLike {
			likeOp = "NOT " + likeOp
		}
		return qualifiedCol + " " + likeOp + " ? ESCAPE '\\'", likePattern
	}
	sqlOp := tc.sqlOperator(op)
	// Case-insensitive equality for string and non-numeric meta fields
	if (fd.Type == FieldString || fd.Type == FieldMeta) && (op.Type == TokenEq || op.Type == TokenNeq) {
		if fd.Type == FieldMeta && isNumericValue(val) {
			return qualifiedCol + " " + sqlOp + " ?", val
		}
		return "LOWER(" + qualifiedCol + ") " + sqlOp + " LOWER(?)", val
	}
	return qualifiedCol + " " + sqlOp + " ?", val
}

// translateFKChainScalar translates a chained traversal ending in a scalar field
// comparison. Example: owner.parent.name = "Vacation" → nested IN subqueries.
func (tc *translateContext) translateFKChainScalar(db *gorm.DB, steps []fkStep, leafCol string, op Token, val interface{}, leafFd FieldDef) (*gorm.DB, error) {
	isNegated := op.Type == TokenNeq || op.Type == TokenNotLike
	isChildrenRoot := tc.isChildrenStep(steps[0])

	if isNegated && isChildrenRoot {
		// Children negation uses NOT EXISTS semantics: flip to positive match with NOT IN wrapper.
		positiveOp := tc.flipOperator(op)
		innerAlias := steps[len(steps)-1].alias
		innerWhere, innerVal := tc.buildScalarClause(innerAlias+"."+leafCol, positiveOp, val, leafFd)
		sql, vals := tc.wrapChainSubqueries(steps, innerWhere, []interface{}{innerVal})
		// Replace the outermost IN with NOT IN
		sql = strings.Replace(sql, steps[0].fkExpr+" IN ", steps[0].fkExpr+" NOT IN ", 1)
		sql = "(" + sql + " OR " + tc.negatedNullClause(steps[0]) + ")"
		db = db.Where(sql, vals...)
		return db, nil
	}

	innerAlias := steps[len(steps)-1].alias
	innerWhere, innerVal := tc.buildScalarClause(innerAlias+"."+leafCol, op, val, leafFd)
	sql, vals := tc.wrapChainSubqueries(steps, innerWhere, []interface{}{innerVal})
	if isNegated {
		sql = "(" + sql + " OR " + tc.negatedNullClause(steps[0]) + ")"
	}
	db = db.Where(sql, vals...)
	return db, nil
}

// translateFKChainTag translates a chained traversal ending in a tags comparison.
// Example: owner.tags = "photo" → nested IN with group_tags join.
func (tc *translateContext) translateFKChainTag(db *gorm.DB, steps []fkStep, op Token, val interface{}) (*gorm.DB, error) {
	isNegated := op.Type == TokenNeq || op.Type == TokenNotLike
	isLike := op.Type == TokenLike || op.Type == TokenNotLike
	isChildrenRoot := tc.isChildrenStep(steps[0])

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

	if isNegated && isChildrenRoot {
		// Children negation uses NOT EXISTS semantics: use a positive tag match
		// and flip the outermost IN to NOT IN.
		innerAlias := steps[len(steps)-1].alias
		innerWhere := fmt.Sprintf("%s.id IN (SELECT gt.group_id FROM group_tags gt JOIN tags t ON t.id = gt.tag_id WHERE %s)",
			innerAlias, tagMatchClause)
		sql, vals := tc.wrapChainSubqueries(steps, innerWhere, []interface{}{tagMatchVal})
		// Replace the outermost IN with NOT IN
		sql = strings.Replace(sql, steps[0].fkExpr+" IN ", steps[0].fkExpr+" NOT IN ", 1)
		sql = "(" + sql + " OR " + tc.negatedNullClause(steps[0]) + ")"
		db = db.Where(sql, vals...)
		return db, nil
	}

	inOrNotIn := "IN"
	if isNegated {
		inOrNotIn = "NOT IN"
	}

	innerAlias := steps[len(steps)-1].alias
	innerWhere := fmt.Sprintf("%s.id %s (SELECT gt.group_id FROM group_tags gt JOIN tags t ON t.id = gt.tag_id WHERE %s)",
		innerAlias, inOrNotIn, tagMatchClause)
	sql, vals := tc.wrapChainSubqueries(steps, innerWhere, []interface{}{tagMatchVal})
	if isNegated {
		sql = "(" + sql + " OR " + tc.negatedNullClause(steps[0]) + ")"
	}
	db = db.Where(sql, vals...)
	return db, nil
}

// negatedNullClause returns the SQL clause to include entities that have no related
// record for the given FK step. For forward FKs (owner/parent), this is
// "table.owner_id IS NULL". For reverse FKs (children), this is "table.id NOT IN
// (SELECT owner_id FROM groups WHERE owner_id IS NOT NULL)" — i.e., leaf groups.
func (tc *translateContext) negatedNullClause(step fkStep) string {
	if tc.isChildrenStep(step) {
		return step.fkExpr + " NOT IN (SELECT owner_id FROM groups WHERE owner_id IS NOT NULL)"
	}
	// For forward FK (owner/parent): the fkExpr is like "resources.owner_id"
	return step.fkExpr + " IS NULL"
}

// isChildrenStep returns true if the step represents a reverse FK (children) traversal.
func (tc *translateContext) isChildrenStep(step fkStep) bool {
	return strings.HasSuffix(step.selectCol, ".owner_id")
}

// flipOperator converts a negated operator to its positive counterpart for
// children NOT EXISTS semantics.
func (tc *translateContext) flipOperator(op Token) Token {
	flipped := op
	switch op.Type {
	case TokenNeq:
		flipped.Type = TokenEq
	case TokenNotLike:
		flipped.Type = TokenLike
	}
	return flipped
}

// translateChainedComparison is the main router for multi-part traversal
// comparisons (owner.X, parent.X, children.X, owner.parent.X, etc.).
func (tc *translateContext) translateChainedComparison(db *gorm.DB, expr *ComparisonExpr) (*gorm.DB, error) {
	parts := expr.Field.Parts

	// Handle meta subpath leaf: owner.meta.region, owner.meta.a.b.c
	// Detected when any part (after root) is "meta" — everything after it is the subpath.
	for i := 1; i < len(parts); i++ {
		if parts[i].Value == "meta" {
			return tc.translateChainedMetaComparison(db, expr)
		}
	}

	leaf := parts[len(parts)-1].Value

	// Look up the leaf field on the group entity (all traversals resolve to groups)
	subFd, ok := LookupField(EntityGroup, leaf)
	if !ok && !IsCommonField(leaf) {
		return nil, &TranslateError{Message: fmt.Sprintf("unknown field %q for traversal", leaf), Pos: parts[len(parts)-1].Pos}
	}
	if IsCommonField(leaf) && !ok {
		subFd, _ = LookupField(EntityGroup, leaf)
	}

	val, err := tc.resolveValue(expr.Value, subFd)
	if err != nil {
		return nil, err
	}

	steps := tc.buildTraversalChain(parts)

	// Handle relation sub-fields: tags
	if subFd.Type == FieldRelation && subFd.Column == "tags" {
		return tc.translateFKChainTag(db, steps, expr.Operator, val)
	}

	// Scalar sub-field
	return tc.translateFKChainScalar(db, steps, subFd.Column, expr.Operator, val, subFd)
}

// translateChainedMetaComparison handles traversal chains ending in meta subpath
// (e.g., owner.meta.region = "eu", owner.meta.a.b.c ~ "val*").
// Reuses the shared meta JSON helpers for consistent behavior.
func (tc *translateContext) translateChainedMetaComparison(db *gorm.DB, expr *ComparisonExpr) (*gorm.DB, error) {
	parts := expr.Field.Parts

	// Find the "meta" part in the chain
	metaIdx := -1
	for i := 1; i < len(parts); i++ {
		if parts[i].Value == "meta" {
			metaIdx = i
			break
		}
	}

	// Extract subpath segments (everything after "meta")
	segments := make([]string, 0, len(parts)-metaIdx-1)
	for i := metaIdx + 1; i < len(parts); i++ {
		segments = append(segments, parts[i].Value)
	}

	if err := validateMetaSegments(segments); err != nil {
		return nil, &TranslateError{Message: err.Error(), Pos: parts[metaIdx+1].Pos}
	}

	// Build FK chain for everything before "meta" (e.g., [owner] or [owner, parent]).
	// Append a dummy leaf so buildTraversalChain processes all traversal steps.
	chainParts := make([]Token, 0, metaIdx+1)
	chainParts = append(chainParts, parts[:metaIdx]...)
	chainParts = append(chainParts, Token{Value: "_meta_leaf"})
	steps := tc.buildTraversalChain(chainParts)

	// Resolve the comparison value
	metaFd := FieldDef{Name: "meta." + strings.Join(segments, "."), Type: FieldMeta, Column: "meta." + strings.Join(segments, ".")}
	val, err := tc.resolveValue(expr.Value, metaFd)
	if err != nil {
		return nil, err
	}

	innerAlias := steps[len(steps)-1].alias
	isNumericVal := isNumericValue(val)
	var jsonExpr string
	var numericFilter string
	if isNumericVal {
		jsonExpr = tc.metaNumericExprOn(innerAlias, segments)
		numericFilter = tc.metaTypeFilterOn(innerAlias, segments)
	} else {
		jsonExpr = tc.metaJsonExprOn(innerAlias, segments)
	}

	textExpr := tc.metaJsonTextExprOn(innerAlias, segments)
	innerWhere, innerVal := tc.buildMetaClauseV2(jsonExpr, textExpr, expr.Operator, val, isNumericVal)
	if numericFilter != "" {
		innerWhere = numericFilter + " AND " + innerWhere
	}

	isNegated := expr.Operator.Type == TokenNeq || expr.Operator.Type == TokenNotLike
	isChildrenRoot := tc.isChildrenStep(steps[0])

	if isNegated && isChildrenRoot {
		positiveOp := tc.flipOperator(expr.Operator)
		posWhere, posVal := tc.buildMetaClauseV2(jsonExpr, textExpr, positiveOp, val, isNumericVal)
		if numericFilter != "" {
			posWhere = numericFilter + " AND " + posWhere
		}
		sql, vals := tc.wrapChainSubqueries(steps, posWhere, []interface{}{posVal})
		sql = strings.Replace(sql, steps[0].fkExpr+" IN ", steps[0].fkExpr+" NOT IN ", 1)
		sql = "(" + sql + " OR " + tc.negatedNullClause(steps[0]) + ")"
		db = db.Where(sql, vals...)
		return db, nil
	}

	sql, vals := tc.wrapChainSubqueries(steps, innerWhere, []interface{}{innerVal})
	if isNegated {
		sql = "(" + sql + " OR " + tc.negatedNullClause(steps[0]) + ")"
	}
	db = db.Where(sql, vals...)
	return db, nil
}

// buildMetaClause builds a WHERE clause for a meta JSON comparison,
// handling LIKE, case-insensitive string equality, and numeric comparisons.
func (tc *translateContext) buildMetaClause(jsonExpr string, op Token, val interface{}, isNumericVal bool, alias string, key string) (string, interface{}) {
	if op.Type == TokenLike || op.Type == TokenNotLike {
		// LIKE always operates on text
		textExpr := jsonExpr
		if tc.isPostgres() && isNumericVal {
			textExpr = fmt.Sprintf("%s.meta->>'%s'", alias, key)
		}
		likePattern := convertMRQLWildcards(fmt.Sprint(val))
		likeOp := tc.likeOperator()
		if op.Type == TokenNotLike {
			likeOp = "NOT " + likeOp
		}
		return textExpr + " " + likeOp + " ? ESCAPE '\\'", likePattern
	}

	sqlOp := tc.sqlOperator(op)

	// Case-insensitive string equality for non-numeric values
	if !isNumericVal && (op.Type == TokenEq || op.Type == TokenNeq) {
		return "LOWER(" + jsonExpr + ") " + sqlOp + " LOWER(?)", val
	}

	return jsonExpr + " " + sqlOp + " ?", val
}

// buildMetaClauseV2 builds a WHERE clause for a meta JSON comparison.
// Unlike buildMetaClause, it receives pre-built JSON and text expressions
// so it works with both single keys and subpaths.
func (tc *translateContext) buildMetaClauseV2(jsonExpr string, textExpr string, op Token, val interface{}, isNumericVal bool) (string, interface{}) {
	if op.Type == TokenLike || op.Type == TokenNotLike {
		likePattern := convertMRQLWildcards(fmt.Sprint(val))
		likeOp := tc.likeOperator()
		if op.Type == TokenNotLike {
			likeOp = "NOT " + likeOp
		}
		return textExpr + " " + likeOp + " ? ESCAPE '\\'", likePattern
	}

	sqlOp := tc.sqlOperator(op)

	if !isNumericVal && (op.Type == TokenEq || op.Type == TokenNeq) {
		return "LOWER(" + jsonExpr + ") " + sqlOp + " LOWER(?)", val
	}

	return jsonExpr + " " + sqlOp + " ?", val
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

	// Wrap the OR in a nested Where so it's parenthesized when combined
	// with prior AND conditions: A AND (B OR C), not A AND B OR C.
	orGroup := tc.db.Session(&gorm.Session{NewDB: true}).Where(leftDB).Or(rightDB)
	db = db.Where(orGroup)

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

	// Handle traversal chains: owner.X, parent.X, children.X, owner.parent.X, etc.
	if len(expr.Field.Parts) >= 2 {
		root := expr.Field.Parts[0].Value
		if traversalFieldNames[root] {
			return tc.translateChainedComparison(db, expr)
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
	nameFd := FieldDef{Name: "name", Type: FieldString, Column: "name"}
	switch fd.Column {
	case "tags":
		return tc.translateTagComparison(db, op, val)
	case "groups":
		return tc.translateGroupComparison(db, op, val)
	case "owner_id":
		// owner = "name" → find entities whose owner group has the given name
		steps := tc.buildTraversalChain([]Token{{Value: "owner"}, {Value: "name"}})
		return tc.translateFKChainScalar(db, steps, "name", op, val, nameFd)
	case "parent_id":
		// parent = "name" → find groups whose parent has the given name
		steps := tc.buildTraversalChain([]Token{{Value: "parent"}, {Value: "name"}})
		return tc.translateFKChainScalar(db, steps, "name", op, val, nameFd)
	case "children":
		// children = "name" → find groups that have a child with the given name
		steps := tc.buildTraversalChain([]Token{{Value: "children"}, {Value: "name"}})
		return tc.translateFKChainScalar(db, steps, "name", op, val, nameFd)
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


// translateTraversalIsNull handles traversal.X IS [NOT] NULL (e.g. parent.name IS NULL,
// owner.description IS NOT NULL, children.category IS NULL).
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
	// The "parent" field has a logical Column of "parent_id" but the actual
	// DB column on groups is "owner_id". Map it here.
	if col == "parent_id" {
		col = "owner_id"
	}

	// Determine the FK column based on the root traversal type
	if root == "children" {
		// children.X IS NULL → has some child where X is null (or no children)
		// children.X IS NOT NULL → has some child where X is not null
		srcCol := tc.tableName + ".id"
		if expr.Negated {
			db = db.Where(
				fmt.Sprintf("%s IN (SELECT c.owner_id FROM groups c WHERE c.%s IS NOT NULL AND c.owner_id IS NOT NULL)", srcCol, col),
			)
		} else {
			db = db.Where(
				fmt.Sprintf("(%s IN (SELECT c.owner_id FROM groups c WHERE c.%s IS NULL AND c.owner_id IS NOT NULL) OR %s NOT IN (SELECT owner_id FROM groups WHERE owner_id IS NOT NULL))", srcCol, col, srcCol),
			)
		}
	} else {
		// parent/owner: forward FK traversal
		// X.field IS NULL → FK target exists but field is null, OR no FK at all
		// X.field IS NOT NULL → FK target exists and field is not null
		fkCol := tc.tableName + ".owner_id"
		if expr.Negated {
			db = db.Where(
				fmt.Sprintf("%s IN (SELECT p.id FROM groups p WHERE p.%s IS NOT NULL)", fkCol, col),
			)
		} else {
			db = db.Where(
				fmt.Sprintf("(%s IN (SELECT p.id FROM groups p WHERE p.%s IS NULL) OR %s IS NULL)", fkCol, col, fkCol),
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

// validateMetaSegments checks that each segment in a meta subpath is safe for
// interpolation into JSON extraction paths.
func validateMetaSegments(segments []string) error {
	for _, seg := range segments {
		if !isValidMetaKey(seg) {
			return fmt.Errorf("invalid meta key segment %q: must contain only alphanumeric characters and underscores", seg)
		}
	}
	return nil
}

// metaSubpathSegments extracts the subpath segments from a meta field name.
// "meta.a.b.c" → ["a", "b", "c"]
func metaSubpathSegments(fieldName string) []string {
	return strings.Split(strings.TrimPrefix(fieldName, "meta."), ".")
}

// metaJsonExpr builds the JSON extraction expression for a meta subpath.
// On SQLite: json_extract(table.meta, '$.a.b.c')
// On Postgres: table.meta->'a'->'b'->>'c'  (text extraction on final key)
func (tc *translateContext) metaJsonExpr(segments []string) string {
	return tc.metaJsonExprOn(tc.tableName, segments)
}

// metaJsonExprOn builds the JSON extraction expression using a specific table alias.
func (tc *translateContext) metaJsonExprOn(alias string, segments []string) string {
	if tc.isPostgres() {
		return pgJsonTextPath(alias, segments)
	}
	return sqliteJsonPath(alias, segments)
}

// metaJsonTextExpr returns a text-typed JSON extraction expression.
func (tc *translateContext) metaJsonTextExpr(segments []string) string {
	return tc.metaJsonExpr(segments)
}

// metaJsonTextExprOn returns a text-typed JSON extraction expression for a specific alias.
func (tc *translateContext) metaJsonTextExprOn(alias string, segments []string) string {
	return tc.metaJsonExprOn(alias, segments)
}

// metaNumericExpr builds a safe numeric cast expression for a meta subpath.
func (tc *translateContext) metaNumericExpr(segments []string) string {
	return tc.metaNumericExprOn(tc.tableName, segments)
}

// metaNumericExprOn builds a safe numeric cast expression using a specific table alias.
func (tc *translateContext) metaNumericExprOn(alias string, segments []string) string {
	if tc.isPostgres() {
		textExpr := pgJsonTextPath(alias, segments)
		return fmt.Sprintf(
			"CASE WHEN %s ~ '^-{0,1}[0-9]+(\\.[0-9]+){0,1}$' THEN (%s)::numeric ELSE NULL END",
			textExpr, textExpr,
		)
	}
	return sqliteJsonPath(alias, segments)
}

// metaTypeFilterOn returns a WHERE clause filtering to numeric JSON type rows (SQLite only).
func (tc *translateContext) metaTypeFilterOn(alias string, segments []string) string {
	if tc.isPostgres() {
		return ""
	}
	path := "$." + strings.Join(segments, ".")
	return fmt.Sprintf("json_type(%s.meta, '%s') IN ('integer', 'real')", alias, path)
}

// pgJsonTextPath builds Postgres chained arrow JSON path: table.meta->'a'->'b'->>'c'
func pgJsonTextPath(alias string, segments []string) string {
	if len(segments) == 1 {
		return fmt.Sprintf("%s.meta->>'%s'", alias, segments[0])
	}
	var b strings.Builder
	b.WriteString(alias)
	b.WriteString(".meta")
	for i, seg := range segments {
		if i == len(segments)-1 {
			b.WriteString("->>'" + seg + "'")
		} else {
			b.WriteString("->'" + seg + "'")
		}
	}
	return b.String()
}

// sqliteJsonPath builds SQLite json_extract path: json_extract(table.meta, '$.a.b.c')
func sqliteJsonPath(alias string, segments []string) string {
	path := "$." + strings.Join(segments, ".")
	return fmt.Sprintf("json_extract(%s.meta, '%s')", alias, path)
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
	segments := metaSubpathSegments(fd.Name)

	if err := validateMetaSegments(segments); err != nil {
		return nil, &TranslateError{Message: err.Error(), Pos: 0}
	}

	isNumericVal := isNumericValue(val)
	var jsonExpr string
	if isNumericVal {
		jsonExpr = tc.metaNumericExpr(segments)
		if filter := tc.metaTypeFilterOn(tc.tableName, segments); filter != "" {
			db = db.Where(filter)
		}
	} else {
		jsonExpr = tc.metaJsonExpr(segments)
	}

	sqlOp := tc.sqlOperator(op)

	if op.Type == TokenLike || op.Type == TokenNotLike {
		textExpr := tc.metaJsonTextExpr(segments)
		likePattern := convertMRQLWildcards(fmt.Sprint(val))
		likeOp := tc.likeOperator()
		if op.Type == TokenNotLike {
			likeOp = "NOT " + likeOp
		}
		db = db.Where(textExpr+" "+likeOp+" ? ESCAPE '\\'", likePattern)
		return db, nil
	}

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
		segments := metaSubpathSegments(fd.Name)
		if err := validateMetaSegments(segments); err != nil {
			return nil, &TranslateError{Message: err.Error()}
		}
		jsonExpr := tc.metaJsonExpr(segments)
		// Case-insensitive IN for string meta values
		hasStrings := false
		for _, v := range values {
			if _, ok := v.(string); ok {
				hasStrings = true
				break
			}
		}
		if hasStrings {
			lowerValues := make([]interface{}, len(values))
			for i, v := range values {
				lowerValues[i] = strings.ToLower(fmt.Sprint(v))
			}
			if expr.Negated {
				db = db.Where("LOWER("+jsonExpr+") NOT IN (?)", lowerValues)
			} else {
				db = db.Where("LOWER("+jsonExpr+") IN (?)", lowerValues)
			}
		} else {
			if expr.Negated {
				db = db.Where(jsonExpr+" NOT IN (?)", values)
			} else {
				db = db.Where(jsonExpr+" IN (?)", values)
			}
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

	// Handle traversal IS NULL / IS NOT NULL via traversal subquery
	if len(expr.Field.Parts) == 2 && expr.IsNull {
		root := expr.Field.Parts[0].Value
		if traversalFieldNames[root] {
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
			segments := metaSubpathSegments(fd.Name)
			column = tc.metaJsonExpr(segments)
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
		segments := metaSubpathSegments(fd.Name)
		column = tc.metaJsonExpr(segments)
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

	case "owner_id", "parent_id":
		// owner/parent IS EMPTY → owner_id IS NULL
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
		// PostgreSQL: check if search_vector column exists (handles -skip-fts)
		var colExists int
		tc.db.Raw(
			"SELECT COUNT(*) FROM information_schema.columns WHERE table_name = ? AND column_name = 'search_vector'",
			tc.tableName,
		).Scan(&colExists)

		if colExists > 0 {
			subquery := fmt.Sprintf(
				"%s.search_vector @@ plainto_tsquery('english', ?)",
				tc.tableName,
			)
			db = db.Where(subquery, searchTerm)
		} else {
			// Fallback: ILIKE search on name and description
			likePattern := "%" + searchTerm + "%"
			db = db.Where(
				fmt.Sprintf("(%s.name ILIKE ? OR %s.description ILIKE ?)",
					tc.tableName, tc.tableName),
				likePattern, likePattern,
			)
		}
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
// existing SQL wildcards in the value. If the value contains no MRQL wildcards,
// it is wrapped with % on both sides so ~ acts as a "contains" match.
func convertMRQLWildcards(s string) string {
	hasWildcards := strings.ContainsAny(s, "*?")
	// First escape existing SQL wildcards
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	// Then convert MRQL wildcards to SQL wildcards
	s = strings.ReplaceAll(s, "*", "%")
	s = strings.ReplaceAll(s, "?", "_")
	// If no wildcards were present, wrap with % for contains semantics
	if !hasWildcards {
		s = "%" + s + "%"
	}
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

// GroupByResult holds the result of a GROUP BY query.
type GroupByResult struct {
	Mode string           `json:"mode"` // "aggregated" or "bucketed"
	Rows []map[string]any `json:"rows,omitempty"`
}

// TranslateGroupBy translates and executes a GROUP BY query.
// For aggregated mode (aggregates present), it returns flat rows.
// For bucketed mode (no aggregates), it returns nil -- the caller handles bucketing.
func TranslateGroupBy(q *Query, db *gorm.DB) (*GroupByResult, error) {
	if q.GroupBy == nil {
		return nil, &TranslateError{Message: "TranslateGroupBy called without GROUP BY clause", Pos: 0}
	}

	entityType := q.EntityType
	if entityType == EntityUnspecified {
		entityType = ExtractEntityType(q)
	}
	if entityType == EntityUnspecified {
		return nil, &TranslateError{Message: "entity type is required for GROUP BY", Pos: 0}
	}

	tc := &translateContext{
		db:         db,
		entityType: entityType,
		tableName:  entityTableName(entityType),
	}

	result := db.Table(tc.tableName)

	// Apply WHERE clause
	if q.Where != nil {
		var err error
		result, err = tc.translateNode(result, q.Where)
		if err != nil {
			return nil, err
		}
	}

	if len(q.GroupBy.Aggregates) > 0 {
		return tc.translateAggregatedGroupBy(result, q)
	}

	// Bucketed mode -- return nil to signal caller should handle it
	return nil, nil
}

// translateAggregatedGroupBy builds SELECT ... GROUP BY ... and executes.
func (tc *translateContext) translateAggregatedGroupBy(db *gorm.DB, q *Query) (*GroupByResult, error) {
	// Add JOINs for relation fields (tags, owner, groups) if used in GROUP BY
	var relationExprs map[string]groupByRelExpr
	db, relationExprs = tc.groupByRelationJoins(db, q.GroupBy.Fields)

	var selectCols []string
	var groupCols []string

	// Build SELECT and GROUP BY column lists
	for _, f := range q.GroupBy.Fields {
		fieldName := f.Name()
		// Check if this field has a relation-based expression
		if rel, ok := relationExprs[fieldName]; ok {
			if rel.selectExpr != rel.groupExpr {
				// PostgreSQL requires non-grouped columns to be aggregated.
				// Since we group by a unique ID/FK, MAX(name) returns the one name.
				selectCols = append(selectCols, "MAX("+rel.selectExpr+`) AS "`+fieldName+`"`)
			} else {
				selectCols = append(selectCols, rel.selectExpr+` AS "`+fieldName+`"`)
			}
			groupCols = append(groupCols, rel.groupExpr)
		} else {
			selectExpr, groupExpr := tc.groupByFieldExprs(fieldName)
			selectCols = append(selectCols, selectExpr+` AS "`+fieldName+`"`)
			groupCols = append(groupCols, groupExpr)
		}
	}

	// Build aggregate SELECT expressions
	for _, agg := range q.GroupBy.Aggregates {
		selectExpr, alias := tc.aggregateExpr(agg)
		selectCols = append(selectCols, selectExpr+` AS "`+alias+`"`)
	}

	db = db.Select(strings.Join(selectCols, ", "))

	for _, gc := range groupCols {
		db = db.Group(gc)
	}

	// Build alias resolution map: any original field name → surviving SELECT alias.
	// After deduplication, "group" and "groups" both resolve to whichever survived
	// (e.g., "groups"). ORDER BY must use the surviving alias, not the original name.
	aliasMap := tc.buildGroupByAliasMap(q)

	// ORDER BY
	for _, ob := range q.OrderBy {
		obName := ob.Field.Name()
		if resolved, ok := aliasMap[obName]; ok {
			obName = resolved
		}
		direction := "ASC"
		if !ob.Ascending {
			direction = "DESC"
		}
		db = db.Order(`"` + obName + `" ` + direction)
	}

	// LIMIT / OFFSET
	if q.Limit >= 0 {
		db = db.Limit(q.Limit)
	}
	if q.Offset >= 0 {
		db = db.Offset(q.Offset)
	}

	var rows []map[string]any
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}

	return &GroupByResult{
		Mode: "aggregated",
		Rows: rows,
	}, nil
}

// buildGroupByAliasMap creates a mapping from all original GROUP BY field names
// (including dropped aliases) to the surviving SELECT alias name. This ensures
// ORDER BY on a dropped alias (e.g., "group" when "groups" survived) resolves
// to the correct SQL alias.
func (tc *translateContext) buildGroupByAliasMap(q *Query) map[string]string {
	if q.GroupBy == nil {
		return nil
	}

	// Build canonical column → surviving field name
	canonToSurvivor := make(map[string]string)
	for _, f := range q.GroupBy.Fields {
		col := f.Name()
		if len(f.Parts) == 1 {
			if fd, ok := LookupField(tc.entityType, f.Parts[0].Value); ok {
				col = fd.Column
			}
		}
		canonToSurvivor[col] = f.Name()
	}

	// Map every original name to the surviving alias
	aliasMap := make(map[string]string)
	for name := range q.GroupBy.AllFieldNames {
		col := name
		if fd, ok := LookupField(tc.entityType, name); ok {
			col = fd.Column
		}
		if survivor, ok := canonToSurvivor[col]; ok {
			aliasMap[name] = survivor
		}
	}
	// Also map aggregate output keys to themselves
	for _, agg := range q.GroupBy.Aggregates {
		if agg.Field == nil {
			aliasMap["count"] = "count"
		} else {
			key := strings.ToLower(agg.Name) + "_" + agg.Field.Name()
			aliasMap[key] = key
		}
	}
	return aliasMap
}

// groupByFieldExprs returns the SELECT expression and GROUP BY expression for a field.
func (tc *translateContext) groupByFieldExprs(fieldName string) (string, string) {
	// Meta fields
	if strings.HasPrefix(fieldName, "meta.") {
		key := strings.TrimPrefix(fieldName, "meta.")
		if tc.isPostgres() {
			expr := fmt.Sprintf("%s.meta->>'%s'", tc.tableName, key)
			return expr, expr
		}
		expr := fmt.Sprintf("json_extract(%s.meta, '$.%s')", tc.tableName, key)
		return expr, expr
	}

	fd, ok := LookupField(tc.entityType, fieldName)
	if !ok {
		return tc.tableName + "." + fieldName, tc.tableName + "." + fieldName
	}

	col := tc.qualifiedColumn(fd.Column)
	return col, col
}

// groupByRelExpr holds separate SELECT and GROUP BY expressions for a relation field.
// For tags, both are the same (tag name). For owner/groups, we GROUP BY the ID (unique)
// but SELECT the name (display), avoiding merges of distinct entities with the same name.
type groupByRelExpr struct {
	selectExpr string // what to show (e.g., _gb_owner.name)
	groupExpr  string // what to group by (e.g., _gb_owner.id)
}

// groupByRelationJoins modifies the db query to add JOINs for relation fields
// used in GROUP BY. Returns the db and a map of fieldName -> select/group expressions.
func (tc *translateContext) groupByRelationJoins(db *gorm.DB, fields []*FieldExpr) (*gorm.DB, map[string]groupByRelExpr) {
	exprMap := make(map[string]groupByRelExpr)
	for i, f := range fields {
		fieldName := f.Name()

		// Handle traversal paths: owner.name, owner.parent.name, parent.name, children.name, etc.
		if len(f.Parts) >= 2 {
			root := f.Parts[0].Value
			if traversalFieldNames[root] {
				var expr string
				db, expr = tc.groupByTraversalJoins(db, f, i)
				// Traversal leaves are scalar columns or meta extracts —
				// the expression is the same for SELECT and GROUP BY.
				exprMap[fieldName] = groupByRelExpr{selectExpr: expr, groupExpr: expr}
				continue
			}
		}

		fd, ok := LookupField(tc.entityType, fieldName)
		if !ok || fd.Type != FieldRelation {
			continue
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
			db = db.Joins(fmt.Sprintf("LEFT JOIN %s _gb_jt ON _gb_jt.%s = %s.id", junctionTable, entityCol, tc.tableName))
			db = db.Joins("LEFT JOIN tags _gb_t ON _gb_t.id = _gb_jt.tag_id")
			// Tags have unique names, so grouping by name is safe and produces
			// better output than opaque IDs.
			exprMap[fieldName] = groupByRelExpr{selectExpr: "_gb_t.name", groupExpr: "_gb_t.name"}

		case "owner_id":
			db = db.Joins(fmt.Sprintf("LEFT JOIN groups _gb_owner ON _gb_owner.id = %s.owner_id", tc.tableName))
			// Group by FK column (unique per entity) and display the joined name.
			// Group names are NOT unique, so grouping by name would merge distinct groups.
			exprMap[fieldName] = groupByRelExpr{selectExpr: "_gb_owner.name", groupExpr: tc.tableName + ".owner_id"}

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
			db = db.Joins(fmt.Sprintf("LEFT JOIN %s _gb_grp_jt ON _gb_grp_jt.%s = %s.id", junctionTable, entityCol, tc.tableName))
			db = db.Joins("LEFT JOIN groups _gb_g ON _gb_g.id = _gb_grp_jt.group_id")
			// Group by junction group_id (unique per association) and display name.
			exprMap[fieldName] = groupByRelExpr{selectExpr: "_gb_g.name", groupExpr: "_gb_grp_jt.group_id"}

		case "parent_id":
			// parent on groups — logical Column is "parent_id", actual DB column is owner_id
			db = db.Joins(fmt.Sprintf("LEFT JOIN groups _gb_parent ON _gb_parent.id = %s.owner_id", tc.tableName))
			exprMap[fieldName] = groupByRelExpr{selectExpr: "_gb_parent.name", groupExpr: tc.tableName + ".owner_id"}

		case "children":
			// children on groups — reverse FK: child.owner_id = parent.id
			db = db.Joins(fmt.Sprintf("LEFT JOIN groups _gb_child ON _gb_child.owner_id = %s.id", tc.tableName))
			exprMap[fieldName] = groupByRelExpr{selectExpr: "_gb_child.name", groupExpr: "_gb_child.id"}
		}
	}
	return db, exprMap
}

// groupByTraversalJoins builds LEFT JOIN chains for traversal paths in GROUP BY.
// For example, "owner.name" joins groups via owner_id; "owner.parent.name" chains
// two joins. Returns the modified db and the SQL expression for the leaf column.
func (tc *translateContext) groupByTraversalJoins(db *gorm.DB, f *FieldExpr, fieldIdx int) (*gorm.DB, string) {
	parts := f.Parts
	leaf := parts[len(parts)-1].Value

	// Handle meta.key leaf: owner.meta.abc, owner.parent.meta.xyz
	// Detect by checking if the second-to-last part is "meta"
	if len(parts) >= 3 && parts[len(parts)-2].Value == "meta" {
		metaKey := leaf
		// Build chain JOINs for everything before "meta"
		lastAlias := tc.groupByBuildChainJoins(&db, parts[:len(parts)-2], fieldIdx)
		if tc.isPostgres() {
			return db, fmt.Sprintf("%s.meta->>'%s'", lastAlias, metaKey)
		}
		return db, fmt.Sprintf("json_extract(%s.meta, '$.%s')", lastAlias, metaKey)
	}

	// Resolve the leaf field on the group entity (all traversals resolve to groups)
	leafFd, ok := LookupField(EntityGroup, leaf)
	if !ok && IsCommonField(leaf) {
		leafFd, _ = LookupField(EntityGroup, leaf)
	}

	// Handle leaf = "tags" (junction table on the final group)
	if ok && leafFd.Type == FieldRelation && leafFd.Column == "tags" {
		lastAlias := tc.groupByBuildChainJoins(&db, parts[:len(parts)-1], fieldIdx)
		tagAlias := fmt.Sprintf("_gbt_%d", fieldIdx)
		jtAlias := fmt.Sprintf("_gbjt_%d", fieldIdx)
		db = db.Joins(fmt.Sprintf("LEFT JOIN group_tags %s ON %s.group_id = %s.id", jtAlias, jtAlias, lastAlias))
		db = db.Joins(fmt.Sprintf("LEFT JOIN tags %s ON %s.id = %s.tag_id", tagAlias, tagAlias, jtAlias))
		return db, tagAlias + ".name"
	}

	// Scalar leaf: build chain JOINs then reference leaf column on last alias
	lastAlias := tc.groupByBuildChainJoins(&db, parts[:len(parts)-1], fieldIdx)
	return db, lastAlias + "." + leafFd.Column
}

// groupByBuildChainJoins adds LEFT JOINs for each step in a traversal chain.
// steps are the traversal tokens (e.g., [owner], [owner, parent]).
// Returns the alias of the last joined table.
func (tc *translateContext) groupByBuildChainJoins(db **gorm.DB, steps []Token, fieldIdx int) string {
	prevRef := tc.tableName
	var lastAlias string

	for i, step := range steps {
		alias := fmt.Sprintf("_gbr_%d_%d", fieldIdx, i)
		fieldName := step.Value

		if fieldName == "children" {
			// Reverse FK: children's owner_id points to parent's id
			*db = (*db).Joins(fmt.Sprintf("LEFT JOIN groups %s ON %s.owner_id = %s.id", alias, alias, prevRef))
		} else {
			// Forward FK (owner/parent): source.owner_id = target.id
			*db = (*db).Joins(fmt.Sprintf("LEFT JOIN groups %s ON %s.id = %s.owner_id", alias, alias, prevRef))
		}

		prevRef = alias
		lastAlias = alias
	}

	return lastAlias
}

// aggregateExpr returns the SQL aggregate expression and the output alias.
func (tc *translateContext) aggregateExpr(agg AggregateFunc) (string, string) {
	switch agg.Name {
	case "COUNT":
		return "COUNT(*)", "count"
	default:
		fieldName := agg.Field.Name()
		// SUM/AVG on meta require numeric cast (non-numeric → NULL).
		// MIN/MAX on meta use text extraction — this includes all values (strings
		// and numbers) but gives lexicographic order on PG for multi-digit numbers.
		// This is the correct trade-off: casting would silently drop all string
		// metadata from MIN/MAX, which is worse than imperfect numeric ordering.
		needsNumericCast := (agg.Name == "SUM" || agg.Name == "AVG") && strings.HasPrefix(fieldName, "meta.")
		col := tc.resolveAggregateColumn(fieldName, needsNumericCast)
		alias := strings.ToLower(agg.Name) + "_" + fieldName
		return fmt.Sprintf("%s(%s)", agg.Name, col), alias
	}
}

// resolveAggregateColumn converts a field name to its SQL column expression.
// numericCast controls whether meta fields are cast to numeric on PostgreSQL.
// SUM/AVG require numeric (non-numeric → NULL). MIN/MAX use text extraction
// to include all values including non-numeric strings.
func (tc *translateContext) resolveAggregateColumn(fieldName string, numericCast bool) string {
	if strings.HasPrefix(fieldName, "meta.") {
		key := strings.TrimPrefix(fieldName, "meta.")
		if tc.isPostgres() {
			if numericCast {
				// Safe numeric cast: returns NULL for non-numeric values instead of crashing.
				// Uses the same guarded CASE pattern as translateMetaComparison.
				return fmt.Sprintf(
					"CASE WHEN %s.meta->>'%s' ~ '^-{0,1}[0-9]+(\\.[0-9]+){0,1}$' THEN (%s.meta->>'%s')::numeric ELSE NULL END",
					tc.tableName, key, tc.tableName, key,
				)
			}
			return fmt.Sprintf("%s.meta->>'%s'", tc.tableName, key)
		}
		return fmt.Sprintf("json_extract(%s.meta, '$.%s')", tc.tableName, key)
	}

	fd, ok := LookupField(tc.entityType, fieldName)
	if !ok {
		return tc.tableName + "." + fieldName
	}
	return tc.qualifiedColumn(fd.Column)
}

