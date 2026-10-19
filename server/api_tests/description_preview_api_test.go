package api_tests

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// POST /v1/<entity>/previewDescription renders a Description the way the
// entity's detail page does (description.tpl: markdown, then mentions, then
// shortcodes against the entity) without saving it. docs/plans/
// 2026-10-19-markdown-description-editor-design.md, decisions 5, 7 and 8.

func previewDescription(tc *TestContext, bearer, entity string, id uint, text string) *httptest.ResponseRecorder {
	path := fmt.Sprintf("/v1/%s/previewDescription", entity)
	if id != 0 {
		path += fmt.Sprintf("?id=%d", id)
	}
	form := url.Values{"text": {text}}
	headers := map[string]string{
		"Accept":       "application/json",
		"Content-Type": "application/x-www-form-urlencoded",
	}
	if bearer != "" {
		headers["Authorization"] = bearer
	}
	return doReq(tc, http.MethodPost, path, headers, nil, strings.NewReader(form.Encode()))
}

func previewHTML(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var body struct {
		HTML string `json:"html"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (%s)", err, rr.Body.String())
	}
	return body.HTML
}

func TestPreviewDescription_RendersMarkdownAndMentions(t *testing.T) {
	tc := SetupTestEnv(t)
	tag := &models.Tag{Name: "preview tag"}
	tc.DB.Create(tag)
	group := &models.Group{Name: "Mentioned Group"}
	tc.DB.Create(group)

	text := fmt.Sprintf("**bold** and @[group:%d:Mentioned Group]", group.ID)
	html := previewHTML(t, previewDescription(tc, "", "tag", tag.ID, text))
	if !strings.Contains(html, "<strong>bold</strong>") {
		t.Errorf("markdown not rendered: %s", html)
	}
	if !strings.Contains(html, fmt.Sprintf("/group?id=%d", group.ID)) {
		t.Errorf("mention not rendered as a link: %s", html)
	}
	// Nothing was saved.
	var reloaded models.Tag
	tc.DB.First(&reloaded, tag.ID)
	if reloaded.Description != "" {
		t.Errorf("preview saved the description: %q", reloaded.Description)
	}
}

func TestPreviewDescription_ShortcodesRunAgainstTheEntity(t *testing.T) {
	tc := SetupTestEnv(t)
	note := &models.Note{Name: "Previewed Note Name"}
	tc.DB.Create(note)

	html := previewHTML(t, previewDescription(tc, "", "note", note.ID, `[property path="Name"]`))
	if !strings.Contains(html, "Previewed Note Name") || strings.Contains(html, "[property") {
		t.Errorf("shortcode not rendered against the note: %s", html)
	}
}

// Markdown escapes the shortcode's quotes; the browser shows them as typed,
// exactly as the detail page does.
func TestPreviewDescription_WithoutAnEntityShortcodesStayLiteral(t *testing.T) {
	tc := SetupTestEnv(t)
	html := previewHTML(t, previewDescription(tc, "", "note", 0, "**x** [property path=\"Name\"]"))
	if !strings.Contains(html, "<strong>x</strong>") {
		t.Errorf("markdown not rendered on a create-form preview: %s", html)
	}
	if !strings.Contains(html, `[property path=&quot;Name&quot;]`) {
		t.Errorf("shortcode should stay literal without an entity: %s", html)
	}
}

func TestPreviewDescription_TypesWithoutShortcodesLeaveThemLiteral(t *testing.T) {
	tc := SetupTestEnv(t)
	tag := &models.Tag{Name: "literal tag"}
	tc.DB.Create(tag)
	html := previewHTML(t, previewDescription(tc, "", "tag", tag.ID, `[property path="Name"]`))
	if !strings.Contains(html, `[property path=&quot;Name&quot;]`) {
		t.Errorf("a Tag description renders shortcodes literally on its detail page; preview must too: %s", html)
	}
}

func TestPreviewDescription_UnknownEntityIsNotFound(t *testing.T) {
	tc := SetupTestEnv(t)
	// The route exists (a create-form preview, no id, succeeds)...
	previewHTML(t, previewDescription(tc, "", "query", 0, "x"))
	// ...and a missing id is a 404 from it.
	for _, entity := range []string{"note", "tag", "query", "relationType"} {
		if rr := previewDescription(tc, "", entity, 999999, "x"); rr.Code != http.StatusNotFound {
			t.Errorf("%s: expected 404 for a missing id, got %d", entity, rr.Code)
		}
	}
}

// Every entity type's preview is gated like saving that entity's Description:
// the route sits under the entity's own path prefix (server/authz_policy.go).
func TestPreviewDescription_RoleMatrix(t *testing.T) {
	tc := setupAuthEnv(t)
	userB := roleBearer(t, tc, models.RoleUser)
	editorB := roleBearer(t, tc, models.RoleEditor)
	guestB := roleBearer(t, tc, models.RoleGuest)
	adminB := roleBearer(t, tc, models.RoleAdmin)

	cases := []struct {
		entity              string
		user, editor, admin bool
	}{
		{"note", true, true, true},
		{"tag", true, true, true},
		{"query", false, true, true},
		{"relation", false, true, true},
		{"relationType", false, true, true},
		{"noteType", false, true, true},
		{"category", false, false, true},
		{"resourceCategory", false, false, true},
		{"templatePartial", false, false, true},
	}
	for _, c := range cases {
		check := func(who, bearer string, allowed bool) {
			rr := previewDescription(tc, bearer, c.entity, 0, "x")
			if allowed && rr.Code != http.StatusOK {
				t.Errorf("%s → %s preview: expected 200, got %d", who, c.entity, rr.Code)
			}
			if !allowed && rr.Code != http.StatusForbidden {
				t.Errorf("%s → %s preview: expected 403, got %d", who, c.entity, rr.Code)
			}
		}
		check("user", userB, c.user)
		check("editor", editorB, c.editor)
		check("admin", adminB, c.admin)
		check("guest", guestB, false)
	}
}

func TestPreviewDescription_ScopedPrincipalIsConfined(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildScopingFixture(t, tc)

	if rr := previewDescription(tc, f.bearer, "note", f.nOutID, "x"); rr.Code != http.StatusNotFound {
		t.Errorf("out-of-subtree note: expected 404, got %d", rr.Code)
	}
	if rr := previewDescription(tc, f.bearer, "group", f.outsideID, "x"); rr.Code != http.StatusNotFound {
		t.Errorf("out-of-subtree group: expected 404, got %d", rr.Code)
	}
	html := previewHTML(t, previewDescription(tc, f.bearer, "note", f.nInID, `[property path="Name"]`))
	if !strings.Contains(html, "sf-nIn") {
		t.Errorf("in-subtree note should preview with its shortcodes: %s", html)
	}
	// An [mrql] in an in-subtree note's preview sees only the subtree.
	html = previewHTML(t, previewDescription(tc, f.bearer, "note", f.nInID, `[mrql query="type = note" format="list"]`))
	if strings.Contains(html, "sf-nOut") {
		t.Errorf("preview's [mrql] leaked an out-of-subtree note: %s", html)
	}
	if !strings.Contains(html, "sf-nIn") {
		t.Errorf("preview's [mrql] did not list the in-subtree note, so the check above proves nothing: %s", html)
	}
}
