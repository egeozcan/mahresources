package mrql

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// MetadataIndex identifies a query expression to index, not a constraint on
// stored metadata. Numeric and text indexes can coexist for a mixed-type key.
type MetadataIndex struct {
	Entity string `json:"entity"`
	Key    string `json:"key"`
	Kind   string `json:"kind"`
}

const MaxMetadataIndexes = 32

func ParseMetadataIndexes(raw string) ([]MetadataIndex, error) {
	if len(raw) > 16384 {
		return nil, fmt.Errorf("metadata indexes must fit in 16 KiB")
	}
	var indexes []MetadataIndex
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&indexes); err != nil {
		return nil, fmt.Errorf("metadata indexes must be a JSON array: %w", err)
	}
	if indexes == nil {
		return nil, fmt.Errorf("metadata indexes must be an array; use [] to remove all indexes")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("unexpected data after metadata indexes")
	}
	if len(indexes) > MaxMetadataIndexes {
		return nil, fmt.Errorf("at most %d metadata indexes are supported", MaxMetadataIndexes)
	}
	seen := map[MetadataIndex]bool{}
	for i, index := range indexes {
		if err := index.Validate(); err != nil {
			return nil, fmt.Errorf("index %d: %w", i+1, err)
		}
		if seen[index] {
			return nil, fmt.Errorf("duplicate metadata index: %s %s %s", index.Entity, index.Key, index.Kind)
		}
		seen[index] = true
	}
	return indexes, nil
}

func (index MetadataIndex) Validate() error {
	if _, ok := ValidEntityTypes[index.Entity]; !ok {
		return fmt.Errorf("entity must be resource, note, or group")
	}
	if index.Kind != "numeric" && index.Kind != "text" {
		return fmt.Errorf("kind must be numeric or text")
	}
	if len(index.Key) == 0 || len(index.Key) > 128 {
		return fmt.Errorf("metadata key must contain 1–128 characters")
	}
	segments := strings.Split(index.Key, ".")
	if len(segments) > 8 {
		return fmt.Errorf("metadata keys support at most 8 path segments")
	}
	return validateMetaSegments(segments)
}

func (index MetadataIndex) Table() string { return entityTableName(ValidEntityTypes[index.Entity]) }

// Version the namespace when an indexed expression's semantics change. The
// reconciler builds replacements before retiring indexes from earlier versions.
func (index MetadataIndex) Name() string {
	return index.physicalName("")
}

func (index MetadataIndex) physicalName(role string) string {
	sum := sha256.Sum256([]byte(index.Entity + "\x00" + index.Key + "\x00" + index.Kind))
	if role != "" {
		sum = sha256.Sum256([]byte(index.Entity + "\x00" + index.Key + "\x00" + index.Kind + "\x00" + role))
	}
	return fmt.Sprintf("mah_midx_v2_%x", sum[:16])
}

type PhysicalMetadataIndex struct {
	Name string
	SQL  string
}

// CreateSQL returns the primary index's DDL. Use Indexes to create the complete
// index set, including PostgreSQL numeric indexes' oversized-value fallback.
func (index MetadataIndex) CreateSQL(dialect string) (string, error) {
	indexes, err := index.Indexes(dialect)
	if err != nil {
		return "", err
	}
	return indexes[0].SQL, nil
}

// Indexes uses the same expression builders as MRQL. Both engines match an
// expression index by its parsed expression, so a separate approximation here
// would silently yield indexes that the generated queries cannot use.
func (index MetadataIndex) Indexes(dialect string) ([]PhysicalMetadataIndex, error) {
	if err := index.Validate(); err != nil {
		return nil, err
	}
	segments := strings.Split(index.Key, ".")
	var expression, concurrently, include, method string
	switch dialect {
	case "postgres":
		expression = pgJsonTextPath("", segments)
		if index.Kind == "numeric" {
			expression = pgBoundedMetaNumericExpr(expression)
		}
		concurrently = " CONCURRENTLY"
		if index.Kind == "text" {
			method = " USING HASH"
		} else {
			include = " INCLUDE (id)"
		}
	case "sqlite":
		expression = sqliteJsonPath("", segments)
	default:
		return nil, fmt.Errorf("metadata indexes are unsupported on %q", dialect)
	}
	if index.Kind == "text" {
		expression = "LOWER(" + expression + ")"
	}
	indexes := []PhysicalMetadataIndex{{Name: index.Name(), SQL: fmt.Sprintf(`CREATE INDEX%s "%s" ON "%s"%s ((%s))%s`, concurrently, index.Name(), index.Table(), method, expression, include)}}
	if dialect == "postgres" && index.Kind == "numeric" {
		// The fallback indexes only IDs, never the oversized value. This keeps
		// arbitrary valid JSON writable while making rare outliers discoverable
		// without scanning the whole entity table on every numeric comparison.
		name := index.physicalName("oversized")
		indexes = append(indexes, PhysicalMetadataIndex{Name: name, SQL: fmt.Sprintf(`CREATE INDEX CONCURRENTLY "%s" ON "%s" (id) WHERE %s`, name, index.Table(), pgMetaLengthGuard(pgJsonTextPath("", segments), ">"))})
	}
	return indexes, nil
}

func pgMetaNumericExpr(alias string, segments []string) string {
	return pgMetaNumericTextExpr(pgJsonTextPath(alias, segments))
}

func pgMetaNumericTextExpr(textExpr string) string {
	return fmt.Sprintf("CASE WHEN %s ~ '^-{0,1}[0-9]+(\\.[0-9]+){0,1}$' THEN (%s)::numeric ELSE NULL END", textExpr, textExpr)
}

// 1,000 decimal characters fit comfortably in a PostgreSQL B-tree tuple and
// numeric's supported precision. This bounds only index storage, not MRQL or
// metadata: longer values are evaluated through the companion index.
const pgIndexedNumericMaxLength = 1000

func pgMetaLengthGuard(textExpr, op string) string {
	return fmt.Sprintf("length(%s) %s %d", textExpr, op, pgIndexedNumericMaxLength)
}

func pgBoundedMetaNumericExpr(textExpr string) string {
	return fmt.Sprintf("CASE WHEN %s THEN %s ELSE NULL END", pgMetaLengthGuard(textExpr, "<="), pgMetaNumericTextExpr(textExpr))
}

func pgIndexedMetaNumericComparison(textExpr, op string) string {
	short := pgMetaLengthGuard(textExpr, "<=")
	long := pgMetaLengthGuard(textExpr, ">")
	// Both length guards are required for three-valued logic under NOT: a
	// nonmatching long numeric value must be FALSE, not NULL OR FALSE. Keep
	// the long guard outside CASE for partial-index implication, and inside
	// CASE too so the planner cannot evaluate its cast on unrelated rows.
	return fmt.Sprintf("((%s AND %s %s ?) OR (%s AND CASE WHEN %s THEN %s %s ? ELSE FALSE END))", short, pgBoundedMetaNumericExpr(textExpr), op, long, long, pgMetaNumericTextExpr(textExpr), op)
}

func metaColumn(alias string) string {
	if alias == "" {
		return "meta"
	}
	return alias + ".meta"
}

// MetadataIndexKey is stored on a category; its entity type is supplied by the
// carrier (resource category, group category, or note type), never user SQL.
type MetadataIndexKey struct {
	Key  string `json:"key"`
	Kind string `json:"kind"`
}

func ParseMetadataIndexKeys(raw, entity string) ([]MetadataIndex, error) {
	if strings.TrimSpace(raw) == "" {
		return []MetadataIndex{}, nil
	}
	if len(raw) > 16384 {
		return nil, fmt.Errorf("indexed metadata keys must fit in 16 KiB")
	}
	var keys []MetadataIndexKey
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&keys); err != nil {
		return nil, fmt.Errorf("indexed metadata keys must be a JSON array: %w", err)
	}
	if keys == nil {
		return nil, fmt.Errorf("indexed metadata keys must be an array; use [] to remove them")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("unexpected data after indexed metadata keys")
	}
	indexes := make([]MetadataIndex, 0, len(keys))
	for _, key := range keys {
		indexes = append(indexes, MetadataIndex{Entity: entity, Key: key.Key, Kind: key.Kind})
	}
	encoded, _ := json.Marshal(indexes)
	return ParseMetadataIndexes(string(encoded))
}
