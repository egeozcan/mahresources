package api_tests

import (
	"encoding/json"
	"fmt"
	"html"
	"mahresources/application_context"
	"mahresources/models"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// GET /v1/note/renderedTextBlocks?noteId=N renders every text block of a note
// the way the note's Description is rendered on its page (markdown, mentions,
// shortcodes against the note), as <template data-rendered-text-block="ID">
// elements. docs/plans/2026-10-19-markdown-description-editor-design.md,
// decision 13.

func textBlockJSON(t *testing.T, text string) string {
	t.Helper()
	b, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func getRenderedTextBlocks(tc *TestContext, bearer string, noteID uint) *httptest.ResponseRecorder {
	headers := map[string]string{}
	if bearer != "" {
		headers["Authorization"] = bearer
	}
	return doReq(tc, http.MethodGet, fmt.Sprintf("/v1/note/renderedTextBlocks?noteId=%d", noteID), headers, nil, nil)
}

var renderedBlockRe = regexp.MustCompile(`(?s)<template data-rendered-text-block="(\d+)"[^>]*>(.*?)</template>`)

func renderedBlocks(t *testing.T, rr *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	out := map[string]string{}
	for _, m := range renderedBlockRe.FindAllStringSubmatch(rr.Body.String(), -1) {
		out[m[1]] = m[2]
	}
	return out
}

func TestRenderedTextBlocks_RenderLikeTheDescription(t *testing.T) {
	tc := SetupTestEnv(t)
	note := &models.Note{Name: "Block Host Note"}
	tc.DB.Create(note)
	group := &models.Group{Name: "Block Mention"}
	tc.DB.Create(group)

	first := tc.CreateDummyBlock(note.ID, "text", textBlockJSON(t, "# Heading\n\n- item"), "d")
	second := tc.CreateDummyBlock(note.ID, "text", textBlockJSON(t, fmt.Sprintf(`[property path="Name"] @[group:%d:Block Mention]`, group.ID)), "h")
	tc.CreateDummyBlock(note.ID, "divider", "{}", "p")

	blocks := renderedBlocks(t, getRenderedTextBlocks(tc, "", note.ID))
	if len(blocks) != 2 {
		t.Fatalf("expected the two text blocks only, got %d: %v", len(blocks), blocks)
	}
	if b := blocks[fmt.Sprint(first.ID)]; !strings.Contains(b, "<h1") || !strings.Contains(b, "<li>item</li>") {
		t.Errorf("markdown headings and lists not rendered: %s", b)
	}
	b := blocks[fmt.Sprint(second.ID)]
	if !strings.Contains(b, "Block Host Note") {
		t.Errorf("shortcode not rendered against the note: %s", b)
	}
	if !strings.Contains(b, fmt.Sprintf("/group?id=%d", group.ID)) {
		t.Errorf("mention not rendered: %s", b)
	}
}

// Each <template> says which text it was rendered from, escaped for an
// attribute, so the client can tell an outdated rendering from a current one
// even while its own copy of a block holds text not saved yet.
func TestRenderedTextBlocks_CarryTheirSourceText(t *testing.T) {
	tc := SetupTestEnv(t)
	note := &models.Note{Name: "Source Host"}
	tc.DB.Create(note)
	text := "a \"quoted\" <b>tag</b> & 'apostrophe'\r\nnext line"
	block := tc.CreateDummyBlock(note.ID, "text", textBlockJSON(t, text), "d")

	body := getRenderedTextBlocks(tc, "", note.ID).Body.String()
	m := regexp.MustCompile(fmt.Sprintf(`<template data-rendered-text-block="%d" data-source="([^"]*)">`, block.ID)).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no data-source attribute: %s", body)
	}
	if got := html.UnescapeString(m[1]); got != text {
		t.Errorf("data-source = %q, want %q", got, text)
	}
}

// Display rendering uses the page's limits, not the form preview's 5-row cap,
// so a block shows the same rows before and after an edit.
func TestRenderedTextBlocks_NoPreviewRowCap(t *testing.T) {
	tc := SetupTestEnv(t)
	note := &models.Note{Name: "Cap Host"}
	tc.DB.Create(note)
	for i := range 7 {
		tc.DB.Create(&models.Note{Name: fmt.Sprintf("capnote-%d", i)})
	}
	// scope="global": the host note has no owner, so the default entity scope
	// would match nothing.
	block := tc.CreateDummyBlock(note.ID, "text", textBlockJSON(t, `[mrql query='type = note' limit=20 format=list scope="global"]`), "d")

	b := renderedBlocks(t, getRenderedTextBlocks(tc, "", note.ID))[fmt.Sprint(block.ID)]
	if n := strings.Count(b, "capnote-"); n != 7 {
		t.Errorf("expected all 7 notes (no preview cap), got %d: %s", n, b)
	}
}

func TestRenderedTextBlocks_ScopedPrincipalIsConfined(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildScopingFixture(t, tc)
	tc.CreateDummyBlock(f.nOutID, "text", textBlockJSON(t, "secret"), "d")
	in := tc.CreateDummyBlock(f.nInID, "text", textBlockJSON(t, `[property path="Name"]`), "d")

	if rr := getRenderedTextBlocks(tc, f.bearer, f.nOutID); rr.Code != http.StatusNotFound {
		t.Errorf("out-of-subtree note: expected 404, got %d (%s)", rr.Code, rr.Body.String())
	}
	if b := renderedBlocks(t, getRenderedTextBlocks(tc, f.bearer, f.nInID))[fmt.Sprint(in.ID)]; !strings.Contains(b, "sf-nIn") {
		t.Errorf("in-subtree note's block should render: %s", b)
	}
}

// Reading a note's blocks is a read: a guest may, for a note it can view.
func TestRenderedTextBlocks_GuestMayRead(t *testing.T) {
	tc := setupAuthEnv(t)
	scope := &models.Group{Name: "guest scope"}
	tc.DB.Create(scope)
	note := &models.Note{Name: "Guest Readable", OwnerId: &scope.ID}
	tc.DB.Create(note)
	block := tc.CreateDummyBlock(note.ID, "text", textBlockJSON(t, "**hello**"), "d")
	guest, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "guestreader", Password: "password1", Role: models.RoleGuest, ScopeGroupId: &scope.ID,
	})
	if err != nil {
		t.Fatalf("create guest: %v", err)
	}
	raw, _, err := tc.AppCtx.CreateApiToken(guest.ID, "t", nil)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	b := renderedBlocks(t, getRenderedTextBlocks(tc, "Bearer "+raw, note.ID))[fmt.Sprint(block.ID)]
	if !strings.Contains(b, "<strong>hello</strong>") {
		t.Errorf("guest should get the rendered block: %s", b)
	}
}
