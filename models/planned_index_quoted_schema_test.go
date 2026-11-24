package models

import "testing"

// A schema name that needs quoting puts a space (or a keyword like "on") in the
// definition Postgres prints, and readiness must still recognise the index.
func TestPlannedIndexDefinitionAcceptsAQuotedSchema(t *testing.T) {
	index := similarityDistanceIndexes[0]
	for _, def := range []string{
		`CREATE INDEX idx_sim_r1_eff_dist ON "my library".resource_similarities USING btree (resource_id1, COALESCE(p_distance, hamming_distance), resource_id2)`,
		`CREATE INDEX idx_sim_r1_eff_dist ON "a ""b"" on c".resource_similarities USING btree (resource_id1, COALESCE(p_distance, hamming_distance), resource_id2)`,
		`CREATE INDEX "idx on x" ON "my library"."resource similarities" USING btree (resource_id1, COALESCE(p_distance, hamming_distance), resource_id2)`,
	} {
		if !index.definitionMatches(def) {
			t.Errorf("a valid definition was refused: %s", def)
		}
	}
	if index.definitionMatches(`CREATE INDEX idx_sim_r1_eff_dist ON "my library".resource_similarities USING btree (resource_id1)`) {
		t.Error("a quoted schema made another column list match")
	}
}
