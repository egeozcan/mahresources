package api_tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/models"
)

type previewResponse struct {
	HTML   string      `json:"html"`
	CSS    string      `json:"css"`
	Issues []lintIssue `json:"issues"`
}

func TestPreviewTemplate_HappyPath(t *testing.T) {
	tc := SetupTestEnv(t) // auth off

	g := &models.Group{Name: "Preview Group"}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	rr := tc.MakeRequest(http.MethodPost, "/v1/category/previewTemplate", map[string]any{
		"entityId": g.ID,
		"content":  `<h1>[property path="Name"]</h1>`,
		"css":      `.x { color: red; }`,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	var resp previewResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(resp.HTML, "Preview Group") {
		t.Errorf("expected rendered html to contain the group name, got %q", resp.HTML)
	}
	if !strings.Contains(resp.CSS, "color: red") {
		t.Errorf("expected css to be echoed, got %q", resp.CSS)
	}
}

func TestPreviewTemplate_NotFound(t *testing.T) {
	tc := SetupTestEnv(t)
	rr := tc.MakeRequest(http.MethodPost, "/v1/category/previewTemplate", map[string]any{
		"entityId": 999999,
		"content":  "x",
	})
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing entity, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

func TestPreviewTemplate_MissingEntityId(t *testing.T) {
	tc := SetupTestEnv(t)
	rr := tc.MakeRequest(http.MethodPost, "/v1/category/previewTemplate", map[string]any{
		"content": "x",
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without entityId, got %d", rr.Code)
	}
}

func TestPreviewTemplate_IssuesPiggybacked(t *testing.T) {
	tc := SetupTestEnv(t)
	g := &models.Group{Name: "Group"}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	rr := tc.MakeRequest(http.MethodPost, "/v1/category/previewTemplate", map[string]any{
		"entityId": g.ID,
		"content":  `[conditional]broken`,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
	var resp previewResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var sawError bool
	for _, iss := range resp.Issues {
		if iss.Severity == "error" {
			sawError = true
		}
	}
	if !sawError {
		t.Errorf("expected piggybacked lint errors for broken conditional, got %+v", resp.Issues)
	}
}

// TestPreviewTemplate_RoleMatrix verifies the preview endpoints are gated at the
// same capability as saving the corresponding template: category /
// resourceCategory require admin (capTaxonomy); noteType requires editor.
func TestPreviewTemplate_RoleMatrix(t *testing.T) {
	tc := setupAuthEnv(t)

	g := &models.Group{Name: "G"}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	n := &models.Note{Name: "N"}
	if err := tc.DB.Create(n).Error; err != nil {
		t.Fatalf("create note: %v", err)
	}

	adminB := roleBearer(t, tc, models.RoleAdmin)
	editorB := roleBearer(t, tc, models.RoleEditor)
	guestB := roleBearer(t, tc, models.RoleGuest)

	post := func(bearer, path string, entityID uint) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"entityId": entityID, "content": `[property path="Name"]`})
		headers := map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": bearer}
		return doReq(tc, http.MethodPost, path, headers, nil, bytes.NewReader(body))
	}

	// editor is denied the taxonomy-level category preview...
	if rr := post(editorB, "/v1/category/previewTemplate", g.ID); rr.Code != http.StatusForbidden {
		t.Errorf("editor → category preview: expected 403, got %d", rr.Code)
	}
	if rr := post(editorB, "/v1/resourceCategory/previewTemplate", 1); rr.Code != http.StatusForbidden {
		t.Errorf("editor → resourceCategory preview: expected 403, got %d", rr.Code)
	}
	// ...but allowed the editor-level noteType preview.
	if rr := post(editorB, "/v1/noteType/previewTemplate", n.ID); rr.Code == http.StatusForbidden {
		t.Errorf("editor → noteType preview: expected allowed, got 403 (body: %s)", rr.Body.String())
	}

	// guest is denied everywhere.
	for _, path := range []string{"/v1/category/previewTemplate", "/v1/resourceCategory/previewTemplate", "/v1/noteType/previewTemplate"} {
		if rr := post(guestB, path, g.ID); rr.Code != http.StatusForbidden {
			t.Errorf("guest → %s: expected 403, got %d", path, rr.Code)
		}
	}

	// admin is allowed the category preview.
	if rr := post(adminB, "/v1/category/previewTemplate", g.ID); rr.Code == http.StatusForbidden {
		t.Errorf("admin → category preview: expected allowed, got 403 (body: %s)", rr.Body.String())
	}
}
