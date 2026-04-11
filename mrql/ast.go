package mrql

// Node is the interface implemented by all AST nodes.
type Node interface {
	nodeType() string
	Pos() int // start position in the source string
}

// BinaryExpr represents: left AND/OR right
type BinaryExpr struct {
	Left     Node
	Operator Token // AND, OR
	Right    Node
}

func (b *BinaryExpr) nodeType() string { return "BinaryExpr" }
func (b *BinaryExpr) Pos() int         { return b.Left.Pos() }

// NotExpr represents: NOT expr
type NotExpr struct {
	Token Token
	Expr  Node
}

func (n *NotExpr) nodeType() string { return "NotExpr" }
func (n *NotExpr) Pos() int         { return n.Token.Pos }

// ComparisonExpr represents: field op value
type ComparisonExpr struct {
	Field    *FieldExpr
	Operator Token
	Value    Node // StringLiteral, NumberLiteral, RelDate, FuncCall
}

func (c *ComparisonExpr) nodeType() string { return "ComparisonExpr" }
func (c *ComparisonExpr) Pos() int         { return c.Field.Pos() }

// InExpr represents: field IN ("a", "b") or field NOT IN ("a", "b")
type InExpr struct {
	Field   *FieldExpr
	Negated bool
	Values  []Node // list of StringLiteral or NumberLiteral
	InToken Token
}

func (i *InExpr) nodeType() string { return "InExpr" }
func (i *InExpr) Pos() int         { return i.Field.Pos() }

// IsExpr represents: field IS [NOT] EMPTY/NULL
type IsExpr struct {
	Field   *FieldExpr
	Negated bool
	IsNull  bool // true = IS [NOT] NULL, false = IS [NOT] EMPTY
	IsToken Token
}

func (e *IsExpr) nodeType() string { return "IsExpr" }
func (e *IsExpr) Pos() int         { return e.Field.Pos() }

// TextSearchExpr represents: TEXT ~ "query"
type TextSearchExpr struct {
	TextToken Token
	Value     *StringLiteral
}

func (t *TextSearchExpr) nodeType() string { return "TextSearchExpr" }
func (t *TextSearchExpr) Pos() int         { return t.TextToken.Pos }

// FieldExpr represents a field reference: name, meta.key, parent.name
type FieldExpr struct {
	Parts []Token // e.g., ["parent", "name"] or ["meta", "rating"] or ["name"]
}

func (f *FieldExpr) nodeType() string { return "FieldExpr" }
func (f *FieldExpr) Pos() int         { return f.Parts[0].Pos }

func (f *FieldExpr) Name() string {
	if len(f.Parts) == 1 {
		return f.Parts[0].Value
	}
	result := f.Parts[0].Value
	for _, p := range f.Parts[1:] {
		result += "." + p.Value
	}
	return result
}

// StringLiteral is a quoted string value.
type StringLiteral struct {
	Token Token
	Value string // unescaped value
}

func (s *StringLiteral) nodeType() string { return "StringLiteral" }
func (s *StringLiteral) Pos() int         { return s.Token.Pos }

// NumberLiteral is a numeric value, optionally with a unit (kb, mb, gb).
type NumberLiteral struct {
	Token Token
	Value float64
	Unit  string // "", "kb", "mb", "gb"
	Raw   int64  // value converted to base unit (bytes for file sizes)
}

func (n *NumberLiteral) nodeType() string { return "NumberLiteral" }
func (n *NumberLiteral) Pos() int         { return n.Token.Pos }

// RelDateLiteral is a relative date like -7d, -3m, -1y.
type RelDateLiteral struct {
	Token  Token
	Amount int
	Unit   string // "d", "w", "m", "y"
}

func (r *RelDateLiteral) nodeType() string { return "RelDateLiteral" }
func (r *RelDateLiteral) Pos() int         { return r.Token.Pos }

// FuncCall represents a date function like NOW(), START_OF_DAY(), etc.
type FuncCall struct {
	Token Token
	Name  string
}

func (f *FuncCall) nodeType() string { return "FuncCall" }
func (f *FuncCall) Pos() int         { return f.Token.Pos }

// AggregateFunc represents an aggregate function call: COUNT(), SUM(field), etc.
type AggregateFunc struct {
	Token Token      // the aggregate keyword token (COUNT, SUM, etc.)
	Name  string     // uppercase: "COUNT", "SUM", "AVG", "MIN", "MAX"
	Field *FieldExpr // nil for COUNT(), required for SUM/AVG/MIN/MAX
}

// GroupByClause holds GROUP BY fields and optional aggregate functions.
type GroupByClause struct {
	Fields        []*FieldExpr    // the fields to group by (deduplicated by validator)
	Aggregates    []AggregateFunc // aggregate functions (empty = bucketed mode)
	AllFieldNames map[string]bool // all original field names including dropped aliases (set by validator)
}

// OrderByClause is a single ORDER BY column+direction.
type OrderByClause struct {
	Field     *FieldExpr
	Ascending bool // true = ASC, false = DESC
}

// ScopeClause restricts query results to a group's ownership subtree.
// Value is either a NumberLiteral (group ID) or StringLiteral (group name).
type ScopeClause struct {
	Token Token // the SCOPE keyword token
	Value Node  // NumberLiteral or StringLiteral
}

// EntityType identifies which entity a query targets.
type EntityType int

const (
	EntityUnspecified EntityType = iota
	EntityResource
	EntityNote
	EntityGroup
)

func (e EntityType) String() string {
	switch e {
	case EntityResource:
		return "resource"
	case EntityNote:
		return "note"
	case EntityGroup:
		return "group"
	default:
		return "unspecified"
	}
}

// Query is the top-level AST node for a complete MRQL query.
type Query struct {
	Where       Node             // the filter expression (may be nil)
	Scope       *ScopeClause     // SCOPE clause (nil when absent)
	GroupBy     *GroupByClause   // GROUP BY clause (nil when absent)
	OrderBy     []OrderByClause  // ORDER BY clauses (may be empty)
	Limit       int              // -1 if not specified; per-bucket item cap in grouped mode
	Offset      int              // -1 if not specified; bucket page offset in grouped mode
	BucketLimit int              // -1 if not specified; max buckets per page (set by API, not syntax)
	EntityType  EntityType       // populated by validator or caller
}
