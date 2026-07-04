package mrql

import (
	"strings"
	"testing"
)

// TestParseOrderByRandom verifies RANDOM() parses into an OrderByClause{Random:true}.
func TestParseOrderByRandom(t *testing.T) {
	q := mustParse(t, `type = "resource" ORDER BY RANDOM()`)
	if len(q.OrderBy) != 1 || !q.OrderBy[0].Random {
		t.Fatalf("expected one Random ORDER BY clause, got %+v", q.OrderBy)
	}
	if q.OrderBy[0].Field != nil {
		t.Fatalf("expected nil Field for RANDOM(), got %+v", q.OrderBy[0].Field)
	}
}

// TestParseOrderByRandomTiebreak allows RANDOM() after another key.
func TestParseOrderByRandomTiebreak(t *testing.T) {
	q := mustParse(t, `type = "resource" ORDER BY name, RANDOM()`)
	if len(q.OrderBy) != 2 {
		t.Fatalf("expected two ORDER BY clauses, got %d", len(q.OrderBy))
	}
	if q.OrderBy[0].Random {
		t.Fatalf("first clause should be the name field, not random")
	}
	if !q.OrderBy[1].Random {
		t.Fatalf("second clause should be random")
	}
}

// TestOrderByRandomDirectionRejected rejects a direction after RANDOM().
func TestOrderByRandomDirectionRejected(t *testing.T) {
	pe := mustFail(t, `type = "resource" ORDER BY RANDOM() DESC`)
	if !strings.Contains(pe.Message, "does not take a direction") {
		t.Fatalf("expected direction error, got %q", pe.Message)
	}
}

// TestOrderByRandomGroupByRejected rejects RANDOM() with GROUP BY.
func TestOrderByRandomGroupByRejected(t *testing.T) {
	q := mustParse(t, `type = "resource" GROUP BY hash COUNT() ORDER BY RANDOM()`)
	q.EntityType = EntityResource
	err := Validate(q)
	if err == nil || !strings.Contains(err.Error(), "not supported with GROUP BY") {
		t.Fatalf("expected GROUP BY rejection, got %v", err)
	}
}

// TestOrderByRandomSQL checks the emitted SQL contains ORDER BY RANDOM().
func TestOrderByRandomSQL(t *testing.T) {
	db := setupTestDB(t)
	got := dryRunSQL(t, db, `type = "resource" ORDER BY RANDOM()`, EntityResource)
	if !strings.Contains(strings.ToUpper(got), "ORDER BY RANDOM()") {
		t.Fatalf("expected ORDER BY RANDOM() in SQL, got: %s", got)
	}
}

// TestOrderByRandomExecutionFullSet returns the full row set (just reordered).
func TestOrderByRandomExecutionFullSet(t *testing.T) {
	db := setupTestDB(t)
	result := parseAndTranslate(t, `type = "resource" ORDER BY RANDOM()`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 4 {
		t.Fatalf("expected all 4 resources, got %d", len(resources))
	}
}
