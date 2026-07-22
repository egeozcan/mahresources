package mrql

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"strconv"
	"strings"
)

const QueryShapeFingerprintVersion = "mrql-shape-v1"

// ScopeShape describes how authorization and an explicit SCOPE clause affect
// an Effective MRQL Query. Scope identities are deliberately absent: the
// fingerprint distinguishes the execution shape without retaining group IDs.
type ScopeShape string

const (
	ScopeShapeNone     ScopeShape = "none"
	ScopeShapeExplicit ScopeShape = "explicit"
	ScopeShapeForced   ScopeShape = "forced"
	ScopeShapeDenied   ScopeShape = "denied"
)

// QueryShapePolicy captures runtime settings that can change translated SQL.
// Values here are operator policy rather than user query literals.
type QueryShapePolicy struct {
	Dialect             string
	SimilarityThreshold int
	AHashThreshold      uint64
	FTSAvailable        bool
}

// QueryShapeFingerprint uses the zero translation policy. Application paths
// should use QueryShapeFingerprintWithPolicy with their effective settings.
func QueryShapeFingerprint(q *Query, scope ScopeShape) string {
	return QueryShapeFingerprintWithPolicy(q, scope, QueryShapePolicy{})
}

// QueryShapeFingerprintWithPolicy returns a stable, value-redacted fingerprint
// of a validated Effective MRQL Query. It walks the AST read-only: source text,
// token positions, literal values, parameter names, search text, and scope IDs
// are never written to the digest input. Structural values and effective
// runtime policy that change emitted SQL are retained.
func QueryShapeFingerprintWithPolicy(q *Query, scope ScopeShape, policy QueryShapePolicy) string {
	h := sha256.New()
	shapeWrite(h, "version", QueryShapeFingerprintVersion)
	shapeWrite(h, "scope", string(scope))
	shapeWrite(h, "dialect", policy.Dialect)
	shapeWrite(h, "similarity-threshold", strconv.Itoa(policy.SimilarityThreshold))
	shapeWrite(h, "ahash-threshold", strconv.FormatUint(policy.AHashThreshold, 10))
	shapeWrite(h, "fts-available", strconv.FormatBool(policy.FTSAvailable))
	shapeWrite(h, "entity", strconv.Itoa(int(q.EntityType)))
	shapeWrite(h, "limit", strconv.Itoa(q.Limit))
	shapeWrite(h, "offset", strconv.Itoa(q.Offset))
	shapeWrite(h, "bucket-limit", strconv.Itoa(q.BucketLimit))
	shapeNode(h, q.Where, "")
	shapeGroupBy(h, q.GroupBy)
	shapeWrite(h, "orders", strconv.Itoa(len(q.OrderBy)))
	for _, order := range q.OrderBy {
		shapeWrite(h, "order-random", strconv.FormatBool(order.Random))
		if order.Random {
			continue
		}
		shapeField(h, order.Field)
		shapeWrite(h, "order-ascending", strconv.FormatBool(order.Ascending))
	}
	return QueryShapeFingerprintVersion + ":" + hex.EncodeToString(h.Sum(nil))
}

func shapeWrite(h hash.Hash, key, value string) {
	_, _ = fmt.Fprintf(h, "%d:%s=%d:%s;", len(key), key, len(value), value)
}

func shapeField(h hash.Hash, field *FieldExpr) {
	if field == nil {
		shapeWrite(h, "field", "nil")
		return
	}
	shapeWrite(h, "field-parts", strconv.Itoa(len(field.Parts)))
	for _, part := range field.Parts {
		shapeWrite(h, "field-part", part.Value)
	}
}

func shapeNode(h hash.Hash, node Node, comparisonField string) {
	if node == nil {
		shapeWrite(h, "node", "nil")
		return
	}
	shapeWrite(h, "node", node.nodeType())
	switch n := node.(type) {
	case *BinaryExpr:
		shapeWrite(h, "operator", strconv.Itoa(int(n.Operator.Type)))
		shapeNode(h, n.Left, "")
		shapeNode(h, n.Right, "")
	case *NotExpr:
		shapeNode(h, n.Expr, "")
	case *ComparisonExpr:
		shapeField(h, n.Field)
		shapeWrite(h, "operator", strconv.Itoa(int(n.Operator.Type)))
		fieldName := ""
		if n.Field != nil {
			fieldName = strings.ToLower(n.Field.Name())
		}
		shapeNode(h, n.Value, fieldName)
	case *InExpr:
		shapeField(h, n.Field)
		shapeWrite(h, "negated", strconv.FormatBool(n.Negated))
		shapeWrite(h, "value-count", strconv.Itoa(len(n.Values)))
		for _, value := range n.Values {
			shapeNode(h, value, "")
		}
	case *IsExpr:
		shapeField(h, n.Field)
		shapeWrite(h, "negated", strconv.FormatBool(n.Negated))
		shapeWrite(h, "is-null", strconv.FormatBool(n.IsNull))
	case *TextSearchExpr:
		shapeWrite(h, "text-class", map[bool]string{true: "empty", false: "nonempty"}[n.Value == nil || strings.TrimSpace(n.Value.Value) == ""])
	case *SimilarToExpr:
		shapeWrite(h, "within", map[bool]string{true: "default", false: "explicit"}[n.Within < 0])
	case *FieldExpr:
		shapeField(h, n)
	case *StringLiteral:
		if comparisonField == "type" {
			shapeWrite(h, "entity-literal", strings.ToLower(n.Value))
		} else {
			shapeWrite(h, "literal", "string")
		}
	case *NumberLiteral:
		shapeWrite(h, "literal", "number")
		shapeWrite(h, "number-unit", strings.ToLower(n.Unit))
	case *BooleanLiteral:
		shapeWrite(h, "literal", "boolean")
		shapeWrite(h, "boolean", strconv.FormatBool(n.Value))
	case *RelDateLiteral:
		shapeWrite(h, "literal", "relative-date")
		shapeWrite(h, "relative-date-unit", strings.ToLower(n.Unit))
	case *FuncCall:
		shapeWrite(h, "function", strings.ToUpper(n.Name))
	case *ParamRef:
		shapeWrite(h, "literal", "parameter")
	case *HavingComparison:
		shapeAggregate(h, n.Agg)
		shapeWrite(h, "operator", strconv.Itoa(int(n.Operator.Type)))
		shapeNode(h, n.Value, "")
	default:
		shapeWrite(h, "unknown-node", fmt.Sprintf("%T", node))
	}
}

func shapeAggregate(h hash.Hash, aggregate AggregateFunc) {
	shapeWrite(h, "aggregate", strings.ToUpper(aggregate.Name))
	shapeField(h, aggregate.Field)
}

func shapeGroupBy(h hash.Hash, group *GroupByClause) {
	if group == nil {
		shapeWrite(h, "group-by", "nil")
		return
	}
	shapeWrite(h, "group-fields", strconv.Itoa(len(group.Fields)))
	for _, field := range group.Fields {
		shapeField(h, field)
	}
	shapeWrite(h, "aggregates", strconv.Itoa(len(group.Aggregates)))
	for _, aggregate := range group.Aggregates {
		shapeAggregate(h, aggregate)
	}
	shapeNode(h, group.Having, "")
}
