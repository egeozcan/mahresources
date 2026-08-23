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
	HTML      string          `json:"html"`
	CSS       string          `json:"css"`
	Entity    json.RawMessage `json:"entity"`
	Issues    []lintIssue     `json:"issues"`
	CSSIssues []lintIssue     `json:"cssIssues"`
}

// allIssues is what the pane displays: the two lists concatenated. Assertions
// about *which* buffer a diagnostic came from must read the fields directly.
func (r previewResponse) allIssues() []lintIssue {
	return append(append([]lintIssue{}, r.Issues...), r.CSSIssues...)
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

// TestPreviewTemplate_ReturnsEntity verifies the response carries the entity
// marshaled exactly like the display pages' `{{ group|json }}` (plain
// json.Marshal of the model), so the preview frame can recreate the
// `x-data="{ entity: ... }"` Alpine scope those pages wrap the Custom* slots in.
func TestPreviewTemplate_ReturnsEntity(t *testing.T) {
	tc := SetupTestEnv(t)

	g := &models.Group{Name: "Entity Scope Group"}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	rr := tc.MakeRequest(http.MethodPost, "/v1/category/previewTemplate", map[string]any{
		"entityId": g.ID,
		"content":  `<span x-text="entity.Name"></span>`,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	var resp previewResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Entity) == 0 || string(resp.Entity) == "null" {
		t.Fatalf("expected entity JSON in preview response, got %q", string(resp.Entity))
	}

	var entity struct {
		ID   uint   `json:"ID"`
		Name string `json:"Name"`
	}
	if err := json.Unmarshal(resp.Entity, &entity); err != nil {
		t.Fatalf("entity unmarshal: %v", err)
	}
	if entity.ID != g.ID || entity.Name != "Entity Scope Group" {
		t.Errorf("expected entity {ID:%d Name:%q}, got %+v", g.ID, "Entity Scope Group", entity)
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

func countPreviewIssues(issues []lintIssue, want string) int {
	n := 0
	for _, iss := range issues {
		if strings.Contains(iss.Message, want) {
			n++
		}
	}
	return n
}

// TestPreviewTemplate_CSSPlacementIssues pins the preview pane's issue list
// against the editor gutter, which lints the same buffers through
// POST /v1/shortcodes/lint. Both halves were silent: the selected slot was
// never linted as a stylesheet even when it was the CustomCSS slot, and the
// separate css buffer was never linted at all.
func TestPreviewTemplate_CSSPlacementIssues(t *testing.T) {
	const cssBuffer = `.badge{color:[meta path="colour" inline="true"]}`

	// The content buffer alone, with no css to borrow a verdict from: the CSS
	// reading can only have come from the content pass, so this is the subtest
	// that actually pins the slot-driven mode.
	t.Run("a named CustomCSS slot makes the content buffer a stylesheet", func(t *testing.T) {
		tc := SetupTestEnv(t)
		g := &models.Group{Name: "CSS Content Only Group"}
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatalf("create group: %v", err)
		}

		rr := tc.MakeRequest(http.MethodPost, "/v1/category/previewTemplate", map[string]any{
			"entityId": g.ID,
			"slot":     "CustomCSS",
			"content":  cssBuffer,
		})
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
		}
		var resp previewResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if n := countPreviewIssues(resp.Issues, "CSS slot"); n != 1 {
			t.Fatalf("the content pass must judge a CustomCSS slot as CSS, got %d: %+v", n, resp.Issues)
		}
		if len(resp.CSSIssues) != 0 {
			t.Fatalf("no css buffer was sent, so its list must be empty: %+v", resp.CSSIssues)
		}
	})

	t.Run("the selected CustomCSS slot is judged as a stylesheet, once", func(t *testing.T) {
		tc := SetupTestEnv(t)
		g := &models.Group{Name: "CSS Slot Group"}
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatalf("create group: %v", err)
		}

		// What the editor sends with the CustomCSS slot selected: content and
		// css are the same buffer, so a naive "lint both" doubles every issue.
		rr := tc.MakeRequest(http.MethodPost, "/v1/category/previewTemplate", map[string]any{
			"entityId": g.ID,
			"slot":     "CustomCSS",
			"content":  cssBuffer,
			"css":      cssBuffer,
		})
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
		}
		var resp previewResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if n := countPreviewIssues(resp.allIssues(), "CSS slot"); n != 1 {
			t.Fatalf("expected exactly one CSS-placement issue, got %d: %+v", n, resp.allIssues())
		}
		// One document, so the second pass never ran.
		if len(resp.CSSIssues) != 0 {
			t.Fatalf("the css buffer is the content buffer here, so it is not linted twice: %+v", resp.CSSIssues)
		}
	})

	t.Run("the css buffer is linted while another slot is selected", func(t *testing.T) {
		tc := SetupTestEnv(t)
		g := &models.Group{Name: "Header Slot Group"}
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatalf("create group: %v", err)
		}

		rr := tc.MakeRequest(http.MethodPost, "/v1/category/previewTemplate", map[string]any{
			"entityId": g.ID,
			"slot":     "CustomHeader",
			"content":  `<h1>[property path="Name"]</h1>`,
			"css":      cssBuffer,
		})
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
		}
		var resp previewResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if n := countPreviewIssues(resp.CSSIssues, "CSS slot"); n != 1 {
			t.Fatalf("expected the css buffer's placement issue, got %d: %+v", n, resp.CSSIssues)
		}
		if n := countPreviewIssues(resp.Issues, "CSS slot"); n != 0 {
			t.Fatalf("the markup slot's own content must not be judged as CSS, got %d: %+v", n, resp.Issues)
		}
	})

	t.Run("an HTML slot is not judged as a stylesheet", func(t *testing.T) {
		tc := SetupTestEnv(t)
		g := &models.Group{Name: "Plain Slot Group"}
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatalf("create group: %v", err)
		}

		rr := tc.MakeRequest(http.MethodPost, "/v1/category/previewTemplate", map[string]any{
			"entityId": g.ID,
			"slot":     "CustomHeader",
			"content":  `<div>[meta path="colour" inline="true"]</div>`,
		})
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
		}
		var resp previewResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if n := countPreviewIssues(resp.Issues, "CSS slot"); n != 0 {
			t.Fatalf("an HTML slot must stay ordinary text, got %d: %+v", n, resp.Issues)
		}
	})

	// A client that predates the slot field still gets the css buffer linted,
	// and its content buffer degrades to the previous (non-CSS) judgement
	// rather than being blanked or refused.
	t.Run("an unnamed slot degrades to the previous behaviour", func(t *testing.T) {
		tc := SetupTestEnv(t)
		g := &models.Group{Name: "Legacy Client Group"}
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatalf("create group: %v", err)
		}

		rr := tc.MakeRequest(http.MethodPost, "/v1/category/previewTemplate", map[string]any{
			"entityId": g.ID,
			"content":  `<div>[meta path="colour" inline="true"]</div>`,
			"css":      cssBuffer,
		})
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
		}
		var resp previewResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if n := countPreviewIssues(resp.CSSIssues, "CSS slot"); n != 1 {
			t.Fatalf("expected the css buffer's placement issue, got %d: %+v", n, resp.CSSIssues)
		}
		if n := countPreviewIssues(resp.Issues, "CSS slot"); n != 0 {
			t.Fatalf("an unnamed slot reads content as markup, got %d: %+v", n, resp.Issues)
		}
	})

	// An older client with the CustomCSS slot selected names no slot and sends
	// one buffer as both content and css, so which document it is cannot be
	// known. Both readings are reported, minus what they agree on. Guessing CSS
	// instead would be unsafe in one direction and noisy in the other: the CSS
	// branch stands in place of the markup checks rather than adding to them,
	// so a markup buffer read as CSS loses its XSS warning, while linting both
	// naively repeats every mode-independent issue.
	t.Run("an unnamed slot sending one buffer twice gets both readings, once each", func(t *testing.T) {
		tc := SetupTestEnv(t)
		g := &models.Group{Name: "Legacy CSS Client Group"}
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatalf("create group: %v", err)
		}

		// raw= warns differently under each reading, and the missing path is an
		// error under both — so one buffer counts the passes and the two
		// placement messages count the modes.
		const buffer = `.badge{color:[meta inline="true" raw="true"]}`
		rr := tc.MakeRequest(http.MethodPost, "/v1/category/previewTemplate", map[string]any{
			"entityId": g.ID,
			"content":  buffer,
			"css":      buffer,
		})
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
		}
		var resp previewResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		all := resp.allIssues()
		if n := countPreviewIssues(all, "CSS slot"); n != 1 {
			t.Fatalf("expected the stylesheet reading's placement issue, got %d: %+v", n, all)
		}
		if n := countPreviewIssues(all, "becomes real elements"); n != 1 {
			t.Fatalf("the markup reading's XSS warning must survive, got %d: %+v", n, all)
		}
		if n := countPreviewIssues(all, `required attribute "path"`); n != 1 {
			t.Fatalf("what both readings agree on is reported once, got %d: %+v", n, all)
		}
	})

	// The converse, and the reason the dedupe may not be plain text equality:
	// with a markup slot selected the two buffers are two documents even when
	// they read the same, and the passes are entitled to disagree.
	t.Run("a markup slot and an identical css buffer are two documents", func(t *testing.T) {
		tc := SetupTestEnv(t)
		g := &models.Group{Name: "Identical Buffers Group"}
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatalf("create group: %v", err)
		}

		const buffer = `.badge{color:[meta inline="true"]}`
		rr := tc.MakeRequest(http.MethodPost, "/v1/category/previewTemplate", map[string]any{
			"entityId": g.ID,
			"slot":     "CustomHeader",
			"content":  buffer,
			"css":      buffer,
		})
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
		}
		var resp previewResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		// Only the css pass runs in CSS mode...
		if n := countPreviewIssues(resp.CSSIssues, "CSS slot"); n != 1 {
			t.Fatalf("expected the css buffer's placement issue, got %d: %+v", n, resp.CSSIssues)
		}
		if n := countPreviewIssues(resp.Issues, "CSS slot"); n != 0 {
			t.Fatalf("the markup slot's content is not a stylesheet, got %d: %+v", n, resp.Issues)
		}
		// ...but both documents are linted and neither is deduped away, so a
		// blanket equality dedupe would silently drop the header's diagnostics.
		if n := countPreviewIssues(resp.allIssues(), `required attribute "path"`); n != 2 {
			t.Fatalf("two documents must each be linted, got %d: %+v", n, resp.allIssues())
		}
	})
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
