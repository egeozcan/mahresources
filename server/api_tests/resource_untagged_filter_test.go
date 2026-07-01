//go:build json1 && fts5

package api_tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
	"mahresources/models/query_models"
)

// Tier 3 — "Tag Untagged Only": Untagged=true must select only resources with
// zero rows in resource_tags, composed correctly with RBAC subtree scoping.
func TestResources_Untagged_FiltersToZeroTagsOnly(t *testing.T) {
	tc := SetupTestEnv(t)

	tagged := &models.Resource{Name: "tagged-once.png", ContentType: "image/png"}
	taggedTwice := &models.Resource{Name: "tagged-twice.png", ContentType: "image/png"}
	untagged := &models.Resource{Name: "untagged.png", ContentType: "image/png"}
	require.NoError(t, tc.DB.Create(tagged).Error)
	require.NoError(t, tc.DB.Create(taggedTwice).Error)
	require.NoError(t, tc.DB.Create(untagged).Error)

	tagA := &models.Tag{Name: "ut-tagA"}
	tagB := &models.Tag{Name: "ut-tagB"}
	require.NoError(t, tc.DB.Create(tagA).Error)
	require.NoError(t, tc.DB.Create(tagB).Error)
	require.NoError(t, tc.DB.Model(tagged).Association("Tags").Append([]*models.Tag{tagA}))
	require.NoError(t, tc.DB.Model(taggedTwice).Association("Tags").Append([]*models.Tag{tagA, tagB}))

	got, err := tc.AppCtx.GetResources(0, 100, &query_models.ResourceSearchQuery{Untagged: true})
	require.NoError(t, err)

	names := make([]string, 0, len(got))
	for _, r := range got {
		names = append(names, r.Name)
	}

	assert.Contains(t, names, "untagged.png")
	assert.NotContains(t, names, "tagged-once.png")
	assert.NotContains(t, names, "tagged-twice.png")
}

// Negative: without Untagged, all resources (tagged and untagged) are returned.
func TestResources_Untagged_FalseReturnsEverything(t *testing.T) {
	tc := SetupTestEnv(t)

	tagged := &models.Resource{Name: "neg-tagged.png", ContentType: "image/png"}
	untagged := &models.Resource{Name: "neg-untagged.png", ContentType: "image/png"}
	require.NoError(t, tc.DB.Create(tagged).Error)
	require.NoError(t, tc.DB.Create(untagged).Error)
	tag := &models.Tag{Name: "neg-tag"}
	require.NoError(t, tc.DB.Create(tag).Error)
	require.NoError(t, tc.DB.Model(tagged).Association("Tags").Append([]*models.Tag{tag}))

	got, err := tc.AppCtx.GetResources(0, 100, &query_models.ResourceSearchQuery{})
	require.NoError(t, err)

	names := make([]string, 0, len(got))
	for _, r := range got {
		names = append(names, r.Name)
	}
	assert.Contains(t, names, "neg-tagged.png")
	assert.Contains(t, names, "neg-untagged.png")
}

// Edge: a resource that HAD tags and had every one removed (zero rows left in
// resource_tags) must be treated as untagged, same as one that never had any.
func TestResources_Untagged_TreatsAllTagsRemovedAsUntagged(t *testing.T) {
	tc := SetupTestEnv(t)

	r := &models.Resource{Name: "stripped.png", ContentType: "image/png"}
	require.NoError(t, tc.DB.Create(r).Error)
	tag := &models.Tag{Name: "stripped-tag"}
	require.NoError(t, tc.DB.Create(tag).Error)
	require.NoError(t, tc.DB.Model(r).Association("Tags").Append([]*models.Tag{tag}))
	require.NoError(t, tc.DB.Model(r).Association("Tags").Clear())

	got, err := tc.AppCtx.GetResources(0, 100, &query_models.ResourceSearchQuery{Untagged: true})
	require.NoError(t, err)

	names := make([]string, 0, len(got))
	for _, r := range got {
		names = append(names, r.Name)
	}
	assert.Contains(t, names, "stripped.png")
}

// HTTP layer: GET /v1/resources?Untagged=1 with Accept: application/json
// returns only the untagged resource.
func TestResources_Untagged_HTTP(t *testing.T) {
	tc := SetupTestEnv(t)

	tagged := &models.Resource{Name: "http-tagged.png", ContentType: "image/png"}
	untagged := &models.Resource{Name: "http-untagged.png", ContentType: "image/png"}
	require.NoError(t, tc.DB.Create(tagged).Error)
	require.NoError(t, tc.DB.Create(untagged).Error)
	tag := &models.Tag{Name: "http-tag"}
	require.NoError(t, tc.DB.Create(tag).Error)
	require.NoError(t, tc.DB.Model(tagged).Association("Tags").Append([]*models.Tag{tag}))

	rr := doReq(tc, http.MethodGet, "/v1/resources?Untagged=1",
		map[string]string{"Accept": "application/json"}, nil, nil)
	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())

	var got []models.Resource
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&got))

	names := make([]string, 0, len(got))
	for _, r := range got {
		names = append(names, r.Name)
	}
	assert.Contains(t, names, "http-untagged.png")
	assert.NotContains(t, names, "http-tagged.png")
}

// Scoped-user (RBAC): a group-limited principal querying Untagged=true sees
// only untagged resources inside its subtree; an untagged resource owned by an
// out-of-subtree group must not leak through. Proves the predicate ANDs with
// the scope clause rather than bypassing it.
func TestResources_Untagged_ScopedToSubtree(t *testing.T) {
	tc := setupAuthEnv(t)

	root := &models.Group{Name: "ut-root"}
	tc.DB.Create(root)
	child := &models.Group{Name: "ut-child", OwnerId: &root.ID}
	tc.DB.Create(child)
	outside := &models.Group{Name: "ut-outside"}
	tc.DB.Create(outside)

	inUntagged := &models.Resource{Name: "ut-in-untagged", OwnerId: &child.ID}
	tc.DB.Create(inUntagged)
	outUntagged := &models.Resource{Name: "ut-out-untagged", OwnerId: &outside.ID}
	tc.DB.Create(outUntagged)

	bearer := scopedUserBearer(t, tc, root.ID)
	h := map[string]string{"Accept": "application/json", "Authorization": bearer}

	rr := doReq(tc, http.MethodGet, "/v1/resources?Untagged=1", h, nil, nil)
	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())

	var got []models.Resource
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&got))
	names := make([]string, 0, len(got))
	for _, r := range got {
		names = append(names, r.Name)
	}
	assert.Contains(t, names, "ut-in-untagged", "scoped user should see in-subtree untagged resources")
	assert.NotContains(t, names, "ut-out-untagged", "scoped user must NOT see out-of-subtree untagged resources")
}
