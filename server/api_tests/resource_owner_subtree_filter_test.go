//go:build json1 && fts5

package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
	"mahresources/models/query_models"
)

// ownerSubtreeFixtures creates a root → child → grandchild group chain plus an
// unrelated group, each owning exactly one resource.
func ownerSubtreeFixtures(t *testing.T, tc *TestContext) (root, child, grandchild, unrelated *models.Group) {
	t.Helper()

	root = &models.Group{Name: "os-root"}
	require.NoError(t, tc.DB.Create(root).Error)
	child = &models.Group{Name: "os-child", OwnerId: &root.ID}
	require.NoError(t, tc.DB.Create(child).Error)
	grandchild = &models.Group{Name: "os-grandchild", OwnerId: &child.ID}
	require.NoError(t, tc.DB.Create(grandchild).Error)
	unrelated = &models.Group{Name: "os-unrelated"}
	require.NoError(t, tc.DB.Create(unrelated).Error)

	for name, owner := range map[string]*models.Group{
		"os-root-res":       root,
		"os-child-res":      child,
		"os-grandchild-res": grandchild,
		"os-unrelated-res":  unrelated,
	} {
		r := &models.Resource{Name: name, ContentType: "image/png", OwnerId: &owner.ID}
		require.NoError(t, tc.DB.Create(r).Error)
	}
	return root, child, grandchild, unrelated
}

func resourceNames(got []models.Resource) []string {
	names := make([]string, 0, len(got))
	for _, r := range got {
		names = append(names, r.Name)
	}
	return names
}

// IncludeSubgroups=true widens the OwnerId filter to the whole subtree:
// resources owned by the group itself and by all recursive descendants.
func TestResources_OwnerSubtree_IncludesDescendants(t *testing.T) {
	tc := SetupTestEnv(t)
	root, _, _, _ := ownerSubtreeFixtures(t, tc)

	got, err := tc.AppCtx.GetResources(0, 100, &query_models.ResourceSearchQuery{
		OwnerId:          root.ID,
		IncludeSubgroups: true,
	})
	require.NoError(t, err)

	names := resourceNames(got)
	assert.Contains(t, names, "os-root-res")
	assert.Contains(t, names, "os-child-res")
	assert.Contains(t, names, "os-grandchild-res")
	assert.NotContains(t, names, "os-unrelated-res")

	count, err := tc.AppCtx.GetResourceCount(&query_models.ResourceSearchQuery{
		OwnerId:          root.ID,
		IncludeSubgroups: true,
	})
	require.NoError(t, err)
	assert.EqualValues(t, len(got), count, "count must use the same subtree semantics as the list")
}

// Regression guard: without the flag, the OwnerId filter stays an exact match.
func TestResources_OwnerSubtree_OffIsExactMatch(t *testing.T) {
	tc := SetupTestEnv(t)
	root, _, _, _ := ownerSubtreeFixtures(t, tc)

	got, err := tc.AppCtx.GetResources(0, 100, &query_models.ResourceSearchQuery{OwnerId: root.ID})
	require.NoError(t, err)

	names := resourceNames(got)
	assert.Contains(t, names, "os-root-res")
	assert.NotContains(t, names, "os-child-res")
	assert.NotContains(t, names, "os-grandchild-res")
	assert.NotContains(t, names, "os-unrelated-res")
}

// The flag alone (OwnerId=0) is a no-op: everything is returned.
func TestResources_OwnerSubtree_FlagAloneIsNoop(t *testing.T) {
	tc := SetupTestEnv(t)
	ownerSubtreeFixtures(t, tc)

	got, err := tc.AppCtx.GetResources(0, 100, &query_models.ResourceSearchQuery{IncludeSubgroups: true})
	require.NoError(t, err)

	names := resourceNames(got)
	assert.Contains(t, names, "os-root-res")
	assert.Contains(t, names, "os-child-res")
	assert.Contains(t, names, "os-grandchild-res")
	assert.Contains(t, names, "os-unrelated-res")
}

// HTTP layer: proves the query param binds through gorilla/schema end-to-end.
func TestResources_OwnerSubtree_HTTP(t *testing.T) {
	tc := SetupTestEnv(t)
	root, _, _, _ := ownerSubtreeFixtures(t, tc)

	url := fmt.Sprintf("/v1/resources?ownerId=%d&includeSubgroups=1", root.ID)
	rr := doReq(tc, http.MethodGet, url, map[string]string{"Accept": "application/json"}, nil, nil)
	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())

	var got []models.Resource
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&got))

	names := make([]string, 0, len(got))
	for _, r := range got {
		names = append(names, r.Name)
	}
	assert.Contains(t, names, "os-root-res")
	assert.Contains(t, names, "os-child-res")
	assert.Contains(t, names, "os-grandchild-res")
	assert.NotContains(t, names, "os-unrelated-res")
}

// Scoped-user (RBAC): a principal confined to the child subtree who asks for
// ownerId=root&includeSubgroups=1 must only get results inside its own scope —
// the user-facing subtree filter intersects with RBAC scoping, never widens it.
func TestResources_OwnerSubtree_ScopedToSubtree(t *testing.T) {
	tc := setupAuthEnv(t)
	root, child, _, _ := ownerSubtreeFixtures(t, tc)

	bearer := scopedUserBearer(t, tc, child.ID)
	h := map[string]string{"Accept": "application/json", "Authorization": bearer}

	url := fmt.Sprintf("/v1/resources?ownerId=%d&includeSubgroups=1", root.ID)
	rr := doReq(tc, http.MethodGet, url, h, nil, nil)
	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())

	var got []models.Resource
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&got))

	names := make([]string, 0, len(got))
	for _, r := range got {
		names = append(names, r.Name)
	}
	assert.Contains(t, names, "os-child-res")
	assert.Contains(t, names, "os-grandchild-res")
	assert.NotContains(t, names, "os-root-res", "scoped user must NOT see resources above their scope group")
	assert.NotContains(t, names, "os-unrelated-res")
}
