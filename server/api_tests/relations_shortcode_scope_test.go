package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/types"
	"mahresources/server/api_handlers"
)

// [relations] lists an edge only when the viewer may see both of its groups,
// on every surface that renders it. Each negative assertion below has a
// positive control: the inside edge for the confined viewer, and the outside
// group for an admin.

const relationsUnavailable = "relations unavailable in this context"

type relationsFixture struct {
	cat                     *models.Category
	root, child, outside    *models.Group
	dependsOn, blocks       *models.GroupRelationType
	relationsHiddenSections types.JSON
}

// The outside group's name is distinctive, so its absence from a whole page
// means the shortcode did not render it, and nothing else did either: the
// built-in relation panels are turned off.
const relSecret = "rel-secret-outside"

func buildRelationsFixture(t *testing.T, tc *TestContext, sidebar string) relationsFixture {
	t.Helper()
	mk := func(v any) {
		t.Helper()
		if err := tc.DB.Create(v).Error; err != nil {
			t.Fatalf("create %T: %v", v, err)
		}
	}
	f := relationsFixture{relationsHiddenSections: types.JSON(`{"relations":{"state":"off"}}`)}
	f.cat = &models.Category{Name: "Rel Cat", CustomSidebar: sidebar, SectionConfig: f.relationsHiddenSections}
	mk(f.cat)
	f.root = &models.Group{Name: "rel-root", CategoryId: &f.cat.ID}
	mk(f.root)
	f.child = &models.Group{Name: "rel-child", OwnerId: &f.root.ID, CategoryId: &f.cat.ID, Meta: types.JSON(`{"status":"child-green"}`)}
	mk(f.child)
	f.outside = &models.Group{Name: relSecret, CategoryId: &f.cat.ID, Description: "secret description", Meta: types.JSON(`{"status":"secret-status"}`)}
	mk(f.outside)
	f.dependsOn = &models.GroupRelationType{Name: "Depends on"}
	f.blocks = &models.GroupRelationType{Name: "Blocks"}
	mk(f.dependsOn)
	mk(f.blocks)
	edge := func(from, to *models.Group, typ *models.GroupRelationType) {
		mk(&models.GroupRelation{FromGroupId: &from.ID, ToGroupId: &to.ID, RelationTypeId: &typ.ID})
	}
	edge(f.root, f.child, f.dependsOn)
	edge(f.root, f.outside, f.dependsOn)
	edge(f.child, f.root, f.blocks)
	edge(f.outside, f.root, f.blocks)
	return f
}

func relationsBlock(html string) string {
	start := strings.Index(html, `<div class="relations-shortcode"`)
	if start < 0 {
		return ""
	}
	end := strings.Index(html[start:], "</ul></li></ul></div>")
	if end < 0 {
		return html[start:]
	}
	return html[start : start+end+len("</ul></li></ul></div>")]
}

func TestRelationsShortcode_GroupPageIsScoped(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildRelationsFixture(t, tc, `<div id="rel">[relations][relations direction="in"]</div>`)
	user := scopedRoleBearer(t, tc, models.RoleUser, f.root.ID, "rel-user")
	guest := scopedRoleBearer(t, tc, models.RoleGuest, f.root.ID, "rel-guest")
	admin := roleBearer(t, tc, models.RoleAdmin)

	for name, bearer := range map[string]string{"user": user, "guest": guest} {
		page := getHTML(t, tc, bearer, fmt.Sprintf("/group?id=%d", f.root.ID))
		block := relationsBlock(page)
		if !strings.Contains(block, fmt.Sprintf(`<a href="/group?id=%d">rel-child</a>`, f.child.ID)) {
			t.Errorf("%s: the inside edge is missing: %q", name, block)
		}
		if !strings.Contains(page, "Incoming: Blocks") {
			t.Errorf("%s: the inside incoming edge is missing", name)
		}
		if strings.Contains(page, relSecret) || strings.Contains(page, fmt.Sprintf("/group?id=%d", f.outside.ID)) {
			t.Errorf("%s: the page names the group outside the subtree", name)
		}
	}

	page := getHTML(t, tc, admin, fmt.Sprintf("/group?id=%d", f.root.ID))
	if !strings.Contains(relationsBlock(page), relSecret) || strings.Count(page, relSecret) < 2 {
		t.Errorf("control: an admin sees the outside group in both directions")
	}
}

func TestRelationsShortcode_DeferredPreviewAndLightboxAreScoped(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildRelationsFixture(t, tc, `[relations]`)
	user := scopedRoleBearer(t, tc, models.RoleUser, f.root.ID, "rel-user")
	admin := roleBearer(t, tc, models.RoleAdmin)

	deferred := func(bearer, body string) string {
		t.Helper()
		payload, _ := json.Marshal(map[string]any{"token": signToken(tc, "group", f.root.ID, body)})
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
	// The sealed body of a [lazy]; reading the far group's Meta takes the
	// details path too.
	body := `[relations]<i>[item path="GroupName"]=[item path="GroupMeta.status"]</i>[/relations]`
	if html := deferred(user, body); !strings.Contains(html, "<i>rel-child=child-green</i>") || strings.Contains(html, "secret") {
		t.Errorf("deferred body as the confined user: %q", html)
	}
	if html := deferred(admin, body); !strings.Contains(html, "<i>"+relSecret+"=secret-status</i>") {
		t.Errorf("control: deferred body as admin: %q", html)
	}

	preview := func(bearer string) string {
		return previewHTML(t, previewDescription(tc, bearer, "group", f.root.ID, `[relations direction="in"]`))
	}
	if html := preview(user); !strings.Contains(html, "rel-child") || strings.Contains(html, relSecret) {
		t.Errorf("description preview as the confined user: %q", html)
	}
	if html := preview(admin); !strings.Contains(html, relSecret) {
		t.Errorf("control: description preview as admin: %q", html)
	}

	lightbox := func(bearer string) string {
		rr := doReq(tc, http.MethodGet, fmt.Sprintf("/group.json?id=%d", f.root.ID),
			map[string]string{"Accept": "application/json", "Authorization": bearer}, nil, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("group.json: %d %s", rr.Code, rr.Body.String())
		}
		return rr.Body.String()
	}
	if body := lightbox(user); !strings.Contains(body, "relations-shortcode") || !strings.Contains(body, "rel-child") || strings.Contains(body, relSecret) {
		t.Errorf("JSON render as the confined user leaked or lost edges")
	}
	if body := lightbox(admin); !strings.Contains(body, relSecret) {
		t.Errorf("control: JSON render as admin lacks the outside group")
	}

	// The template preview is an admin/editor surface and renders as the viewer.
	payload, _ := json.Marshal(map[string]any{"entityId": f.root.ID, "content": `[relations type="Depends on"]`})
	rr := doReq(tc, http.MethodPost, "/v1/category/previewTemplate",
		map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": admin}, nil, bytes.NewReader(payload))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "rel-child") || !strings.Contains(rr.Body.String(), relSecret) {
		t.Errorf("template preview: %d %s", rr.Code, rr.Body.String())
	}
}

// The MRQL render API is bound for MRQL only, with no ORM scope: an admin is
// answered, a confined viewer gets the unavailable comment, never an unscoped
// read.
func TestRelationsShortcode_MRQLRenderAPI(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildRelationsFixture(t, tc, "")
	if err := tc.DB.Model(f.cat).Update("custom_mrql_result", `<i>[relations]</i>`).Error; err != nil {
		t.Fatal(err)
	}
	user := scopedRoleBearer(t, tc, models.RoleUser, f.root.ID, "rel-user")
	admin := roleBearer(t, tc, models.RoleAdmin)
	run := func(bearer string) string {
		body, _ := json.Marshal(map[string]any{"query": `type = group AND name = "rel-root"`})
		rr := doReq(tc, http.MethodPost, "/v1/mrql?render=1",
			map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": bearer}, nil, bytes.NewReader(body))
		if rr.Code != http.StatusOK {
			t.Fatalf("mrql: %d %s", rr.Code, rr.Body.String())
		}
		return rr.Body.String()
	}
	if body := run(admin); !strings.Contains(body, "relations-shortcode") || !strings.Contains(body, relSecret) {
		t.Errorf("MRQL render as admin: %s", body)
	}
	if body := run(user); !strings.Contains(body, relationsUnavailable) || strings.Contains(body, relSecret) || strings.Contains(body, "relations-shortcode") {
		t.Errorf("MRQL render as the confined user: %s", body)
	}
}

func TestRelationsShortcode_UnavailableSurfaces(t *testing.T) {
	tc := SetupTestEnv(t)
	f := buildRelationsFixture(t, tc, "")

	// The entity picker renders a page of cards per keystroke.
	if err := tc.DB.Model(f.cat).Update("custom_entity_picker_result", `<i>[relations]</i>`).Error; err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	api_handlers.GetEntityPickerHandler(tc.AppCtx)(w, httptest.NewRequest("GET", "/v1/entity-picker?entity=group", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), relationsUnavailable) || strings.Contains(w.Body.String(), "relations-shortcode") {
		t.Errorf("entity picker: %d %s", w.Code, w.Body.String())
	}

	// A hover card is a tooltip: the names render as text, not links.
	if err := tc.DB.Model(f.cat).Update("custom_hover_card", `[relations]`).Error; err != nil {
		t.Fatal(err)
	}
	card := getHTML(t, tc, "", fmt.Sprintf("/hovercard?type=group&id=%d", f.root.ID))
	block := relationsBlock(card)
	if !strings.Contains(block, "rel-child") || strings.Contains(block, "<a ") {
		t.Errorf("hover card: %q", block)
	}

	// On a note it is a comment, not an error.
	nt := &models.NoteType{Name: "Rel notes", CustomHeader: `<i>[relations]</i>`}
	if err := tc.DB.Create(nt).Error; err != nil {
		t.Fatal(err)
	}
	note := &models.Note{Name: "rel-note", OwnerId: &f.root.ID, NoteTypeId: &nt.ID}
	if err := tc.DB.Create(note).Error; err != nil {
		t.Fatal(err)
	}
	if page := getHTML(t, tc, "", fmt.Sprintf("/note?id=%d", note.ID)); !strings.Contains(page, "relations: only groups have relations") {
		t.Errorf("note page should carry the only-groups comment")
	}
	// ...and [using] switches to a group, where it works.
	if err := tc.DB.Model(nt).Update("custom_header", `<i>[using rel="owner"][relations][/using]</i>`).Error; err != nil {
		t.Fatal(err)
	}
	if page := getHTML(t, tc, "", fmt.Sprintf("/note?id=%d", note.ID)); !strings.Contains(relationsBlock(page), "rel-child") {
		t.Errorf("[using] to the owner should list its relations")
	}
}

// The AI template run tools render previews for a model, with their own
// masking of user data; relations would bypass it.
func TestRelationsShortcode_TemplateRunToolPreviewIsUnavailable(t *testing.T) {
	f := newToolFixture(t, nil, nil)
	other := &models.GroupRelationType{Name: "Depends on"}
	if err := f.tc.DB.Create(other).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.tc.DB.Create(&models.GroupRelation{FromGroupId: &f.child.ID, ToGroupId: &f.outside.ID, RelationTypeId: &other.ID}).Error; err != nil {
		t.Fatal(err)
	}
	out := f.call(t, "preview", map[string]any{"field": "CustomHeader", "content": `<i>[relations]</i>`})
	if out.IsError {
		t.Fatal(out.Text)
	}
	html := decode(t, payload(t, out))["html"].(string)
	if !strings.Contains(html, relationsUnavailable) || strings.Contains(html, "Elsewhere") || strings.Contains(html, "relations-shortcode") {
		t.Errorf("run-tool preview rendered [relations]: %s", html)
	}
}

// Each statement costs one unit of the shared page budget: two units for the
// first [relations] (its body reads GroupMeta), and the [relations] after it
// is refused.
func TestRelationsShortcode_SharesThePageBudget(t *testing.T) {
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) { c.MRQLPageQueryBudget = 2 })
	f := buildRelationsFixture(t, tc, `[relations]<b>[item path="GroupMeta.status"]</b>[/relations][relations direction="in"]`)
	page := getHTML(t, tc, "", fmt.Sprintf("/group?id=%d", f.root.ID))
	if !strings.Contains(page, "<b>child-green</b>") {
		t.Errorf("the first [relations] should render with the far group's Meta")
	}
	if strings.Contains(page, "Incoming: Blocks") || !strings.Contains(page, "inline query budget exceeded (2 per page)") {
		t.Errorf("the second [relations] should be the budget marker")
	}
}

func TestRelationsShortcode_ListCardsAreScoped(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildRelationsFixture(t, tc, "")
	if err := tc.DB.Model(f.cat).Update("custom_summary", `<div>[relations type="Depends on"]</div>`).Error; err != nil {
		t.Fatal(err)
	}
	guest := scopedRoleBearer(t, tc, models.RoleGuest, f.root.ID, "rel-guest-list")
	admin := roleBearer(t, tc, models.RoleAdmin)
	mrqlList := func(t *testing.T, bearer string) string {
		t.Helper()
		body := bytes.NewBufferString(`{"query":"type = group AND name = \"rel-root\""}`)
		rr := doReq(tc, http.MethodPost, "/v1/mrql?render=list",
			map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": bearer}, nil, body)
		if rr.Code != http.StatusOK {
			t.Fatalf("mrql: %d %s", rr.Code, rr.Body.String())
		}
		var resp struct{ Groups []models.Group }
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil || len(resp.Groups) != 1 {
			t.Fatalf("expected one group: %v, %s", err, rr.Body.String())
		}
		return resp.Groups[0].RenderedHTML
	}
	for name, render := range map[string]func(*testing.T, string) string{
		"/groups":          func(t *testing.T, bearer string) string { return getHTML(t, tc, bearer, "/groups") },
		"MRQL render=list": mrqlList,
	} {
		t.Run(name, func(t *testing.T) {
			page := render(t, guest)
			if !strings.Contains(relationsBlock(page), fmt.Sprintf(`<a href="/group?id=%d">rel-child</a>`, f.child.ID)) || strings.Contains(page, relSecret) {
				t.Errorf("guest card lost the visible edge or leaked the decoy: %q", relationsBlock(page))
			}
			if page := render(t, admin); !strings.Contains(relationsBlock(page), relSecret) {
				t.Errorf("control: admin card lacks the outside relation: %q", relationsBlock(page))
			}
		})
	}
}

func TestRelationsShortcode_PublicShareIsUnavailable(t *testing.T) {
	tc := setupAuthEnv(t)
	nt := &models.NoteType{
		Name: "Shared relations template", ApplyTemplatesToShares: true,
		CustomHeader: `<p>Public relation header [relations]</p>`,
	}
	if err := tc.DB.Create(nt).Error; err != nil {
		t.Fatal(err)
	}
	note := &models.Note{Name: "Shared note", NoteTypeId: &nt.ID}
	if err := tc.DB.Create(note).Error; err != nil {
		t.Fatal(err)
	}
	page := renderShare(t, tc, note.ID)
	if !strings.Contains(page, "Public relation header <!-- mr:"+relationsUnavailable+" -->") || strings.Contains(page, "relations-shortcode") {
		t.Errorf("share did not render the restricted header: %s", page)
	}
}
