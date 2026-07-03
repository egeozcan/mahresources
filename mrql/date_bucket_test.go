package mrql

import (
	"regexp"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func TestDateBucketValidation(t *testing.T) {
	valid := []struct {
		name       string
		query      string
		entityType EntityType
	}{
		{"month aggregated", `type = "note" GROUP BY created.month COUNT()`, EntityNote},
		{"month aggregated ordered", `type = "note" GROUP BY created.month COUNT() ORDER BY created.month ASC`, EntityNote},
		{"week aggregated", `type = "resource" GROUP BY updated.week COUNT()`, EntityResource},
		{"day bucketed", `type = "resource" GROUP BY created.day`, EntityResource},
		{"year with fields", `type = "group" GROUP BY created.year COUNT()`, EntityGroup},
		{"bucketed order by bucket key", `type = "resource" GROUP BY created.month ORDER BY created.month ASC`, EntityResource},
		{"order by count key", `type = "resource" GROUP BY created.month COUNT() ORDER BY count DESC`, EntityResource},
	}
	for _, tc := range valid {
		t.Run("valid/"+tc.name, func(t *testing.T) {
			if err := parseAndValidate(t, tc.query, tc.entityType); err != nil {
				t.Fatalf("expected valid, got: %v", err)
			}
		})
	}

	whereErr := `date bucket fields are only valid in GROUP BY; use a date range in WHERE (created >= "2026-07-01" AND created < "2026-08-01")`

	invalid := []struct {
		name       string
		query      string
		entityType EntityType
		wantSubstr string
	}{
		{"bucket in WHERE", `created.month = "2026-07"`, EntityResource, whereErr},
		{"bucket in WHERE comparison", `updated.year > 2020`, EntityResource, whereErr},
		{"bucket in WHERE IN", `created.month IN ("2026-07")`, EntityResource, whereErr},
		{"bucket in WHERE IS", `created.month IS EMPTY`, EntityResource, whereErr},
		{"bucket order by without group by", `type = "resource" ORDER BY created.month ASC`, EntityResource, whereErr},
		{"unknown suffix", `type = "resource" GROUP BY created.hour COUNT()`, EntityResource, ``},
		{"bucket on non-date field", `type = "resource" GROUP BY name.month COUNT()`, EntityResource, ``},
	}
	for _, tc := range invalid {
		t.Run("invalid/"+tc.name, func(t *testing.T) {
			err := parseAndValidate(t, tc.query, tc.entityType)
			if err == nil {
				t.Fatalf("expected validation error for %q, got nil", tc.query)
			}
			if tc.wantSubstr != "" && !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error mismatch:\nwant substring: %s\ngot: %v", tc.wantSubstr, err)
			}
		})
	}
}

func TestDateBucketExprsSQLite(t *testing.T) {
	db := setupTestDB(t)
	tc := &translateContext{db: db, entityType: EntityResource, tableName: "resources"}

	cases := []struct {
		field string
		want  string
	}{
		{"created.day", `strftime('%Y-%m-%d', resources.created_at)`},
		{"created.week", `date(resources.created_at, '-6 days', 'weekday 1')`},
		{"created.month", `strftime('%Y-%m', resources.created_at)`},
		{"created.year", `strftime('%Y', resources.created_at)`},
		{"updated.month", `strftime('%Y-%m', resources.updated_at)`},
	}
	for _, c := range cases {
		sel, grp := tc.groupByFieldExprs(c.field)
		if sel != c.want || grp != c.want {
			t.Errorf("%s: expected %q, got select=%q group=%q", c.field, c.want, sel, grp)
		}
	}
}

func TestDateBucketExprsPostgres(t *testing.T) {
	db, err := gorm.Open(mockPostgresDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open mock postgres db: %v", err)
	}
	tc := &translateContext{db: db, entityType: EntityNote, tableName: "notes"}

	cases := []struct {
		field string
		want  string
	}{
		{"created.day", `to_char(notes.created_at, 'YYYY-MM-DD')`},
		{"created.week", `to_char(date_trunc('week', notes.created_at), 'YYYY-MM-DD')`},
		{"created.month", `to_char(notes.created_at, 'YYYY-MM')`},
		{"created.year", `to_char(notes.created_at, 'YYYY')`},
		{"updated.year", `to_char(notes.updated_at, 'YYYY')`},
	}
	for _, c := range cases {
		sel, grp := tc.groupByFieldExprs(c.field)
		if sel != c.want || grp != c.want {
			t.Errorf("%s: expected %q, got select=%q group=%q", c.field, c.want, sel, grp)
		}
	}
}

func TestDateBucketAggregatedExecution(t *testing.T) {
	db := setupTestDB(t)

	q, err := Parse(`type = "resource" GROUP BY created.month COUNT() ORDER BY created.month ASC`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	q.EntityType = EntityResource
	if err := Validate(q); err != nil {
		t.Fatalf("validation error: %v", err)
	}
	result, err := TranslateGroupBy(q, db, TranslateOptions{})
	if err != nil {
		t.Fatalf("translate error: %v", err)
	}

	// Seed: 3 resources created now, 1 created 45 days ago → 2 month buckets.
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 month buckets, got %d: %v", len(result.Rows), result.Rows)
	}

	monthLabel := regexp.MustCompile(`^\d{4}-\d{2}$`)
	for _, row := range result.Rows {
		label, ok := deref(row["created.month"]).(string)
		if !ok || !monthLabel.MatchString(label) {
			t.Errorf("expected YYYY-MM label, got %v", deref(row["created.month"]))
		}
	}

	// Ordered ASC: older month first with count 1, newer month with count 3.
	first, second := result.Rows[0], result.Rows[1]
	if toInt(t, first["count"]) != 1 || toInt(t, second["count"]) != 3 {
		t.Errorf("expected counts [1, 3], got [%v, %v]", deref(first["count"]), deref(second["count"]))
	}
	if deref(first["created.month"]).(string) >= deref(second["created.month"]).(string) {
		t.Errorf("expected ascending month order, got %v then %v", deref(first["created.month"]), deref(second["created.month"]))
	}
}

func TestDateBucketBucketedExecution(t *testing.T) {
	db := setupTestDB(t)

	q, err := Parse(`type = "resource" GROUP BY created.month ORDER BY created.month ASC`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	q.EntityType = EntityResource
	if err := Validate(q); err != nil {
		t.Fatalf("validation error: %v", err)
	}

	keys, err := TranslateGroupByKeys(q, db, TranslateOptions{})
	if err != nil {
		t.Fatalf("keys error: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 month keys, got %d: %v", len(keys), keys)
	}

	// Materialize one bucket and check the items belong to it.
	bucketDB, err := TranslateGroupByBucket(q, db, keys[0], TranslateOptions{})
	if err != nil {
		t.Fatalf("bucket error: %v", err)
	}
	var resources []testResource
	if err := bucketDB.Find(&resources).Error; err != nil {
		t.Fatalf("bucket query error: %v", err)
	}
	if len(resources) == 0 {
		t.Fatal("expected items in first month bucket")
	}
}

func TestDateBucketWeekLabelSortable(t *testing.T) {
	db := setupTestDB(t)

	q, err := Parse(`type = "resource" GROUP BY created.week COUNT() ORDER BY created.week ASC`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	q.EntityType = EntityResource
	if err := Validate(q); err != nil {
		t.Fatalf("validation error: %v", err)
	}
	result, err := TranslateGroupBy(q, db, TranslateOptions{})
	if err != nil {
		t.Fatalf("translate error: %v", err)
	}

	weekLabel := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	prev := ""
	for _, row := range result.Rows {
		label, ok := deref(row["created.week"]).(string)
		if !ok || !weekLabel.MatchString(label) {
			t.Fatalf("expected YYYY-MM-DD week label, got %v", deref(row["created.week"]))
		}
		if prev != "" && label < prev {
			t.Errorf("week labels not ascending: %s after %s", label, prev)
		}
		prev = label
	}
}

// deref unwraps *interface{} values that GORM's map scan produces for
// computed (expression) columns. encoding/json does this transparently for
// API responses; tests inspecting rows directly must do it themselves.
func deref(v any) any {
	if p, ok := v.(*any); ok {
		return *p
	}
	return v
}

// toInt normalizes SQLite/PG numeric scan types in row maps.
func toInt(t *testing.T, v any) int64 {
	t.Helper()
	v = deref(v)
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		t.Fatalf("unexpected numeric type %T (%v)", v, v)
		return 0
	}
}
