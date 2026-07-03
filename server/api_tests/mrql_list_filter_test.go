package api_tests

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
)

// createTaggedResource inserts a resource with the given name and owner, tagged
// with each of the named tags (creating tags as needed).
func createTaggedResource(t *testing.T, tc *TestContext, name string, ownerID *uint, tagNames ...string) *models.Resource {
	t.Helper()
	r := &models.Resource{Name: name, OwnerId: ownerID}
	if err := tc.DB.Create(r).Error; err != nil {
		t.Fatalf("create resource %q: %v", name, err)
	}
	for _, tn := range tagNames {
		var tag models.Tag
		if err := tc.DB.Where("name = ?", tn).FirstOrCreate(&tag, models.Tag{Name: tn}).Error; err != nil {
			t.Fatalf("create tag %q: %v", tn, err)
		}
		if err := tc.DB.Model(r).Association("Tags").Append(&tag); err != nil {
			t.Fatalf("associate tag %q: %v", tn, err)
		}
	}
	return r
}

// resourceListURL builds /v1/resources with an mrql filter query parameter.
func resourceListURL(mrql string) string {
	q := url.Values{}
	q.Set("mrql", mrql)
	return "/v1/resources?" + q.Encode()
}

func TestMRQLListFilter_NarrowsResources(t *testing.T) {
	tc := SetupTestEnv(t)

	rIn := createTaggedResource(t, tc, "vacation-photo", nil, "vacation")
	rOut := createTaggedResource(t, tc, "work-doc", nil, "work")

	rr := tc.MakeRequest(http.MethodGet, resourceListURL(`tags = "vacation"`), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var got []models.Resource
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rr.Body.String())
	}
	ids := map[uint]bool{}
	for _, r := range got {
		ids[r.ID] = true
	}
	if !ids[rIn.ID] {
		t.Errorf("expected filtered list to contain vacation resource %d", rIn.ID)
	}
	if ids[rOut.ID] {
		t.Errorf("expected filtered list to exclude work resource %d", rOut.ID)
	}
}

// TestMRQLListFilter_CountAgreesWithList verifies the count call sees the same
// MRQL predicate as the list, so pagination doesn't lie.
func TestMRQLListFilter_CountAgreesWithList(t *testing.T) {
	tc := SetupTestEnv(t)

	createTaggedResource(t, tc, "v1", nil, "vacation")
	createTaggedResource(t, tc, "v2", nil, "vacation")
	createTaggedResource(t, tc, "w1", nil, "work")

	q := &query_models.ResourceSearchQuery{MRQL: `tags = "vacation"`}

	list, err := tc.AppCtx.GetResources(0, 100, q)
	if err != nil {
		t.Fatalf("GetResources: %v", err)
	}
	count, err := tc.AppCtx.GetResourceCount(q)
	if err != nil {
		t.Fatalf("GetResourceCount: %v", err)
	}
	if int64(len(list)) != count {
		t.Fatalf("count %d != list length %d", count, len(list))
	}
	if count != 2 {
		t.Fatalf("expected 2 vacation resources, got %d", count)
	}
}

// TestMRQLListFilter_PopularTagsAgreesWithFilter verifies the tag sidebar only
// reflects tags present in the MRQL-filtered set.
func TestMRQLListFilter_PopularTagsAgreesWithFilter(t *testing.T) {
	tc := SetupTestEnv(t)

	createTaggedResource(t, tc, "v1", nil, "vacation", "beach")
	createTaggedResource(t, tc, "w1", nil, "work", "office")

	q := &query_models.ResourceSearchQuery{MRQL: `tags = "vacation"`}
	tags, err := tc.AppCtx.GetPopularResourceTags(q)
	if err != nil {
		t.Fatalf("GetPopularResourceTags: %v", err)
	}
	names := map[string]bool{}
	for _, pt := range tags {
		names[pt.Name] = true
	}
	if !names["vacation"] || !names["beach"] {
		t.Errorf("expected vacation+beach tags in sidebar, got %v", names)
	}
	if names["work"] || names["office"] {
		t.Errorf("expected work/office tags excluded from filtered sidebar, got %v", names)
	}
}

func TestMRQLListFilter_InvalidExpressionIs400(t *testing.T) {
	tc := SetupTestEnv(t)
	createTaggedResource(t, tc, "v1", nil, "vacation")

	cases := []struct {
		name string
		expr string
	}{
		{"clause keyword", `tags = "vacation" ORDER BY name`},
		{"type field", `type = "note"`},
		{"garbage", `tags = = =`},
	}
	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			rr := tc.MakeRequest(http.MethodGet, resourceListURL(tcase.expr), nil)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected 400 for %q, got %d: %s", tcase.expr, rr.Code, rr.Body.String())
			}
		})
	}
}

// TestMRQLListFilter_ScopedPrincipalCannotEscape verifies a group-limited
// principal cannot widen scope through the mrql filter: the outer scoped query
// intersects the MRQL subquery, so out-of-subtree rows never appear.
func TestMRQLListFilter_ScopedPrincipalCannotEscape(t *testing.T) {
	tc := setupAuthEnv(t)

	// Scope group for a group-limited user.
	scopeGroup := &models.Group{Name: "scope"}
	if err := tc.DB.Create(scopeGroup).Error; err != nil {
		t.Fatalf("create scope group: %v", err)
	}
	// A separate, out-of-scope group.
	otherGroup := &models.Group{Name: "other"}
	if err := tc.DB.Create(otherGroup).Error; err != nil {
		t.Fatalf("create other group: %v", err)
	}

	rIn := createTaggedResource(t, tc, "in-scope", &scopeGroup.ID, "shared")
	rOut := createTaggedResource(t, tc, "out-scope", &otherGroup.ID, "shared")

	u, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "scoped", Password: "password1", Role: models.RoleUser, ScopeGroupId: &scopeGroup.ID,
	})
	if err != nil {
		t.Fatalf("create scoped user: %v", err)
	}
	raw, _, err := tc.AppCtx.CreateApiToken(u.ID, "t", nil)
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	rr := doReq(tc, http.MethodGet, resourceListURL(`tags = "shared"`),
		map[string]string{"Authorization": "Bearer " + raw}, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var got []models.Resource
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	ids := map[uint]bool{}
	for _, r := range got {
		ids[r.ID] = true
	}
	if !ids[rIn.ID] {
		t.Errorf("expected in-scope resource %d to be visible", rIn.ID)
	}
	if ids[rOut.ID] {
		t.Errorf("scoped principal escaped scope via mrql: out-of-scope resource %d leaked", rOut.ID)
	}
}
