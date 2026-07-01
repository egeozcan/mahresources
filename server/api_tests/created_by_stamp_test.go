package api_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
)

// userWithBearer creates a distinct (non-root) user of the given role and
// returns its id plus an Authorization header value.
func userWithBearer(t *testing.T, tc *TestContext, username string, role models.Role) (uint, string) {
	t.Helper()
	u, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: username, Password: "password1", Role: role,
	})
	if err != nil {
		t.Fatalf("create %s: %v", username, err)
	}
	raw, _, err := tc.AppCtx.CreateApiToken(u.ID, "t", nil)
	if err != nil {
		t.Fatalf("token for %s: %v", username, err)
	}
	return u.ID, "Bearer " + raw
}

// createdByFromResponse parses the createdByUserId field out of a create
// response body (present because the models carry `json:"createdByUserId"`).
func createdByFromResponse(t *testing.T, body []byte) (uint, bool) {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("parse response %q: %v", string(body), err)
	}
	v, ok := m["createdByUserId"]
	if !ok || v == nil {
		return 0, false
	}
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("createdByUserId not a number: %v", v)
	}
	return uint(f), true
}

// TestCreatedBy_APIStampsBearer drives the create handlers across privilege
// tiers with distinct non-root bearers and asserts every created entity is
// stamped with the acting bearer's id (not NULL, not root). This is the guard
// that no create path silently regresses to NULL at the HTTP layer. It covers
// the handler_factory handlers request-scoped in Phase 2b (Tag/Category/
// ResourceCategory), the SavedMRQL handler, and the bespoke request-scoped
// Series route.
func TestCreatedBy_APIStampsBearer(t *testing.T) {
	tc := setupAuthEnv(t)
	userID, userBearer := userWithBearer(t, tc, "tier_user", models.RoleUser)
	editorID, editorBearer := userWithBearer(t, tc, "tier_editor", models.RoleEditor)
	adminID, adminBearer := userWithBearer(t, tc, "tier_admin", models.RoleAdmin)

	cases := []struct {
		name, path, body, bearer string
		wantID                   uint
	}{
		// user tier (capWrite)
		{"tag", "/v1/tag", `{"name":"cb_tag"}`, userBearer, userID},
		{"note", "/v1/note", `{"name":"cb_note"}`, userBearer, userID},
		{"group", "/v1/group", `{"name":"cb_group"}`, userBearer, userID},
		// editor tier (capEditor)
		{"noteType", "/v1/note/noteType", `{"name":"cb_nt"}`, editorBearer, editorID},
		{"series", "/v1/series/create", `{"name":"cb_series"}`, editorBearer, editorID},
		{"query", "/v1/query", `{"name":"cb_query","text":"resources"}`, editorBearer, editorID},
		{"savedMRQL", "/v1/mrql/saved", `{"name":"cb_mrql","query":"id > 0"}`, editorBearer, editorID},
		// admin tier (capTaxonomy)
		{"category", "/v1/category", `{"name":"cb_cat"}`, adminBearer, adminID},
		{"resourceCategory", "/v1/resourceCategory", `{"name":"cb_rc"}`, adminBearer, adminID},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			headers := map[string]string{
				"Accept":        "application/json",
				"Content-Type":  "application/json",
				"Authorization": c.bearer,
			}
			rr := doReq(tc, http.MethodPost, c.path, headers, nil, strings.NewReader(c.body))
			if rr.Code >= 300 {
				t.Fatalf("%s create: status %d body=%s", c.name, rr.Code, rr.Body.String())
			}
			got, ok := createdByFromResponse(t, rr.Body.Bytes())
			if !ok {
				t.Fatalf("%s: createdByUserId missing/NULL in response %s", c.name, rr.Body.String())
			}
			if got != c.wantID {
				t.Fatalf("%s: created_by=%d, want acting bearer %d", c.name, got, c.wantID)
			}
		})
	}
}

// TestCreatedBy_ResourceUploadStampsUploader covers the multipart resource
// upload path (request-scoped via scopedCtx) and the Phase 2c raw-SQL implicit
// series creation: both the resource and its auto-created series must be stamped
// with the uploader.
func TestCreatedBy_ResourceUploadStampsUploader(t *testing.T) {
	tc := setupAuthEnv(t)
	userID, userBearer := userWithBearer(t, tc, "uploader", models.RoleUser)

	body, ct := makeMultipartUpload(t, "resource", "u.svg",
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><rect width="8" height="8"/></svg>`),
		map[string]string{"Name": "cb_upload", "seriesSlug": "cb_upload_series"})

	req := httptest.NewRequest(http.MethodPost, "/v1/resource", body)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", userBearer)
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	if rr.Code >= 300 {
		t.Fatalf("upload: status %d body=%s", rr.Code, rr.Body.String())
	}

	// The uploaded resource is stamped by the uploader.
	var res models.Resource
	if err := tc.DB.Where("name = ?", "cb_upload").First(&res).Error; err != nil {
		t.Fatalf("load uploaded resource: %v", err)
	}
	if res.CreatedByUserId == nil || *res.CreatedByUserId != userID {
		t.Fatalf("resource created_by=%v, want uploader %d", res.CreatedByUserId, userID)
	}

	// The implicitly-created series (Phase 2c raw SQL) is stamped by the uploader.
	var series models.Series
	if err := tc.DB.Where("slug = ?", "cb_upload_series").First(&series).Error; err != nil {
		t.Fatalf("load implicit series: %v", err)
	}
	if series.CreatedByUserId == nil || *series.CreatedByUserId != userID {
		t.Fatalf("implicit series created_by=%v, want uploader %d", series.CreatedByUserId, userID)
	}
}
