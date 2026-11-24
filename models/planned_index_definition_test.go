package models

import "testing"

// An expression index passes readiness only when its definition is the planned
// one: a cast on a column changes the expression's type, so a query written
// against COALESCE(p_distance, hamming_distance) cannot use it.
func TestPlannedIndexDefinitionRefusesCastsThatChangeTheType(t *testing.T) {
	index := similarityDistanceIndexes[0]
	cases := []struct {
		name string
		def  string
		want bool
	}{
		{"postgres spelling", "CREATE INDEX idx_sim_r1_eff_dist ON public.resource_similarities USING btree (resource_id1, COALESCE(p_distance, hamming_distance), resource_id2)", true},
		{"sqlite spelling", "CREATE INDEX idx_sim_r1_eff_dist ON resource_similarities (resource_id1, (COALESCE(p_distance, hamming_distance)), resource_id2)", true},
		{"text cast", "CREATE INDEX idx_sim_r1_eff_dist ON public.resource_similarities USING btree (resource_id1, COALESCE((p_distance)::text, (hamming_distance)::text), resource_id2)", false},
		{"bigint cast on the columns", "CREATE INDEX idx_sim_r1_eff_dist ON public.resource_similarities USING btree (resource_id1, COALESCE((p_distance)::bigint, (hamming_distance)::bigint), resource_id2)", false},
		{"other columns", "CREATE INDEX idx_sim_r1_eff_dist ON public.resource_similarities USING btree (resource_id1)", false},
	}
	for _, c := range cases {
		if got := index.definitionMatches(c.def); got != c.want {
			t.Errorf("%s: definitionMatches = %v, want %v", c.name, got, c.want)
		}
	}
}

// A cast Postgres writes on a literal is its own spelling, not a different expression.
func TestPlannedIndexDefinitionToleratesCastsOnLiterals(t *testing.T) {
	index := plannedIndex{"idx_x", "t", "a, (COALESCE(b, 0))"}
	for _, def := range []string{
		"CREATE INDEX idx_x ON public.t USING btree (a, COALESCE(b, 0))",
		"CREATE INDEX idx_x ON public.t USING btree (a, COALESCE(b, (0)::bigint))",
		"CREATE INDEX idx_x ON public.t USING btree (a, COALESCE(b, 0::bigint))",
	} {
		if !index.definitionMatches(def) {
			t.Errorf("definition refused: %s", def)
		}
	}
	text := plannedIndex{"idx_y", "t", "a, (COALESCE(b, 'x'))"}
	if !text.definitionMatches("CREATE INDEX idx_y ON public.t USING btree (a, COALESCE(b, 'x'::text))") {
		t.Error("a cast on a text literal was refused")
	}
}
