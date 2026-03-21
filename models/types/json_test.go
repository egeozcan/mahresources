package types

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ── JSON type tests (Value / Scan / Marshal / Unmarshal) ─────────────────────

func TestJSON_Value_Empty(t *testing.T) {
	j := JSON{}
	val, err := j.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != nil {
		t.Fatalf("expected nil, got %v", val)
	}
}

func TestJSON_Value_ValidJSON(t *testing.T) {
	j := JSON(`{"key":"value"}`)
	val, err := j.Value()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string, got %T", val)
	}
	if s != `{"key":"value"}` {
		t.Fatalf("expected %q, got %q", `{"key":"value"}`, s)
	}
}

func TestJSON_Scan_Nil(t *testing.T) {
	var j JSON
	if err := j.Scan(nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(j) != "null" {
		t.Fatalf("expected %q, got %q", "null", string(j))
	}
}

func TestJSON_Scan_Bytes(t *testing.T) {
	var j JSON
	if err := j.Scan([]byte(`{"a":1}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(j) != `{"a":1}` {
		t.Fatalf("expected %q, got %q", `{"a":1}`, string(j))
	}
}

func TestJSON_Scan_String(t *testing.T) {
	var j JSON
	if err := j.Scan(`[1,2,3]`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(j) != `[1,2,3]` {
		t.Fatalf("expected %q, got %q", `[1,2,3]`, string(j))
	}
}

func TestJSON_Scan_UnsupportedType(t *testing.T) {
	var j JSON
	err := j.Scan(12345)
	if err == nil {
		t.Fatal("expected error for unsupported type, got nil")
	}
}

func TestJSON_Scan_PreservesRawBytes(t *testing.T) {
	// Scan no longer re-parses through json.Unmarshal (matches upstream).
	// The DB already stores valid JSON, so Scan just copies the bytes.
	var j JSON
	if err := j.Scan([]byte(`{  "spaced": true  }`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Whitespace is preserved (no re-marshaling)
	if string(j) != `{  "spaced": true  }` {
		t.Fatalf("expected preserved whitespace, got %q", string(j))
	}
}

func TestJSON_Scan_EmptyBytes(t *testing.T) {
	var j JSON
	if err := j.Scan([]byte{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(j) != 0 {
		t.Fatalf("expected empty JSON for empty bytes, got %q", string(j))
	}
}

func TestJSON_MarshalUnmarshal_RoundTrip(t *testing.T) {
	original := JSON(`{"nested":{"key":"val"},"arr":[1,2]}`)
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded JSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if string(decoded) != string(original) {
		t.Fatalf("round-trip mismatch: got %q, want %q", string(decoded), string(original))
	}
}

func TestJSON_String(t *testing.T) {
	j := JSON(`{"x":1}`)
	if j.String() != `{"x":1}` {
		t.Fatalf("expected %q, got %q", `{"x":1}`, j.String())
	}
}

func TestJSON_GormDataType(t *testing.T) {
	j := JSON(`{}`)
	if j.GormDataType() != "json" {
		t.Fatalf("expected %q, got %q", "json", j.GormDataType())
	}
}

func TestJSON_GormDBDataType_SQLite(t *testing.T) {
	db := openTestDB(t)
	j := JSON(`{}`)
	dt := j.GormDBDataType(db, nil)
	if dt != "JSON" {
		t.Fatalf("expected %q for sqlite, got %q", "JSON", dt)
	}
}

func TestJSON_ValueDriverInterface(t *testing.T) {
	// Verify JSON implements driver.Valuer
	var _ driver.Valuer = JSON(`{}`)
}

func TestJSON_GormValue_Empty_ReturnsNULL(t *testing.T) {
	j := JSON{}
	db := openTestDB(t)
	expr := j.GormValue(context.Background(), db)
	if expr.SQL != "NULL" {
		t.Fatalf("expected SQL NULL for empty JSON, got %q", expr.SQL)
	}
}

func TestJSON_GormValue_NonEmpty(t *testing.T) {
	j := JSON(`{"a":1}`)
	db := openTestDB(t)
	expr := j.GormValue(context.Background(), db)
	if expr.SQL != "?" {
		t.Fatalf("expected ? placeholder, got %q", expr.SQL)
	}
}

// ── JSONQueryExpression tests ────────────────────────────────────────────────

func TestJSONQuery_HasKey_SingleKey(t *testing.T) {
	sql, vars := buildResult(t, JSONQuery("meta").HasKey("name"))
	assertContains(t, sql, `JSON_EXTRACT`)
	assertContains(t, sql, `IS NOT NULL`)
	assertVarContains(t, vars, "$.name")
}

func TestJSONQuery_HasKey_NestedKeys(t *testing.T) {
	sql, vars := buildResult(t, JSONQuery("meta").HasKey("user", "profile", "name"))
	assertContains(t, sql, `IS NOT NULL`)
	assertVarContains(t, vars, "$.user.profile.name")
}

func TestJSONQuery_Operation_Equals_String(t *testing.T) {
	sql, vars := buildResult(t, JSONQuery("meta").Operation(OperatorEquals, "hello", "key"))
	assertContains(t, sql, `JSON_EXTRACT`)
	assertContains(t, sql, `) = `)
	assertVarContains(t, vars, "$.key")
	assertVarContains(t, vars, "hello")
}

func TestJSONQuery_Operation_Equals_Float(t *testing.T) {
	sql, vars := buildResult(t, JSONQuery("meta").Operation(OperatorEquals, float64(42), "count"))
	assertContains(t, sql, `) = `)
	assertVarContains(t, vars, "$.count")
	assertVarContains(t, vars, float64(42))
}

func TestJSONQuery_Operation_Equals_Bool(t *testing.T) {
	sql, _ := buildResult(t, JSONQuery("meta").Operation(OperatorEquals, true, "active"))
	assertContains(t, sql, `) = `)
	// Bools are written inline (not as bind vars) in the sqlite/mysql path
	assertContains(t, sql, `true`)
}

func TestJSONQuery_Operation_NotEquals(t *testing.T) {
	sql, _ := buildResult(t, JSONQuery("meta").Operation(OperatorNotEquals, "x", "key"))
	assertContains(t, sql, `) <> `)
}

func TestJSONQuery_Operation_GreaterThan(t *testing.T) {
	sql, _ := buildResult(t, JSONQuery("meta").Operation(OperatorGreaterThan, float64(10), "score"))
	assertContains(t, sql, `) > `)
}

func TestJSONQuery_Operation_LessThanOrEquals(t *testing.T) {
	sql, _ := buildResult(t, JSONQuery("meta").Operation(OperatorLessThanOrEquals, float64(5), "level"))
	assertContains(t, sql, `) <= `)
}

func TestJSONQuery_Operation_Like_FirstCall(t *testing.T) {
	sql, vars := buildResult(t, JSONQuery("meta").Operation(OperatorLike, "foo", "name"))
	assertContains(t, sql, `) LIKE `)
	assertVarContains(t, vars, "%foo%")
}

func TestJSONQuery_Operation_NotLike(t *testing.T) {
	sql, vars := buildResult(t, JSONQuery("meta").Operation(OperatorNotLike, "bar", "tag"))
	assertContains(t, sql, `) NOT LIKE `)
	assertVarContains(t, vars, "%bar%")
}

func TestJSONQuery_Operation_DotSeparatedKey(t *testing.T) {
	expr := JSONQuery("meta").Operation(OperatorEquals, "v", "a.b.c")
	// Verify the key was split
	if len(expr.keys) != 3 || expr.keys[0] != "a" || expr.keys[1] != "b" || expr.keys[2] != "c" {
		t.Fatalf("expected keys [a b c], got %v", expr.keys)
	}
	_, vars := buildResult(t, expr)
	assertVarContains(t, vars, "$.a.b.c")
}

func TestJSONQuery_Operation_MultipleKeys(t *testing.T) {
	expr := JSONQuery("meta").Operation(OperatorEquals, "v", "a", "b", "c")
	if len(expr.keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(expr.keys))
	}
	_, vars := buildResult(t, expr)
	assertVarContains(t, vars, "$.a.b.c")
}

func TestJSONQuery_EmptyKeys_NoOutput(t *testing.T) {
	sql, _ := buildResult(t, JSONQuery("meta").Operation(OperatorEquals, "v"))
	if strings.TrimSpace(sql) != "" {
		t.Fatalf("expected empty SQL for no keys, got %q", sql)
	}
}

func TestJSONQuery_HasKey_EmptyKeys_NoOutput(t *testing.T) {
	sql, _ := buildResult(t, JSONQuery("meta").HasKey())
	if strings.TrimSpace(sql) != "" {
		t.Fatalf("expected empty SQL for HasKey with no keys, got %q", sql)
	}
}

// ── Bug regression: LIKE value mutation on repeated Build() calls ────────────

func TestBuild_LikeValueNotMutatedOnRepeatedCalls(t *testing.T) {
	expr := JSONQuery("meta").Operation(OperatorLike, "foo", "name")

	_, vars1 := buildResult(t, expr)
	_, vars2 := buildResult(t, expr)

	// Both calls must produce identical bind variables.
	// Before the fix, the second call would double-wrap: %foo% → %%foo%%
	v1 := fmt.Sprint(vars1)
	v2 := fmt.Sprint(vars2)
	if v1 != v2 {
		t.Fatalf("Build() mutated value between calls.\n  1st vars: %v\n  2nd vars: %v", vars1, vars2)
	}
}

func TestBuild_NotLikeValueNotMutatedOnRepeatedCalls(t *testing.T) {
	expr := JSONQuery("meta").Operation(OperatorNotLike, "bar", "tag")

	_, vars1 := buildResult(t, expr)
	_, vars2 := buildResult(t, expr)

	v1 := fmt.Sprint(vars1)
	v2 := fmt.Sprint(vars2)
	if v1 != v2 {
		t.Fatalf("Build() mutated value between calls.\n  1st vars: %v\n  2nd vars: %v", vars1, vars2)
	}
}

// ── Bug regression: SQLite nil value produces = NULL instead of IS NULL ──────

func TestBuild_SQLite_EqualsNil_UsesISNULL(t *testing.T) {
	sql, _ := buildResult(t, JSONQuery("meta").Operation(OperatorEquals, nil, "key"))
	// Must use IS NULL, not = NULL (which is always UNKNOWN in SQL)
	assertContains(t, sql, "IS NULL")
	if strings.Contains(sql, "= NULL") {
		t.Fatalf("expected IS NULL, got = NULL in: %s", sql)
	}
}

func TestBuild_SQLite_NotEqualsNil_UsesISNOTNULL(t *testing.T) {
	sql, _ := buildResult(t, JSONQuery("meta").Operation(OperatorNotEquals, nil, "key"))
	// Must use IS NOT NULL, not <> NULL
	assertContains(t, sql, "IS NOT NULL")
	if strings.Contains(sql, "<> NULL") {
		t.Fatalf("expected IS NOT NULL, got <> NULL in: %s", sql)
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	return db
}

// buildResult creates a fresh GORM statement, calls Build on the expression,
// and returns both the generated SQL fragment and bind variables.
func buildResult(t *testing.T, expr *JSONQueryExpression) (string, []any) {
	t.Helper()
	db := openTestDB(t)
	stmt := db.Statement
	stmt.DB = db
	expr.Build(stmt)
	return stmt.SQL.String(), stmt.Vars
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected SQL to contain %q, got: %q", substr, s)
	}
}

func assertVarContains(t *testing.T, vars []any, want any) {
	t.Helper()
	for _, v := range vars {
		if fmt.Sprint(v) == fmt.Sprint(want) {
			return
		}
	}
	t.Errorf("expected vars to contain %v, got: %v", want, vars)
}
