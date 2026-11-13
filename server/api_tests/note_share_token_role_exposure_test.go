package api_tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
)

// A note's share token is a bearer credential for the unauthenticated share
// server: whoever holds it can read the note through the unscoped context and
// write todo state, and it outlives the account that read it. Disclosure is
// tied to the capability that may mint it — CanWrite, the one POST
// /v1/note/share demands — so a guest (read-only, always subtree-scoped) must
// not see it on any surface that serializes the note, top-level or nested
// (#107). It sees hasShare and a badge instead.

type shareTokenFixture struct {
	group    *models.Group
	note     *models.Note
	tag      *models.Tag
	noteType *models.NoteType
	resource *models.Resource
	token    string
}

func buildShareTokenFixture(t *testing.T, tc *TestContext) shareTokenFixture {
	t.Helper()
	g := &models.Group{Name: "st107-group"}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	nt := &models.NoteType{Name: "st107-notetype"}
	if err := tc.DB.Create(nt).Error; err != nil {
		t.Fatalf("create note type: %v", err)
	}
	tag := &models.Tag{Name: "st107-tag"}
	if err := tc.DB.Create(tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	r := &models.Resource{Name: "st107-resource", OwnerId: &g.ID, Location: "st107.txt"}
	if err := tc.DB.Create(r).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}
	n := &models.Note{
		Name: "st107-note", OwnerId: &g.ID, NoteTypeId: &nt.ID,
		Tags: []*models.Tag{tag}, Resources: []*models.Resource{r}, Groups: []*models.Group{g},
	}
	if err := tc.DB.Create(n).Error; err != nil {
		t.Fatalf("create note: %v", err)
	}
	token, err := tc.AppCtx.ShareNote(n.ID)
	if err != nil || token == "" {
		t.Fatalf("share note: %q %v", token, err)
	}
	return shareTokenFixture{group: g, note: n, tag: tag, noteType: nt, resource: r, token: token}
}

// scopedRoleBearer creates an account of the given role confined to scopeID and
// returns its bearer header. roleBearer cannot be used here: it gives a guest a
// fresh scope group of its own, and the point is a reader inside the note's.
func scopedRoleBearer(t *testing.T, tc *TestContext, role models.Role, scopeID uint, username string) string {
	t.Helper()
	u, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: username, Password: "password1", Role: role, ScopeGroupId: &scopeID,
	})
	if err != nil {
		t.Fatalf("create %s: %v", username, err)
	}
	raw, _, err := tc.AppCtx.CreateApiToken(u.ID, "t", nil)
	if err != nil {
		t.Fatalf("token for %s: %v", username, err)
	}
	return "Bearer " + raw
}

func shareTokenOfJSONNote(t *testing.T, body []byte) (string, bool) {
	t.Helper()
	var out struct {
		ShareToken *string `json:"shareToken"`
		HasShare   bool    `json:"hasShare"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode note: %v: %s", err, body)
	}
	if out.ShareToken == nil {
		return "", out.HasShare
	}
	return *out.ShareToken, out.HasShare
}

func TestNoteShareToken_HiddenFromGuests(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildShareTokenFixture(t, tc)
	guest := scopedRoleBearer(t, tc, models.RoleGuest, f.group.ID, "st107_guest")

	id := func(u uint) string { return fmt.Sprint(u) }
	saved, err := tc.AppCtx.CreateSavedMRQLQuery("st107-group-by-shared", "type = note GROUP BY shared COUNT()", "")
	if err != nil {
		t.Fatalf("save mrql query: %v", err)
	}
	surfaces := []struct {
		name, method, path, accept, body string
		// want is text proving the response actually carries the note (or its
		// owner), so "no token" is not satisfied by a 403, a 404 or an empty list.
		want string
	}{
		{"v1 note", http.MethodGet, "/v1/note?id=" + id(f.note.ID), "application/json", "", f.note.Name},
		{"v1 notes", http.MethodGet, "/v1/notes", "application/json", "", f.note.Name},
		{"v1 notes shared", http.MethodGet, "/v1/notes?Shared=1", "application/json", "", f.note.Name},
		{"note json", http.MethodGet, "/note.json?id=" + id(f.note.ID), "application/json", "", f.note.Name},
		{"note html", http.MethodGet, "/note?id=" + id(f.note.ID), "text/html", "", f.note.Name},
		{"hovercard", http.MethodGet, "/hovercard?type=note&id=" + id(f.note.ID), "text/html", "", f.note.Name},
		{"mrql", http.MethodPost, "/v1/mrql", "application/json", `{"query":"type = note"}`, f.note.Name},
		// GROUP BY returns its keys as plain rows, not models.Note, so json:"-"
		// does not reach them: `shared` must group on presence, not the column.
		{"mrql group by shared aggregated", http.MethodPost, "/v1/mrql", "application/json", `{"query":"type = note GROUP BY shared COUNT()"}`, `"shared"`},
		{"mrql group by shared bucketed", http.MethodPost, "/v1/mrql", "application/json", `{"query":"type = note GROUP BY shared"}`, f.note.Name},
		{"mrql export group by shared", http.MethodGet, "/v1/mrql/export?format=csv&query=" + url.QueryEscape("type = note GROUP BY shared COUNT()"), "text/csv", "", "shared"},
		{"mrql saved run group by shared", http.MethodPost, "/v1/mrql/saved/run?id=" + id(saved.ID), "application/json", "", `"shared"`},
		{"v1 group", http.MethodGet, "/v1/group?id=" + id(f.group.ID), "application/json", "", f.note.Name},
		{"group json", http.MethodGet, "/group.json?id=" + id(f.group.ID), "application/json", "", f.note.Name},
		{"group html", http.MethodGet, "/group?id=" + id(f.group.ID), "text/html", "", f.note.Name},
		{"v1 resource", http.MethodGet, "/v1/resource?id=" + id(f.resource.ID), "application/json", "", f.note.Name},
		{"resource html", http.MethodGet, "/resource?id=" + id(f.resource.ID), "text/html", "", f.resource.Name},
		{"tag json", http.MethodGet, "/tag.json?id=" + id(f.tag.ID), "application/json", "", f.tag.Name},
		{"note type json", http.MethodGet, "/noteType.json?id=" + id(f.noteType.ID), "application/json", "", f.note.Name},
	}
	for _, s := range surfaces {
		t.Run(s.name, func(t *testing.T) {
			headers := map[string]string{"Accept": s.accept, "Authorization": guest}
			var body io.Reader
			if s.body != "" {
				headers["Content-Type"] = "application/json"
				body = strings.NewReader(s.body)
			}
			res := doReq(tc, s.method, s.path, headers, nil, body)
			if res.Code != http.StatusOK {
				t.Fatalf("guest %s %s = %d, want 200: %s", s.method, s.path, res.Code, res.Body.String())
			}
			got := res.Body.String()
			if !strings.Contains(got, s.want) {
				t.Fatalf("guest %s %s does not carry %q, so the absence check below proves nothing", s.method, s.path, s.want)
			}
			// The raw token, not the JSON key: HTML forms entity-escape the key.
			if strings.Contains(got, f.token) {
				t.Errorf("guest %s %s leaks the share token", s.method, s.path)
			}
		})
	}

	t.Run("hasShare on note json", func(t *testing.T) {
		res := doReq(tc, http.MethodGet, "/v1/note?id="+id(f.note.ID),
			map[string]string{"Accept": "application/json", "Authorization": guest}, nil, nil)
		if _, hasShare := shareTokenOfJSONNote(t, res.Body.Bytes()); !hasShare {
			t.Errorf("GET /v1/note: hasShare = false for a shared note: %s", res.Body.String())
		}
		list := doReq(tc, http.MethodGet, "/v1/notes",
			map[string]string{"Accept": "application/json", "Authorization": guest}, nil, nil)
		var notes []struct {
			ID       uint `json:"ID"`
			HasShare bool `json:"hasShare"`
		}
		if err := json.Unmarshal(list.Body.Bytes(), &notes); err != nil {
			t.Fatalf("decode notes: %v", err)
		}
		found := false
		for _, n := range notes {
			if n.ID == f.note.ID {
				found = true
				if !n.HasShare {
					t.Errorf("GET /v1/notes: hasShare = false on the shared note")
				}
			}
		}
		if !found {
			t.Errorf("GET /v1/notes does not list note %d", f.note.ID)
		}
	})

	t.Run("mrql group by shared keys on presence", func(t *testing.T) {
		res := doReq(tc, http.MethodPost, "/v1/mrql",
			map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": guest},
			nil, strings.NewReader(`{"query":"type = note GROUP BY shared COUNT()"}`))
		var out struct {
			Rows []map[string]any `json:"rows"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode mrql: %v: %s", err, res.Body.String())
		}
		if len(out.Rows) != 1 || fmt.Sprint(out.Rows[0]["shared"]) != "1" || fmt.Sprint(out.Rows[0]["count"]) != "1" {
			t.Errorf("GROUP BY shared rows = %v, want one row {shared: 1, count: 1}", out.Rows)
		}
	})

	t.Run("detail page shows a badge only", func(t *testing.T) {
		res := doReq(tc, http.MethodGet, "/note?id="+id(f.note.ID),
			map[string]string{"Accept": "text/html", "Authorization": guest}, nil, nil)
		body := res.Body.String()
		if !strings.Contains(body, `data-testid="note-shared-badge"`) {
			t.Errorf("guest note page lacks the Shared badge")
		}
		for _, control := range []string{"Share Note", "Unshare", "/v1/note/share"} {
			if strings.Contains(body, control) {
				t.Errorf("guest note page renders the share control %q", control)
			}
		}
	})

	t.Run("shares dashboard", func(t *testing.T) {
		res := doReq(tc, http.MethodGet, "/admin/shares",
			map[string]string{"Accept": "text/html", "Authorization": guest}, nil, nil)
		if res.Code != http.StatusForbidden {
			t.Errorf("guest GET /admin/shares = %d, want 403", res.Code)
		}
	})
}

// Positive controls: a principal that may share still gets the token from the
// single-note read and the detail sidebar, and still does not get it from the
// list and nested surfaces, which never needed it.
func TestNoteShareToken_DisclosedToWriters(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildShareTokenFixture(t, tc)
	writers := map[string]string{
		"scoped user": scopedRoleBearer(t, tc, models.RoleUser, f.group.ID, "st107_user"),
		"editor":      roleBearer(t, tc, models.RoleEditor),
		"admin":       roleBearer(t, tc, models.RoleAdmin),
	}
	id := func(u uint) string { return fmt.Sprint(u) }
	for who, bearer := range writers {
		t.Run(who, func(t *testing.T) {
			js := map[string]string{"Accept": "application/json", "Authorization": bearer}
			res := doReq(tc, http.MethodGet, "/v1/note?id="+id(f.note.ID), js, nil, nil)
			if res.Code != http.StatusOK {
				t.Fatalf("GET /v1/note = %d: %s", res.Code, res.Body.String())
			}
			if got, _ := shareTokenOfJSONNote(t, res.Body.Bytes()); got != f.token {
				t.Errorf("GET /v1/note shareToken = %q, want %q", got, f.token)
			}

			html := doReq(tc, http.MethodGet, "/note?id="+id(f.note.ID),
				map[string]string{"Accept": "text/html", "Authorization": bearer}, nil, nil)
			if !strings.Contains(html.Body.String(), f.token) {
				t.Errorf("note page does not show the share token to a writer")
			}
			if !strings.Contains(html.Body.String(), "Unshare") {
				t.Errorf("note page does not offer Unshare to a writer")
			}

			for _, path := range []string{"/v1/notes", "/v1/group?id=" + id(f.group.ID)} {
				body := doReq(tc, http.MethodGet, path, js, nil, nil).Body.String()
				if !strings.Contains(body, f.note.Name) {
					t.Errorf("GET %s does not carry the note", path)
				}
				if strings.Contains(body, f.token) {
					t.Errorf("GET %s carries the share token; only GET /v1/note discloses it", path)
				}
			}
		})
	}
}

// A note type's templates render against the note itself, and the built-in
// [property] shortcode reads the Go field by reflection, so json:"-" does not
// keep ShareToken out of them. The note detail provider's strip is the only
// thing between such a template and a reader that may not share the note: the
// default share panel is gated on canShare on its own, so the other tests here
// stay green without the strip. Here every detail-page slot prints the field
// between markers, and the markers prove the slot rendered.
func TestNoteShareToken_NoteTypeTemplateCannotPrintIt(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildShareTokenFixture(t, tc)
	slots := map[string]string{
		"custom_header":        "HDR107:",
		"custom_sidebar":       "SIDE107:",
		"custom_detail_footer": "FOOT107:",
	}
	updates := map[string]any{}
	for col, marker := range slots {
		updates[col] = marker + `[property path="ShareToken"]:END107`
	}
	if err := tc.DB.Model(&models.NoteType{}).Where("id = ?", f.noteType.ID).Updates(updates).Error; err != nil {
		t.Fatalf("set note type templates: %v", err)
	}
	readers := map[string]struct {
		bearer    string
		wantToken bool
	}{
		"scoped guest": {scopedRoleBearer(t, tc, models.RoleGuest, f.group.ID, "st107_tpl_guest"), false},
		"scoped user":  {scopedRoleBearer(t, tc, models.RoleUser, f.group.ID, "st107_tpl_user"), true},
		"editor":       {roleBearer(t, tc, models.RoleEditor), true},
	}
	for who, r := range readers {
		for _, page := range []struct{ path, accept string }{
			{fmt.Sprintf("/note?id=%d", f.note.ID), "text/html"},
			{fmt.Sprintf("/note.json?id=%d", f.note.ID), "application/json"},
		} {
			t.Run(who+" "+page.path, func(t *testing.T) {
				res := doReq(tc, http.MethodGet, page.path,
					map[string]string{"Accept": page.accept, "Authorization": r.bearer}, nil, nil)
				if res.Code != http.StatusOK {
					t.Fatalf("GET %s = %d: %s", page.path, res.Code, res.Body.String())
				}
				body := res.Body.String()
				for _, marker := range slots {
					if !strings.Contains(body, marker) {
						t.Fatalf("GET %s did not render the note type slot %q, so the token check proves nothing", page.path, marker)
					}
					printed := strings.Contains(body, marker+f.token+":END107")
					if printed != r.wantToken {
						t.Errorf("GET %s: slot %q printed the share token = %v, want %v", page.path, marker, printed, r.wantToken)
					}
				}
				if !r.wantToken && strings.Contains(body, f.token) {
					t.Errorf("GET %s leaks the share token to a reader that may not share", page.path)
				}
			})
		}
	}
}

// With auth off every request runs as the super-user, which may share, so GET
// /v1/note keeps returning shareToken. The mr CLI doctest in
// cmd/mr/commands/notes_help/note_share.md pins that contract.
func TestNoteShareToken_AuthOffStillDisclosed(t *testing.T) {
	tc := SetupTestEnv(t)
	note := tc.CreateDummyNote("st107 auth-off note")
	token, err := tc.AppCtx.ShareNote(note.ID)
	if err != nil {
		t.Fatalf("share: %v", err)
	}
	res := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/v1/note?id=%d", note.ID), nil)
	if res.Code != http.StatusOK {
		t.Fatalf("GET /v1/note = %d", res.Code)
	}
	if got, _ := shareTokenOfJSONNote(t, res.Body.Bytes()); got != token {
		t.Errorf("auth-off GET /v1/note shareToken = %q, want %q", got, token)
	}
}
