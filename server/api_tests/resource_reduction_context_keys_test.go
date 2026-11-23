package api_tests

import (
	"encoding/json"
	"fmt"
	"mahresources/contracts"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flosch/pongo2/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/template_handlers/template_context_providers"
)

func reviewPageContext(t *testing.T, c *application_context.MahresourcesContext, id uint, rawQuery string) pongo2.Context {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/reduction?id="+uintToPath(id)+rawQuery, nil)
	page := template_context_providers.ResourceReductionContextProvider(c)(request)
	require.Nil(t, page["errorMessage"], "%v", page["errorMessage"])
	return page
}

// The page's context keys the review UI reads, on the default (Open) view.
func TestReviewPageContextCarriesTheNewKeys(t *testing.T) {
	tc := SetupTestEnv(t)
	a := clusterOf(t, tc, "ctx-a", 2)
	b := clusterOf(t, tc, "ctx-b", 2)
	red := createReduction(t, tc, "Keys", append(append([]uint{}, a...), b...))
	plan := computeReduction(t, tc, red.ID)
	override(t, tc, red.ID, clusterFor(plan, b[0]).ID, application_context.ReductionActionSkip, 0)

	page := reviewPageContext(t, tc.AppCtx, red.ID, "")
	assert.Equal(t, "open", page["statusTab"], "Open is the default tab")
	assert.Equal(t, "copies", page["reviewSort"])
	assert.Equal(t, 20, page["pageSize"])
	assert.Equal(t, false, page["settingsChanged"])
	assert.Nil(t, page["lastApply"])
	assert.False(t, page["reviewFilterActive"].(bool), "the default tab is not a filter the reviewer set")
	assert.Equal(t, pongo2.Context{"open": 1, "skipped": 1, "stale": 0, "applied": 0}, page["tabCounts"])
	assert.Len(t, page["clusters"], 1, "only the open Cluster is on the default view")

	skipped := reviewPageContext(t, tc.AppCtx, red.ID, "&status=skipped&sort=size&pageSize=50&approval=bogus")
	assert.Equal(t, "skipped", skipped["statusTab"])
	assert.Equal(t, "size", skipped["reviewSort"])
	assert.Equal(t, 50, skipped["pageSize"])
	assert.Equal(t, "", skipped["reviewApproval"])
	assert.True(t, skipped["reviewFilterActive"].(bool))
	assert.Len(t, skipped["clusters"], 1)
}

// Every new key obeys the redaction rule: the page's context, which is also what
// the .json twin serialises, names nothing about a Cluster the viewer may not see
// and counts none of it.
func TestReviewPageContextKeysNeverExposeAWithheldCluster(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	wide, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Wide"})
	require.NoError(t, err)
	narrow, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Narrow", OwnerId: wide.ID})
	require.NoError(t, err)

	hidden := clusterOf(t, tc, "ctx-hidden-hash", 2)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", hidden[0]).Update("owner_id", narrow.ID).Error)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", hidden[1]).Update("owner_id", wide.ID).Error)
	setDimensions(t, tc, hidden[0], 100, 100)
	setDimensions(t, tc, hidden[1], 900, 900)

	wideCtx := scopedTo(t, tc, wide.ID)
	red, err := wideCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name: "Keys redaction", ResourceIds: hidden,
	}, owner, restricted)
	require.NoError(t, err)
	fresh, err := wideCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = wideCtx.RequestReductionCompute(red.ID, fresh.Version, owner, restricted, nil)
	require.NoError(t, err)
	plan := awaitReduction(t, tc, red.ID)
	hiddenID := plan.Clusters[0].ID
	// The wide reviewer applied it, and the narrow one reads the record.
	current, err := wideCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = wideCtx.ApplyResourceReduction(&query_models.ReductionApply{ID: red.ID, Version: current.Version}, owner, restricted)
	require.NoError(t, err)

	narrowCtx := scopedTo(t, tc, narrow.ID)
	for _, query := range []string{"", "&status=all", "&status=applied", "&attention=true&sort=size"} {
		page := reviewPageContext(t, narrowCtx, red.ID, query)
		keys := pongo2.Context{}
		for _, k := range []string{"tabCounts", "applyFigures", "bulk", "settingsChanged", "lastApply", "clusters", "statusTab", "extentAll"} {
			keys[k] = page[k]
		}
		body, err := json.Marshal(keys)
		require.NoError(t, err)
		rendered := string(body)
		assert.NotContains(t, rendered, hiddenID, "query %q", query)
		assert.NotContains(t, rendered, "ctx-hidden-hash", "query %q", query)

		assert.Equal(t, pongo2.Context{"open": 0, "skipped": 0, "stale": 0, "applied": 0}, page["tabCounts"],
			"the hidden Cluster is on no tab (query %q)", query)
		assert.Equal(t, 0, page["bulk"].(pongo2.Context)["identicalOpen"])
		figures := page["applyFigures"].(pongo2.Context)
		assert.Equal(t, 0, figures["identical"].(pongo2.Context)["clusters"])
		assert.Equal(t, 0, figures["identical"].(pongo2.Context)["losers"])
	}
}

func TestReviewSelectionNamesAreVisibleOnesOnly(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()
	wide, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Wide"})
	require.NoError(t, err)
	narrow, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Narrow", OwnerId: wide.ID})
	require.NoError(t, err)
	var ids []uint
	for i := 0; i < 23; i++ {
		r := addResourceWithBody(t, tc, fmt.Sprintf("sel-%02d.txt", i), fmt.Sprintf("sel body %d", i))
		ids = append(ids, r.ID)
		require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", r.ID).Update("owner_id", narrow.ID).Error)
	}
	hidden := addResourceWithBody(t, tc, "hidden-sel.txt", "hidden sel")
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", hidden.ID).Update("owner_id", wide.ID).Error)

	red, err := tc.AppCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name: "Names", ResourceIds: append(ids, hidden.ID), GroupIds: []uint{narrow.ID, wide.ID},
	}, owner, restricted)
	require.NoError(t, err)

	sel := reviewPageContext(t, scopedTo(t, tc, narrow.ID), red.ID, "")["reviewSelection"].(pongo2.Context)
	resources := sel["resources"].([]contracts.ReductionNamedRef)
	assert.Len(t, resources, 20)
	assert.Equal(t, 3, sel["moreResources"], "the hidden Resource is not counted")
	groups := sel["groups"].([]contracts.ReductionNamedRef)
	require.Len(t, groups, 1)
	assert.Equal(t, "Narrow", groups[0].Name)
	assert.Equal(t, 0, sel["moreGroups"])
}
