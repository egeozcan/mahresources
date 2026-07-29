package api_tests

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"mahresources/models"
)

var regionTokenPattern = regexp.MustCompile(`data-shortcode-region="([^"]+)"`)

// renderGroupPage seeds a category whose CustomHeader is header, attaches a group
// to it, and returns the rendered display page HTML along with the group.
func renderGroupPage(t *testing.T, tc *TestContext, header string) (string, *models.Group) {
	t.Helper()

	cat := &models.Category{Name: "Reload Cat " + t.Name(), CustomHeader: header}
	if err := tc.DB.Create(cat).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
	catID := cat.ID
	g := &models.Group{Name: "Reload Group", CategoryId: &catID}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	rr := doReq(tc, http.MethodGet, "/group?id="+itoa(int(g.ID)),
		map[string]string{"Accept": "text/html"}, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /group: expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
	return rr.Body.String(), g
}

// A [reload] with no deferred block around it needs the slot itself to be
// reloadable, so the display page must wrap that slot in a region carrying a
// token for the whole slot body.
func TestReloadRegion_WrapsSlotContainingReloadButton(t *testing.T) {
	tc := SetupTestEnv(t)

	body, g := renderGroupPage(t, tc, `<div class="stats">[property path="Name"]</div>[reload label="Refresh"]`)

	if !strings.Contains(body, "<reload-shortcode>") {
		t.Fatalf("display page should render the reload button, got: %s", body)
	}
	m := regionTokenPattern.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("display page should wrap the slot in a reloadable region, got: %s", body)
	}

	// Change the entity behind the slot. The token must seal the slot's RAW body,
	// not the HTML it rendered to: sealing the rendered output would bake this
	// value in and make every reload return the same stale content.
	if err := tc.DB.Model(&models.Group{}).Where("id = ?", g.ID).Update("name", "Renamed Group").Error; err != nil {
		t.Fatalf("rename group: %v", err)
	}

	// The region token round-trips through the same endpoint [lazy] uses.
	rr := tc.MakeRequest(http.MethodPost, "/v1/shortcodes/deferred", map[string]any{"token": m[1]})
	if rr.Code != http.StatusOK {
		t.Fatalf("region token should render: got %d (body: %s)", rr.Code, rr.Body.String())
	}
	var resp deferredRenderResp
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(resp.HTML, "Renamed Group") || strings.Contains(resp.HTML, "Reload Group") {
		t.Errorf("reloaded region should re-evaluate its shortcodes against fresh data, got %q", resp.HTML)
	}
	if !strings.Contains(resp.HTML, `<div class="stats">`) {
		t.Errorf("reloaded region should re-render the whole slot, got %q", resp.HTML)
	}
	// The button re-renders with the slot, so the control survives its own reload.
	if !strings.Contains(resp.HTML, "<reload-shortcode>") {
		t.Errorf("reloaded region should still contain its reload button, got %q", resp.HTML)
	}
}

// A slot with no reload button must be untouched: the wrapper is opt-in, so no
// existing template gains a div it did not have before.
func TestReloadRegion_AbsentWithoutReloadButton(t *testing.T) {
	tc := SetupTestEnv(t)

	body, _ := renderGroupPage(t, tc, `<div class="stats">[property path="Name"]</div>`)

	if strings.Contains(body, "data-shortcode-region") {
		t.Errorf("a slot without [reload] must not gain a region wrapper")
	}
	if !strings.Contains(body, `<div class="stats">Reload Group</div>`) {
		t.Errorf("slot should render normally, got: %s", body)
	}
}

// A [reload] behind a [lazy] is never expanded during the page render, so the
// slot needs no region: at click time the button will find that <lazy-shortcode>
// as its nearest reloadable ancestor.
func TestReloadRegion_AbsentWhenReloadIsBehindLazy(t *testing.T) {
	tc := SetupTestEnv(t)

	body, _ := renderGroupPage(t, tc, `[lazy][reload]<b>inner</b>[/lazy]`)

	if !strings.Contains(body, "<lazy-shortcode ") {
		t.Fatalf("expected a deferred lazy placeholder, got: %s", body)
	}
	if strings.Contains(body, "data-shortcode-region") {
		t.Errorf("a reload button behind a deferred block must not force a region wrapper")
	}
	if strings.Contains(body, "<reload-shortcode>") {
		t.Errorf("a reload button behind a deferred block must not render on the page")
	}
}

// The list-header carrier renders against a Category, which the deferred
// endpoint cannot load by (type, id). Minting a region token there would produce
// a button that always fails, so the slot stays unwrapped and the button falls
// back to reloading the page.
func TestReloadRegion_SkippedForCarrierSlots(t *testing.T) {
	tc := SetupTestEnv(t)

	cat := &models.Category{Name: "Carrier Cat", CustomListHeader: `[reload]`}
	if err := tc.DB.Create(cat).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}

	rr := doReq(tc, http.MethodGet, "/groups?categories="+itoa(int(cat.ID)),
		map[string]string{"Accept": "text/html"}, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /groups: expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "<reload-shortcode>") {
		t.Fatalf("carrier slot should still render the button, got: %s", body)
	}
	if strings.Contains(body, "data-shortcode-region") {
		t.Errorf("carrier slot must not be wrapped in a reloadable region")
	}
}

// Two slots on one page each need their own region: a shared or reused token
// would make one slot's reload button re-render the other slot's body.
func TestReloadRegion_EachSlotGetsItsOwnToken(t *testing.T) {
	tc := SetupTestEnv(t)

	cat := &models.Category{
		Name:          "Two Slot Cat",
		CustomHeader:  `<div class="hdr">HEADER</div>[reload]`,
		CustomSidebar: `<div class="side">SIDEBAR</div>[reload]`,
	}
	if err := tc.DB.Create(cat).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
	catID := cat.ID
	g := &models.Group{Name: "Two Slot Group", CategoryId: &catID}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	rr := doReq(tc, http.MethodGet, "/group?id="+itoa(int(g.ID)),
		map[string]string{"Accept": "text/html"}, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /group: expected 200, got %d", rr.Code)
	}

	matches := regionTokenPattern.FindAllStringSubmatch(rr.Body.String(), -1)
	if len(matches) != 2 {
		t.Fatalf("expected one region per slot, got %d", len(matches))
	}
	if matches[0][1] == matches[1][1] {
		t.Fatalf("the two slots must not share a region token")
	}

	// Each token must render only its own slot.
	bodies := make([]string, 0, 2)
	for _, m := range matches {
		resp := tc.MakeRequest(http.MethodPost, "/v1/shortcodes/deferred", map[string]any{"token": m[1]})
		if resp.Code != http.StatusOK {
			t.Fatalf("region token should render: got %d (%s)", resp.Code, resp.Body.String())
		}
		var decoded deferredRenderResp
		if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
			t.Fatalf("decode: %v", err)
		}
		bodies = append(bodies, decoded.HTML)
	}

	joined := strings.Join(bodies, "\n")
	if !strings.Contains(joined, `<div class="hdr">`) || !strings.Contains(joined, `<div class="side">`) {
		t.Fatalf("each slot should be reachable through its own token, got %q", joined)
	}
	for _, body := range bodies {
		if strings.Contains(body, `<div class="hdr">`) && strings.Contains(body, `<div class="side">`) {
			t.Errorf("a region token must render only its own slot, got %q", body)
		}
	}
}

// A commented-out [reload] still expands (shortcode expansion is text-level), but
// the button it produces is invisible, so the slot must not pay for a region.
func TestReloadRegion_AbsentForCommentedOutButton(t *testing.T) {
	tc := SetupTestEnv(t)

	body, _ := renderGroupPage(t, tc, `<div class="stats">[property path="Name"]</div><!-- [reload] -->`)

	if strings.Contains(body, "data-shortcode-region") {
		t.Errorf("a commented-out reload button must not mint a region wrapper")
	}
}
