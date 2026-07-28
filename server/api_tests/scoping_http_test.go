package api_tests

import (
	"net/http"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
)

// scopedUserBearer creates a User confined to scopeGroupID and returns its bearer header.
func scopedUserBearer(t *testing.T, tc *TestContext, scopeGroupID uint) string {
	t.Helper()
	u, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "scoped", Password: "password1", Role: models.RoleUser, ScopeGroupId: &scopeGroupID,
	})
	if err != nil {
		t.Fatalf("create scoped user: %v", err)
	}
	raw, _, err := tc.AppCtx.CreateApiToken(u.ID, "t", nil)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	return "Bearer " + raw
}

// scopingHTTPFixture builds root>child + outside groups with a resource and note
// (each with a stored file) in both child and outside. Returns the bearer for a
// user scoped to root and the IDs/locations needed for assertions.
type scopeFixture struct {
	bearer                       string
	rootID, childID, outsideID   uint
	rInID, rOutID, nInID, nOutID uint
	inLoc, outLoc                string
}

func buildScopingFixture(t *testing.T, tc *TestContext) scopeFixture {
	t.Helper()
	root := &models.Group{Name: "sf-root"}
	tc.DB.Create(root)
	child := &models.Group{Name: "sf-child", OwnerId: &root.ID}
	tc.DB.Create(child)
	outside := &models.Group{Name: "sf-outside"}
	tc.DB.Create(outside)

	inLoc, outLoc := "scope-in.txt", "scope-out.txt"
	rIn := &models.Resource{Name: "sf-rIn", OwnerId: &child.ID, Location: inLoc}
	rOut := &models.Resource{Name: "sf-rOut", OwnerId: &outside.ID, Location: outLoc}
	tc.DB.Create(rIn)
	tc.DB.Create(rOut)
	nIn := &models.Note{Name: "sf-nIn", OwnerId: &child.ID}
	nOut := &models.Note{Name: "sf-nOut", OwnerId: &outside.ID}
	tc.DB.Create(nIn)
	tc.DB.Create(nOut)

	// The raw /files server guard (FilePathInScope) is proven by a unit test in
	// the application_context package; the bare-MemMapFs test harness here cannot
	// serve file bytes over HTTP (it works only in the real ephemeral server's
	// storage fs), so we do not assert byte serving at this layer.

	return scopeFixture{
		bearer: scopedUserBearer(t, tc, root.ID),
		rootID: root.ID, childID: child.ID, outsideID: outside.ID,
		rInID: rIn.ID, rOutID: rOut.ID, nInID: nIn.ID, nOutID: nOut.ID,
		inLoc: inLoc, outLoc: outLoc,
	}
}

func TestScopedUser_ListsOnlySubtree(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildScopingFixture(t, tc)
	h := map[string]string{"Accept": "application/json", "Authorization": f.bearer}

	resBody := doReq(tc, http.MethodGet, "/v1/resources", h, nil, nil).Body.String()
	if !strings.Contains(resBody, "sf-rIn") || strings.Contains(resBody, "sf-rOut") {
		t.Fatalf("resources list should contain only sf-rIn, got: %s", resBody)
	}

	noteBody := doReq(tc, http.MethodGet, "/v1/notes", h, nil, nil).Body.String()
	if !strings.Contains(noteBody, "sf-nIn") || strings.Contains(noteBody, "sf-nOut") {
		t.Fatalf("notes list should contain only sf-nIn, got: %s", noteBody)
	}

	groupBody := doReq(tc, http.MethodGet, "/v1/groups", h, nil, nil).Body.String()
	if !strings.Contains(groupBody, "sf-root") || !strings.Contains(groupBody, "sf-child") || strings.Contains(groupBody, "sf-outside") {
		t.Fatalf("groups list should contain only subtree groups, got: %s", groupBody)
	}
}

func TestScopedUser_SingleGetOutsideIs404(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildScopingFixture(t, tc)
	h := map[string]string{"Accept": "application/json", "Authorization": f.bearer}

	in := doReq(tc, http.MethodGet, "/v1/resource?id="+itoa(int(f.rInID)), h, nil, nil)
	if in.Code != http.StatusOK {
		t.Fatalf("in-subtree resource should be 200, got %d", in.Code)
	}
	out := doReq(tc, http.MethodGet, "/v1/resource?id="+itoa(int(f.rOutID)), h, nil, nil)
	if out.Code == http.StatusOK {
		t.Fatalf("out-of-subtree resource get should not be 200, got %d", out.Code)
	}
}

func TestScopedUser_SearchAndMRQLConfined(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildScopingFixture(t, tc)
	h := map[string]string{"Accept": "application/json", "Authorization": f.bearer}

	// The handler reads the "q" parameter. This request previously said
	// "query=", so the term never arrived: every response was
	// {"query":"","total":0,"results":[]} and the confinement assertion below
	// held for free. Scoped search was, in effect, untested. The positive
	// control now fails loudly if that ever recurs.
	search := doReq(tc, http.MethodGet, "/v1/search?q=sf", h, nil, nil).Body.String()
	if !strings.Contains(search, "sf-rIn") || !strings.Contains(search, "sf-nIn") {
		t.Fatalf("control invalid: scoped search surfaced no in-subtree entities, so the "+
			"confinement assertion proves nothing, got: %s", search)
	}
	if strings.Contains(search, "sf-rOut") || strings.Contains(search, "sf-nOut") ||
		strings.Contains(search, "sf-outside") {
		t.Fatalf("search must not surface out-of-subtree entities, got: %s", search)
	}

	mrqlBody := strings.NewReader(`{"query":"name ~ \"sf-r*\""}`)
	mh := map[string]string{"Accept": "application/json", "Authorization": f.bearer, "Content-Type": "application/json"}
	mrql := doReq(tc, http.MethodPost, "/v1/mrql", mh, nil, mrqlBody).Body.String()
	if strings.Contains(mrql, "sf-rOut") {
		t.Fatalf("MRQL must be force-scoped to the subtree, got: %s", mrql)
	}
	if !strings.Contains(mrql, "sf-rIn") {
		t.Fatalf("MRQL should still return in-subtree resources, got: %s", mrql)
	}
}

func TestScopedUser_GroupTreeConfined(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildScopingFixture(t, tc)
	h := map[string]string{"Accept": "application/json", "Authorization": f.bearer}

	// Roots (parentId=0) for a scoped user are rooted at their scope group.
	roots := doReq(tc, http.MethodGet, "/v1/group/tree/children?parentId=0", h, nil, nil).Body.String()
	if !strings.Contains(roots, "sf-root") || strings.Contains(roots, "sf-outside") {
		t.Fatalf("scoped tree roots should be the scope group only, got: %s", roots)
	}

	// Expanding an out-of-subtree group yields nothing.
	outChildren := doReq(tc, http.MethodGet, "/v1/group/tree/children?parentId="+itoa(int(f.outsideID)), h, nil, nil).Body.String()
	if strings.Contains(outChildren, "sf-") && !strings.Contains(outChildren, "[]") {
		t.Fatalf("expanding an out-of-subtree group should return nothing, got: %s", outChildren)
	}
}

func TestScopedUser_ExportConfined(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildScopingFixture(t, tc)
	h := map[string]string{"Accept": "application/json", "Authorization": f.bearer, "Content-Type": "application/json"}

	// Exporting an in-subtree group is permitted (not a 404 from the guard).
	inResp := doReq(tc, http.MethodPost, "/v1/groups/export/estimate", h, nil,
		strings.NewReader(`{"rootGroupIds":[`+itoa(int(f.rootID))+`]}`))
	if inResp.Code == http.StatusNotFound {
		t.Fatalf("scoped user should be able to export their own subtree, got 404")
	}

	// Exporting an out-of-subtree group is blocked.
	outResp := doReq(tc, http.MethodPost, "/v1/groups/export/estimate", h, nil,
		strings.NewReader(`{"rootGroupIds":[`+itoa(int(f.outsideID))+`]}`))
	if outResp.Code != http.StatusNotFound {
		t.Fatalf("scoped user must not export an out-of-subtree group, got %d", outResp.Code)
	}
}

// Template pages are confined by being built against a per-request,
// principal-scoped context: routes.go calls info.contextFn(sc) inside the
// request handler, not once at wiring time. Binding the providers to the
// singleton instead would unscope every HTML page while leaving the /v1 API —
// and therefore every other test in this file — perfectly correct.
//
// e2e covers this through the browser, but only there; this is the fast pin.
func TestScopedUser_TemplatePagesConfined(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildScopingFixture(t, tc)
	scopedH := map[string]string{"Accept": "application/json", "Authorization": f.bearer}
	adminH := map[string]string{"Accept": "application/json", "Authorization": unscopedAdminBearer(t, tc)}

	// The template routes serve their pongo2 context as JSON under .json, which
	// is the same context the HTML page renders from.
	for _, path := range []string{"/groups.json", "/notes.json", "/resources.json"} {
		// Control: an unscoped admin sees the out-of-subtree entities through
		// this exact route, so their absence below is enforcement and not an
		// empty or broken page.
		ctrl := doReq(tc, http.MethodGet, path, adminH, nil, nil).Body.String()
		if !strings.Contains(ctrl, "sf-outside") && !strings.Contains(ctrl, "sf-rOut") &&
			!strings.Contains(ctrl, "sf-nOut") {
			t.Fatalf("control invalid: %s did not surface any out-of-subtree entity for an admin, "+
				"so the scoped assertion proves nothing, got: %s", path, ctrl)
		}

		scoped := doReq(tc, http.MethodGet, path, scopedH, nil, nil).Body.String()
		for _, secret := range []string{"sf-outside", "sf-rOut", "sf-nOut"} {
			if strings.Contains(scoped, secret) {
				t.Errorf("%s leaked out-of-subtree entity %q to a group-limited principal; the page "+
					"context was not built against the request-scoped context", path, secret)
			}
		}
	}

	// Positive: the scoped user's own page still has their in-subtree data.
	groups := doReq(tc, http.MethodGet, "/groups.json", scopedH, nil, nil).Body.String()
	if !strings.Contains(groups, "sf-root") || !strings.Contains(groups, "sf-child") {
		t.Errorf("scoped /groups.json lost in-subtree groups, got: %s", groups)
	}
}

// The global search result cache is process-wide and keyed on the lowercased
// search term alone — it carries no principal in the key. Correctness therefore
// rests entirely on GlobalSearch bypassing the cache, for both reads and writes,
// whenever the caller is group-limited. Neither direction was covered.
//
// Each sub-test gets its own server so it starts from an empty cache; the two
// directions fail in different ways and would otherwise share one entry.
func TestScopedUser_SearchCacheNotSharedAcrossScopes(t *testing.T) {
	const q = "/v1/search?q=sf"

	t.Run("scoped read does not hit an admin-populated entry", func(t *testing.T) {
		tc := setupAuthEnv(t)
		f := buildScopingFixture(t, tc)
		adminH := map[string]string{"Accept": "application/json", "Authorization": unscopedAdminBearer(t, tc)}
		scopedH := map[string]string{"Accept": "application/json", "Authorization": f.bearer}

		// Admin first, filling the shared entry with out-of-subtree entities.
		admin := doReq(tc, http.MethodGet, q, adminH, nil, nil).Body.String()
		if !strings.Contains(admin, "sf-rOut") {
			t.Fatalf("control invalid: the admin search did not surface out-of-subtree entities, so "+
				"the cache entry holds nothing that could leak, got: %s", admin)
		}

		scoped := doReq(tc, http.MethodGet, q, scopedH, nil, nil).Body.String()
		if strings.Contains(scoped, "sf-rOut") || strings.Contains(scoped, "sf-nOut") ||
			strings.Contains(scoped, "sf-outside") {
			t.Fatalf("a group-limited principal was served another scope's cached results: %s", scoped)
		}
		if !strings.Contains(scoped, "sf-rIn") {
			t.Fatalf("control invalid: the scoped search returned nothing in-subtree, so its lack of "+
				"out-of-subtree entities proves nothing, got: %s", scoped)
		}
	})

	t.Run("scoped write does not poison the entry for an admin", func(t *testing.T) {
		tc := setupAuthEnv(t)
		f := buildScopingFixture(t, tc)
		adminH := map[string]string{"Accept": "application/json", "Authorization": unscopedAdminBearer(t, tc)}
		scopedH := map[string]string{"Accept": "application/json", "Authorization": f.bearer}

		// Scoped first. If its truncated result set were written to the shared
		// cache, the admin below would silently lose the out-of-subtree rows.
		scoped := doReq(tc, http.MethodGet, q, scopedH, nil, nil).Body.String()
		if !strings.Contains(scoped, "sf-rIn") {
			t.Fatalf("control invalid: the scoped search returned nothing, so it could not poison "+
				"anything, got: %s", scoped)
		}

		admin := doReq(tc, http.MethodGet, q, adminH, nil, nil).Body.String()
		for _, want := range []string{"sf-rOut", "sf-nOut", "sf-outside"} {
			if !strings.Contains(admin, want) {
				t.Errorf("admin search is missing %q after a scoped search ran first; the scoped "+
					"results were written to the shared cache, got: %s", want, admin)
			}
		}
	})
}

// unscopedAdminBearer returns a bearer for an admin with no scope group, used as
// the control that a route's 403 comes from the group-limited deny and not from
// the route being missing or broken for everyone.
func unscopedAdminBearer(t *testing.T, tc *TestContext) string {
	t.Helper()
	u, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "unscoped-admin", Password: "password1", Role: models.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	raw, _, err := tc.AppCtx.CreateApiToken(u.ID, "t", nil)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	return "Bearer " + raw
}

// Import is denied outright for group-limited principals, not confined. Import
// creates new top-level groups a scoped principal could not place inside its
// subtree, and the caller-supplied ShellGroupAction/DanglingAction destination
// IDs drive association and GroupRelation writes the GORM scope callbacks do not
// guard. All five import routes are therefore wrapped in denyScopedPrincipal.
//
// This is a boundary to preserve across the groupio extraction, not a feature to
// add: the refactor must not quietly make any import entry point reachable by a
// scoped principal. Enabling scoped import is its own project.
func TestScopedUser_ImportRoutesDenied(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildScopingFixture(t, tc)
	adminBearer := unscopedAdminBearer(t, tc)

	routes := []struct {
		method, path string
	}{
		{http.MethodPost, "/v1/groups/import/parse"},
		{http.MethodGet, "/v1/imports/job-1/plan"},
		{http.MethodDelete, "/v1/imports/job-1"},
		{http.MethodPost, "/v1/imports/job-1/apply"},
		{http.MethodGet, "/v1/imports/job-1/result"},
	}

	for _, rt := range routes {
		scopedH := map[string]string{"Accept": "application/json", "Authorization": f.bearer, "Content-Type": "application/json"}
		resp := doReq(tc, rt.method, rt.path, scopedH, nil, strings.NewReader(`{}`))
		if resp.Code != http.StatusForbidden {
			t.Errorf("%s %s: group-limited principal got %d, want 403 — the import surface must stay fail-closed",
				rt.method, rt.path, resp.Code)
		}

		// Control: the same route reached by an unscoped admin must NOT 403.
		// Without this, a route that 403s for everyone (or a path typo answered
		// uniformly) would satisfy the assertion above while proving nothing
		// about the group-limited deny specifically.
		adminH := map[string]string{"Accept": "application/json", "Authorization": adminBearer, "Content-Type": "application/json"}
		ctrl := doReq(tc, rt.method, rt.path, adminH, nil, strings.NewReader(`{}`))
		if ctrl.Code == http.StatusForbidden {
			t.Errorf("%s %s: control invalid — an unscoped admin also got 403, so the scoped 403 "+
				"does not demonstrate the group-limited deny", rt.method, rt.path)
		}
	}
}

func TestScopedUser_CannotMutateOutsideBlock(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildScopingFixture(t, tc)

	// A block on the out-of-subtree note.
	block := &models.NoteBlock{NoteID: f.nOutID, Type: "text", Position: "a", Content: []byte(`{"text":"secret"}`), State: []byte("{}")}
	if err := tc.DB.Create(block).Error; err != nil {
		t.Fatalf("create block: %v", err)
	}

	h := map[string]string{"Accept": "application/json", "Authorization": f.bearer, "Content-Type": "application/json"}
	resp := doReq(tc, http.MethodPut, "/v1/note/block?id="+itoa(int(block.ID)), h, nil,
		strings.NewReader(`{"content":{"text":"hacked"}}`))
	if resp.Code >= 200 && resp.Code < 300 {
		t.Fatalf("scoped user should not edit a block of an out-of-subtree note, got %d", resp.Code)
	}

	// The block content is unchanged.
	var after models.NoteBlock
	tc.DB.First(&after, block.ID)
	if strings.Contains(string(after.Content), "hacked") {
		t.Fatalf("out-of-subtree block was modified: %s", after.Content)
	}
}

func TestScopedUser_BulkOpRejectsOutsideIDs(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildScopingFixture(t, tc)
	tag := &models.Tag{Name: "sf-tag"}
	tc.DB.Create(tag)

	h := map[string]string{"Accept": "application/json", "Authorization": f.bearer, "Content-Type": "application/x-www-form-urlencoded"}
	resp := doReq(tc, http.MethodPost, "/v1/notes/addTags", h, nil,
		strings.NewReader("ID="+itoa(int(f.nOutID))+"&EditedId="+itoa(int(tag.ID))))
	if resp.Code >= 200 && resp.Code < 300 {
		t.Fatalf("bulk addTags to an out-of-subtree note should fail, got %d", resp.Code)
	}

	// The out-of-subtree note did not get the tag.
	var count int64
	tc.DB.Table("note_tags").Where("note_id = ? AND tag_id = ?", f.nOutID, tag.ID).Count(&count)
	if count != 0 {
		t.Fatalf("out-of-subtree note must not have been tagged")
	}
}

func TestScopedUser_CannotReadAuditLogs(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildScopingFixture(t, tc)
	h := map[string]string{"Accept": "application/json", "Authorization": f.bearer}

	for _, path := range []string{"/v1/logs", "/v1/log?id=1", "/v1/logs/entity?type=note&id=1"} {
		if c := doReq(tc, http.MethodGet, path, h, nil, nil).Code; c != http.StatusForbidden {
			t.Fatalf("scoped user GET %s should be 403, got %d", path, c)
		}
	}
}

func TestExportDownloadOwnership(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildScopingFixture(t, tc)
	adminBearer := roleBearer(t, tc, models.RoleAdmin)
	adminH := map[string]string{"Accept": "application/json", "Authorization": adminBearer, "Content-Type": "application/json"}

	// Admin submits an export job.
	submit := doReq(tc, http.MethodPost, "/v1/groups/export", adminH, nil,
		strings.NewReader(`{"rootGroupIds":[`+itoa(int(f.outsideID))+`]}`))
	if submit.Code != http.StatusAccepted {
		t.Fatalf("admin export submit should be 202, got %d (%s)", submit.Code, submit.Body.String())
	}
	jobID := extractJSONString(submit.Body.String(), "jobId")
	if jobID == "" {
		t.Fatalf("no jobId in submit response: %s", submit.Body.String())
	}

	// A different (scoped) user must not be able to download the admin's archive.
	other := doReq(tc, http.MethodGet, "/v1/exports/"+jobID+"/download",
		map[string]string{"Authorization": f.bearer}, nil, nil)
	if other.Code != http.StatusNotFound {
		t.Fatalf("non-owner export download should be 404, got %d", other.Code)
	}

	// The owning admin passes the ownership check (not a 404).
	owner := doReq(tc, http.MethodGet, "/v1/exports/"+jobID+"/download",
		map[string]string{"Authorization": adminBearer}, nil, nil)
	if owner.Code == http.StatusNotFound {
		t.Fatalf("owner/admin export download should not be 404")
	}
}

func TestScopedUser_WriteOutsideSubtreeRejected(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildScopingFixture(t, tc)
	h := map[string]string{"Accept": "application/json", "Authorization": f.bearer, "Content-Type": "application/json"}

	// Creating a note owned by an in-subtree group succeeds.
	okBody := strings.NewReader(`{"name":"ok-note","ownerId":` + itoa(int(f.childID)) + `}`)
	ok := doReq(tc, http.MethodPost, "/v1/note", h, nil, okBody)
	if ok.Code == http.StatusForbidden || ok.Code >= 500 {
		t.Fatalf("in-subtree note create should succeed, got %d (%s)", ok.Code, ok.Body.String())
	}

	// Creating a note owned by an out-of-subtree group is rejected.
	badBody := strings.NewReader(`{"name":"bad-note","ownerId":` + itoa(int(f.outsideID)) + `}`)
	bad := doReq(tc, http.MethodPost, "/v1/note", h, nil, badBody)
	if bad.Code >= 200 && bad.Code < 300 {
		t.Fatalf("out-of-subtree note create should fail, got %d", bad.Code)
	}
	// And no such note exists in the outside group.
	var count int64
	tc.DB.Model(&models.Note{}).Where("name = ?", "bad-note").Count(&count)
	if count != 0 {
		t.Fatalf("out-of-subtree note should not have been created")
	}
}
