package api_tests

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
	"mahresources/models/query_models"
)

// The members endpoint names every copy's inExtent, false included. The review page
// reads `inExtent === false` to withhold "Include again" from a copy outside the
// selection, and a JSON encoder that omits the false value leaves it looking like one
// inside.
func TestMembersEndpointAlwaysSaysWhetherACopyIsInTheSelection(t *testing.T) {
	tc := SetupTestEnv(t)
	inside := addImage(t, tc, "inside.jpg", 400, 400)
	twin := addImage(t, tc, "twin.jpg", 200, 200)
	outside := addImage(t, tc, "outside.jpg", 800, 800)
	require.NoError(t, tc.DB.Model(&models.Resource{}).
		Where("id IN ?", []uint{inside.ID, twin.ID, outside.ID}).Update("hash", "shared").Error)

	creator := query_models.ResourceReductionCreator{Name: "Outsider", ResourceIds: []uint{inside.ID, twin.ID}}
	response := tc.MakeRequest(http.MethodPost, "/v1/reduction", creator)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var created struct{ ID uint }
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &created))
	plan := computeReduction(t, tc, created.ID)
	require.Len(t, plan.Clusters, 1)

	response = tc.MakeRequest(http.MethodGet, "/v1/reduction/members?id="+strconv.Itoa(int(created.ID))+"&cluster="+plan.Clusters[0].ID, nil)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var page struct {
		Members []map[string]any `json:"members"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
	require.Len(t, page.Members, 3)
	byID := map[float64]map[string]any{}
	for _, m := range page.Members {
		byID[m["resourceId"].(float64)] = m
		assert.Contains(t, m, "inExtent", "every copy says whether it is in the selection")
	}
	assert.Equal(t, false, byID[float64(outside.ID)]["inExtent"])
	assert.Equal(t, true, byID[float64(twin.ID)]["inExtent"])
}

// A kept-copy id past 32 bits is a whole number; it names no copy of the Cluster, so the
// page is a conflict, not a malformed request.
func TestMembersEndpointParsesAWinnerPast32Bits(t *testing.T) {
	tc := SetupTestEnv(t)
	a := addImage(t, tc, "a.jpg", 400, 400)
	b := addImage(t, tc, "b.jpg", 200, 200)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id IN ?", []uint{a.ID, b.ID}).Update("hash", "shared").Error)
	response := tc.MakeRequest(http.MethodPost, "/v1/reduction", query_models.ResourceReductionCreator{Name: "Wide", ResourceIds: []uint{a.ID, b.ID}})
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var created struct{ ID uint }
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &created))
	plan := computeReduction(t, tc, created.ID)
	require.Len(t, plan.Clusters, 1)

	url := "/v1/reduction/members?id=" + strconv.Itoa(int(created.ID)) + "&cluster=" + plan.Clusters[0].ID + "&winner=4294967297"
	response = tc.MakeRequest(http.MethodGet, url, nil)
	assert.Equal(t, http.StatusConflict, response.Code, response.Body.String())
}
