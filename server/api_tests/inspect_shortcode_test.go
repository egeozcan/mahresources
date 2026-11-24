package api_tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
	"mahresources/models/types"
)

// [inspect] renders its panel in the template preview and nowhere else.

func postPreview(t *testing.T, tc *TestContext, path string, body map[string]any) previewResponse {
	t.Helper()
	rr := tc.MakeRequest(http.MethodPost, path, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("%s: expected 200, got %d (body: %s)", path, rr.Code, rr.Body.String())
	}
	var resp previewResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}

func TestInspect_PreviewRendersThePanel(t *testing.T) {
	tc := SetupTestEnv(t) // auth off

	g := &models.Group{Name: "Apollo Group", Meta: types.JSON(`{"project":{"name":"Apollo"}}`)}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	resp := postPreview(t, tc, "/v1/category/previewTemplate", map[string]any{"entityId": g.ID, "content": `<div>[inspect]</div>`})
	for _, want := range []string{
		`class="inspect-shortcode"`,
		`<code>project.name</code>`,
		`&#34;Apollo&#34;`,
		`value="&#91;meta path=&#34;project.name&#34;]"`,
		`<code>Name</code></th><td>string</td><td>&#34;Apollo Group&#34;</td>`,
	} {
		if !strings.Contains(resp.HTML, want) {
			t.Errorf("preview html lacks %q:\n%s", want, resp.HTML)
		}
	}
	for _, iss := range resp.Issues {
		t.Errorf("unexpected lint issue: %+v", iss)
	}

	// The CustomCSS slot sends a stylesheet: no panel there.
	css := postPreview(t, tc, "/v1/category/previewTemplate", map[string]any{"entityId": g.ID, "slot": "CustomCSS", "content": `.x{}[inspect]`, "css": `.y{}[inspect]`})
	if strings.Contains(css.HTML, "inspect-shortcode") || strings.Contains(css.CSS, "inspect-shortcode") {
		t.Errorf("a stylesheet rendered a panel: %q / %q", css.HTML, css.CSS)
	}
}

// A note preloads its owner and the owner's owner. The panel names those
// associations but never reads through them, so no name from another group
// appears, and the note's share token is never listed.
func TestInspect_PreviewNeverReadsThroughAssociations(t *testing.T) {
	tc := SetupTestEnv(t)

	parent := &models.Group{Name: "Outside Parent Secret"}
	if err := tc.DB.Create(parent).Error; err != nil {
		t.Fatal(err)
	}
	owner := &models.Group{Name: "Owner Group Secret", OwnerId: &parent.ID}
	if err := tc.DB.Create(owner).Error; err != nil {
		t.Fatal(err)
	}
	nt := &models.NoteType{Name: "Inspected"}
	if err := tc.DB.Create(nt).Error; err != nil {
		t.Fatal(err)
	}
	n := &models.Note{Name: "Inspected Note", OwnerId: &owner.ID, NoteTypeId: &nt.ID, Meta: types.JSON(`{"k":1}`)}
	if err := tc.DB.Create(n).Error; err != nil {
		t.Fatal(err)
	}
	token, err := tc.AppCtx.ShareNote(n.ID)
	if err != nil || token == "" {
		t.Fatalf("share note: %v", err)
	}

	resp := postPreview(t, tc, "/v1/noteType/previewTemplate", map[string]any{"entityId": n.ID, "content": `[inspect]`})
	if !strings.Contains(resp.HTML, `<code>Owner</code></th><td>relation</td>`) {
		t.Errorf("Owner not listed as a relation:\n%s", resp.HTML)
	}
	for _, leak := range []string{"Outside Parent Secret", "Owner Group Secret", token, "ShareToken"} {
		if strings.Contains(resp.HTML, leak) {
			t.Errorf("panel leaked %q", leak)
		}
	}
}

// The preview routes are the gate. Roles that may not save a template get the
// existing 403, [inspect] content or not; a group-confined account is a user or
// guest, so it never reaches a preview at all.
func TestInspect_PreviewGateIsUnchanged(t *testing.T) {
	tc := setupAuthEnv(t)

	scope := &models.Group{Name: "scope"}
	if err := tc.DB.Create(scope).Error; err != nil {
		t.Fatal(err)
	}
	g := &models.Group{Name: "G", OwnerId: &scope.ID, Meta: types.JSON(`{"a":1}`)}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatal(err)
	}
	n := &models.Note{Name: "N", OwnerId: &scope.ID}
	if err := tc.DB.Create(n).Error; err != nil {
		t.Fatal(err)
	}

	post := func(bearer, path string, id uint) (int, string) {
		body, _ := json.Marshal(map[string]any{"entityId": id, "content": `[inspect]`})
		headers := map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": bearer}
		rr := doReq(tc, http.MethodPost, path, headers, nil, bytes.NewReader(body))
		return rr.Code, rr.Body.String()
	}

	userB := roleBearer(t, tc, models.RoleUser)
	confinedB := scopedRoleBearer(t, tc, models.RoleUser, scope.ID, "confined-inspect")
	for _, bearer := range []string{userB, confinedB} {
		for path, id := range map[string]uint{"/v1/category/previewTemplate": g.ID, "/v1/resourceCategory/previewTemplate": 1, "/v1/noteType/previewTemplate": n.ID} {
			if code, body := post(bearer, path, id); code != http.StatusForbidden || strings.Contains(body, "inspect-shortcode") {
				t.Errorf("user → %s: expected 403, got %d (%s)", path, code, body)
			}
		}
	}

	// An editor may preview a note type and sees the panel.
	if code, body := post(roleBearer(t, tc, models.RoleEditor), "/v1/noteType/previewTemplate", n.ID); code != http.StatusOK || !strings.Contains(body, "inspect-shortcode") {
		t.Errorf("editor → noteType preview: %d %s", code, body)
	}
}

func TestInspect_DisplayPageRendersNothing(t *testing.T) {
	tc := SetupTestEnv(t)
	body, _ := renderGroupPage(t, tc, `<i id="inspect-probe">[inspect]</i><i id="inspect-probe-2">[details summary="S"][inspect]D[/details]</i>`)
	if !strings.Contains(body, `<i id="inspect-probe"></i>`) {
		t.Errorf("display page did not render [inspect] as nothing")
	}
	// The category's own JSON, template source included, is embedded in the
	// page's Alpine scope as before; what matters is that nothing rendered.
	if strings.Contains(body, "inspect-shortcode") {
		t.Errorf("display page shows the panel")
	}
}

func TestInspect_DeferredBodyRendersNothing(t *testing.T) {
	tc := SetupTestEnv(t)
	g := &models.Group{Name: "Deferred Inspect", Meta: types.JSON(`{"secret":"meta-value"}`)}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatal(err)
	}
	token := signToken(tc, "group", g.ID, `<b>[inspect]</b>`)
	rr := tc.MakeRequest(http.MethodPost, "/v1/shortcodes/deferred", map[string]any{"token": token})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	var resp struct {
		HTML string `json:"html"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.HTML != "<b></b>" {
		t.Errorf("deferred html = %q, want <b></b>", resp.HTML)
	}
}

func TestInspect_SharePageRendersNothing(t *testing.T) {
	tc := SetupTestEnv(t)
	nt := &models.NoteType{Name: "Shared Inspect", ApplyTemplatesToShares: true, CustomHeader: `<i id="inspect-probe">[inspect]</i>`}
	if err := tc.DB.Create(nt).Error; err != nil {
		t.Fatal(err)
	}
	note := &models.Note{Name: "SharedInspect", NoteTypeId: &nt.ID, Meta: types.JSON(`{"secret":"meta-value"}`)}
	if err := tc.DB.Create(note).Error; err != nil {
		t.Fatal(err)
	}
	body := renderShare(t, tc, note.ID)
	if !strings.Contains(body, `<i id="inspect-probe"></i>`) || strings.Contains(body, "inspect-shortcode") {
		t.Errorf("share page rendered [inspect]")
	}
}

// The AI template run tools render previews for a model. They must not carry
// the flag: the panel would hand raw Meta to the model past privacy masking.
func TestInspect_TemplateRunToolPreviewRendersNothing(t *testing.T) {
	f := newToolFixture(t, nil, nil)
	out := f.call(t, "preview", map[string]any{"field": "CustomHeader", "content": `<h1>[meta path="title" inline="true"]</h1><i>[inspect]</i>`})
	if out.IsError {
		t.Fatal(out.Text)
	}
	html := decode(t, payload(t, out))["html"].(string)
	if !strings.Contains(html, "<i></i>") || strings.Contains(html, "inspect-shortcode") || strings.Contains(html, f.longMeta) {
		t.Errorf("run-tool preview rendered [inspect]: %s", html)
	}
}
