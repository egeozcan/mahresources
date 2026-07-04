package mrql

import (
	"testing"
)

// TestBetweenDesugarShape verifies `f BETWEEN a AND b` parses into
// (f >= a AND f <= b) with separate field nodes and synthesized operators.
func TestBetweenDesugarShape(t *testing.T) {
	q := mustParse(t, `fileSize BETWEEN 1mb AND 10mb`)

	bin, ok := q.Where.(*BinaryExpr)
	if !ok {
		t.Fatalf("expected *BinaryExpr, got %T", q.Where)
	}
	if bin.Operator.Type != TokenAnd {
		t.Fatalf("expected AND join, got %v", bin.Operator.Type)
	}

	lo, ok := bin.Left.(*ComparisonExpr)
	if !ok || lo.Operator.Type != TokenGte {
		t.Fatalf("expected left = ComparisonExpr(>=), got %T op=%v", bin.Left, lo.Operator.Type)
	}
	hi, ok := bin.Right.(*ComparisonExpr)
	if !ok || hi.Operator.Type != TokenLte {
		t.Fatalf("expected right = ComparisonExpr(<=), got %T op=%v", bin.Right, hi.Operator.Type)
	}
	if lo.Field.Name() != "fileSize" || hi.Field.Name() != "fileSize" {
		t.Fatalf("expected both fields fileSize, got %q / %q", lo.Field.Name(), hi.Field.Name())
	}
	// Separate FieldExpr nodes (no shared aliasing).
	if lo.Field == hi.Field {
		t.Fatalf("expected separate FieldExpr nodes for the two bounds")
	}
	// Positions of synthesized operators point at the BETWEEN token.
	if lo.Operator.Pos == 0 || lo.Operator.Pos != hi.Operator.Pos {
		t.Fatalf("expected synthesized operators to share the BETWEEN position, got %d / %d", lo.Operator.Pos, hi.Operator.Pos)
	}
}

// TestNotBetweenDesugarShape verifies `f NOT BETWEEN a AND b` wraps the AND in NotExpr.
func TestNotBetweenDesugarShape(t *testing.T) {
	q := mustParse(t, `fileSize NOT BETWEEN 1mb AND 10mb`)
	not, ok := q.Where.(*NotExpr)
	if !ok {
		t.Fatalf("expected *NotExpr, got %T", q.Where)
	}
	if _, ok := not.Expr.(*BinaryExpr); !ok {
		t.Fatalf("expected NotExpr wrapping BinaryExpr, got %T", not.Expr)
	}
}

// TestBetweenMissingAnd surfaces a positioned error when AND is missing.
func TestBetweenMissingAnd(t *testing.T) {
	pe := mustFail(t, `fileSize BETWEEN 1mb 10mb`)
	if pe.Message == "" {
		t.Fatalf("expected error message")
	}
}

// TestBetweenMetaKeyStillParses ensures a meta key literally named "between" works.
func TestBetweenMetaKeyStillParses(t *testing.T) {
	q := mustParse(t, `meta.between = "x"`)
	cmp, ok := q.Where.(*ComparisonExpr)
	if !ok {
		t.Fatalf("expected *ComparisonExpr, got %T", q.Where)
	}
	if cmp.Field.Name() != "meta.between" {
		t.Fatalf("expected field meta.between, got %q", cmp.Field.Name())
	}
}

// TestBetweenExecFileSize runs BETWEEN with unit literals against seeded data.
func TestBetweenExecFileSize(t *testing.T) {
	db := setupTestDB(t)
	// fileSizes: 1024000, 2048000, 512000, 100. Between 1mb(1048576) and 10mb → only 2048000.
	result := parseAndTranslate(t, `type = "resource" AND fileSize BETWEEN 1mb AND 10mb`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 || resources[0].ID != 2 {
		t.Fatalf("expected only resource 2 (2048000 bytes), got %+v", resources)
	}
}

// TestNotBetweenExecFileSize verifies NOT BETWEEN excludes the in-range row.
func TestNotBetweenExecFileSize(t *testing.T) {
	db := setupTestDB(t)
	result := parseAndTranslate(t, `type = "resource" AND fileSize NOT BETWEEN 1mb AND 10mb`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	for _, r := range resources {
		if r.ID == 2 {
			t.Fatalf("NOT BETWEEN should exclude resource 2, got %+v", resources)
		}
	}
	if len(resources) != 3 {
		t.Fatalf("expected 3 rows outside range, got %d", len(resources))
	}
}

// TestBetweenParamBounds binds $lo/$hi params into BETWEEN bounds.
func TestBetweenParamBounds(t *testing.T) {
	q, err := Parse(`type = "resource" AND fileSize BETWEEN $lo AND $hi`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if err := BindParams(q, map[string]any{"lo": 1000000, "hi": 5000000}); err != nil {
		t.Fatalf("bind error: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate error: %v", err)
	}
	db := setupTestDB(t)
	q.EntityType = EntityResource
	result, err := Translate(q, db)
	if err != nil {
		t.Fatalf("translate error: %v", err)
	}
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	// 1024000 and 2048000 are within [1000000, 5000000].
	if len(resources) != 2 {
		t.Fatalf("expected 2 rows in param range, got %d (%+v)", len(resources), resources)
	}
}

// TestBetweenFilterBarAccepts ensures ParseFilter (list-page bar) accepts BETWEEN.
func TestBetweenFilterBarAccepts(t *testing.T) {
	q, err := ParseFilter(EntityResource, `fileSize BETWEEN 1mb AND 10mb`)
	if err != nil {
		t.Fatalf("ParseFilter rejected BETWEEN: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate error: %v", err)
	}
}
