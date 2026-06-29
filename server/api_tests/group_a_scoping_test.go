package api_tests

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/download_queue"
	"mahresources/models"
	"mahresources/models/types"
	"mahresources/plugin_system"
	"mahresources/server/api_handlers"
)

// uptr returns a pointer to a uint literal.
func uptr(u uint) *uint { return &u }

// plainUserBearer creates an unscoped (non-admin) user and returns its bearer
// header plus its user ID.
func plainUserBearer(t *testing.T, tc *TestContext, username string) (string, uint) {
	t.Helper()
	u, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: username, Password: "password1", Role: models.RoleUser,
	})
	if err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	raw, _, err := tc.AppCtx.CreateApiToken(u.ID, "t", nil)
	if err != nil {
		t.Fatalf("token for %s: %v", username, err)
	}
	return "Bearer " + raw, u.ID
}

// Gap 4: the SQLite meta-keys query uses a multi-table FROM clause that the GORM
// scope callback can't match, so a scoped principal must be filtered explicitly.
func TestScopedUser_MetaKeysConfined(t *testing.T) {
	tc := setupAuthEnv(t)

	root := &models.Group{Name: "mk-root"}
	tc.DB.Create(root)
	child := &models.Group{Name: "mk-child", OwnerId: &root.ID}
	tc.DB.Create(child)
	outside := &models.Group{Name: "mk-outside"}
	tc.DB.Create(outside)

	tc.DB.Create(&models.Note{Name: "mk-nIn", OwnerId: &child.ID, Meta: types.JSON(`{"inkey":1}`)})
	tc.DB.Create(&models.Note{Name: "mk-nOut", OwnerId: &outside.ID, Meta: types.JSON(`{"outkey":1}`)})

	bearer := scopedUserBearer(t, tc, root.ID)
	h := map[string]string{"Accept": "application/json", "Authorization": bearer}

	body := doReq(tc, http.MethodGet, "/v1/notes/meta/keys", h, nil, nil).Body.String()
	if !strings.Contains(body, "inkey") {
		t.Fatalf("scoped meta-keys should include in-subtree key 'inkey', got: %s", body)
	}
	if strings.Contains(body, "outkey") {
		t.Fatalf("scoped meta-keys must NOT include out-of-subtree key 'outkey', got: %s", body)
	}

	// An admin still sees every key.
	adminBearer := roleBearer(t, tc, models.RoleAdmin)
	adminBody := doReq(tc, http.MethodGet, "/v1/notes/meta/keys",
		map[string]string{"Accept": "application/json", "Authorization": adminBearer}, nil, nil).Body.String()
	if !strings.Contains(adminBody, "outkey") || !strings.Contains(adminBody, "inkey") {
		t.Fatalf("admin meta-keys should include every key, got: %s", adminBody)
	}
}

// Gap 1: background jobs are per-user. A non-admin only sees the jobs it created;
// admins see all.
func TestJobVisibilityByOwner(t *testing.T) {
	tc := setupAuthEnv(t)
	aBearer, aID := plainUserBearer(t, tc, "job-user-a")
	bBearer, _ := plainUserBearer(t, tc, "job-user-b")
	adminBearer := roleBearer(t, tc, models.RoleAdmin)

	job, err := tc.AppCtx.DownloadManager().SubmitJob("test", "queued",
		func(ctx context.Context, j *download_queue.DownloadJob, sink download_queue.ProgressSink) error {
			return nil
		})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}
	job.SetOwnerUserID(aID)
	jobID := job.ID

	qa := doReq(tc, http.MethodGet, "/v1/jobs/queue", map[string]string{"Accept": "application/json", "Authorization": aBearer}, nil, nil).Body.String()
	if !strings.Contains(qa, jobID) {
		t.Fatalf("owner A should see its job in the queue, got: %s", qa)
	}
	qb := doReq(tc, http.MethodGet, "/v1/jobs/queue", map[string]string{"Accept": "application/json", "Authorization": bBearer}, nil, nil).Body.String()
	if strings.Contains(qb, jobID) {
		t.Fatalf("user B must not see A's job in the queue, got: %s", qb)
	}
	qadm := doReq(tc, http.MethodGet, "/v1/jobs/queue", map[string]string{"Accept": "application/json", "Authorization": adminBearer}, nil, nil).Body.String()
	if !strings.Contains(qadm, jobID) {
		t.Fatalf("admin should see every job in the queue, got: %s", qadm)
	}

	// Single-job fetch: owner 200, non-owner 404 (not 403, to prevent ID enumeration).
	if c := doReq(tc, http.MethodGet, "/v1/jobs/get?id="+jobID, map[string]string{"Authorization": aBearer}, nil, nil).Code; c != http.StatusOK {
		t.Fatalf("owner GET /v1/jobs/get should be 200, got %d", c)
	}
	if c := doReq(tc, http.MethodGet, "/v1/jobs/get?id="+jobID, map[string]string{"Authorization": bBearer}, nil, nil).Code; c != http.StatusNotFound {
		t.Fatalf("non-owner GET /v1/jobs/get should be 404, got %d", c)
	}
}

// Gap 3: group import has no subtree-confined semantics (it creates new top-level
// groups), so the whole import surface is denied to group-limited principals.
func TestScopedUser_ImportDenied(t *testing.T) {
	tc := setupAuthEnv(t)
	g := &models.Group{Name: "imp-root"}
	tc.DB.Create(g)
	scopedBearer := scopedUserBearer(t, tc, g.ID)
	adminBearer := roleBearer(t, tc, models.RoleAdmin)

	scopedH := map[string]string{"Accept": "application/json", "Authorization": scopedBearer, "Content-Type": "application/json"}

	cases := []struct {
		method, path string
	}{
		{http.MethodPost, "/v1/groups/import/parse"},
		{http.MethodPost, "/v1/imports/abc/apply"},
		{http.MethodGet, "/v1/imports/abc/plan"},
		{http.MethodGet, "/v1/imports/abc/result"},
		{http.MethodDelete, "/v1/imports/abc"},
	}
	for _, c := range cases {
		if code := doReq(tc, c.method, c.path, scopedH, nil, strings.NewReader("{}")).Code; code != http.StatusForbidden {
			t.Fatalf("scoped %s %s should be 403, got %d", c.method, c.path, code)
		}
	}

	// An admin passes the scope guard (reaches the handler, which 4xx's on the
	// empty body — but is never 403 from the guard).
	if code := doReq(tc, http.MethodPost, "/v1/groups/import/parse",
		map[string]string{"Authorization": adminBearer, "Content-Type": "application/json"}, nil, strings.NewReader("{}")).Code; code == http.StatusForbidden {
		t.Fatalf("admin import should not be blocked by the scope guard, got 403")
	}
}

// scopeRunner is a PluginActionRunner whose entity visibility is controlled by a
// map, simulating subtree confinement without a real scoped DB.
type scopeRunner struct {
	pm      *plugin_system.PluginManager
	reader  plugin_system.EntityRefReader
	visible map[uint]bool
}

func (r *scopeRunner) PluginManager() *plugin_system.PluginManager          { return r.pm }
func (r *scopeRunner) ActionEntityRefReader() plugin_system.EntityRefReader { return r.reader }
func (r *scopeRunner) ResourceVisible(id uint) bool                         { return r.visible[id] }
func (r *scopeRunner) NoteVisible(id uint) bool                             { return r.visible[id] }
func (r *scopeRunner) GroupVisible(id uint) bool                            { return r.visible[id] }

// Gap 2: a group-limited principal may only run a plugin action on entities
// inside its subtree.
func TestScopedUser_ActionRunConfinedToSubtree(t *testing.T) {
	tc := SetupTestEnv(t)
	pm := enableTestPluginWithEntityRef(t, t.TempDir()) // action "act", entity "resource"

	inRes := tc.CreateResourceWithType(t, "in-res", "image/png")
	runner := &scopeRunner{
		pm:      pm,
		reader:  tc.AppCtx.ActionEntityRefReader(),
		visible: map[uint]bool{inRes.ID: true},
	}
	handler := api_handlers.GetActionRunHandler(runner)
	scoped := &auth.Principal{UserID: 7, Role: models.RoleUser, ScopeGroupID: uptr(1)}

	// Targeting an out-of-subtree resource is rejected before the action runs.
	outBody := fmt.Sprintf(`{"plugin":"ref-plugin","action":"act","entity_ids":[%d],"params":{}}`, inRes.ID+1)
	outReq := httptest.NewRequest(http.MethodPost, "/v1/jobs/action/run", strings.NewReader(outBody))
	outReq.Header.Set("Content-Type", "application/json")
	outReq = outReq.WithContext(auth.WithPrincipal(outReq.Context(), scoped))
	outRR := httptest.NewRecorder()
	handler(outRR, outReq)
	if outRR.Code != http.StatusForbidden {
		t.Fatalf("scoped action on out-of-subtree entity should be 403, got %d body=%s", outRR.Code, outRR.Body.String())
	}

	// Targeting an in-subtree resource passes the scope gate.
	inBody := fmt.Sprintf(`{"plugin":"ref-plugin","action":"act","entity_ids":[%d],"params":{"extras":[%d]}}`, inRes.ID, inRes.ID)
	inReq := httptest.NewRequest(http.MethodPost, "/v1/jobs/action/run", strings.NewReader(inBody))
	inReq.Header.Set("Content-Type", "application/json")
	inReq = inReq.WithContext(auth.WithPrincipal(inReq.Context(), scoped))
	inRR := httptest.NewRecorder()
	handler(inRR, inReq)
	if inRR.Code == http.StatusForbidden {
		t.Fatalf("scoped action on in-subtree entity should pass the gate, got 403 body=%s", inRR.Body.String())
	}
}
