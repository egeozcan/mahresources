package api_tests

import (
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
)

func templateEnv(t *testing.T) (*mcpEnv, *models.Category, *models.Group) {
	t.Helper()
	env := newMCPEnv(t, false)
	cat := &models.Category{Name: "Projects", CustomSidebar: "<p>keep me</p>",
		MetaSchema: `{"type":"object","properties":{"status":{"type":"string","title":"Status"}}}`}
	env.tc.DB.Create(cat)
	g := &models.Group{Name: "Apollo", CategoryId: &cat.ID, Meta: []byte(`{"status":"active"}`)}
	env.tc.DB.Create(g)
	return env, cat, g
}

func TestMCPTemplate_GuideCarriesTheRunReference(t *testing.T) {
	env, cat, _ := templateEnv(t)
	cs := env.connect(t, clientOpts{})
	res := callTool(t, cs, "template_guide", map[string]any{"carrierType": "category", "carrierId": cat.ID, "slot": "CustomSidebar"})
	text := resultText(res)
	if res.IsError {
		t.Fatalf("guide: %s", text)
	}
	for _, want := range []string{"[meta", `"status"`, "Apollo", "keep me", "lint"} {
		if !strings.Contains(text, want) {
			t.Errorf("guide lacks %q (%d bytes)", want, len(text))
		}
	}
	if strings.Contains(text, "How this run works") || strings.Contains(text, "User request:") {
		t.Errorf("guide carries the run's own instructions")
	}
}

func TestMCPTemplate_LintAndPreviewUseTheRunTools(t *testing.T) {
	env, cat, g := templateEnv(t)
	cs := env.connect(t, clientOpts{})
	editing := map[string]any{"carrierType": "category", "carrierId": cat.ID}

	lint := callTool(t, cs, "template_lint", map[string]any{"field": "CustomHeader",
		"content": `<h1>[meta path="statsu"]</h1> [nosuch] [mrql query="type = " ]`, "editing": editing})
	t.Logf("lint: %s", resultText(lint))
	if lint.IsError || !strings.Contains(resultText(lint), "issues") {
		t.Fatalf("lint = %v %s", lint.IsError, resultText(lint))
	}

	prev := callTool(t, cs, "template_preview", map[string]any{"field": "CustomHeader",
		"content": `<h1>[property path="Name"]: [meta path="status"]</h1>`, "editing": editing})
	t.Logf("preview: %s", resultText(prev))
	if prev.IsError || !strings.Contains(resultText(prev), "Apollo") {
		t.Fatalf("preview = %v %s", prev.IsError, resultText(prev))
	}
	_ = g

	saved := callTool(t, cs, "template_preview", map[string]any{"field": "CustomSidebar", "editing": editing})
	if !strings.Contains(resultText(saved), "keep me") {
		t.Fatalf("preview of the saved field = %s", resultText(saved))
	}

	bad := callTool(t, cs, "template_lint", map[string]any{"field": "CustomNope", "content": "x", "editing": editing})
	if !bad.IsError && !strings.Contains(resultText(bad), "not a template field") {
		t.Fatalf("unknown field lint = %v %s", bad.IsError, resultText(bad))
	}
}

func TestMCPTemplate_EditSavesAndReportsLintAndPreview(t *testing.T) {
	env, cat, _ := templateEnv(t)
	cs := env.connect(t, clientOpts{})
	res := callTool(t, cs, "template_edit", map[string]any{"carrierType": "category", "carrierId": cat.ID,
		"fields": map[string]any{"CustomHeader": `<h1>[property path="Name"] [meta path="status"]</h1>`, "CustomHeaderCSS": "h1{margin:0}"}})
	text := resultText(res)
	t.Logf("edit: %s", text)
	if res.IsError || !strings.Contains(text, "Lint:") || !strings.Contains(text, "Preview of CustomHeader") || !strings.Contains(text, "Apollo") {
		t.Fatalf("edit = %v %s", res.IsError, text)
	}
	var got models.Category
	env.tc.DB.First(&got, cat.ID)
	if !strings.Contains(got.CustomHeader, `[meta path="status"]`) || got.CustomHeaderCSS != "h1{margin:0}" || got.CustomSidebar != "<p>keep me</p>" {
		t.Fatalf("saved = header %q css %q sidebar %q", got.CustomHeader, got.CustomHeaderCSS, got.CustomSidebar)
	}

	refused := callTool(t, cs, "template_edit", map[string]any{"carrierType": "category", "carrierId": cat.ID,
		"fields": map[string]any{"CustomHeader": "<h1>changed</h1>", "CustomNope": "x"}})
	if !refused.IsError || !strings.Contains(resultText(refused), "Nothing was saved") {
		t.Fatalf("unknown field edit = %v %s", refused.IsError, resultText(refused))
	}
	env.tc.DB.First(&got, cat.ID)
	if strings.Contains(got.CustomHeader, "changed") {
		t.Fatal("a refused edit saved a field")
	}
}

func TestMCPTemplate_NoteTypeEditNeedsConfirmation(t *testing.T) {
	env := newMCPEnv(t, false)
	nt := &models.NoteType{Name: "Meeting"}
	env.tc.DB.Create(nt)
	cs := env.connect(t, clientOpts{}) // cannot confirm
	res := callTool(t, cs, "template_edit", map[string]any{"carrierType": "noteType", "carrierId": nt.ID,
		"fields": map[string]any{"CustomHeader": "<h1>x</h1>"}})
	if !res.IsError || !strings.Contains(resultText(res), "needs a confirmation") {
		t.Fatalf("note type edit = %v %s", res.IsError, resultText(res))
	}
	ok := env.connect(t, clientOpts{answer: accept})
	if res := callTool(t, ok, "template_edit", map[string]any{"carrierType": "noteType", "carrierId": nt.ID,
		"fields": map[string]any{"CustomHeader": "<h1>x</h1>"}}); res.IsError {
		t.Fatalf("confirmed note type edit: %s", resultText(res))
	} else {
		t.Logf("confirmed edit: %s", resultText(res))
	}
	var got models.NoteType
	env.tc.DB.First(&got, nt.ID)
	if got.CustomHeader != "<h1>x</h1>" {
		t.Fatalf("note type header = %q", got.CustomHeader)
	}
}

func TestTemplateToolEndpointRefusals(t *testing.T) {
	tc := SetupTestEnv(t)
	for _, body := range []map[string]any{{"tool": "submit"}, {"tool": "nope"}} {
		rr := tc.MakeRequest(http.MethodPost, "/v1/category/templateTool", body)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("%v = %d %s", body, rr.Code, rr.Body.String())
		}
	}

	auth := setupAuthEnv(t)
	root := &models.Group{Name: "tt-root"}
	auth.DB.Create(root)
	bearer := scopedUserBearer(t, auth, root.ID)
	rr := doReq(auth, http.MethodPost, "/v1/noteType/templateTool", map[string]string{"Authorization": bearer, "Content-Type": "application/json", "Accept": "application/json"},
		nil, strings.NewReader(`{"tool":"lint","arguments":{"field":"CustomHeader","content":"x"}}`))
	if rr.Code != http.StatusForbidden {
		t.Errorf("scoped template tool = %d %s", rr.Code, rr.Body.String())
	}
}

// An explicit preview entity is honoured even outside the carrier (review
// round 2): grounding samples only members.
func TestMCPTemplate_PreviewHonoursAnExplicitEntity(t *testing.T) {
	env, cat, _ := templateEnv(t)
	other := &models.Category{Name: "Other"}
	env.tc.DB.Create(other)
	outsider := &models.Group{Name: "Outsider", CategoryId: &other.ID}
	env.tc.DB.Create(outsider)
	cs := env.connect(t, clientOpts{})
	res := callTool(t, cs, "template_preview", map[string]any{"field": "CustomHeader", "content": `[property path="Name"]`,
		"editing": map[string]any{"carrierType": "category", "carrierId": cat.ID, "entityId": outsider.ID}})
	if res.IsError || !strings.Contains(resultText(res), "Outsider") || strings.Contains(resultText(res), "Apollo") {
		t.Fatalf("preview = %v %s", res.IsError, resultText(res))
	}
}

// Review round 3: a boolean field saves as a boolean, and lint is read after
// the save, so a schema saved in the same call is the one paths are checked
// against.
func TestMCPTemplate_EditTypedFieldsAndFreshLint(t *testing.T) {
	env, cat, _ := templateEnv(t)
	cs := env.connect(t, clientOpts{answer: accept})
	res := callTool(t, cs, "template_edit", map[string]any{"carrierType": "category", "carrierId": cat.ID, "fields": map[string]any{
		"MetaSchema":   `{"type":"object","properties":{"budget":{"type":"number"}}}`,
		"CustomHeader": `<p>[meta path="budget"]</p>`,
	}})
	if res.IsError || strings.Contains(resultText(res), "not declared") {
		t.Fatalf("combined edit = %v %s", res.IsError, resultText(res))
	}

	nt := &models.NoteType{Name: "Shared"}
	env.tc.DB.Create(nt)
	for _, v := range []any{true, "false", true} {
		out := callTool(t, cs, "template_edit", map[string]any{"carrierType": "noteType", "carrierId": nt.ID,
			"fields": map[string]any{"ApplyTemplatesToShares": v}})
		if out.IsError {
			t.Fatalf("ApplyTemplatesToShares=%v: %s", v, resultText(out))
		}
	}
	var got models.NoteType
	env.tc.DB.First(&got, nt.ID)
	if !got.ApplyTemplatesToShares {
		t.Fatal("ApplyTemplatesToShares was not saved")
	}
	bad := callTool(t, cs, "template_edit", map[string]any{"carrierType": "noteType", "carrierId": nt.ID,
		"fields": map[string]any{"ApplyTemplatesToShares": "maybe"}})
	if !bad.IsError || !strings.Contains(resultText(bad), "Nothing was saved") {
		t.Fatalf("a bad boolean = %v %s", bad.IsError, resultText(bad))
	}
}
