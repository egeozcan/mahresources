//go:build postgres

package api_tests

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// The distance lookup's COALESCE over smallint columns and its UNION ALL with
// IN lists, and the rendered order, behave on Postgres as on SQLite.
func TestSimilarShortcode_Postgres(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	f := buildSimilarOrderFixture(t, tc)

	got, err := tc.AppCtx.SimilarityDistancesTo(context.Background(), f.target.ID, []uint{f.d1.ID, f.d4.ID, f.d5.ID, f.d9.ID})
	if err != nil {
		t.Fatal(err)
	}
	want := map[uint]int{f.d1.ID: 1, f.d4.ID: 4, f.d5.ID: 5, f.d9.ID: 9}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("distances = %v, want %v", got, want)
	}
	// Both stored directions resolve.
	got, err = tc.AppCtx.SimilarityDistancesTo(context.Background(), f.d1.ID, []uint{f.target.ID})
	if err != nil || got[f.target.ID] != 1 {
		t.Fatalf("reverse direction: %v, %v", got, err)
	}

	out := renderSimilarFor(t, tc.AppCtx, f.target, "[similar]")
	last := -1
	for _, step := range []struct {
		id    uint
		badge string
	}{{f.d1.ID, "Duplicate · 1"}, {f.d4.ID, "Similar · 4"}, {f.d5.ID, "Similar · 5"}, {f.d9.ID, "Similar · 9"}} {
		idx := strings.Index(out, fmt.Sprintf(`href="/resource?id=%d"`, step.id))
		if idx < last || idx < 0 {
			t.Fatalf("order on Postgres: %s", out)
		}
		last = idx
		if !strings.Contains(out, step.badge) {
			t.Errorf("no %q: %s", step.badge, out)
		}
	}
}
