package api_tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/models"
	"mahresources/models/types"
	"mahresources/server"
)

// renderShare shares the given note and returns the /s/<token> page HTML.
func renderShare(t *testing.T, tc *TestContext, noteID uint) string {
	t.Helper()
	token, err := tc.AppCtx.ShareNote(noteID)
	if err != nil {
		t.Fatalf("share note: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/s/"+token, nil)
	w := httptest.NewRecorder()
	server.NewShareServer(tc.AppCtx).Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

// Phase 6 item 2: when a NoteType opts in via ApplyTemplatesToShares, its
// CustomHeader and CustomCSS render on the public share page in restricted mode.
func TestShareServer_NoteTemplates_FlagOn(t *testing.T) {
	tc := SetupTestEnv(t)

	header := `HDR=[property path="Name"] R=[meta path="rating" default="N/A"] ` +
		`Q=[mrql query="type = 'note'" value="count"] P=[plugin:demo:widget]`
	nt := &models.NoteType{
		Name:                   "Shareable",
		ApplyTemplatesToShares: true,
		CustomHeader:           header,
		CustomCSS:              ".custom-note-header{color:red}",
	}
	if err := tc.DB.Create(nt).Error; err != nil {
		t.Fatalf("create note type: %v", err)
	}
	note := &models.Note{Name: "SharedNote", NoteTypeId: &nt.ID, Meta: types.JSON(`{"rating":5}`)}
	if err := tc.DB.Create(note).Error; err != nil {
		t.Fatalf("create note: %v", err)
	}

	body := renderShare(t, tc, note.ID)

	// CustomHeader is rendered above the note content.
	assertContains(t, body, `class="custom-note-header`, "CustomHeader wrapper present")
	// [property] resolves against the note (its own Name).
	assertContains(t, body, "HDR=SharedNote", "property renders note name")
	// CustomCSS injected as an inline style block.
	assertContains(t, body, "<style>.custom-note-header{color:red}</style>", "CustomCSS style block")

	// [mrql] and [plugin] are inert on the anonymous surface: rendered as HTML
	// comments, never as results and never as leaked raw shortcode text.
	assertContains(t, body, "<!-- mr:mrql unavailable in this context -->", "mrql comment")
	assertContains(t, body, "<!-- mr:plugin unavailable in this context -->", "plugin comment")
	assertNotContains(t, body, "[mrql", "no raw [mrql leaked")
	assertNotContains(t, body, "[plugin:", "no raw [plugin leaked")

	// [meta] renders read-only — no edit affordance that would POST to the primary server.
	assertContains(t, body, `data-editable="false"`, "meta read-only")
	assertNotContains(t, body, `data-editable="true"`, "no editable meta")
}

// Flag off: a NoteType with the same templates but ApplyTemplatesToShares=false
// must not change the share page — no header, no injected style.
func TestShareServer_NoteTemplates_FlagOff(t *testing.T) {
	tc := SetupTestEnv(t)

	nt := &models.NoteType{
		Name:                   "NotShared",
		ApplyTemplatesToShares: false,
		CustomHeader:           `HDR=[property path="Name"]`,
		CustomCSS:              ".custom-note-header{color:red}",
	}
	if err := tc.DB.Create(nt).Error; err != nil {
		t.Fatalf("create note type: %v", err)
	}
	note := &models.Note{Name: "SharedNote", NoteTypeId: &nt.ID}
	if err := tc.DB.Create(note).Error; err != nil {
		t.Fatalf("create note: %v", err)
	}

	body := renderShare(t, tc, note.ID)

	assertNotContains(t, body, "custom-note-header", "no CustomHeader wrapper when opted out")
	assertNotContains(t, body, "HDR=SharedNote", "no rendered header content")
	assertNotContains(t, body, ".custom-note-header{color:red}", "no injected CustomCSS")
}

// A shared note whose type has no opt-in and no templates renders exactly as
// before Phase 6 — the baseline behaviour is unchanged.
func TestShareServer_NoteTemplates_NoNoteType(t *testing.T) {
	tc := SetupTestEnv(t)
	note := tc.CreateDummyNote("Plain shared note")

	body := renderShare(t, tc, note.ID)
	assertNotContains(t, body, "custom-note-header", "typeless note has no custom header")
}

func assertContains(t *testing.T, haystack, needle, msg string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("%s: expected to find %q", msg, needle)
	}
}

func assertNotContains(t *testing.T, haystack, needle, msg string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Errorf("%s: expected NOT to find %q", msg, needle)
	}
}
