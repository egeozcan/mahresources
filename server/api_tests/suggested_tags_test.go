package api_tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"mahresources/models"
)

type suggestedTagsBody struct {
	Suggestions []struct {
		ID      uint     `json:"ID"`
		Name    string   `json:"Name"`
		Score   float64  `json:"score"`
		Sources []string `json:"sources"`
	} `json:"suggestions"`
}

func decodeSuggestions(t *testing.T, raw string) suggestedTagsBody {
	t.Helper()
	var b suggestedTagsBody
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		t.Fatalf("decode suggestions: %v (raw: %s)", err, raw)
	}
	return b
}

func suggestionHasName(b suggestedTagsBody, name string) bool {
	for _, s := range b.Suggestions {
		if s.Name == name {
			return true
		}
	}
	return false
}

// TestSuggestedTags_HTTP_GroupRanked: GET returns 200 with the owner group's
// tags ranked by usage.
func TestSuggestedTags_HTTP_GroupRanked(t *testing.T) {
	tc := SetupTestEnv(t)

	owner := &models.Group{Name: "st-owner"}
	tc.DB.Create(owner)
	alpha := &models.Tag{Name: "st-alpha"}
	beta := &models.Tag{Name: "st-beta"}
	tc.DB.Create(alpha)
	tc.DB.Create(beta)

	r1 := &models.Resource{Name: "st-r1", OwnerId: &owner.ID, Meta: []byte("{}"), OwnMeta: []byte("{}")}
	r2 := &models.Resource{Name: "st-r2", OwnerId: &owner.ID, Meta: []byte("{}"), OwnMeta: []byte("{}")}
	tc.DB.Create(r1)
	tc.DB.Create(r2)
	tc.DB.Model(r1).Association("Tags").Append([]*models.Tag{alpha, beta})
	tc.DB.Model(r2).Association("Tags").Append([]*models.Tag{alpha})

	target := &models.Resource{Name: "st-target", OwnerId: &owner.ID, Meta: []byte("{}"), OwnMeta: []byte("{}")}
	tc.DB.Create(target)

	rr := tc.MakeRequest(http.MethodGet, "/v1/resource/suggestedTags?id="+itoa(int(target.ID)), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	b := decodeSuggestions(t, rr.Body.String())
	if !suggestionHasName(b, "st-alpha") || !suggestionHasName(b, "st-beta") {
		t.Fatalf("expected st-alpha and st-beta, got %s", rr.Body.String())
	}
	if b.Suggestions[0].Name != "st-alpha" {
		t.Fatalf("expected st-alpha ranked first, got %s", rr.Body.String())
	}
}

// TestSuggestedTags_HTTP_BadAndMissing: zero/missing id → 400, nonexistent → 404.
func TestSuggestedTags_HTTP_BadAndMissing(t *testing.T) {
	tc := SetupTestEnv(t)

	if rr := tc.MakeRequest(http.MethodGet, "/v1/resource/suggestedTags", nil); rr.Code != http.StatusBadRequest {
		t.Fatalf("missing id should be 400, got %d", rr.Code)
	}
	if rr := tc.MakeRequest(http.MethodGet, "/v1/resource/suggestedTags?id=0", nil); rr.Code != http.StatusBadRequest {
		t.Fatalf("zero id should be 400, got %d", rr.Code)
	}
	if rr := tc.MakeRequest(http.MethodGet, "/v1/resource/suggestedTags?id=999999", nil); rr.Code != http.StatusNotFound {
		t.Fatalf("nonexistent id should be 404, got %d", rr.Code)
	}
}

// TestSuggestedTags_HTTP_EmptyDegrades: an ownerless target with no similarity
// returns 200 and an empty array.
func TestSuggestedTags_HTTP_EmptyDegrades(t *testing.T) {
	tc := SetupTestEnv(t)
	target := &models.Resource{Name: "st-lonely", Meta: []byte("{}"), OwnMeta: []byte("{}")}
	tc.DB.Create(target)

	rr := tc.MakeRequest(http.MethodGet, "/v1/resource/suggestedTags?id="+itoa(int(target.ID)), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	b := decodeSuggestions(t, rr.Body.String())
	if len(b.Suggestions) != 0 {
		t.Fatalf("expected empty suggestions, got %s", rr.Body.String())
	}
}

// TestSuggestedTags_HTTP_RBAC: a group-limited user is 404'd on an
// out-of-subtree resource and only ever sees in-subtree-sourced suggestions.
func TestSuggestedTags_HTTP_RBAC(t *testing.T) {
	tc := setupAuthEnv(t)

	root := &models.Group{Name: "stb-root"}
	tc.DB.Create(root)
	child := &models.Group{Name: "stb-child", OwnerId: &root.ID}
	tc.DB.Create(child)
	outside := &models.Group{Name: "stb-outside"}
	tc.DB.Create(outside)

	inTag := &models.Tag{Name: "stb-insider"}
	outTag := &models.Tag{Name: "stb-outsider"}
	tc.DB.Create(inTag)
	tc.DB.Create(outTag)

	// In-subtree: a sibling resource carrying inTag drives the group source.
	sibling := &models.Resource{Name: "stb-sibling", OwnerId: &child.ID, Meta: []byte("{}"), OwnMeta: []byte("{}")}
	tc.DB.Create(sibling)
	tc.DB.Model(sibling).Association("Tags").Append([]*models.Tag{inTag})
	target := &models.Resource{Name: "stb-target", OwnerId: &child.ID, Meta: []byte("{}"), OwnMeta: []byte("{}")}
	tc.DB.Create(target)

	// Out-of-subtree resource carrying outTag.
	outsideRes := &models.Resource{Name: "stb-outsideRes", OwnerId: &outside.ID, Meta: []byte("{}"), OwnMeta: []byte("{}")}
	tc.DB.Create(outsideRes)
	tc.DB.Model(outsideRes).Association("Tags").Append([]*models.Tag{outTag})

	bearer := scopedUserBearer(t, tc, root.ID)
	h := map[string]string{"Accept": "application/json", "Authorization": bearer}

	// Out-of-subtree resource → 404.
	out := doReq(tc, http.MethodGet, "/v1/resource/suggestedTags?id="+itoa(int(outsideRes.ID)), h, nil, nil)
	if out.Code != http.StatusNotFound {
		t.Fatalf("scoped user should get 404 for out-of-subtree resource, got %d", out.Code)
	}

	// In-subtree target → 200, contains insider, never outsider.
	in := doReq(tc, http.MethodGet, "/v1/resource/suggestedTags?id="+itoa(int(target.ID)), h, nil, nil)
	if in.Code != http.StatusOK {
		t.Fatalf("scoped user should get 200 for in-subtree resource, got %d (%s)", in.Code, in.Body.String())
	}
	b := decodeSuggestions(t, in.Body.String())
	if !suggestionHasName(b, "stb-insider") {
		t.Fatalf("expected insider suggestion, got %s", in.Body.String())
	}
	if suggestionHasName(b, "stb-outsider") {
		t.Fatalf("scoped user must not receive out-of-subtree-sourced tag, got %s", in.Body.String())
	}
}
