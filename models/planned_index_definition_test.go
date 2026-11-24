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
		// Removing the parentheses made these equal to the planned three-key list.
		{"one expression key folding the third column in", "CREATE INDEX idx_sim_r1_eff_dist ON resource_similarities (resource_id1, COALESCE(p_distance, hamming_distance, resource_id2))", false},
		{"one parenthesised expression key folding the third column in", "CREATE INDEX idx_sim_r1_eff_dist ON resource_similarities (resource_id1, (COALESCE(p_distance, hamming_distance, resource_id2)))", false},
		{"two keys", "CREATE INDEX idx_sim_r1_eff_dist ON resource_similarities (resource_id1, (COALESCE(p_distance, hamming_distance, resource_id2)), resource_id2)", false},
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

// An index with the planned columns after an extra leading column serves other
// reads: the match is on the whole key list, not on its tail.
func TestPlannedIndexDefinitionRequiresTheExactKeyList(t *testing.T) {
	index := plannedIndex{"idx_jobs_state_phase", "jobs", "state, phase, accepted_at, id"}
	cases := []struct {
		name string
		def  string
		want bool
	}{
		{"postgres exact", "CREATE INDEX idx_jobs_state_phase ON public.jobs USING btree (state, phase, accepted_at, id)", true},
		{"sqlite exact", "CREATE INDEX idx_jobs_state_phase ON jobs (state, phase, accepted_at, id)", true},
		{"postgres extra leading column", "CREATE INDEX idx_jobs_state_phase ON public.jobs USING btree (owner_user_id, state, phase, accepted_at, id)", false},
		{"sqlite extra leading column", "CREATE INDEX idx_jobs_state_phase ON jobs (owner_user_id, state, phase, accepted_at, id)", false},
		{"sqlite extra leading column, no space", "CREATE INDEX idx_jobs_state_phase ON jobs(x,state,phase,accepted_at,id)", false},
		{"partial", "CREATE INDEX idx_jobs_state_phase ON public.jobs USING btree (state, phase, accepted_at, id) WHERE (id > 0)", false},
		{"included column", "CREATE INDEX idx_jobs_state_phase ON public.jobs USING btree (state, phase, accepted_at, id) INCLUDE (name)", false},
		{"truncated", "CREATE INDEX idx_jobs_state_phase ON public.jobs USING btree (phase, accepted_at, id)", false},
		{"same keys on a BRIN index", "CREATE INDEX idx_jobs_state_phase ON public.jobs USING brin (state, phase, accepted_at, id)", false},
		{"same keys on a hash index", "CREATE INDEX idx_jobs_state_phase ON public.jobs USING hash (state, phase, accepted_at, id)", false},
		{"btree in upper case", "CREATE INDEX idx_jobs_state_phase ON public.jobs USING BTREE (state, phase, accepted_at, id)", true},
	}
	for _, c := range cases {
		if got := index.definitionMatches(c.def); got != c.want {
			t.Errorf("%s: definitionMatches = %v, want %v", c.name, got, c.want)
		}
	}
}
