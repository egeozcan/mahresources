package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
)

// [excerpt] runs no query: it previews the entity the surface already loaded.
// These tests show that a group-confined viewer therefore never sees an excerpt
// of an entity outside their subtree on any surface that renders it, that the
// more= link survives a read-only render, and that the public share page,
// which serves no detail pages, gets no link.

const (
	excerptInText  = "in-subtree-text alpha beta"
	excerptOutText = "out-subtree-text gamma delta"
)

// buildExcerptFixture is buildScopingFixture with Descriptions on its two
// notes. Every query in these tests is scope="global", so only the viewer's
// own subtree confines what it returns.
func buildExcerptFixture(t *testing.T, tc *TestContext) scopeFixture {
	t.Helper()
	f := buildScopingFixture(t, tc)
	for id, text := range map[uint]string{f.nInID: excerptInText, f.nOutID: excerptOutText} {
		if err := tc.DB.Model(&models.Note{}).Where("id = ?", id).Update("description", text).Error; err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func TestExcerptShortcode_DescriptionPreviewIsScoped(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildExcerptFixture(t, tc)
	admin := roleBearer(t, tc, models.RoleAdmin)

	list := `[mrql query="type = note" scope="global"]<p>[excerpt]</p>[/mrql]`
	html := previewHTML(t, previewDescription(tc, f.bearer, "note", f.nInID, list))
	if !strings.Contains(html, "in-subtree-text") || strings.Contains(html, "out-subtree-text") {
		t.Errorf("confined user's [mrql] excerpts: %s", html)
	}
	if html := previewHTML(t, previewDescription(tc, admin, "note", f.nInID, list)); !strings.Contains(html, "out-subtree-text") {
		t.Errorf("control: an admin should see the outside note's excerpt: %s", html)
	}

	using := `[using query='type = note AND name = "sf-nOut"' scope="global"][excerpt][else]none[/using]`
	html = previewHTML(t, previewDescription(tc, f.bearer, "note", f.nInID, using))
	if !strings.Contains(html, "none") || strings.Contains(html, "out-subtree-text") {
		t.Errorf("confined user's [using] excerpt: %s", html)
	}
	if html := previewHTML(t, previewDescription(tc, admin, "note", f.nInID, using)); !strings.Contains(html, "out-subtree-text") {
		t.Errorf("control: an admin should switch to the outside note: %s", html)
	}
}

func TestExcerptShortcode_ReadOnlyGuestPagesAndCards(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildExcerptFixture(t, tc)
	guest := scopedRoleBearer(t, tc, models.RoleGuest, f.rootID, "excerpt-guest")
	admin := roleBearer(t, tc, models.RoleAdmin)

	cat := &models.Category{Name: "Excerpt Cat", CustomHeader: `<div id="ex-header">[mrql query="type = note" scope="global"]<p>[excerpt words="2" more="Read more"]</p>[/mrql]</div>`}
	nt := &models.NoteType{Name: "Excerpt Type", CustomSummary: `<p class="ex-card">[excerpt words="2" more="Read more"]</p>`}
	for _, v := range []any{cat, nt} {
		if err := tc.DB.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := tc.DB.Model(&models.Group{}).Where("id = ?", f.childID).Update("category_id", cat.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tc.DB.Model(&models.Note{}).Where("id IN ?", []uint{f.nInID, f.nOutID}).Update("note_type_id", nt.ID).Error; err != nil {
		t.Fatal(err)
	}

	// A guest cannot write, so the page renders read-only; the link is
	// navigation and stays.
	link := fmt.Sprintf(`in-subtree-text alpha… <a href="/note?id=%d" class="underline" aria-label="Read more: sf-nIn">Read more</a>`, f.nInID)
	for _, path := range []string{fmt.Sprintf("/group?id=%d", f.childID), "/notes"} {
		page := getHTML(t, tc, guest, path)
		if !strings.Contains(page, link) {
			t.Errorf("guest %s lacks the excerpt with its link", path)
		}
		if strings.Contains(page, "out-subtree-text") {
			t.Errorf("guest %s shows the outside note's excerpt", path)
		}
		if !strings.Contains(getHTML(t, tc, admin, path), "out-subtree-text") {
			t.Errorf("control: admin %s should show the outside note's excerpt", path)
		}
	}
}

func TestExcerptShortcode_DeferredAndMRQLRenderAreScoped(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildExcerptFixture(t, tc)
	guest := scopedRoleBearer(t, tc, models.RoleGuest, f.rootID, "excerpt-guest")

	body := `[mrql query="type = note" scope="global"]<p>[excerpt]</p>[/mrql]`
	payload, _ := json.Marshal(map[string]any{"token": signToken(tc, "group", f.childID, body)})
	rr := doReq(tc, http.MethodPost, "/v1/shortcodes/deferred",
		map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": guest}, nil, bytes.NewReader(payload))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "in-subtree-text") || strings.Contains(rr.Body.String(), "out-subtree-text") {
		t.Errorf("deferred body: %d %s", rr.Code, rr.Body.String())
	}

	nt := &models.NoteType{Name: "Excerpt Result", CustomMRQLResult: `<i class="ex-result">[excerpt]</i>`}
	if err := tc.DB.Create(nt).Error; err != nil {
		t.Fatal(err)
	}
	if err := tc.DB.Model(&models.Note{}).Where("id IN ?", []uint{f.nInID, f.nOutID}).Update("note_type_id", nt.ID).Error; err != nil {
		t.Fatal(err)
	}
	rr = doReq(tc, http.MethodPost, "/v1/mrql?render=1",
		map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": guest}, nil,
		strings.NewReader(`{"query":"type = note"}`))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "in-subtree-text") || strings.Contains(rr.Body.String(), "out-subtree-text") {
		t.Errorf("MRQL render API: %d %s", rr.Code, rr.Body.String())
	}
}

func TestExcerptShortcode_SharePageHasNoMoreLink(t *testing.T) {
	tc := SetupTestEnv(t)
	nt := &models.NoteType{Name: "Shared Excerpt", ApplyTemplatesToShares: true,
		CustomHeader: `<div id="ex-share">[excerpt words="1" more="Read more"]</div>`}
	if err := tc.DB.Create(nt).Error; err != nil {
		t.Fatal(err)
	}
	note := &models.Note{Name: "Shared note", Description: "**first** second third", NoteTypeId: &nt.ID}
	if err := tc.DB.Create(note).Error; err != nil {
		t.Fatal(err)
	}

	share := renderShare(t, tc, note.ID)
	if !strings.Contains(share, `<div id="ex-share">first…</div>`) {
		t.Errorf("share page lacks the excerpt without a link")
	}
	if strings.Contains(share, "Read more") {
		t.Errorf("share page has a more= link to a page it does not serve")
	}

	// Control: the note's own page on the main server keeps the link.
	page := getHTML(t, tc, "", fmt.Sprintf("/note?id=%d", note.ID))
	if !strings.Contains(page, fmt.Sprintf(`first… <a href="/note?id=%d" class="underline" aria-label="Read more: Shared note">Read more</a>`, note.ID)) {
		t.Errorf("note page lacks the more= link")
	}
}
