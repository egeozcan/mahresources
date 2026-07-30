package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"mahresources/models"
)

// WS12 — taxonomy and template authoring, from the 2026-07-29 UI bug hunt.
//
//	17/93 — an invalid Meta JSON Schema was accepted and persisted verbatim
//	28    — the "Copy from existing" picker was capped at 50 per carrier
//	155   — a [partial] reference to a name that does not exist got no diagnostic
//	156   — the template-partial detail page printed the entity type twice
//	157   — a duplicate partial name surfaced as raw SQL

const invalidSchema = `{ not valid json ][`

// The schema the seeded "Media" category uses, as a known-good control.
const validSchema = `{"type":"object","properties":{"camera":{"type":"string"}}}`

// Findings 17/93. handler_factory.go only decoded the field and
// category_context.go assigned the raw string, so `{ not valid json ][` saved with
// HTTP 200 and reloaded verbatim, and every group form in that category silently
// fell back to the freeform meta editor. SectionConfig, declared right beside it,
// has always been types.JSON and so has always been parse-checked.
func TestInvalidMetaSchemaIsRejectedOnEveryCarrier(t *testing.T) {
	type carrier struct {
		name       string
		createPath string
		updatePath string
		nameField  string
	}
	carriers := []carrier{
		{"category", "/v1/category", "/v1/category/edit", "Name"},
		{"resourceCategory", "/v1/resourceCategory", "/v1/resourceCategory/edit", "Name"},
		{"noteType", "/v1/note/noteType", "/v1/note/noteType", "Name"},
	}

	for _, c := range carriers {
		t.Run(c.name, func(t *testing.T) {
			tc := SetupTestEnv(t)

			// Positive control first: a valid schema must still save, or "the invalid
			// one was rejected" is satisfied by an endpoint that rejects everything.
			ok := tc.MakeRequest(http.MethodPost, c.createPath, map[string]any{
				c.nameField:  "ws12-valid-" + c.name,
				"MetaSchema": validSchema,
			})
			if ok.Code != http.StatusOK {
				t.Fatalf("a VALID schema was rejected with %d: %s — this test measured nothing",
					ok.Code, ok.Body.String())
			}
			var created map[string]any
			_ = json.Unmarshal(ok.Body.Bytes(), &created)
			id, _ := created["ID"].(float64)
			if id == 0 {
				t.Fatalf("no ID in the create response: %s", ok.Body.String())
			}

			// Create with an unparseable schema.
			bad := tc.MakeRequest(http.MethodPost, c.createPath, map[string]any{
				c.nameField:  "ws12-invalid-" + c.name,
				"MetaSchema": invalidSchema,
			})
			if bad.Code == http.StatusOK {
				t.Errorf("create accepted an invalid Meta JSON Schema (HTTP 200): %s", bad.Body.String())
			} else if bad.Code != http.StatusBadRequest {
				t.Errorf("create returned %d for an invalid schema, want 400: %s", bad.Code, bad.Body.String())
			}
			if !strings.Contains(strings.ToLower(bad.Body.String()), "json") {
				t.Errorf("the rejection does not mention JSON, so it cannot tell the author what is wrong: %s",
					bad.Body.String())
			}

			// Update the valid one with an unparseable schema — the path the report
			// actually reproduced on.
			badUpdate := tc.MakeRequest(http.MethodPost, c.updatePath, map[string]any{
				"ID":         id,
				c.nameField:  "ws12-valid-" + c.name,
				"MetaSchema": invalidSchema,
			})
			if badUpdate.Code == http.StatusOK {
				t.Errorf("update accepted an invalid Meta JSON Schema (HTTP 200): %s", badUpdate.Body.String())
			}
		})
	}
}

// Findings 17/93, the storage half: nothing unparseable may reach the column, and
// an empty schema stays legal (that is how a freeform carrier is spelled).
func TestValidAndEmptyMetaSchemasStillSave(t *testing.T) {
	tc := SetupTestEnv(t)

	empty := tc.MakeRequest(http.MethodPost, "/v1/category", map[string]any{
		"Name": "ws12-empty-schema", "MetaSchema": "",
	})
	if empty.Code != http.StatusOK {
		t.Fatalf("an empty MetaSchema was rejected with %d: %s", empty.Code, empty.Body.String())
	}

	valid := tc.MakeRequest(http.MethodPost, "/v1/category", map[string]any{
		"Name": "ws12-valid-schema", "MetaSchema": validSchema,
	})
	if valid.Code != http.StatusOK {
		t.Fatalf("a valid MetaSchema was rejected with %d: %s", valid.Code, valid.Body.String())
	}

	var stored models.Category
	if err := tc.DB.Where("name = ?", "ws12-valid-schema").First(&stored).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if stored.MetaSchema != validSchema {
		t.Errorf("MetaSchema = %q, want it stored verbatim", stored.MetaSchema)
	}
}

// Finding 155. The linter reported an invalid [mrql] query right beside an unknown
// [partial] and said nothing about the partial. Measured on the unfixed code: one
// lint marker, covering `query='SELECT bogus FROM nothing']` only.
func TestLintReportsAnUnknownPartialReference(t *testing.T) {
	tc := SetupTestEnv(t)

	existing := &models.TemplatePartial{Name: "ws12-real-partial", Content: "<b>x</b>"}
	if err := tc.DB.Create(existing).Error; err != nil {
		t.Fatalf("create partial: %v", err)
	}

	lint := func(content string) []map[string]any {
		t.Helper()
		resp := tc.MakeRequest(http.MethodPost, "/v1/shortcodes/lint", map[string]any{"content": content})
		if resp.Code != http.StatusOK {
			t.Fatalf("POST /v1/shortcodes/lint = %d: %s", resp.Code, resp.Body.String())
		}
		var out struct {
			Issues []map[string]any `json:"issues"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out.Issues
	}

	missingIssues := lint(`OK:[partial name="ws12-real-partial"] MISSING:[partial name="does-not-exist"]`)
	found := false
	for _, iss := range missingIssues {
		if msg, _ := iss["message"].(string); strings.Contains(msg, "does-not-exist") {
			found = true
		}
	}
	if !found {
		t.Errorf("no diagnostic names the unknown partial. issues: %+v", missingIssues)
	}

	// Positive control: the partial that does exist must NOT be flagged, or the
	// check is just "every [partial] is an error".
	for _, iss := range missingIssues {
		if msg, _ := iss["message"].(string); strings.Contains(msg, "ws12-real-partial") {
			t.Errorf("an existing partial was flagged as missing: %s", msg)
		}
	}

	// And a template referencing only real partials lints clean of this class.
	cleanIssues := lint(`[partial name="ws12-real-partial"]`)
	for _, iss := range cleanIssues {
		if msg, _ := iss["message"].(string); strings.Contains(msg, "no template partial named") {
			t.Errorf("a template with only real partials produced a missing-partial issue: %s", msg)
		}
	}
}

// Finding 156. The provider set both prefix ("Template Partial") and pageTitle
// ("Template Partial: <name>"), and with no mainEntity the h1 falls back to
// pageTitle — so the pill and the heading both said the type. Measured: pill
// "Template Partial", h1 innerText "Template Partial Template Partial:
// b11-probe-partial". Every other detail page (Tag, Category, Query) shows the
// pill plus the bare name.
func TestTemplatePartialHeadingDoesNotRepeatTheEntityType(t *testing.T) {
	tc := SetupTestEnv(t)

	p := &models.TemplatePartial{Name: "ws12-heading-partial", Content: "<b>x</b>"}
	if err := tc.DB.Create(p).Error; err != nil {
		t.Fatalf("create partial: %v", err)
	}

	resp := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/templatePartial?id=%d", p.ID), nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /templatePartial = %d, want 200", resp.Code)
	}
	body := resp.Body.String()

	// Positive control: the page really is the partial's detail page.
	if !strings.Contains(body, "ws12-heading-partial") {
		t.Fatal("the partial's name is not on the page — this test measured nothing")
	}
	// The pill still names the type.
	if !strings.Contains(body, `>Template Partial</small>`) {
		t.Error("the type pill is gone; 156 asked for one type label, not zero")
	}
	// The <title> keeps the type, because that is what tells two tabs apart.
	if !strings.Contains(body, "<title>Template Partial: ws12-heading-partial") {
		t.Error("the document <title> should still carry the entity type")
	}

	h1 := heading1Text(body)
	if h1 == "" {
		t.Fatal("no <h1> on the page")
	}
	if strings.Contains(h1, "Template Partial: ") {
		t.Errorf("the h1 repeats the entity type next to the pill: %q", h1)
	}
	if !strings.Contains(h1, "ws12-heading-partial") {
		t.Errorf("the h1 does not name the partial: %q", h1)
	}
}

// heading1Text returns the text of the first <h1>, tags stripped and whitespace
// collapsed. Written here rather than reused because the h1 in title.tpl nests a
// <small> pill and an optional <inline-edit>.
func heading1Text(body string) string {
	start := strings.Index(body, "<h1")
	if start < 0 {
		return ""
	}
	end := strings.Index(body[start:], "</h1>")
	if end < 0 {
		return ""
	}
	inner := body[start : start+end]
	var sb strings.Builder
	depth := 0
	for i := 0; i < len(inner); i++ {
		switch inner[i] {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				sb.WriteByte(inner[i])
			}
		}
	}
	return strings.Join(strings.Fields(sb.String()), " ")
}

// Finding 157's residue, found live while checking the URL round trip: a duplicate
// template-partial name surfaced as `UNIQUE constraint failed:
// template_partials.name` in the form's error banner. Every other entity already
// runs through friendlyUniqueNameError.
func TestDuplicateTemplatePartialNameIsHumanReadable(t *testing.T) {
	tc := SetupTestEnv(t)

	form := url.Values{"Name": {"ws12-dup-partial"}, "Content": {"<b>a</b>"}}
	first := tc.MakeFormRequest(http.MethodPost, "/v1/templatePartial", form)
	if first.Code != http.StatusOK && first.Code != http.StatusFound {
		t.Fatalf("first create = %d: %s", first.Code, first.Body.String())
	}

	second := tc.MakeFormRequest(http.MethodPost, "/v1/templatePartial", form)
	location := second.Header().Get("Location")
	combined := second.Body.String() + " " + location

	if strings.Contains(combined, "UNIQUE constraint") {
		t.Errorf("the raw SQL constraint reached the user: %s", combined)
	}
	// Positive control: something was reported at all.
	if !strings.Contains(strings.ToLower(combined), "already exists") {
		t.Errorf("the duplicate was not reported in readable terms: %s", combined)
	}
}

// Finding 28's server-side precondition. The "Copy from existing" picker offered 50
// categories and 50 note types out of 73 and 67, because the client issued one
// un-paged GET and the list endpoints cap at constants.MaxResultsPerPage and ignore
// maxResults. The client fix pages; this asserts the paging it relies on actually
// works, and that maxResults really is ignored (so nobody "simplifies" the client
// back to a single request).
func TestCategoryListPagesPastTheFiftyRowCap(t *testing.T) {
	tc := SetupTestEnv(t)

	const total = 62
	for i := 0; i < total; i++ {
		cat := &models.Category{Name: fmt.Sprintf("ws12-cat-%03d", i)}
		if err := tc.DB.Create(cat).Error; err != nil {
			t.Fatalf("create category %d: %v", i, err)
		}
	}

	count := func(path string) int {
		t.Helper()
		resp := tc.MakeRequest(http.MethodGet, path, nil)
		if resp.Code != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", path, resp.Code, resp.Body.String())
		}
		var rows []map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &rows); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		return len(rows)
	}

	if got := count("/v1/categories"); got != 50 {
		t.Errorf("page 1 returned %d rows, want the 50-row page size", got)
	}
	if got := count("/v1/categories?maxResults=500"); got != 50 {
		t.Errorf("maxResults=500 returned %d rows; the endpoint is supposed to ignore it, "+
			"so the client must page. If this changed, revisit templateBundle.loadSources.", got)
	}
	// Deltas rather than an exact count: seed data adds categories of its own.
	page2 := count("/v1/categories?page=2")
	if page2 == 0 {
		t.Error("page 2 is empty, so the client cannot page past the cap at all")
	}
	if got := count("/v1/categories?page=1") + page2; got < total {
		t.Errorf("pages 1+2 hold %d rows, fewer than the %d created", got, total)
	}
}

// Findings 17/93, the other side of the rule: validation stops an unparseable
// schema getting *in*, and the forms still have to survive one that is already
// there. A row can hold non-JSON for several reasons validation cannot reach — it
// predates the rule, a plugin wrote it (`mah.db.*` is a documented v1 gap), or an
// operator edited the database.
//
// This replaces two Playwright tests from e2e/tests/schema/editor-bugfixes.spec.ts
// ("group edit page does not crash with non-JSON MetaSchema" and "dynamic category
// selection handles invalid MetaSchema without crashing"), whose fixtures created
// the category over HTTP and can no longer do so. Planting the row takes one line
// here, and CI runs `go test` — it does not run the browser suite.
func TestFormsSurviveALegacyNonJSONMetaSchema(t *testing.T) {
	tc := SetupTestEnv(t)

	// Straight to the column, past every write path's validation — exactly the
	// legacy state the guard is about.
	cat := &models.Category{Name: "ws12-legacy-schema"}
	if err := tc.DB.Create(cat).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
	if err := tc.DB.Model(&models.Category{}).Where("id = ?", cat.ID).
		Update("meta_schema", "this is not json").Error; err != nil {
		t.Fatalf("plant the legacy schema: %v", err)
	}

	// Positive control: the row really does hold the garbage, so the assertions
	// below are not measuring a category with an empty schema.
	var reloaded models.Category
	if err := tc.DB.First(&reloaded, cat.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.MetaSchema != "this is not json" {
		t.Fatalf("MetaSchema = %q, want the planted garbage — this test measured nothing",
			reloaded.MetaSchema)
	}

	group := tc.CreateDummyGroup("ws12-legacy-schema-member")
	if err := tc.DB.Model(&models.Group{}).Where("id = ?", group.ID).
		Update("category_id", cat.ID).Error; err != nil {
		t.Fatalf("attach the group to the category: %v", err)
	}

	for _, path := range []string{
		fmt.Sprintf("/group/edit?id=%d", group.ID),
		"/group/new",
		fmt.Sprintf("/category/edit?id=%d", cat.ID),
	} {
		resp := tc.MakeRequest(http.MethodGet, path, nil)
		if resp.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200 — a legacy schema must not break the page", path, resp.Code)
		}
	}

	// And the value must reach the page escaped, whichever way it is embedded. The
	// browser half of this (no page error, no alert) stays in the schema specs with
	// a payload that is valid JSON and still hostile.
	body := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/group/edit?id=%d", group.ID), nil).Body.String()
	if strings.Contains(body, `x-data="this is not json`) {
		t.Error("the raw schema was interpolated into an x-data attribute unescaped")
	}
}

// Findings 17/93: a hostile-but-valid schema — one an author can legitimately save
// — must not be able to break out of the attribute it is injected into. Kept in Go
// as well as the browser because CI runs this one.
func TestHostileButValidMetaSchemaIsEscapedInMarkup(t *testing.T) {
	tc := SetupTestEnv(t)

	hostile := `{"type":"object","description":"'; alert('xss'); '"}`
	resp := tc.MakeRequest(http.MethodPost, "/v1/category", map[string]any{
		"Name": "ws12-hostile-valid", "MetaSchema": hostile,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("a valid schema carrying quotes was rejected with %d: %s — the validation is "+
			"too strict, it should only reject unparseable JSON", resp.Code, resp.Body.String())
	}

	var cat models.Category
	if err := tc.DB.Where("name = ?", "ws12-hostile-valid").First(&cat).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	group := tc.CreateDummyGroup("ws12-hostile-member")
	if err := tc.DB.Model(&models.Group{}).Where("id = ?", group.ID).
		Update("category_id", cat.ID).Error; err != nil {
		t.Fatalf("attach: %v", err)
	}

	page := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/group/edit?id=%d", group.ID), nil)
	if page.Code != http.StatusOK {
		t.Fatalf("GET /group/edit = %d", page.Code)
	}
	body := page.Body.String()
	// Positive control: the schema is on the page at all.
	if !strings.Contains(body, "alert") {
		t.Fatal("the schema is not on the page — this test measured nothing")
	}
	// The unescaped sequence would close the attribute and start a statement.
	if strings.Contains(body, `'; alert('xss'); '`) {
		t.Error("the hostile quote sequence reached the page unescaped")
	}
}
