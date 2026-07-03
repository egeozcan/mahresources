package api_tests

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"mahresources/models"
)

// seedCountingData seeds entities for the Package 1 (counting/aggregation)
// motivating queries: duplicate hashes, note links, group memberships, tags,
// and spread-out creation dates.
func seedCountingData(t *testing.T, tc *TestContext) {
	t.Helper()

	now := time.Now()
	old := now.AddDate(0, -3, 0) // three months ago

	// Resources 1-2 share a hash (duplicates); 3 has a unique hash; 4 is old.
	tc.DB.Create(&models.Resource{Name: "dupA", ContentType: "image/png", Hash: "samehash"})
	tc.DB.Create(&models.Resource{Name: "dupB", ContentType: "image/png", Hash: "samehash"})
	tc.DB.Create(&models.Resource{Name: "uniq", ContentType: "text/plain", Hash: "uniquehash"})
	tc.DB.Create(&models.Resource{Name: "oldres", ContentType: "text/plain", Hash: "oldhash", CreatedAt: old})
	tc.DB.Exec("UPDATE resources SET created_at = ? WHERE name = 'oldres'", old)

	// Notes: one recent, one three months old.
	tc.DB.Create(&models.Note{Name: "recentNote"})
	tc.DB.Create(&models.Note{Name: "oldNote", CreatedAt: old})
	tc.DB.Exec("UPDATE notes SET created_at = ? WHERE name = 'oldNote'", old)

	// Groups: busy (2 resources), quiet (1 resource), empty (0).
	tc.DB.Create(&models.Group{Name: "busyGroup"})
	tc.DB.Create(&models.Group{Name: "quietGroup"})
	tc.DB.Create(&models.Group{Name: "emptyGroup"})

	// Tags: dupA gets one tag; the rest untagged.
	tc.DB.Create(&models.Tag{Name: "countTag"})
	tc.DB.Exec("INSERT INTO resource_tags (resource_id, tag_id) SELECT r.id, t.id FROM resources r, tags t WHERE r.name = 'dupA' AND t.name = 'countTag'")

	// resource_notes: dupA linked to recentNote.
	tc.DB.Exec("INSERT INTO resource_notes (resource_id, note_id) SELECT r.id, n.id FROM resources r, notes n WHERE r.name = 'dupA' AND n.name = 'recentNote'")

	// groups_related_resources: busyGroup ↔ dupA + dupB, quietGroup ↔ uniq.
	tc.DB.Exec("INSERT INTO groups_related_resources (group_id, resource_id) SELECT g.id, r.id FROM groups g, resources r WHERE g.name = 'busyGroup' AND r.name IN ('dupA', 'dupB')")
	tc.DB.Exec("INSERT INTO groups_related_resources (group_id, resource_id) SELECT g.id, r.id FROM groups g, resources r WHERE g.name = 'quietGroup' AND r.name = 'uniq'")
}

func runMRQL(t *testing.T, tc *TestContext, query string) map[string]any {
	t.Helper()
	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", map[string]any{"query": query})
	assert.Equal(t, http.StatusOK, resp.Code, "query %q failed: %s", query, resp.Body.String())

	var result map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	assert.NoError(t, err)
	return result
}

// TestMRQL_HavingDuplicateHashes covers:
// type = resource GROUP BY hash COUNT() HAVING COUNT() > 1 ORDER BY count DESC
func TestMRQL_HavingDuplicateHashes(t *testing.T) {
	tc := setupMRQLTest(t)
	seedCountingData(t, tc)

	result := runMRQL(t, tc, `type = "resource" GROUP BY hash COUNT() HAVING COUNT() > 1 ORDER BY count DESC`)
	assert.Equal(t, "aggregated", result["mode"])

	rows, ok := result["rows"].([]any)
	assert.True(t, ok, "expected rows array, got %v", result)
	if assert.Len(t, rows, 1, "only the duplicated hash should survive HAVING") {
		row := rows[0].(map[string]any)
		assert.Equal(t, "samehash", row["hash"])
		assert.EqualValues(t, 2, row["count"])
	}
}

// TestMRQL_NotesIsEmptyRecent covers:
// type = resource AND notes IS EMPTY AND created > -30d
func TestMRQL_NotesIsEmptyRecent(t *testing.T) {
	tc := setupMRQLTest(t)
	seedCountingData(t, tc)

	result := runMRQL(t, tc, `type = "resource" AND notes IS EMPTY AND created > -30d ORDER BY name ASC`)
	resources, ok := result["resources"].([]any)
	assert.True(t, ok, "expected resources array, got %v", result)

	// dupB and uniq are recent and have no notes; dupA has a note; oldres is old.
	names := make([]string, 0, len(resources))
	for _, r := range resources {
		names = append(names, r.(map[string]any)["Name"].(string))
	}
	assert.Equal(t, []string{"dupB", "uniq"}, names)
}

// TestMRQL_GroupResourceCountOrdering covers:
// type = group AND resources.count >= N ORDER BY resources.count DESC
func TestMRQL_GroupResourceCountOrdering(t *testing.T) {
	tc := setupMRQLTest(t)
	seedCountingData(t, tc)

	result := runMRQL(t, tc, `type = "group" AND resources.count >= 1 ORDER BY resources.count DESC`)
	groups, ok := result["groups"].([]any)
	assert.True(t, ok, "expected groups array, got %v", result)
	if assert.Len(t, groups, 2, "emptyGroup must be filtered out") {
		assert.Equal(t, "busyGroup", groups[0].(map[string]any)["Name"])
		assert.Equal(t, "quietGroup", groups[1].(map[string]any)["Name"])
	}
}

// TestMRQL_NotesPerMonth covers:
// type = note GROUP BY created.month COUNT() ORDER BY created.month ASC
func TestMRQL_NotesPerMonth(t *testing.T) {
	tc := setupMRQLTest(t)
	seedCountingData(t, tc)

	result := runMRQL(t, tc, `type = "note" GROUP BY created.month COUNT() ORDER BY created.month ASC`)
	assert.Equal(t, "aggregated", result["mode"])

	rows, ok := result["rows"].([]any)
	assert.True(t, ok, "expected rows array, got %v", result)
	if assert.Len(t, rows, 2, "expected two month buckets") {
		first := rows[0].(map[string]any)
		second := rows[1].(map[string]any)
		assert.Regexp(t, `^\d{4}-\d{2}$`, first["created.month"])
		assert.Regexp(t, `^\d{4}-\d{2}$`, second["created.month"])
		assert.Less(t, first["created.month"].(string), second["created.month"].(string))
		assert.EqualValues(t, 1, first["count"])
		assert.EqualValues(t, 1, second["count"])
	}
}

// TestMRQL_TagCountZeroAndGroupsEmpty covers:
// type = resource AND tags.count = 0 AND groups IS EMPTY
func TestMRQL_TagCountZeroAndGroupsEmpty(t *testing.T) {
	tc := setupMRQLTest(t)
	seedCountingData(t, tc)

	result := runMRQL(t, tc, `type = "resource" AND tags.count = 0 AND groups IS EMPTY ORDER BY name ASC`)
	resources, ok := result["resources"].([]any)
	assert.True(t, ok, "expected resources array, got %v", result)

	// dupA has a tag; dupB and uniq are in groups; only oldres is untagged AND ungrouped.
	if assert.Len(t, resources, 1) {
		assert.Equal(t, "oldres", resources[0].(map[string]any)["Name"])
	}
}

// TestMRQL_ValidateEndpointNewSyntax ensures /v1/mrql/validate accepts the new
// syntax and rejects misuse with the designed error messages.
func TestMRQL_ValidateEndpointNewSyntax(t *testing.T) {
	tc := setupMRQLTest(t)

	valid := []string{
		`type = "resource" GROUP BY hash COUNT() HAVING COUNT() > 1`,
		`type = "group" AND resources.count >= 100`,
		`type = "note" GROUP BY created.month COUNT() ORDER BY created.month ASC`,
		`type = "resource" AND notes IS EMPTY`,
	}
	for _, q := range valid {
		resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/validate", map[string]any{"query": q})
		assert.Equal(t, http.StatusOK, resp.Code)
		var vr map[string]any
		assert.NoError(t, json.Unmarshal(resp.Body.Bytes(), &vr))
		assert.Equal(t, true, vr["valid"], "expected %q to validate, got %v", q, vr)
	}

	invalid := map[string]string{
		`type = "resource" AND owner.count > 1`:               "single reference",
		`type = "resource" AND tags.count IN (1, 2)`:          "only supports comparison operators",
		`type = "resource" AND created.month = "2026-07"`:     "only valid in GROUP BY",
		`type = "resource" GROUP BY hash HAVING COUNT() > 1`:  "HAVING requires at least one aggregate",
		`type = "resource" GROUP BY hash COUNT() HAVING name`: "aggregate functions",
	}
	for q, wantSubstr := range invalid {
		resp := tc.MakeRequest(http.MethodPost, "/v1/mrql/validate", map[string]any{"query": q})
		assert.Equal(t, http.StatusOK, resp.Code)
		var vr map[string]any
		assert.NoError(t, json.Unmarshal(resp.Body.Bytes(), &vr))
		assert.Equal(t, false, vr["valid"], "expected %q to be invalid", q)
		b, _ := json.Marshal(vr["errors"])
		assert.Contains(t, string(b), wantSubstr, "for query %q", q)
	}
}
