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

	search := doReq(tc, http.MethodGet, "/v1/search?query=sf-", h, nil, nil).Body.String()
	if strings.Contains(search, "sf-rOut") || strings.Contains(search, "sf-nOut") {
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
