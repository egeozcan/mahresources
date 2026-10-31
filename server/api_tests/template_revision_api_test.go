package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
)

type revisionCarrier struct {
	typ        string
	apiPrefix  string
	pagePrefix string
	id         uint
	adminOnly  bool
}

func seedRevisionCarriers(t *testing.T, tc *TestContext) []revisionCarrier {
	t.Helper()
	cat, err := tc.AppCtx.CreateCategory(&query_models.CategoryCreator{Name: "rev-cat", CustomHeader: "one"})
	if err != nil {
		t.Fatal(err)
	}
	rc, err := tc.AppCtx.CreateResourceCategory(&query_models.ResourceCategoryCreator{Name: "rev-rc", CustomSummary: "one"})
	if err != nil {
		t.Fatal(err)
	}
	nt, err := tc.AppCtx.CreateOrUpdateNoteType(&query_models.NoteTypeEditor{Name: "rev-nt", CustomHeader: "one"})
	if err != nil {
		t.Fatal(err)
	}
	return []revisionCarrier{
		{models.TemplateCarrierCategory, "/v1/category", "/category", cat.ID, true},
		{models.TemplateCarrierResourceCategory, "/v1/resourceCategory", "/resourceCategory", rc.ID, true},
		{models.TemplateCarrierNoteType, "/v1/noteType", "/noteType", nt.ID, false},
	}
}

func revisionRows(t *testing.T, tc *TestContext, c revisionCarrier) []models.TemplateRevision {
	t.Helper()
	var revs []models.TemplateRevision
	if err := tc.DB.Where("carrier_type = ? AND carrier_id = ?", c.typ, c.id).Order("number").Find(&revs).Error; err != nil {
		t.Fatal(err)
	}
	return revs
}

// Revision history is as sensitive as the editor, and the edit pages are
// readable by lower roles, so the generic capRead branches must not cover it.
// Every cell of this matrix is asserted explicitly: a role either gets the
// answer or a 403, for every carrier type and every operation, on the .json and
// .body page variants too.
func TestTemplateRevisionRoleMatrix(t *testing.T) {
	tc := setupAuthEnv(t)
	carriers := seedRevisionCarriers(t, tc)

	scopeGroup := &models.Group{Name: "revision-scope"}
	if err := tc.DB.Create(scopeGroup).Error; err != nil {
		t.Fatal(err)
	}
	bearer := func(name string, role models.Role, scope *uint) string {
		u, err := tc.AppCtx.CreateUser(&application_context.UserInput{Username: name, Password: "password1", Role: role, ScopeGroupId: scope})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		raw, _, err := tc.AppCtx.CreateApiToken(u.ID, "t", nil)
		if err != nil {
			t.Fatal(err)
		}
		return "Bearer " + raw
	}
	principals := []struct {
		name   string
		bearer string
		admin  bool
		editor bool
	}{
		{"admin", bearer("rv_admin", models.RoleAdmin, nil), true, true},
		{"editor", bearer("rv_editor", models.RoleEditor, nil), false, true},
		{"unscoped user", bearer("rv_user", models.RoleUser, nil), false, false},
		{"scoped user", bearer("rv_scoped", models.RoleUser, &scopeGroup.ID), false, false},
		{"guest", bearer("rv_guest", models.RoleGuest, &scopeGroup.ID), false, false},
	}

	for _, c := range carriers {
		id := fmt.Sprint(c.id)
		type op struct {
			name, method, path, body string
			api                      bool
		}
		ops := []op{
			{"list", "GET", c.apiPrefix + "/revisions?id=" + id, "", true},
			{"get", "GET", c.apiPrefix + "/revisions/1?id=" + id, "", true},
			{"diff", "GET", c.apiPrefix + "/revisions/diff?id=" + id + "&from=1", "", true},
			{"label", "POST", c.apiPrefix + "/revisions/1/label?id=" + id, `{"label":"x"}`, true},
			{"restore", "POST", c.apiPrefix + "/revisions/1/restore?id=" + id, `{}`, true},
			{"history page", "GET", c.pagePrefix + "/revisions?id=" + id, "", false},
			{"history page .json", "GET", c.pagePrefix + "/revisions.json?id=" + id, "", false},
			{"history page .body", "GET", c.pagePrefix + "/revisions.body?id=" + id, "", false},
		}
		for _, p := range principals {
			allowed := p.editor
			if c.adminOnly {
				allowed = p.admin
			}
			for _, o := range ops {
				t.Run(c.typ+"/"+p.name+"/"+o.name, func(t *testing.T) {
					headers := map[string]string{"Accept": "application/json", "Authorization": p.bearer}
					var body *strings.Reader
					if o.body != "" {
						headers["Content-Type"] = "application/json"
						body = strings.NewReader(o.body)
					}
					var rr *http.Response
					if body != nil {
						rr = doReq(tc, o.method, o.path, headers, nil, body).Result()
					} else {
						rr = doReq(tc, o.method, o.path, headers, nil, nil).Result()
					}
					switch {
					case !allowed && rr.StatusCode != http.StatusForbidden:
						t.Errorf("%s %s: status %d, want 403", o.method, o.path, rr.StatusCode)
					case allowed && o.api && rr.StatusCode != http.StatusOK:
						t.Errorf("%s %s: status %d, want 200", o.method, o.path, rr.StatusCode)
					case allowed && !o.api && rr.StatusCode == http.StatusForbidden:
						t.Errorf("%s %s: status 403 for an allowed role", o.method, o.path)
					}
				})
			}
		}
	}
}

func TestTemplateRevisionSourcesAcrossIngress(t *testing.T) {
	tc := SetupTestEnv(t)
	cat, err := tc.AppCtx.CreateCategory(&query_models.CategoryCreator{Name: "src-cat"})
	if err != nil {
		t.Fatal(err)
	}
	c := revisionCarrier{typ: models.TemplateCarrierCategory, id: cat.ID}

	// The edit form: a browser posts urlencoded and accepts HTML. revisionApplied
	// may repeat or be comma separated; unknown entries are dropped.
	form := url.Values{}
	form.Set("ID", fmt.Sprint(cat.ID))
	form.Set("Name", "src-cat")
	form.Set("CustomHeader", "from the form")
	form.Add("revisionApplied", "preset:dashboard")
	form.Add("revisionApplied", "generated,loaded:1:CustomHeader,bogus")
	rr := doReq(tc, "POST", "/v1/category", map[string]string{
		"Accept": "text/html", "Content-Type": "application/x-www-form-urlencoded",
	}, nil, strings.NewReader(form.Encode()))
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("form save status = %d: %s", rr.Code, rr.Body.String())
	}

	// The mr CLI posts JSON with the advisory header.
	rr = doReq(tc, "POST", "/v1/category", map[string]string{
		"Accept": "application/json", "Content-Type": "application/json", "X-Mahresources-Client": "mr",
	}, nil, strings.NewReader(fmt.Sprintf(`{"ID":%d,"Name":"src-cat","CustomHeader":"from the cli"}`, cat.ID)))
	if rr.Code != http.StatusOK {
		t.Fatalf("cli save status = %d: %s", rr.Code, rr.Body.String())
	}

	// Any other API client.
	rr = doReq(tc, "POST", "/v1/category", map[string]string{
		"Accept": "application/json", "Content-Type": "application/json",
	}, nil, strings.NewReader(fmt.Sprintf(`{"ID":%d,"Name":"src-cat","CustomHeader":"from curl"}`, cat.ID)))
	if rr.Code != http.StatusOK {
		t.Fatalf("api save status = %d: %s", rr.Code, rr.Body.String())
	}

	revs := revisionRows(t, tc, c)
	var sources []string
	for _, r := range revs {
		sources = append(sources, r.Source)
	}
	if strings.Join(sources, ",") != "create,form,cli,api" {
		t.Fatalf("sources = %v", sources)
	}
	var applied []string
	_ = json.Unmarshal([]byte(revs[1].Applied), &applied)
	if strings.Join(applied, ",") != "preset:dashboard,generated,loaded:1:CustomHeader" {
		t.Errorf("applied = %v", applied)
	}
}

func TestTemplateRevisionEndpointsRoundTrip(t *testing.T) {
	tc := SetupTestEnv(t)
	carriers := seedRevisionCarriers(t, tc)

	for _, c := range carriers {
		t.Run(c.typ, func(t *testing.T) {
			id := fmt.Sprint(c.id)
			get := func(path string) (int, []byte) {
				rr := doReq(tc, "GET", path, map[string]string{"Accept": "application/json"}, nil, nil)
				return rr.Code, rr.Body.Bytes()
			}
			post := func(path, body string) (int, []byte) {
				rr := doReq(tc, "POST", path, map[string]string{"Accept": "application/json", "Content-Type": "application/json"}, nil, strings.NewReader(body))
				return rr.Code, rr.Body.Bytes()
			}

			// Change the carrier through the real save endpoint.
			saveBody := map[string]string{
				models.TemplateCarrierCategory:         `{"ID":%d,"Name":"rev-cat","CustomHeader":"two","CustomHeaderCSS":"h{}"}`,
				models.TemplateCarrierResourceCategory: `{"ID":%d,"Name":"rev-rc","CustomSummary":"two","CustomSummaryCSS":"h{}"}`,
				models.TemplateCarrierNoteType:         `{"ID":%d,"Name":"rev-nt","CustomHeader":"two","CustomHeaderCSS":"h{}"}`,
			}[c.typ]
			savePath := map[string]string{
				models.TemplateCarrierCategory:         "/v1/category",
				models.TemplateCarrierResourceCategory: "/v1/resourceCategory",
				models.TemplateCarrierNoteType:         "/v1/note/noteType",
			}[c.typ]
			if code, body := post(savePath, fmt.Sprintf(saveBody, c.id)); code != 200 {
				t.Fatalf("save: %d %s", code, body)
			}

			code, body := get(c.apiPrefix + "/revisions?id=" + id)
			var list []struct {
				Number       uint     `json:"number"`
				Source       string   `json:"source"`
				ChangedSlots []string `json:"changedSlots"`
			}
			if code != 200 || json.Unmarshal(body, &list) != nil || len(list) != 2 || list[0].Number != 2 || list[1].Number != 1 {
				t.Fatalf("list = %d %s", code, body)
			}
			if len(list[0].ChangedSlots) != 1 {
				t.Errorf("changed slots = %v", list[0].ChangedSlots)
			}

			code, body = get(c.apiPrefix + "/revisions/1?id=" + id)
			var detail struct {
				Snapshot map[string]any `json:"snapshot"`
				Editor   map[string]any `json:"editor"`
			}
			if code != 200 || json.Unmarshal(body, &detail) != nil || detail.Snapshot["CustomHeader"] == nil && detail.Snapshot["CustomSummary"] == nil {
				t.Fatalf("get = %d %s", code, body)
			}
			if _, ok := detail.Editor["SectionConfig"].(string); !ok {
				t.Errorf("editor view must carry the resolved SectionConfig as a string: %v", detail.Editor["SectionConfig"])
			}

			code, body = get(c.apiPrefix + "/revisions/diff?id=" + id + "&from=1")
			var diff struct {
				From  uint  `json:"from"`
				To    *uint `json:"to"`
				Slots []struct {
					Slot   string `json:"slot"`
					Fields []struct {
						Field string `json:"field"`
						Hunks []struct {
							Op    string   `json:"op"`
							Lines []string `json:"lines"`
						} `json:"hunks"`
					} `json:"fields"`
				} `json:"slots"`
			}
			if code != 200 || json.Unmarshal(body, &diff) != nil || diff.To != nil || len(diff.Slots) != 1 {
				t.Fatalf("diff vs current = %d %s", code, body)
			}
			if code, _ := get(c.apiPrefix + "/revisions/diff?id=" + id + "&from=1&to=2"); code != 200 {
				t.Errorf("diff 1..2 = %d", code)
			}
			if code, _ := get(c.apiPrefix + "/revisions/diff?id=" + id); code != 400 {
				t.Errorf("diff without from = %d, want 400", code)
			}
			if code, _ := get(c.apiPrefix + "/revisions/9?id=" + id); code != 404 {
				t.Errorf("missing revision = %d, want 404", code)
			}

			if code, body := post(c.apiPrefix+"/revisions/1/label?id="+id, `{"label":"first cut"}`); code != 200 || !strings.Contains(string(body), "first cut") {
				t.Errorf("label = %d %s", code, body)
			}
			if code, _ := post(c.apiPrefix+"/revisions/1/label?id="+id, fmt.Sprintf(`{"label":%q}`, strings.Repeat("x", 201))); code != 400 {
				t.Errorf("overlong label = %d, want 400", code)
			}

			if code, body := post(c.apiPrefix+"/revisions/1/restore?id="+id, `{}`); code != 200 || !strings.Contains(string(body), `"source":"restore"`) {
				t.Fatalf("restore = %d %s", code, body)
			}
			revs := revisionRows(t, tc, c)
			if len(revs) != 3 || revs[2].Source != models.RevisionSourceRestore || revs[2].RestoredFrom == nil || *revs[2].RestoredFrom != 1 {
				t.Errorf("revisions after restore: %+v", revs)
			}
			if code, _ := post(c.apiPrefix+"/revisions/1/restore?id="+id, `{"slot":"Name"}`); code != 400 {
				t.Errorf("restore of an untracked slot = %d, want 400", code)
			}
		})
	}
}

// A restore of a slot with includeCss=false keeps the CSS that is current.
func TestTemplateRevisionRestoreOneSlotOverHTTP(t *testing.T) {
	tc := SetupTestEnv(t)
	cat, err := tc.AppCtx.CreateCategory(&query_models.CategoryCreator{Name: "slot-cat", CustomHeader: "h1", CustomHeaderCSS: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if rr := doReq(tc, "POST", "/v1/category", map[string]string{"Accept": "application/json", "Content-Type": "application/json"}, nil,
		strings.NewReader(fmt.Sprintf(`{"ID":%d,"Name":"slot-cat","CustomHeader":"h2","CustomHeaderCSS":"c2"}`, cat.ID))); rr.Code != 200 {
		t.Fatal(rr.Body.String())
	}
	rr := doReq(tc, "POST", fmt.Sprintf("/v1/category/revisions/1/restore?id=%d", cat.ID),
		map[string]string{"Accept": "application/json", "Content-Type": "application/json"}, nil,
		strings.NewReader(`{"slot":"CustomHeader","includeCss":false}`))
	if rr.Code != 200 {
		t.Fatalf("restore: %d %s", rr.Code, rr.Body.String())
	}
	got, _ := tc.AppCtx.GetCategory(cat.ID)
	if got.CustomHeader != "h1" || got.CustomHeaderCSS != "c2" {
		t.Errorf("header=%q css=%q, want h1 and the current c2", got.CustomHeader, got.CustomHeaderCSS)
	}
}

// A create is the first save of a preset or an AI generation, so it carries
// the same revisionApplied provenance an update does.
func TestTemplateRevisionCreateKeepsAppliedProvenance(t *testing.T) {
	tc := SetupTestEnv(t)
	form := url.Values{}
	form.Set("Name", "created-with-preset")
	form.Set("CustomHeader", "x")
	form.Add("revisionApplied", "preset:x,generated")
	form.Add("revisionApplied", "bogus")
	rr := doReq(tc, "POST", "/v1/category", map[string]string{
		"Accept": "text/html", "Content-Type": "application/x-www-form-urlencoded",
	}, nil, strings.NewReader(form.Encode()))
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("create status = %d: %s", rr.Code, rr.Body.String())
	}
	var cat models.Category
	if err := tc.DB.Where("name = ?", "created-with-preset").First(&cat).Error; err != nil {
		t.Fatal(err)
	}
	revs := revisionRows(t, tc, revisionCarrier{typ: models.TemplateCarrierCategory, id: cat.ID})
	if len(revs) != 1 || revs[0].Source != models.RevisionSourceCreate {
		t.Fatalf("revisions = %+v", revs)
	}
	var applied []string
	_ = json.Unmarshal([]byte(revs[0].Applied), &applied)
	if strings.Join(applied, ",") != "preset:x,generated" {
		t.Errorf("applied on create = %v", applied)
	}
}
