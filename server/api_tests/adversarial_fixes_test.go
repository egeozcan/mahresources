package api_tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
)

// P0-1: EditResource must validate the (possibly changed) owner against the
// caller's scope. A group-limited user must not relocate a resource it owns into
// another subtree, nor orphan it.
func TestScopedUser_EditResourceOwnerReassignmentBlocked(t *testing.T) {
	tc := setupAuthEnv(t)
	root := &models.Group{Name: "re-root"}
	tc.DB.Create(root)
	outside := &models.Group{Name: "re-outside"}
	tc.DB.Create(outside)
	res := &models.Resource{Name: "re-res", OwnerId: &root.ID}
	tc.DB.Create(res)

	scoped := tc.AppCtx.WithPrincipal(&auth.Principal{UserID: 5, Role: models.RoleUser, ScopeGroupID: &root.ID})
	ownerOf := func() uint {
		var after models.Resource
		tc.DB.First(&after, res.ID)
		if after.OwnerId == nil {
			return 0
		}
		return *after.OwnerId
	}

	// Relocate to an out-of-subtree group → rejected, owner unchanged.
	if _, err := scoped.EditResource(&query_models.ResourceEditor{ID: res.ID, ResourceQueryBase: query_models.ResourceQueryBase{Name: "re-res", OwnerId: outside.ID}}); err == nil {
		t.Fatalf("reassigning a resource to an out-of-subtree group should fail")
	}
	if got := ownerOf(); got != root.ID {
		t.Fatalf("owner must remain root after a blocked reassign, got %d", got)
	}

	// Orphan (clear owner) → rejected for a scoped principal.
	if _, err := scoped.EditResource(&query_models.ResourceEditor{ID: res.ID, ResourceQueryBase: query_models.ResourceQueryBase{Name: "re-res", OwnerId: 0}}); err == nil {
		t.Fatalf("orphaning a resource should fail for a scoped principal")
	}
	if got := ownerOf(); got != root.ID {
		t.Fatalf("owner must remain root after a blocked orphan, got %d", got)
	}

	// In-subtree edit (keep owner = root) → allowed.
	if _, err := scoped.EditResource(&query_models.ResourceEditor{ID: res.ID, ResourceQueryBase: query_models.ResourceQueryBase{Name: "re-res", OwnerId: root.ID}}); err != nil {
		t.Fatalf("in-subtree edit should succeed, got %v", err)
	}

	// An admin may reassign across subtrees.
	if _, err := tc.AppCtx.EditResource(&query_models.ResourceEditor{ID: res.ID, ResourceQueryBase: query_models.ResourceQueryBase{Name: "re-res", OwnerId: outside.ID}}); err != nil {
		t.Fatalf("admin reassign should succeed, got %v", err)
	}
	if got := ownerOf(); got != outside.ID {
		t.Fatalf("admin reassign should move owner to outside, got %d", got)
	}
}

// P0-2: stored SQL queries run on the unscoped read-only DB, so they must be
// denied to group-limited principals (arbitrary SQL can't be subtree-filtered).
func TestScopedUser_StoredQueryDenied(t *testing.T) {
	tc := setupAuthEnv(t)
	g := &models.Group{Name: "q-root"}
	tc.DB.Create(g)
	q := &models.Query{Name: "q-all", Text: "SELECT id FROM resources"}
	tc.DB.Create(q)

	scopedBearer := scopedUserBearer(t, tc, g.ID)
	scopedBody := doReq(tc, http.MethodPost, fmt.Sprintf("/v1/query/run?id=%d", q.ID),
		map[string]string{"Authorization": scopedBearer}, nil, nil).Body.String()
	if !strings.Contains(scopedBody, "group-limited") {
		t.Fatalf("scoped stored-query run should be denied, got: %s", scopedBody)
	}

	// An admin passes the scope gate (does not get the group-limited denial).
	adminBearer := roleBearer(t, tc, models.RoleAdmin)
	adminBody := doReq(tc, http.MethodPost, fmt.Sprintf("/v1/query/run?id=%d", q.ID),
		map[string]string{"Authorization": adminBearer}, nil, nil).Body.String()
	if strings.Contains(adminBody, "group-limited") {
		t.Fatalf("admin stored-query run must not be scope-denied, got: %s", adminBody)
	}
}

// P1: group relations are not an owner-scoped table, so a scoped principal must
// not read relations (or the endpoint group IDs) outside its subtree.
func TestScopedUser_RelationsConfined(t *testing.T) {
	tc := setupAuthEnv(t)
	root := &models.Group{Name: "rel-root"}
	tc.DB.Create(root)
	child := &models.Group{Name: "rel-child", OwnerId: &root.ID}
	tc.DB.Create(child)
	outA := &models.Group{Name: "rel-outA"}
	tc.DB.Create(outA)
	outB := &models.Group{Name: "rel-outB"}
	tc.DB.Create(outB)
	rt := &models.GroupRelationType{Name: "rt"}
	tc.DB.Create(rt)
	inRel := &models.GroupRelation{FromGroupId: &root.ID, ToGroupId: &child.ID, RelationTypeId: &rt.ID}
	tc.DB.Create(inRel)
	outRel := &models.GroupRelation{FromGroupId: &outA.ID, ToGroupId: &outB.ID, RelationTypeId: &rt.ID}
	tc.DB.Create(outRel)

	scoped := tc.AppCtx.WithPrincipal(&auth.Principal{UserID: 3, Role: models.RoleUser, ScopeGroupID: &root.ID})
	rels, err := scoped.GetRelations(0, 100, &query_models.GroupRelationshipQuery{})
	if err != nil {
		t.Fatalf("GetRelations: %v", err)
	}
	for _, r := range rels {
		if r.ID == outRel.ID {
			t.Fatalf("scoped relations must not include the out-of-subtree relation")
		}
	}
	var sawIn bool
	for _, r := range rels {
		if r.ID == inRel.ID {
			sawIn = true
		}
	}
	if !sawIn {
		t.Fatalf("scoped relations should include the in-subtree relation")
	}

	// Single-relation read of an out-of-subtree relation is not found.
	if _, err := scoped.GetRelation(outRel.ID); err == nil {
		t.Fatalf("scoped GetRelation on an out-of-subtree relation should fail")
	}
	if _, err := scoped.GetRelation(inRel.ID); err != nil {
		t.Fatalf("scoped GetRelation on an in-subtree relation should succeed, got %v", err)
	}
}

// P0-3: the series detail preloads its Resources, which must be confined to the
// caller's subtree.
func TestScopedUser_SeriesResourcesConfined(t *testing.T) {
	tc := setupAuthEnv(t)
	root := &models.Group{Name: "s-root"}
	tc.DB.Create(root)
	outside := &models.Group{Name: "s-outside"}
	tc.DB.Create(outside)
	series := &models.Series{Name: "s-one", Slug: "s-one"}
	tc.DB.Create(series)
	tc.DB.Create(&models.Resource{Name: "s-res-in", OwnerId: &root.ID, SeriesID: &series.ID})
	tc.DB.Create(&models.Resource{Name: "s-res-out", OwnerId: &outside.ID, SeriesID: &series.ID})

	scopedBearer := scopedUserBearer(t, tc, root.ID)
	body := doReq(tc, http.MethodGet, fmt.Sprintf("/v1/series?id=%d", series.ID),
		map[string]string{"Accept": "application/json", "Authorization": scopedBearer}, nil, nil).Body.String()
	if strings.Contains(body, "s-res-out") {
		t.Fatalf("scoped series must not leak the out-of-subtree resource, got: %s", body)
	}
	if !strings.Contains(body, "s-res-in") {
		t.Fatalf("scoped series should include the in-subtree resource, got: %s", body)
	}

	// Admin sees every resource in the series.
	adminBearer := roleBearer(t, tc, models.RoleAdmin)
	adminBody := doReq(tc, http.MethodGet, fmt.Sprintf("/v1/series?id=%d", series.ID),
		map[string]string{"Accept": "application/json", "Authorization": adminBearer}, nil, nil).Body.String()
	if !strings.Contains(adminBody, "s-res-out") || !strings.Contains(adminBody, "s-res-in") {
		t.Fatalf("admin series should include all resources, got: %s", adminBody)
	}
}
