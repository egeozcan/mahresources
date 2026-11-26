package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/server/api_handlers"
)

// [adjacent] reads siblings through the request's principal-bound handle, so a
// group-confined viewer never sees a sibling outside their subtree, on any
// surface that renders it. Every other surface renders the unavailable comment.

const adjacentUnavailable = "adjacent unavailable in this context"

type adjacentFixture struct {
	top, s, sSibling, child, outside *models.Group
	notes                            []*models.Note
	series                           []*models.Resource
	noteType                         *models.NoteType
}

func buildAdjacentFixture(t *testing.T, tc *TestContext) adjacentFixture {
	t.Helper()
	at := func(minute int) time.Time { return time.Date(2024, 1, 1, 0, minute, 0, 0, time.UTC) }
	mk := func(v any) {
		t.Helper()
		if err := tc.DB.Create(v).Error; err != nil {
			t.Fatalf("create %T: %v", v, err)
		}
	}
	cat := &models.Category{Name: "Adjacent Cat", CustomHeader: `<div id="adj">[adjacent label="Groups"]</div>`}
	mk(cat)
	f := adjacentFixture{}
	f.top = &models.Group{Name: "adj-top", CategoryId: &cat.ID, CreatedAt: at(0)}
	mk(f.top)
	f.s = &models.Group{Name: "adj-S", OwnerId: &f.top.ID, CategoryId: &cat.ID, CreatedAt: at(1)}
	mk(f.s)
	f.sSibling = &models.Group{Name: "adj-S-sibling-secret", OwnerId: &f.top.ID, CategoryId: &cat.ID, CreatedAt: at(2)}
	mk(f.sSibling)
	f.child = &models.Group{Name: "adj-child", OwnerId: &f.s.ID, CreatedAt: at(3)}
	mk(f.child)
	f.outside = &models.Group{Name: "adj-outside", CreatedAt: at(4)}
	mk(f.outside)

	f.noteType = &models.NoteType{Name: "Meetings", CustomHeader: `<div id="adj">[adjacent label="Meetings"]</div>`, ApplyTemplatesToShares: true}
	mk(f.noteType)
	for i, name := range []string{"adj-n1", "adj-n2", "adj-n3"} {
		n := &models.Note{Name: name, OwnerId: &f.child.ID, NoteTypeId: &f.noteType.ID, CreatedAt: at(10 + i)}
		mk(n)
		f.notes = append(f.notes, n)
	}

	rc := &models.ResourceCategory{Name: "Pages", CustomHeader: `<div id="adj">[adjacent series="true" label="Pages"]</div>`, CustomLightbox: `[adjacent series="true"]`}
	mk(rc)
	series := &models.Series{Name: "adj-book", Slug: "adj-book"}
	mk(series)
	owners := []uint{f.child.ID, f.outside.ID, f.s.ID}
	for i, name := range []string{"adj-p1", "adj-p2-secret", "adj-p3"} {
		owner := owners[i]
		r := &models.Resource{Name: name, OwnerId: &owner, ResourceCategoryId: rc.ID, SeriesID: &series.ID, CreatedAt: at(20 + i), Location: name + ".txt"}
		mk(r)
		f.series = append(f.series, r)
	}
	return f
}

func getHTML(t *testing.T, tc *TestContext, bearer, path string) string {
	t.Helper()
	headers := map[string]string{"Accept": "text/html"}
	if bearer != "" {
		headers["Authorization"] = bearer
	}
	rr := doReq(tc, http.MethodGet, path, headers, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, rr.Code, rr.Body.String())
	}
	return rr.Body.String()
}

// adjacentNav cuts the rendered nav (or "") out of a page, so assertions about
// names are not confused by the rest of the page, which names the entity's
// owner and so on.
func adjacentNav(html string) string {
	start := strings.Index(html, `<nav class="shortcode-adjacent`)
	if start < 0 {
		return ""
	}
	end := strings.Index(html[start:], "</nav>")
	return html[start : start+end+len("</nav>")]
}

func TestAdjacentShortcode_PagesAreScoped(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildAdjacentFixture(t, tc)
	guest := scopedRoleBearer(t, tc, models.RoleGuest, f.s.ID, "adj-guest")
	admin := roleBearer(t, tc, models.RoleAdmin)

	// Notes inside the subtree link each other.
	nav := adjacentNav(getHTML(t, tc, guest, fmt.Sprintf("/note?id=%d", f.notes[1].ID)))
	for _, want := range []string{`aria-label="Meetings"`, "Previous: adj-n1", "Next: adj-n3", fmt.Sprintf(`/note?id=%d`, f.notes[2].ID)} {
		if !strings.Contains(nav, want) {
			t.Errorf("guest note page nav lacks %q: %q", want, nav)
		}
	}

	// The subtree root's siblings are outside the subtree: nothing renders.
	page := getHTML(t, tc, guest, fmt.Sprintf("/group?id=%d", f.s.ID))
	if nav := adjacentNav(page); nav != "" || strings.Contains(page, "adj-S-sibling-secret") || strings.Contains(page, fmt.Sprintf("/group?id=%d", f.sSibling.ID)) {
		t.Errorf("guest saw the subtree root's sibling: %q", nav)
	}
	if nav := adjacentNav(getHTML(t, tc, admin, fmt.Sprintf("/group?id=%d", f.s.ID))); !strings.Contains(nav, "Next: adj-S-sibling-secret") {
		t.Errorf("control: admin should see the sibling, got %q", nav)
	}

	// A Series member outside the subtree is skipped.
	page = getHTML(t, tc, guest, fmt.Sprintf("/resource?id=%d", f.series[0].ID))
	nav = adjacentNav(page)
	if !strings.Contains(nav, "Next: adj-p3") || strings.Contains(page, "adj-p2-secret") || strings.Contains(nav, fmt.Sprintf("?id=%d", f.series[1].ID)) {
		t.Errorf("guest series nav should skip the outside member: %q", nav)
	}
	if nav := adjacentNav(getHTML(t, tc, admin, fmt.Sprintf("/resource?id=%d", f.series[0].ID))); !strings.Contains(nav, "Next: adj-p2-secret") {
		t.Errorf("control: admin should step to the outside member, got %q", nav)
	}
}

func TestAdjacentShortcode_DeferredAndPreviews(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildAdjacentFixture(t, tc)
	guest := scopedRoleBearer(t, tc, models.RoleGuest, f.s.ID, "adj-guest")

	deferred := func(bearer, entityType string, id uint, body string) string {
		t.Helper()
		payload, _ := json.Marshal(map[string]any{"token": signToken(tc, entityType, id, body)})
		rr := doReq(tc, http.MethodPost, "/v1/shortcodes/deferred",
			map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": bearer}, nil, bytes.NewReader(payload))
		if rr.Code != http.StatusOK {
			t.Fatalf("deferred: %d %s", rr.Code, rr.Body.String())
		}
		var resp struct {
			HTML string `json:"html"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		return resp.HTML
	}
	if html := deferred(guest, "note", f.notes[0].ID, `[adjacent]`); !strings.Contains(html, "Next: adj-n2") || strings.Contains(html, `rel="prev"`) {
		t.Errorf("deferred note body: %q", html)
	}
	if html := deferred(guest, "group", f.s.ID, `<b>[adjacent]</b>`); html != "<b></b>" {
		t.Errorf("deferred subtree-root body leaked its siblings: %q", html)
	}
	if html := deferred(guest, "resource", f.series[0].ID, `[adjacent series="true"]`); !strings.Contains(html, "Next: adj-p3") || strings.Contains(html, "secret") {
		t.Errorf("deferred series body: %q", html)
	}

	// The description preview renders as the viewer, confined the same way.
	user := scopedRoleBearer(t, tc, models.RoleUser, f.s.ID, "adj-user")
	if html := previewHTML(t, previewDescription(tc, user, "group", f.s.ID, `[adjacent]`)); strings.Contains(html, "secret") || strings.Contains(html, "<nav") {
		t.Errorf("description preview leaked the subtree root's siblings: %q", html)
	}
	if html := previewHTML(t, previewDescription(tc, user, "note", f.notes[2].ID, `[adjacent]`)); !strings.Contains(html, "Previous: adj-n2") {
		t.Errorf("description preview of a note: %q", html)
	}

	// The template preview is an editor/admin surface; it renders the nav for
	// the previewed entity.
	admin := roleBearer(t, tc, models.RoleAdmin)
	body, _ := json.Marshal(map[string]any{"entityId": f.notes[1].ID, "content": `[adjacent label="Meetings"]`})
	rr := doReq(tc, http.MethodPost, "/v1/noteType/previewTemplate",
		map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": admin}, nil, bytes.NewReader(body))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Previous: adj-n1") || !strings.Contains(rr.Body.String(), "Next: adj-n3") {
		t.Errorf("template preview: %d %s", rr.Code, rr.Body.String())
	}
}

func TestAdjacentShortcode_SharesThePageBudget(t *testing.T) {
	// Two units: the first [adjacent] and the [using] lookup take them, so the
	// [adjacent] inside the [using] is refused.
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) { c.MRQLPageQueryBudget = 2 })
	f := buildAdjacentFixture(t, tc)
	header := fmt.Sprintf(`[adjacent label="First"][using type="note" id="%d"][adjacent label="Second"][/using]`, f.notes[0].ID)
	if err := tc.DB.Model(f.noteType).Update("custom_header", header).Error; err != nil {
		t.Fatal(err)
	}
	page := getHTML(t, tc, "", fmt.Sprintf("/note?id=%d", f.notes[1].ID))
	if !strings.Contains(page, `aria-label="First"`) {
		t.Errorf("the first [adjacent] should render")
	}
	if strings.Contains(page, `aria-label="Second"`) || !strings.Contains(page, "inline query budget exceeded (2 per page)") {
		t.Errorf("the second [adjacent] should be the budget marker")
	}
}

func TestAdjacentShortcode_UnavailableSurfaces(t *testing.T) {
	tc := SetupTestEnv(t)
	f := buildAdjacentFixture(t, tc)

	// The MRQL render API is bound for MRQL only, with no ORM scope.
	if err := tc.DB.Model(f.noteType).Update("custom_mrql_result", `<i>[adjacent]</i>`).Error; err != nil {
		t.Fatal(err)
	}
	rr := tc.MakeRequest(http.MethodPost, "/v1/mrql?render=1", map[string]any{"query": `type = note AND name = "adj-n2"`})
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), adjacentUnavailable) || strings.Contains(rr.Body.String(), "shortcode-adjacent") {
		t.Errorf("MRQL render API: %d %s", rr.Code, rr.Body.String())
	}

	// The entity picker renders cards, not a detail context.
	if err := tc.DB.Model(f.noteType).Update("custom_entity_picker_result", `<i>[adjacent]</i>`).Error; err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	api_handlers.GetEntityPickerHandler(tc.AppCtx)(w, httptest.NewRequest("GET", "/v1/entity-picker?entity=note", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), adjacentUnavailable) || strings.Contains(w.Body.String(), "shortcode-adjacent") {
		t.Errorf("entity picker: %d %s", w.Code, w.Body.String())
	}

	// The public share page.
	share := renderShare(t, tc, f.notes[1].ID)
	if !strings.Contains(share, adjacentUnavailable) || strings.Contains(share, "shortcode-adjacent") || strings.Contains(share, "adj-n1") {
		t.Errorf("share page rendered [adjacent]")
	}

	// A hover card is a tooltip: its content cannot be reached by keyboard, so
	// it holds no navigation.
	if err := tc.DB.Model(f.noteType).Update("custom_hover_card", `<i>[adjacent]</i>`).Error; err != nil {
		t.Fatal(err)
	}
	card := getHTML(t, tc, "", fmt.Sprintf("/hovercard?type=note&id=%d", f.notes[1].ID))
	if !strings.Contains(card, adjacentUnavailable) || strings.Contains(card, "shortcode-adjacent") {
		t.Errorf("hover card rendered [adjacent]: %s", card)
	}

	// The lightbox's JSON render.
	rr = doReq(tc, http.MethodGet, fmt.Sprintf("/resource.json?id=%d", f.series[0].ID), map[string]string{"Accept": "application/json"}, nil, nil)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), adjacentUnavailable) || strings.Contains(rr.Body.String(), "shortcode-adjacent") {
		t.Errorf("lightbox JSON: %d %s", rr.Code, rr.Body.String())
	}
}

// The AI template run tools render previews for a model, which must not read
// neighbouring rows.
func TestAdjacentShortcode_TemplateRunToolPreviewIsUnavailable(t *testing.T) {
	f := newToolFixture(t, nil, nil)
	out := f.call(t, "preview", map[string]any{"field": "CustomHeader", "content": `<i>[adjacent]</i>`})
	if out.IsError {
		t.Fatal(out.Text)
	}
	html := decode(t, payload(t, out))["html"].(string)
	if !strings.Contains(html, adjacentUnavailable) || strings.Contains(html, "shortcode-adjacent") {
		t.Errorf("run-tool preview rendered [adjacent]: %s", html)
	}
}
