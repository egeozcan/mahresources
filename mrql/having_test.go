package mrql

import (
	"strings"
	"testing"
)

func TestLexerHavingKeyword(t *testing.T) {
	l := NewLexer(`HAVING having`)
	tok := l.Next()
	if tok.Type != TokenHaving {
		t.Fatalf("expected TokenHaving for HAVING, got %v", tok)
	}
	tok = l.Next()
	if tok.Type != TokenHaving {
		t.Fatalf("expected TokenHaving for lowercase having, got %v", tok)
	}
}

func TestParseHavingSimple(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY hash COUNT() HAVING COUNT() > 1`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.GroupBy == nil || q.GroupBy.Having == nil {
		t.Fatal("expected GroupBy.Having to be set")
	}
	hc, ok := q.GroupBy.Having.(*HavingComparison)
	if !ok {
		t.Fatalf("expected *HavingComparison, got %T", q.GroupBy.Having)
	}
	if hc.Agg.Name != "COUNT" {
		t.Errorf("expected COUNT aggregate, got %s", hc.Agg.Name)
	}
	if hc.Operator.Type != TokenGt {
		t.Errorf("expected > operator, got %v", hc.Operator)
	}
	nl, ok := hc.Value.(*NumberLiteral)
	if !ok || nl.Value != 1 {
		t.Errorf("expected NumberLiteral 1, got %v", hc.Value)
	}
}

func TestParseHavingBooleanStructure(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY tags COUNT() SUM(fileSize) HAVING SUM(fileSize) > 1gb AND COUNT() >= 10`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	be, ok := q.GroupBy.Having.(*BinaryExpr)
	if !ok {
		t.Fatalf("expected *BinaryExpr, got %T", q.GroupBy.Having)
	}
	if be.Operator.Type != TokenAnd {
		t.Errorf("expected AND, got %v", be.Operator)
	}
	left, ok := be.Left.(*HavingComparison)
	if !ok || left.Agg.Name != "SUM" {
		t.Errorf("expected left SUM comparison, got %T", be.Left)
	}
	if left != nil && left.Agg.Field.Name() != "fileSize" {
		t.Errorf("expected SUM field fileSize, got %q", left.Agg.Field.Name())
	}
}

func TestParseHavingPrecedenceAndParens(t *testing.T) {
	// AND binds tighter than OR
	q, err := Parse(`type = "resource" GROUP BY hash COUNT() HAVING COUNT() > 10 OR COUNT() < 5 AND COUNT() != 3`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	be, ok := q.GroupBy.Having.(*BinaryExpr)
	if !ok || be.Operator.Type != TokenOr {
		t.Fatalf("expected top-level OR, got %T", q.GroupBy.Having)
	}
	if _, ok := be.Right.(*BinaryExpr); !ok {
		t.Errorf("expected right side of OR to be AND expression, got %T", be.Right)
	}

	// NOT and parentheses
	q, err = Parse(`type = "note" GROUP BY noteType COUNT() HAVING NOT (COUNT() < 5)`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	ne, ok := q.GroupBy.Having.(*NotExpr)
	if !ok {
		t.Fatalf("expected *NotExpr, got %T", q.GroupBy.Having)
	}
	if _, ok := ne.Expr.(*HavingComparison); !ok {
		t.Errorf("expected NOT to wrap a HavingComparison, got %T", ne.Expr)
	}
}

func TestParseHavingFollowedByOrderByAndLimit(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY hash COUNT() HAVING COUNT() > 1 ORDER BY count DESC LIMIT 10`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if q.GroupBy.Having == nil {
		t.Fatal("expected Having to be set")
	}
	if len(q.OrderBy) != 1 || q.OrderBy[0].Field.Name() != "count" {
		t.Errorf("expected ORDER BY count, got %v", q.OrderBy)
	}
	if q.Limit != 10 {
		t.Errorf("expected LIMIT 10, got %d", q.Limit)
	}
}

func TestParseHavingErrors(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		wantSubstr string
	}{
		{"plain field", `type = "resource" GROUP BY hash COUNT() HAVING name = "x"`, `HAVING conditions must use aggregate functions; filter plain fields in the WHERE clause instead`},
		{"missing operator", `type = "resource" GROUP BY hash COUNT() HAVING COUNT()`, `expected comparison operator`},
		{"like operator", `type = "resource" GROUP BY hash COUNT() HAVING COUNT() ~ 1`, `expected comparison operator`},
		{"no group by", `type = "resource" HAVING COUNT() > 1`, `HAVING requires a preceding GROUP BY clause`},
		{"unclosed paren", `type = "resource" GROUP BY hash COUNT() HAVING (COUNT() > 1`, `expected ')'`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.query)
			if err == nil {
				t.Fatalf("expected parse error for %q, got nil", tc.query)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error mismatch:\nwant substring: %s\ngot: %v", tc.wantSubstr, err)
			}
		})
	}
}

func TestValidateHaving(t *testing.T) {
	valid := []struct {
		name       string
		query      string
		entityType EntityType
	}{
		{"count having", `type = "resource" GROUP BY hash COUNT() HAVING COUNT() > 1`, EntityResource},
		{"sum having with unit", `type = "resource" GROUP BY tags COUNT() HAVING SUM(fileSize) > 1gb`, EntityResource},
		{"having agg not in select list", `type = "resource" GROUP BY hash COUNT() HAVING SUM(fileSize) > 100`, EntityResource},
		{"minmax datetime having date value", `type = "resource" GROUP BY tags COUNT() HAVING MAX(created) < -1y`, EntityResource},
		{"minmax datetime having string date", `type = "resource" GROUP BY tags COUNT() HAVING MIN(created) > "2020-01-01"`, EntityResource},
		{"not having", `type = "note" GROUP BY noteType COUNT() HAVING NOT (COUNT() < 5)`, EntityNote},
	}
	for _, tc := range valid {
		t.Run("valid/"+tc.name, func(t *testing.T) {
			if err := parseAndValidate(t, tc.query, tc.entityType); err != nil {
				t.Fatalf("expected valid, got: %v", err)
			}
		})
	}

	invalid := []struct {
		name       string
		query      string
		entityType EntityType
		wantSubstr string
	}{
		{"no aggregates", `type = "resource" GROUP BY hash HAVING COUNT() > 1`, EntityResource, `HAVING requires at least one aggregate function in GROUP BY (e.g. GROUP BY hash COUNT() HAVING COUNT() > 1)`},
		{"sum on string field", `type = "resource" GROUP BY hash COUNT() HAVING SUM(name) > 1`, EntityResource, `SUM requires a numeric field`},
		{"min on relation", `type = "resource" GROUP BY hash COUNT() HAVING MIN(tags) > 1`, EntityResource, `MIN requires a numeric or datetime field`},
		{"sum unknown field", `type = "resource" GROUP BY hash COUNT() HAVING SUM(nosuchfield) > 1`, EntityResource, `unknown`},
		{"count string value", `type = "resource" GROUP BY hash COUNT() HAVING COUNT() > "many"`, EntityResource, `numeric`},
		{"sum date value", `type = "resource" GROUP BY hash COUNT() HAVING SUM(fileSize) > -7d`, EntityResource, `numeric`},
	}
	for _, tc := range invalid {
		t.Run("invalid/"+tc.name, func(t *testing.T) {
			err := parseAndValidate(t, tc.query, tc.entityType)
			if err == nil {
				t.Fatalf("expected validation error for %q, got nil", tc.query)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error mismatch:\nwant substring: %s\ngot: %v", tc.wantSubstr, err)
			}
		})
	}
}

func TestHavingExecution(t *testing.T) {
	db := setupTestDB(t)

	run := func(t *testing.T, query string, entityType EntityType) *GroupByResult {
		t.Helper()
		q, err := Parse(query)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		q.EntityType = entityType
		if err := Validate(q); err != nil {
			t.Fatalf("validation error: %v", err)
		}
		result, err := TranslateGroupBy(q, db, TranslateOptions{})
		if err != nil {
			t.Fatalf("translate error: %v", err)
		}
		return result
	}

	t.Run("count filter keeps only matching buckets", func(t *testing.T) {
		// contentType buckets: image/jpeg=1, image/png=1, application/pdf=1, text/plain=1
		// so COUNT() > 1 yields nothing; COUNT() >= 1 yields 4 buckets.
		result := run(t, `type = "resource" GROUP BY contentType COUNT() HAVING COUNT() > 1`, EntityResource)
		if len(result.Rows) != 0 {
			t.Fatalf("expected 0 rows for COUNT() > 1, got %d: %v", len(result.Rows), result.Rows)
		}
		result = run(t, `type = "resource" GROUP BY contentType COUNT() HAVING COUNT() >= 1`, EntityResource)
		if len(result.Rows) != 4 {
			t.Fatalf("expected 4 rows for COUNT() >= 1, got %d: %v", len(result.Rows), result.Rows)
		}
	})

	t.Run("sum filter with byte unit", func(t *testing.T) {
		// fileSize per contentType: image/jpeg=1024000, image/png=2048000,
		// application/pdf=512000, text/plain=100. Only image/png exceeds 1mb (1048576).
		result := run(t, `type = "resource" GROUP BY contentType COUNT() HAVING SUM(fileSize) > 1mb ORDER BY contentType ASC`, EntityResource)
		if len(result.Rows) != 1 {
			t.Fatalf("expected 1 row for SUM(fileSize) > 1mb, got %d: %v", len(result.Rows), result.Rows)
		}
		if ct := result.Rows[0]["contentType"]; ct != "image/png" {
			t.Errorf("expected image/png bucket, got %v", ct)
		}
	})

	t.Run("having aggregate not in select list", func(t *testing.T) {
		result := run(t, `type = "resource" GROUP BY contentType COUNT() HAVING SUM(fileSize) < 1000`, EntityResource)
		if len(result.Rows) != 1 {
			t.Fatalf("expected 1 row (text/plain), got %d: %v", len(result.Rows), result.Rows)
		}
	})

	t.Run("boolean having", func(t *testing.T) {
		// NOT (COUNT() < 1) keeps all 4; combined with OR/AND shapes
		result := run(t, `type = "resource" GROUP BY contentType COUNT() HAVING NOT (COUNT() < 1)`, EntityResource)
		if len(result.Rows) != 4 {
			t.Fatalf("expected 4 rows, got %d", len(result.Rows))
		}
		result = run(t, `type = "resource" GROUP BY contentType COUNT() HAVING SUM(fileSize) > 1mb OR SUM(fileSize) < 1000`, EntityResource)
		if len(result.Rows) != 2 {
			t.Fatalf("expected 2 rows, got %d: %v", len(result.Rows), result.Rows)
		}
	})

	t.Run("duplicate hash detection shape", func(t *testing.T) {
		// give two resources the same hash
		db.Exec("UPDATE resources SET hash = 'dup' WHERE id IN (1, 2)")
		db.Exec("UPDATE resources SET hash = 'uniq' WHERE id = 3")
		result := run(t, `type = "resource" GROUP BY hash COUNT() HAVING COUNT() > 1 ORDER BY count DESC`, EntityResource)
		if len(result.Rows) != 1 {
			t.Fatalf("expected 1 duplicate-hash bucket, got %d: %v", len(result.Rows), result.Rows)
		}
		if h := result.Rows[0]["hash"]; h != "dup" {
			t.Errorf("expected hash 'dup', got %v", h)
		}
	})
}

func TestHavingSQLShape(t *testing.T) {
	db := setupTestDB(t)

	q, err := Parse(`type = "resource" GROUP BY contentType COUNT() HAVING SUM(fileSize) > 1gb AND COUNT() >= 10`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	q.EntityType = EntityResource
	if err := Validate(q); err != nil {
		t.Fatalf("validation error: %v", err)
	}

	tc := &translateContext{db: db, entityType: EntityResource, tableName: "resources"}
	sqlStr, vals, err := tc.buildHavingClause(q.GroupBy.Having)
	if err != nil {
		t.Fatalf("buildHavingClause error: %v", err)
	}
	// The aggregate expression is repeated (not the SELECT alias): PostgreSQL
	// does not permit SELECT aliases in HAVING.
	if sqlStr != `SUM(resources.file_size) > ? AND COUNT(*) >= ?` {
		t.Errorf("unexpected HAVING clause: %s", sqlStr)
	}
	if len(vals) != 2 {
		t.Fatalf("expected 2 bound values, got %v", vals)
	}
	// 1gb must bind as bytes
	if vals[0] != int64(1073741824) {
		t.Errorf("expected 1gb to bind as 1073741824 bytes, got %v", vals[0])
	}
}
