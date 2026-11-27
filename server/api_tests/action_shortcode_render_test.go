package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"mahresources/models"
)

// [action] draws its button by the run endpoint's own rule, on every surface
// that renders templates for a viewer who could run it, and renders one
// identical comment everywhere else.

const actionUnavailable = "<!-- mr:action unavailable in this context -->"

// writeTemplateActionPlugin registers the actions the fixtures below render:
// one per entity type, and a resource action whose filters admit PNGs only.
func writeTemplateActionPlugin(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lua := fmt.Sprintf(`
plugin = { name = %q, version = "1.0", description = "[action] render fixture" }

local function ok(ctx) return { success = true, message = "ran " .. ctx.entity_id } end

function init()
    mah.action({ id = "greet", label = "Greet", entity = "resource", placement = { "detail" }, handler = ok,
        params = { { name = "greeting", type = "text", label = "Greeting", default = "Hello" } } })
    mah.action({ id = "pngs", label = "PNG only", entity = "resource", placement = { "card" },
        filters = { content_types = { "image/png" } }, handler = ok })
    mah.action({ id = "note-act", label = "Note act", entity = "note", handler = ok })
end
`, name)
	if err := os.WriteFile(filepath.Join(dir, "plugin.lua"), []byte(lua), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

type actionFixture struct {
	scope, outside   *models.Group
	png, pdf, outRes *models.Resource
	category         *models.ResourceCategory
	noteType         *models.NoteType
	note             *models.Note
}

// The sidebar holds a real [action] at #a1 and one naming nothing at #a2, so a
// test can compare the two byte for byte.
const actionSidebar = `<div id="a1">[action plugin="tpl-act" name="greet"]</div><div id="a2">[action plugin="no-such" name="x"]</div>`

func buildActionFixture(t *testing.T, tc *TestContext) actionFixture {
	t.Helper()
	mk := func(v any) {
		t.Helper()
		if err := tc.DB.Create(v).Error; err != nil {
			t.Fatalf("create %T: %v", v, err)
		}
	}
	f := actionFixture{}
	f.category = &models.ResourceCategory{
		Name:           "Action Cat",
		CustomSidebar:  actionSidebar,
		CustomSummary:  `<span class="card-act">[action plugin="tpl-act" name="pngs"]</span>`,
		CustomLightbox: `[action plugin="tpl-act" name="greet" label="Greet from lightbox"]`,
		// A stylesheet never gets a button, on the page or in the JSON.
		CustomCSS: `/* [action plugin="tpl-act" name="greet"] */`,
	}
	mk(f.category)
	f.scope = &models.Group{Name: "act-scope"}
	mk(f.scope)
	f.outside = &models.Group{Name: "act-outside"}
	mk(f.outside)
	f.png = &models.Resource{Name: "act-png", ContentType: "image/png", OwnerId: &f.scope.ID, ResourceCategoryId: f.category.ID, Location: "act-png.png"}
	mk(f.png)
	f.pdf = &models.Resource{Name: "act-pdf", ContentType: "application/pdf", OwnerId: &f.scope.ID, ResourceCategoryId: f.category.ID, Location: "act-pdf.pdf"}
	mk(f.pdf)
	f.outRes = &models.Resource{Name: "act-out", ContentType: "image/png", OwnerId: &f.outside.ID, ResourceCategoryId: f.category.ID, Location: "act-out.png"}
	mk(f.outRes)
	f.noteType = &models.NoteType{Name: "Action Notes", CustomHeader: `<div id="n1">[action plugin="tpl-act" name="note-act"]</div>`, ApplyTemplatesToShares: true}
	mk(f.noteType)
	f.note = &models.Note{Name: "act-note", OwnerId: &f.scope.ID, NoteTypeId: &f.noteType.ID}
	mk(f.note)
	return f
}

var actionDiv = regexp.MustCompile(`(?s)<div id="(a1|a2|n1)">(.*?)</div>`)

// actionSlots maps #a1/#a2/#n1 to their rendered content.
func actionSlots(page string) map[string]string {
	out := map[string]string{}
	for _, m := range actionDiv.FindAllStringSubmatch(page, -1) {
		if _, seen := out[m[1]]; !seen {
			out[m[1]] = m[2]
		}
	}
	return out
}

func greetButton(id uint) string {
	return fmt.Sprintf(`<action-shortcode data-plugin="tpl-act" data-action="greet" data-entity-type="resource" data-entity-id="%d"`, id)
}

func TestActionShortcode_DetailPageFollowsTheRunRule(t *testing.T) {
	tc := setupPluginEnv(t, true, writeTemplateActionPlugin, "tpl-act")
	f := buildActionFixture(t, tc)
	admin := roleBearer(t, tc, models.RoleAdmin)
	editor := roleBearer(t, tc, models.RoleEditor)
	guest := scopedRoleBearer(t, tc, models.RoleGuest, f.scope.ID, "act-guest")
	scoped := scopedUserBearer(t, tc, f.scope.ID)
	page := fmt.Sprintf("/resource?id=%d", f.png.ID)

	for name, bearer := range map[string]string{"admin": admin, "editor": editor} {
		html := getHTML(t, tc, bearer, page)
		if !strings.Contains(html, "/* "+actionUnavailable+" */</style>") {
			t.Errorf("%s: the page's CustomCSS should render the comment", name)
		}
		slots := actionSlots(html)
		if !strings.HasPrefix(slots["a1"], greetButton(f.png.ID)) {
			t.Errorf("%s: no button for the page entity: %q", name, slots["a1"])
		}
		if slots["a2"] != actionUnavailable {
			t.Errorf("%s: a nonexistent plugin should render the comment: %q", name, slots["a2"])
		}
	}

	// The guest and the group-limited user whose plugin is not opened see
	// exactly what a plugin that does not exist renders.
	for name, bearer := range map[string]string{"guest": guest, "group-limited user": scoped} {
		slots := actionSlots(getHTML(t, tc, bearer, page))
		if slots["a1"] != actionUnavailable || slots["a1"] != slots["a2"] {
			t.Errorf("%s: a1=%q a2=%q, want both %q", name, slots["a1"], slots["a2"], actionUnavailable)
		}
	}

	// Opening the plugin to group-limited accounts draws the button for them,
	// and still not for the guest, who cannot write.
	if err := tc.AppCtx.SetPluginScopedAccess("tpl-act", true); err != nil {
		t.Fatal(err)
	}
	if slots := actionSlots(getHTML(t, tc, scoped, page)); !strings.HasPrefix(slots["a1"], greetButton(f.png.ID)) {
		t.Errorf("opened plugin: no button for the group-limited user: %q", slots["a1"])
	}
	if slots := actionSlots(getHTML(t, tc, guest, page)); slots["a1"] != actionUnavailable {
		t.Errorf("opened plugin: the guest got %q", slots["a1"])
	}

	// A forged click on an entity outside the subtree gets the run endpoint's
	// own 403, whatever the page drew.
	body, _ := json.Marshal(map[string]any{"plugin": "tpl-act", "action": "greet", "entity_ids": []uint{f.outRes.ID}, "params": map[string]any{"greeting": "x"}})
	rr := doReq(tc, http.MethodPost, "/v1/jobs/action/run",
		map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": scoped}, nil, bytes.NewReader(body))
	if rr.Code != http.StatusForbidden {
		t.Errorf("forged out-of-subtree run: %d %s", rr.Code, rr.Body.String())
	}
	// And the button's own request succeeds for the entity it was drawn on.
	body, _ = json.Marshal(map[string]any{"plugin": "tpl-act", "action": "greet", "entity_ids": []uint{f.png.ID}, "params": map[string]any{"greeting": "Hello"}})
	rr = doReq(tc, http.MethodPost, "/v1/jobs/action/run",
		map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": scoped}, nil, bytes.NewReader(body))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"success":true`) {
		t.Errorf("in-subtree run: %d %s", rr.Code, rr.Body.String())
	}
}

// A Description is written by whoever may edit the entity, so under -auth a
// button there would run with the rights of whoever clicks it.
func TestActionShortcode_UntrustedDescriptionRendersNoButton(t *testing.T) {
	tc := setupPluginEnv(t, true, writeTemplateActionPlugin, "tpl-act")
	f := buildActionFixture(t, tc)
	admin := roleBearer(t, tc, models.RoleAdmin)
	if err := tc.DB.Model(f.pdf).Update("description", `<p>desc [action plugin="tpl-act" name="greet"] end</p>`).Error; err != nil {
		t.Fatal(err)
	}
	page := getHTML(t, tc, admin, fmt.Sprintf("/resource?id=%d", f.pdf.ID))
	if !strings.Contains(page, "desc "+actionUnavailable+" end") {
		t.Errorf("description under -auth should render the comment")
	}
	if strings.Count(page, greetButton(f.pdf.ID)) != 1 {
		t.Errorf("only the sidebar's button should render, got %d", strings.Count(page, greetButton(f.pdf.ID)))
	}
}

func TestActionShortcode_CardsResultsAndFilters(t *testing.T) {
	tc := setupPluginEnv(t, false, writeTemplateActionPlugin, "tpl-act")
	f := buildActionFixture(t, tc)

	// List cards: the PNG-only action is drawn on the PNG cards and not the PDF.
	list := getHTML(t, tc, "", fmt.Sprintf("/resources?OwnerId=%d", f.scope.ID))
	pngCard := fmt.Sprintf(`data-action="pngs" data-entity-type="resource" data-entity-id="%d"`, f.png.ID)
	pdfCard := fmt.Sprintf(`data-action="pngs" data-entity-type="resource" data-entity-id="%d"`, f.pdf.ID)
	if !strings.Contains(list, pngCard) {
		t.Errorf("list: the PNG card has no button")
	}
	if strings.Contains(list, pdfCard) || !strings.Contains(list, `<span class="card-act">`+actionUnavailable+`</span>`) {
		t.Errorf("list: the PDF card should render the comment")
	}

	// [mrql] result rows in a group template: one button per matching row.
	cat := &models.Category{Name: "Action Group Cat", CustomSidebar: `[mrql query='type = resource' limit="10"]<i data-row="[property path="ID"]">[action plugin="tpl-act" name="pngs"]</i>[/mrql]`}
	if err := tc.DB.Create(cat).Error; err != nil {
		t.Fatal(err)
	}
	if err := tc.DB.Model(f.scope).Update("category_id", cat.ID).Error; err != nil {
		t.Fatal(err)
	}
	group := getHTML(t, tc, "", fmt.Sprintf("/group?id=%d", f.scope.ID))
	if !strings.Contains(group, fmt.Sprintf(`<i data-row="%d"><action-shortcode data-plugin="tpl-act" data-action="pngs" data-entity-type="resource" data-entity-id="%d"`, f.png.ID, f.png.ID)) {
		t.Errorf("[mrql] row for the PNG has no button")
	}
	if !strings.Contains(group, fmt.Sprintf(`<i data-row="%d">%s</i>`, f.pdf.ID, actionUnavailable)) {
		t.Errorf("[mrql] row for the PDF should render the comment")
	}
}

func TestActionShortcode_MRQLRenderModes(t *testing.T) {
	tc := setupPluginEnv(t, true, writeTemplateActionPlugin, "tpl-act")
	f := buildActionFixture(t, tc)
	admin := roleBearer(t, tc, models.RoleAdmin)
	if err := tc.DB.Model(f.category).Update("custom_mrql_result", `<i class="mrql-act">[action plugin="tpl-act" name="greet"]</i>`).Error; err != nil {
		t.Fatal(err)
	}

	for _, mode := range []string{"list", "1"} {
		t.Run(mode, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"query": `type = resource AND name = "act-png"`})
			rr := doReq(tc, http.MethodPost, "/v1/mrql?render="+mode,
				map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": admin}, nil, bytes.NewReader(body))
			var result struct{ Resources []models.Resource }
			if rr.Code != http.StatusOK || json.Unmarshal(rr.Body.Bytes(), &result) != nil || len(result.Resources) != 1 {
				t.Fatalf("MRQL render=%s: %d %s", mode, rr.Code, rr.Body.String())
			}
			card := result.Resources[0].RenderedHTML
			const slot = `<i class="mrql-act">`
			if mode == "list" && !strings.Contains(card, slot+greetButton(f.png.ID)) {
				t.Errorf("authorized list card did not render the button: %s", card)
			}
			if mode == "1" && (!strings.Contains(card, slot+actionUnavailable+`</i>`) || strings.Contains(card, "<action-shortcode")) {
				t.Errorf("direct MRQL render should render only the unavailable comment: %s", card)
			}
		})
	}
}

func renderDeferredActionToken(t *testing.T, tc *TestContext, bearer, token string) string {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"token": token})
	rr := doReq(tc, http.MethodPost, "/v1/shortcodes/deferred",
		map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": bearer}, nil, bytes.NewReader(payload))
	if rr.Code != http.StatusOK {
		t.Fatalf("deferred: %d %s", rr.Code, rr.Body.String())
	}
	var resp deferredRenderResp
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.HTML
}

func TestActionShortcode_DeferredDescriptionKeepsSourceTrust(t *testing.T) {
	tc := setupPluginEnv(t, true, writeTemplateActionPlugin, "tpl-act")
	f := buildActionFixture(t, tc)
	admin := roleBearer(t, tc, models.RoleAdmin)
	const lazy = `[lazy][action plugin="tpl-act" name="greet"][/lazy]`
	if err := tc.DB.Model(f.png).Update("description", `action-description `+lazy).Error; err != nil {
		t.Fatal(err)
	}
	if err := tc.DB.Model(f.category).Update("custom_sidebar", `<div id="lazy-template">`+lazy+`</div>`).Error; err != nil {
		t.Fatal(err)
	}

	page := getHTML(t, tc, admin, fmt.Sprintf("/resource?id=%d", f.png.ID))
	render := func(before string) string {
		t.Helper()
		pattern := regexp.MustCompile(regexp.QuoteMeta(before) + `\s*<lazy-shortcode data-token="([^"]+)"`)
		match := pattern.FindStringSubmatch(page)
		if match == nil {
			t.Fatal("page did not mint a lazy token after " + before)
		}
		return renderDeferredActionToken(t, tc, admin, html.UnescapeString(match[1]))
	}
	if rendered := render("action-description"); rendered != actionUnavailable {
		t.Errorf("deferred Description should not render a button: %q", rendered)
	}
	if rendered := render(`<div id="lazy-template">`); !strings.HasPrefix(rendered, greetButton(f.png.ID)) {
		t.Errorf("trusted template did not render the button: %q", rendered)
	}
}

func TestActionShortcode_DeferredAndLightbox(t *testing.T) {
	tc := setupPluginEnv(t, true, writeTemplateActionPlugin, "tpl-act")
	f := buildActionFixture(t, tc)
	admin := roleBearer(t, tc, models.RoleAdmin)
	guest := scopedRoleBearer(t, tc, models.RoleGuest, f.scope.ID, "act-guest")

	deferred := func(bearer, body string) string {
		t.Helper()
		return renderDeferredActionToken(t, tc, bearer, signToken(tc, "resource", f.png.ID, body))
	}
	// An [autorefresh] body re-rendered after a run keeps its button.
	if html := deferred(admin, `[action plugin="tpl-act" name="greet"]`); !strings.HasPrefix(html, greetButton(f.png.ID)) {
		t.Errorf("deferred body for the admin: %q", html)
	}
	if html := deferred(guest, `[action plugin="tpl-act" name="greet"]`); html != actionUnavailable {
		t.Errorf("deferred body for the guest: %q", html)
	}

	// The lightbox's JSON render: the route still encodes, and the slot holds
	// the button.
	rr := doReq(tc, http.MethodGet, fmt.Sprintf("/resource.json?id=%d", f.png.ID),
		map[string]string{"Accept": "application/json", "Authorization": admin}, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("resource.json: %d %s", rr.Code, rr.Body.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("resource.json is not valid JSON: %v", err)
	}
	res, _ := decoded["resource"].(map[string]any)
	category, _ := res["resourceCategory"].(map[string]any)
	lightbox, _ := category["CustomLightbox"].(string)
	if !strings.Contains(lightbox, `<action-shortcode data-plugin="tpl-act" data-action="greet"`) || !strings.Contains(lightbox, "Greet from lightbox</button>") {
		t.Errorf("lightbox slot has no button: %q", lightbox)
	}
	if css, _ := category["CustomCSS"].(string); css != "/* "+actionUnavailable+" */" {
		t.Errorf("CustomCSS should render the comment: %q", css)
	}
}

func TestActionShortcode_SurfacesWithoutAResolver(t *testing.T) {
	tc := setupPluginEnv(t, false, writeTemplateActionPlugin, "tpl-act")
	f := buildActionFixture(t, tc)

	// The public share page: read-only, no plugin manager.
	share := renderShare(t, tc, f.note.ID)
	if !strings.Contains(share, `<div id="n1">`+actionUnavailable+`</div>`) || strings.Contains(share, "<action-shortcode") {
		t.Errorf("share page rendered [action]")
	}
	// Control: the same template on the note's own page draws the button.
	if slots := actionSlots(getHTML(t, tc, "", fmt.Sprintf("/note?id=%d", f.note.ID))); !strings.Contains(slots["n1"], `data-action="note-act" data-entity-type="note"`) {
		t.Errorf("note page: %q", slots["n1"])
	}

	// The live template preview, which is not the page the button would run on.
	body, _ := json.Marshal(map[string]any{"entityId": f.png.ID, "content": `[action plugin="tpl-act" name="greet"]`})
	rr := doReq(tc, http.MethodPost, "/v1/resourceCategory/previewTemplate",
		map[string]string{"Accept": "application/json", "Content-Type": "application/json"}, nil, bytes.NewReader(body))
	var preview struct {
		HTML string `json:"html"`
	}
	if rr.Code != http.StatusOK || json.Unmarshal(rr.Body.Bytes(), &preview) != nil || preview.HTML != actionUnavailable {
		t.Errorf("template preview: %d %s", rr.Code, rr.Body.String())
	}

	// A hover card cannot be entered with the keyboard.
	card := getHTML(t, tc, "", fmt.Sprintf("/hovercard?type=resource&id=%d", f.png.ID))
	if !strings.Contains(card, actionUnavailable) || strings.Contains(card, "<action-shortcode") {
		t.Errorf("hover card rendered [action]: %s", card)
	}
}

func TestActionShortcode_LintWarnsAboutAnActionTheCallerCannotSee(t *testing.T) {
	tc := setupPluginEnv(t, true, writeTemplateActionPlugin, "tpl-act")
	f := buildActionFixture(t, tc)
	admin := roleBearer(t, tc, models.RoleAdmin)
	scoped := scopedUserBearer(t, tc, f.scope.ID)

	lint := func(bearer, content string) string {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"content": content})
		rr := doReq(tc, http.MethodPost, "/v1/shortcodes/lint",
			map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": bearer}, nil, bytes.NewReader(body))
		if rr.Code != http.StatusOK {
			t.Fatalf("lint: %d %s", rr.Code, rr.Body.String())
		}
		return rr.Body.String()
	}
	const missing = `no action \"x\" of plugin \"no-such\" is available`
	if out := lint(admin, `[action plugin="tpl-act" name="greet"]`); out != `{"issues":[]}`+"\n" {
		t.Errorf("a registered action should lint clean: %s", out)
	}
	if out := lint(admin, `[action plugin="no-such" name="x"]`); !strings.Contains(out, missing) {
		t.Errorf("a missing action should warn: %s", out)
	}
	// A plugin the caller may not run reads exactly like one that is missing.
	if out := lint(scoped, `[action plugin="tpl-act" name="greet"]`); !strings.Contains(out, `no action \"greet\" of plugin \"tpl-act\" is available`) {
		t.Errorf("an unreachable plugin should warn like a missing one: %s", out)
	}

	// The template preview piggybacks the same lint.
	body, _ := json.Marshal(map[string]any{"entityId": f.png.ID, "content": `[action plugin="no-such" name="x"]`})
	rr := doReq(tc, http.MethodPost, "/v1/resourceCategory/previewTemplate",
		map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": admin}, nil, bytes.NewReader(body))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), missing) {
		t.Errorf("preview lint: %d %s", rr.Code, rr.Body.String())
	}
}
