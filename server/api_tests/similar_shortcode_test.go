package api_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/types"
	"mahresources/server/template_handlers/template_filters"
	"mahresources/shortcodes"
)

// [similar] reads the stored perceptual-hash pairs through the request's
// principal-scoped MRQL executor, then attaches each returned row's distance.

func mkSim(t *testing.T, tc *TestContext, v any) {
	t.Helper()
	if err := tc.DB.Create(v).Error; err != nil {
		t.Fatalf("create %T: %v", v, err)
	}
}

// similarStrip cuts the rendered strip (or "") out of a page.
func similarStrip(page string) string {
	start := strings.Index(page, `<figure class="similar-strip`)
	if start < 0 {
		return ""
	}
	end := strings.Index(page[start:], "</figure>")
	return page[start : start+end+len("</figure>")]
}

type similarOrderFixture struct {
	rc             *models.ResourceCategory
	target         *models.Resource
	d1, d4, d5, d9 *models.Resource
}

// buildSimilarOrderFixture seeds a target with pairs at p-distance 1, 5 and 9
// and a legacy pair (no p-distance) at Hamming 4.
func buildSimilarOrderFixture(t *testing.T, tc *TestContext) similarOrderFixture {
	t.Helper()
	f := similarOrderFixture{rc: &models.ResourceCategory{Name: "Photos"}}
	mkSim(t, tc, f.rc)
	mk := func(name string) *models.Resource {
		r := &models.Resource{Name: name, ResourceCategoryId: f.rc.ID, Hash: "h-" + name, Location: name + ".jpg"}
		mkSim(t, tc, r)
		return r
	}
	f.target = mk("sim-target")
	f.d9 = mk("sim-d9")
	f.d5 = mk("sim-d5")
	f.d1 = mk("sim-d1")
	f.d4 = mk("sim-d4-legacy")
	seedSimPair(t, tc, f.target.ID, f.d1.ID, u8(1), nil, 30)
	seedSimPair(t, tc, f.target.ID, f.d5.ID, u8(5), nil, 30)
	seedSimPair(t, tc, f.target.ID, f.d9.ID, u8(9), nil, 30)
	seedSimPair(t, tc, f.target.ID, f.d4.ID, nil, nil, 4)
	return f
}

func renderSimilarFor(t *testing.T, appCtx *application_context.MahresourcesContext, r *models.Resource, tpl string) string {
	t.Helper()
	ctx := shortcodes.MetaShortcodeContext{EntityType: "resource", EntityID: r.ID, Entity: r}
	return shortcodes.Process(context.Background(), tpl, ctx, nil, template_filters.BuildQueryExecutor(appCtx))
}

func TestSimilarShortcode_OrderTiersAndLimit(t *testing.T) {
	tc := SetupTestEnv(t)
	f := buildSimilarOrderFixture(t, tc)

	out := renderSimilarFor(t, tc.AppCtx, f.target, "[similar]")
	order := []struct {
		r    *models.Resource
		tier string
		d    int
	}{{f.d1, "Duplicate", 1}, {f.d4, "Similar", 4}, {f.d5, "Similar", 5}, {f.d9, "Similar", 9}}
	last := -1
	for _, o := range order {
		idx := strings.Index(out, fmt.Sprintf(`href="/resource?id=%d"`, o.r.ID))
		if idx < 0 {
			t.Fatalf("strip lacks %s: %s", o.r.Name, out)
		}
		if idx < last {
			t.Errorf("%s is out of order: %s", o.r.Name, out)
		}
		last = idx
		if !strings.Contains(out, fmt.Sprintf("%s · %d</span>", o.tier, o.d)) {
			t.Errorf("%s: no %s · %d badge", o.r.Name, o.tier, o.d)
		}
		if !strings.Contains(out, fmt.Sprintf("/resource/compare?r1=%d&amp;r2=%d", f.target.ID, o.r.ID)) {
			t.Errorf("%s: no compare link", o.r.Name)
		}
	}
	if !strings.Contains(out, `alt="sim-d1, near-certain duplicate, distance 1"`) {
		t.Errorf("alt text: %s", out)
	}

	out = renderSimilarFor(t, tc.AppCtx, f.target, `[similar max-distance="6"]`)
	if strings.Contains(out, fmt.Sprintf("id=%d\"", f.d9.ID)) || !strings.Contains(out, "Similar · 5") {
		t.Errorf("max-distance=6 should drop the 9: %s", out)
	}
	out = renderSimilarFor(t, tc.AppCtx, f.target, `[similar limit="1"]`)
	if strings.Count(out, "<li ") != 1 || !strings.Contains(out, fmt.Sprintf("/resource?id=%d", f.d1.ID)) {
		t.Errorf("limit=1 should keep only the nearest: %s", out)
	}

	// No pairs: nothing at all.
	if out := renderSimilarFor(t, tc.AppCtx, f.d9, `<b>[similar max-distance="2"]</b>`); out != "<b></b>" {
		t.Errorf("no pairs within the distance should render nothing: %q", out)
	}
}

func TestSimilarShortcode_PageAndLightbox(t *testing.T) {
	tc := SetupTestEnv(t)
	f := buildSimilarOrderFixture(t, tc)
	if err := tc.DB.Model(f.rc).Update("custom_sidebar", `[similar]`).Error; err != nil {
		t.Fatal(err)
	}

	page := getHTML(t, tc, "", fmt.Sprintf("/resource?id=%d", f.target.ID))
	strip := similarStrip(page)
	if strip == "" || !strings.Contains(strip, fmt.Sprintf("/resource/compare?r1=%d&amp;r2=%d", f.target.ID, f.d1.ID)) {
		t.Fatalf("resource page sidebar has no strip: %q", strip)
	}

	// The template editor's live preview renders it for the previewed resource.
	body, _ := json.Marshal(map[string]any{"entityId": f.target.ID, "content": `[similar]`})
	rr := doReq(tc, http.MethodPost, "/v1/resourceCategory/previewTemplate",
		map[string]string{"Accept": "application/json", "Content-Type": "application/json"}, nil, bytes.NewReader(body))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "similar-strip") || !strings.Contains(rr.Body.String(), "Duplicate · 1") {
		t.Errorf("template preview: %d %s", rr.Code, rr.Body.String())
	}

	lightbox := func(field string) string {
		t.Helper()
		rr := doReq(tc, http.MethodGet, fmt.Sprintf("/resource.json?id=%d", f.target.ID), map[string]string{"Accept": "application/json"}, nil, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("resource.json: %d %s", rr.Code, rr.Body.String())
		}
		// The lightbox reads resource.resourceCategory.
		var body struct {
			Resource struct {
				ResourceCategory map[string]any `json:"resourceCategory"`
			} `json:"resource"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		s, ok := body.Resource.ResourceCategory[field].(string)
		if !ok {
			t.Fatalf("resource.json has no resource.resourceCategory.%s: %s", field, rr.Body.String())
		}
		return s
	}
	check := func(label, got string) {
		t.Helper()
		for _, want := range []string{`<figure class="similar-strip`, fmt.Sprintf(`href="/resource?id=%d"`, f.d1.ID), "Duplicate · 1"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s lacks %q: %s", label, want, got)
			}
		}
		if strings.Contains(got, "[similar") {
			t.Errorf("%s still holds shortcode text: %s", label, got)
		}
	}

	// The lightbox falls back to CustomSidebar when CustomLightbox is empty.
	check("CustomSidebar (lightbox fallback)", lightbox("CustomSidebar"))

	if err := tc.DB.Model(f.rc).Updates(map[string]any{"custom_sidebar": "", "custom_lightbox": `[similar size="sm"]`}).Error; err != nil {
		t.Fatal(err)
	}
	got := lightbox("CustomLightbox")
	check("CustomLightbox", got)
	if !strings.Contains(got, `width="48" height="48"`) {
		t.Errorf("size=sm in the lightbox: %s", got)
	}
}

type similarScopeFixture struct {
	root, outside, album *models.Group
	r1, r2, r3, r4       *models.Resource
}

// r1 and r2 are inside root, r3 is outside it, r4 has no owner. r1 pairs with
// each of the others. The album group, inside root, names r3 as its cover.
func buildSimilarScopeFixture(t *testing.T, tc *TestContext) similarScopeFixture {
	t.Helper()
	f := similarScopeFixture{root: &models.Group{Name: "sim-root"}, outside: &models.Group{Name: "sim-outside"}}
	mkSim(t, tc, f.root)
	mkSim(t, tc, f.outside)
	mk := func(name string, owner *uint) *models.Resource {
		r := &models.Resource{Name: name, OwnerId: owner, Hash: "hash-" + name, Location: name + ".jpg"}
		mkSim(t, tc, r)
		return r
	}
	f.r1 = mk("sim-in-target", &f.root.ID)
	f.r2 = mk("sim-in-twin", &f.root.ID)
	f.r3 = mk("sim-outside-secret", &f.outside.ID)
	f.r4 = mk("sim-ownerless-secret", nil)
	seedSimPair(t, tc, f.r1.ID, f.r2.ID, u8(1), nil, 9)
	seedSimPair(t, tc, f.r1.ID, f.r3.ID, u8(2), nil, 9)
	seedSimPair(t, tc, f.r1.ID, f.r4.ID, u8(3), nil, 9)
	f.album = &models.Group{Name: "sim-album", OwnerId: &f.root.ID, Meta: types.JSON(fmt.Sprintf(`{"cover":%d}`, f.r3.ID))}
	mkSim(t, tc, f.album)
	return f
}

func deferredHTML(t *testing.T, tc *TestContext, bearer, entityType string, id uint, body string) string {
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

func TestSimilarShortcode_ScopedPrincipal(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildSimilarScopeFixture(t, tc)
	scoped := scopedUserBearer(t, tc, f.root.ID)
	admin := roleBearer(t, tc, models.RoleAdmin)

	html := deferredHTML(t, tc, scoped, "resource", f.r1.ID, "[similar]")
	if !strings.Contains(html, fmt.Sprintf(`href="/resource?id=%d"`, f.r2.ID)) {
		t.Errorf("scoped user should see the in-subtree twin: %s", html)
	}
	for _, r := range []*models.Resource{f.r3, f.r4} {
		for _, leak := range []string{r.Name, r.Hash, fmt.Sprintf("id=%d", r.ID), fmt.Sprintf("r2=%d", r.ID)} {
			if strings.Contains(html, leak) {
				t.Errorf("scoped user saw %q of %s: %s", leak, r.Name, html)
			}
		}
	}

	html = deferredHTML(t, tc, admin, "resource", f.r1.ID, "[similar]")
	for _, r := range []*models.Resource{f.r2, f.r3, f.r4} {
		if !strings.Contains(html, fmt.Sprintf(`href="/resource?id=%d"`, r.ID)) {
			t.Errorf("admin should see %s: %s", r.Name, html)
		}
	}
}

func TestSimilarShortcode_ScopedUsingCover(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildSimilarScopeFixture(t, tc)
	scoped := scopedUserBearer(t, tc, f.root.ID)
	admin := roleBearer(t, tc, models.RoleAdmin)
	tpl := `<b>[using type="resource" id-path="cover"][similar][/using]</b>`

	html := deferredHTML(t, tc, scoped, "group", f.album.ID, tpl)
	if html != "<b></b>" {
		t.Errorf("an out-of-scope cover must render nothing: %q", html)
	}

	html = deferredHTML(t, tc, admin, "group", f.album.ID, tpl)
	strip := similarStrip(html)
	if !strings.Contains(strip, fmt.Sprintf(`data-similar-for="%d"`, f.r3.ID)) ||
		!strings.Contains(strip, fmt.Sprintf("/resource/compare?r1=%d&amp;r2=%d", f.r3.ID, f.r1.ID)) ||
		!strings.Contains(strip, " with sim-outside-secret</span>") {
		t.Errorf("admin should see the cover's strip: %s", html)
	}
}

// distanceCounter is the principal-bound context with SimilarityDistancesTo counted.
type distanceCounter struct {
	*application_context.MahresourcesContext
	calls int
}

func (d *distanceCounter) SimilarityDistancesTo(reqCtx context.Context, targetID uint, ids []uint) (map[uint]int, error) {
	d.calls++
	return d.MahresourcesContext.SimilarityDistancesTo(reqCtx, targetID, ids)
}

// The distance lookup is a Raw query with no scope callback. It is safe only
// because a target the principal cannot see yields no rows first, so the
// lookup is never reached.
func TestSimilarShortcode_HiddenTargetNeverReachesDistances(t *testing.T) {
	tc := SetupTestEnv(t)
	f := buildSimilarScopeFixture(t, tc)
	scoped := &distanceCounter{MahresourcesContext: accountCtx(t, tc, models.RoleUser, &f.root.ID)}
	exec := template_filters.BuildQueryExecutor(scoped)
	opts := shortcodes.QueryOptions{Limit: 8, WantSimilarityDistance: true}

	res, err := exec(context.Background(), fmt.Sprintf("type = resource AND SIMILAR TO resource(%d) ORDER BY distance ASC", f.r3.ID), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 0 || scoped.calls != 0 {
		t.Fatalf("hidden target: %d items, %d distance lookups; want 0 and 0", len(res.Items), scoped.calls)
	}

	// Control: a visible target reaches it with the in-scope rows only.
	res, err = exec(context.Background(), fmt.Sprintf("type = resource AND SIMILAR TO resource(%d) ORDER BY distance ASC", f.r1.ID), opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].EntityID != f.r2.ID || scoped.calls != 1 {
		t.Fatalf("visible target: %+v, %d lookups", res.Items, scoped.calls)
	}
	if d := res.Items[0].SimilarityDistance; d == nil || *d != 1 {
		t.Fatalf("distance = %v, want 1", d)
	}
}

// The JSON render (the lightbox's /resource.json) shares one page query budget
// across every slot, as the HTML page does: a distinct query past it renders
// the budget error instead of running, and an identical one is still free.
func TestSimilarShortcode_JSONRenderHonoursThePageBudget(t *testing.T) {
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) { c.MRQLPageQueryBudget = 1 })
	f := buildSimilarOrderFixture(t, tc)
	if err := tc.DB.Model(f.rc).Updates(map[string]any{
		"custom_header":  `[similar]`,
		"custom_sidebar": `[similar]`,
		"custom_summary": `[similar max-distance="5"]`,
	}).Error; err != nil {
		t.Fatal(err)
	}

	rr := doReq(tc, http.MethodGet, fmt.Sprintf("/resource.json?id=%d", f.target.ID), map[string]string{"Accept": "application/json"}, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("resource.json: %d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Resource struct {
			ResourceCategory map[string]any `json:"resourceCategory"`
		} `json:"resource"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	slot := func(field string) string { s, _ := body.Resource.ResourceCategory[field].(string); return s }
	if !strings.Contains(slot("CustomHeader"), "similar-strip") || !strings.Contains(slot("CustomSidebar"), "similar-strip") {
		t.Fatalf("the first query and its identical repeat should render: header %q, sidebar %q", slot("CustomHeader"), slot("CustomSidebar"))
	}
	if s := slot("CustomSummary"); !strings.Contains(s, "inline query budget exceeded") {
		t.Fatalf("a distinct query past the budget should render the budget error, got %q", s)
	}
}
