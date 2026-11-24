package api_tests

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// GET /v1/reductions answers from the same one-snapshot read as the HTML list: rows,
// count and each row's progress together, and a Reduction still holding the legacy plan
// (plan_schema 0) is reported as upgrading, with no counts, rather than as an empty plan.
func testReductionListJSONCarriesProgressAndUpgrading(t *testing.T, tc *TestContext) {
	a := addResourceWithBody(t, tc, "a.txt", "aaa")
	current := createReduction(t, tc, "Current plan", []uint{a.ID})
	legacy := &models.ResourceReduction{Name: "Legacy plan", Status: models.ReductionStatusReady, MatchingMode: models.MatchingModeBothTiers}
	require.NoError(t, tc.DB.Create(legacy).Error)
	require.NoError(t, tc.DB.Exec("UPDATE resource_reductions SET plan_schema = 0 WHERE id = ?", legacy.ID).Error)

	rr := tc.MakeRequest(http.MethodGet, "/v1/reductions", nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var body struct {
		Reductions []struct {
			ID   uint   `json:"id"`
			Name string `json:"name"`
		} `json:"reductions"`
		Count    int64                      `json:"count"`
		Page     int                        `json:"page"`
		Progress map[string]json.RawMessage `json:"progress"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body), rr.Body.String())
	assert.EqualValues(t, 2, body.Count)
	assert.Len(t, body.Reductions, 2)

	var legacyProgress, currentProgress struct {
		Clusters  int  `json:"clusters"`
		Upgrading bool `json:"upgrading"`
	}
	require.Contains(t, body.Progress, strconv.Itoa(int(legacy.ID)), "every listed Reduction has a progress entry: %s", rr.Body.String())
	require.NoError(t, json.Unmarshal(body.Progress[strconv.Itoa(int(legacy.ID))], &legacyProgress))
	assert.True(t, legacyProgress.Upgrading, "a plan_schema 0 row is reported as being upgraded")
	assert.Zero(t, legacyProgress.Clusters, "and has no counts")

	require.Contains(t, body.Progress, strconv.Itoa(int(current.ID)))
	require.NoError(t, json.Unmarshal(body.Progress[strconv.Itoa(int(current.ID))], &currentProgress))
	assert.False(t, currentProgress.Upgrading)
}

func TestReductionListJSONCarriesProgressAndUpgradingSQLite(t *testing.T) {
	testReductionListJSONCarriesProgressAndUpgrading(t, SetupTestEnv(t))
}
