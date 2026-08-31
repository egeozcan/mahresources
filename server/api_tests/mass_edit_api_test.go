package api_tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/contracts"
	"mahresources/models"
)

// massEditFormPost sends a urlencoded request with an explicit Accept header,
// so a test can ask for the browser's text/html answer or the API's JSON one.
func massEditFormPost(t *testing.T, tc *TestContext, method, target string, accept string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req, err := http.NewRequest(method, target, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", accept)
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	return rr
}

// A form post in the bulk-bar shape works and redirects for Accept: text/html.
func TestMassEditResourcesFormPostRedirectsForHTML(t *testing.T) {
	tc := SetupTestEnv(t)
	tag := &models.Tag{Name: "mass-edit-form-tag"}
	tc.DB.Create(tag)
	r1 := tc.CreateResourceWithType(t, "mass-form-1", "text/plain")
	r2 := tc.CreateResourceWithType(t, "mass-form-2", "text/plain")

	form := url.Values{
		"ID":     {fmt.Sprint(r1.ID), fmt.Sprint(r2.ID)},
		"TagsOp": {"add"},
		"TagIds": {fmt.Sprint(tag.ID)},
	}
	rr := massEditFormPost(t, tc, http.MethodPost, "/v1/resources/massEdit", "text/html", form)
	assert.Equal(t, http.StatusSeeOther, rr.Code, "a browser form post should be redirected: %s", rr.Body.String())
	assert.Equal(t, "/resources", rr.Header().Get("Location"))

	var tagRows int64
	tc.DB.Raw("SELECT COUNT(*) FROM resource_tags WHERE tag_id = ?", tag.ID).Scan(&tagRows)
	assert.Equal(t, int64(2), tagRows, "both resources should be tagged")
}

// The same shape over JSON returns the MassEditResult body.
func TestMassEditResourcesJSONPostReturnsResult(t *testing.T) {
	tc := SetupTestEnv(t)
	tag := &models.Tag{Name: "mass-edit-json-tag"}
	tc.DB.Create(tag)
	r1 := tc.CreateResourceWithType(t, "mass-json-1", "text/plain")
	r2 := tc.CreateResourceWithType(t, "mass-json-2", "text/plain")

	resp := tc.MakeRequest(http.MethodPost, "/v1/resources/massEdit", map[string]any{
		"ID":     []uint{r1.ID, r2.ID},
		"TagsOp": "add",
		"TagIds": []uint{tag.ID},
	})
	assert.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var result contracts.MassEditResult
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode MassEditResult: %v (%s)", err, resp.Body.String())
	}
	assert.Equal(t, "resource", result.Entity)
	assert.Equal(t, int64(2), result.Matched)
	assert.Equal(t, int64(2), result.Affected)
	assert.False(t, result.DryRun)
	if len(result.Ops) != 1 || result.Ops[0].Op != "tags.add" || result.Ops[0].RowsAffected != 2 {
		t.Fatalf("unexpected ops: %+v", result.Ops)
	}
}

// Target=filter works end-to-end over the wire, including the ExpectedCount
// handshake and a DryRun probe first.
func TestMassEditFilterTargetEndToEnd(t *testing.T) {
	tc := SetupTestEnv(t)
	tag := &models.Tag{Name: "mass-filter-tag"}
	tc.DB.Create(tag)
	for i := 0; i < 5; i++ {
		tc.CreateResourceWithType(t, fmt.Sprintf("mass-filter-%d", i), "text/plain")
	}
	other := tc.CreateResourceWithType(t, "mass-unrelated", "text/plain")

	// DryRun first, the way the confirmation dialog does.
	dry := tc.MakeRequest(http.MethodPost, "/v1/resources/massEdit", map[string]any{
		"Target": "filter", "Filter": "name=mass-filter-", "DryRun": true,
		"TagsOp": "add", "TagIds": []uint{tag.ID},
	})
	assert.Equal(t, http.StatusOK, dry.Code, dry.Body.String())
	var dryResult contracts.MassEditResult
	if err := json.Unmarshal(dry.Body.Bytes(), &dryResult); err != nil {
		t.Fatalf("decode dry result: %v", err)
	}
	assert.True(t, dryResult.DryRun)
	assert.Equal(t, int64(5), dryResult.Matched)

	// The real edit, with the count the dry run reported.
	resp := tc.MakeRequest(http.MethodPost, "/v1/resources/massEdit", map[string]any{
		"Target": "filter", "Filter": "name=mass-filter-", "ExpectedCount": 5,
		"TagsOp": "add", "TagIds": []uint{tag.ID},
	})
	assert.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var tagRows int64
	tc.DB.Raw("SELECT COUNT(*) FROM resource_tags WHERE tag_id = ?", tag.ID).Scan(&tagRows)
	assert.Equal(t, int64(5), tagRows)
	var otherRows int64
	tc.DB.Raw("SELECT COUNT(*) FROM resource_tags WHERE resource_id = ?", other.ID).Scan(&otherRows)
	assert.Equal(t, int64(0), otherRows, "an entity outside the filter was edited")

	// And a stale count is refused with a conflict, writing nothing.
	stale := tc.MakeRequest(http.MethodPost, "/v1/resources/massEdit", map[string]any{
		"Target": "filter", "Filter": "name=mass-filter-", "ExpectedCount": 4,
		"TagsOp": "remove", "TagIds": []uint{tag.ID},
	})
	assert.Equal(t, http.StatusConflict, stale.Code, stale.Body.String())
}

func TestMassEditNotesFormPostWorks(t *testing.T) {
	tc := SetupTestEnv(t)
	tag := &models.Tag{Name: "mass-note-tag"}
	tc.DB.Create(tag)
	n1 := tc.CreateDummyNote("mass-note-1")
	n2 := tc.CreateDummyNote("mass-note-2")

	form := url.Values{
		"ID":     {fmt.Sprint(n1.ID), fmt.Sprint(n2.ID)},
		"TagsOp": {"add"},
		"TagIds": {fmt.Sprint(tag.ID)},
	}
	rr := massEditFormPost(t, tc, http.MethodPost, "/v1/notes/massEdit", "application/json", form)
	assert.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var result contracts.MassEditResult
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	assert.Equal(t, "note", result.Entity)
}

func TestMassEditGroupsFormPostWorks(t *testing.T) {
	tc := SetupTestEnv(t)
	g1 := tc.CreateDummyGroup("mass-group-1")
	g2 := tc.CreateDummyGroup("mass-group-2")

	form := url.Values{
		"ID":              {fmt.Sprint(g1.ID)},
		"RelatedGroupsOp": {"add"},
		"RelatedGroupIds": {fmt.Sprint(g2.ID)},
	}
	rr := massEditFormPost(t, tc, http.MethodPost, "/v1/groups/massEdit", "application/json", form)
	assert.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var result contracts.MassEditResult
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	assert.Equal(t, "group", result.Entity)
	assert.Equal(t, int64(1), result.Ops[0].RowsAffected)
}

// The typed mass-edit refusals keep their statuses over the wire and do not
// fall through the substring scan in error_status.go.
func TestMassEditTypedErrorStatusCodes(t *testing.T) {
	tc := SetupTestEnv(t)

	// ErrMassEditSetChanged -> 409 (the request must be well formed, so it
	// carries an op; only the count disagrees)
	setChanged := tc.MakeRequest(http.MethodPost, "/v1/resources/massEdit", map[string]any{
		"Target": "filter", "Filter": "name=nothing-matches", "ExpectedCount": 1,
		"TagsOp": "add", "TagIds": []uint{999999},
	})
	assert.Equal(t, http.StatusConflict, setChanged.Code, setChanged.Body.String())

	// ErrMassEditTooLarge -> 400. The request must be well formed (carry an op)
	// so it reaches the ceiling check rather than the op-count refusal.
	tc.AppCtx.Config.MaxMassEditEntities = 1
	for i := 0; i < 2; i++ {
		tc.CreateResourceWithType(t, fmt.Sprintf("mass-cap-%d", i), "text/plain")
	}
	tooLarge := tc.MakeRequest(http.MethodPost, "/v1/resources/massEdit", map[string]any{
		"Target": "filter", "Filter": "name=mass-cap-", "ExpectedCount": 2,
		"TagsOp": "add", "TagIds": []uint{999999},
	})
	assert.Equal(t, http.StatusBadRequest, tooLarge.Code, tooLarge.Body.String())
	assert.Contains(t, tooLarge.Body.String(), "ceiling")
	tc.AppCtx.Config.MaxMassEditEntities = 0

	// An explicit ID list is bounded by the same ceiling: two real ids over a
	// ceiling of one, refused before the transaction opens.
	tc.AppCtx.Config.MaxMassEditEntities = 1
	ids := []uint{}
	tc.DB.Model(&models.Resource{}).Limit(2).Pluck("id", &ids)
	require.Len(t, ids, 2)
	tooManyIDs := tc.MakeRequest(http.MethodPost, "/v1/resources/massEdit", map[string]any{
		"ID": ids, "OwnerOp": "clear",
	})
	assert.Equal(t, http.StatusBadRequest, tooManyIDs.Code, tooManyIDs.Body.String())
	assert.Contains(t, tooManyIDs.Body.String(), "ceiling")
	tc.AppCtx.Config.MaxMassEditEntities = 0

	// Mass edit over an UNFILTERED list is legitimate: the empty filter string
	// is the set the unfiltered list page shows, scoped to the caller.
	allCount := tc.MakeRequest(http.MethodGet, "/v1/resources", nil)
	assert.Equal(t, http.StatusOK, allCount.Code)
	dryAll := tc.MakeRequest(http.MethodPost, "/v1/resources/massEdit", map[string]any{
		"Target": "filter", "Filter": "", "DryRun": true,
		"TagsOp": "add", "TagIds": []uint{999999},
	})
	assert.Equal(t, http.StatusOK, dryAll.Code, dryAll.Body.String())
	var dryAllResult contracts.MassEditResult
	require.NoError(t, json.Unmarshal(dryAll.Body.Bytes(), &dryAllResult))
	assert.Positive(t, dryAllResult.Matched)

	// ErrMassEditOwnerClearScoped -> 403. A scoped principal needs an account
	// and a session; exercise the context method's mapping through the
	// role-capability error path instead by checking the message mapping
	// directly is covered in the unit tests, and here assert that an
	// anonymous clear of an ownerless set is simply refused as not-found.
	r := tc.CreateResourceWithType(t, "mass-clear-403", "text/plain")
	missing := tc.MakeRequest(http.MethodPost, "/v1/resources/massEdit", map[string]any{
		"ID": []uint{r.ID + 999}, "OwnerOp": "clear",
	})
	assert.Equal(t, http.StatusNotFound, missing.Code, missing.Body.String())

	// ErrMassEditOwnershipCycle -> 409, over the wire on groups.
	parent := tc.CreateDummyGroup("mass-cycle-parent")
	child := tc.CreateDummyGroup("mass-cycle-child")
	if err := tc.DB.Model(&models.Group{}).Where("id = ?", child.ID).Update("owner_id", parent.ID).Error; err != nil {
		t.Fatalf("set child owner: %v", err)
	}
	cycle := tc.MakeRequest(http.MethodPost, "/v1/groups/massEdit", map[string]any{
		"ID": []uint{parent.ID}, "OwnerOp": "set", "OwnerId": child.ID,
	})
	assert.Equal(t, http.StatusConflict, cycle.Code, cycle.Body.String())
}

// CSRF: the new routes are state-changing and cookie-authenticated, so a
// session request without a token is rejected, following csrf_test.go.
func TestMassEditRoutesRequireCSRF(t *testing.T) {
	tc := setupAuthEnv(t)
	cookie, token := loginCookieAndCSRF(t, tc)

	r := tc.CreateResourceWithType(t, "mass-csrf", "text/plain")
	form := url.Values{"ID": {fmt.Sprint(r.ID)}, "OwnerOp": {"clear"}}

	rr := massEditFormPost(t, tc, http.MethodPost, "/v1/resources/massEdit", "application/json", form, cookie)
	assert.Equal(t, http.StatusForbidden, rr.Code, "a cookie POST without a CSRF token must be 403")
	if !strings.Contains(rr.Body.String(), "CSRF") {
		t.Fatalf("403 body should mention CSRF, got: %s", rr.Body.String())
	}

	form.Set("csrf_token", token)
	rr = massEditFormPost(t, tc, http.MethodPost, "/v1/resources/massEdit", "application/json", form, cookie)
	assert.NotEqual(t, http.StatusForbidden, rr.Code, "the form-field token should be accepted: %s", body(t, rr))
}

// loginCookieAndCSRFToken logs in and returns only the CSRF token; the session
// cookie stays inside loginCookieAndCSRF because cookie jars are the caller's
// business in these tests.
func loginCookieAndCSRFToken(t *testing.T, tc *TestContext) string {
	t.Helper()
	_, token := loginCookieAndCSRF(t, tc)
	return token
}

// A group-limited principal may not clear owners over the wire: an owner-less
// row is in nobody's subtree, so the eviction is permanent from their point of
// view. Typed ErrMassEditOwnerClearScoped, mapped before the substring scan.
func TestMassEditOwnerClearScopedIs403OverHTTP(t *testing.T) {
	tc := SetupTestEnv(t)
	tc.AppCtx.Config.AuthEnabled = true

	scope := &models.Group{Name: "mass-clear-scope"}
	require.NoError(t, tc.DB.Create(scope).Error)
	r := tc.CreateResourceWithType(t, "mass-clear-scoped", "text/plain")
	if err := tc.DB.Model(&models.Resource{}).Where("id = ?", r.ID).Update("owner_id", scope.ID).Error; err != nil {
		t.Fatalf("own resource: %v", err)
	}

	user, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username:     "mass-clear-user",
		Password:     "password1",
		Role:         models.RoleUser,
		ScopeGroupId: &scope.ID,
	})
	require.NoError(t, err)
	rawToken, _, err := tc.AppCtx.CreateApiToken(user.ID, "mass-clear-token", nil)
	require.NoError(t, err)

	headers := map[string]string{
		"Accept":        "application/json",
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + rawToken,
	}
	rr := doReq(tc, http.MethodPost, "/v1/resources/massEdit", headers, nil,
		strings.NewReader(fmt.Sprintf(`{"ID": [%d], "OwnerOp": "clear"}`, r.ID)))
	assert.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())

	var count int64
	tc.DB.Model(&models.Resource{}).Where("id = ? AND owner_id IS NOT NULL", r.ID).Count(&count)
	assert.Equal(t, int64(1), count, "a refused owner clear changed the owner")
}

// ExpectedCount=0 is a legitimate confirmed count (an empty set over the empty
// filter), distinguishable from "omitted" via the pointer decode. Omitted — or
// JSON null — still refuses a non-dry-run filter request.
func TestMassEditExpectedCountZeroAndOmitted(t *testing.T) {
	tc := SetupTestEnv(t)
	tag := &models.Tag{Name: "mass-zero-count-tag"}
	require.NoError(t, tc.DB.Create(tag).Error)

	// urlencoded form field "0" must decode as a present zero.
	form := url.Values{
		"Target":        {"filter"},
		"Filter":        {"name=nothing-matches-at-all"},
		"ExpectedCount": {"0"},
		"TagsOp":        {"add"},
		"TagIds":        {fmt.Sprint(tag.ID)},
	}
	rr := massEditFormPost(t, tc, http.MethodPost, "/v1/resources/massEdit", "application/json", form)
	assert.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var result contracts.MassEditResult
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &result))
	assert.Equal(t, int64(0), result.Matched)
	assert.Equal(t, int64(0), result.Affected)

	// JSON 0: a present zero is accepted.
	jsonZero := tc.MakeRequest(http.MethodPost, "/v1/resources/massEdit", map[string]any{
		"Target": "filter", "Filter": "name=nothing-matches-at-all", "ExpectedCount": 0,
		"TagsOp": "add", "TagIds": []uint{tag.ID},
	})
	assert.Equal(t, http.StatusOK, jsonZero.Code, jsonZero.Body.String())
	var zeroResult contracts.MassEditResult
	require.NoError(t, json.Unmarshal(jsonZero.Body.Bytes(), &zeroResult))
	assert.Equal(t, int64(0), zeroResult.Matched)

	// JSON null: indistinguishable from omitted, refused.
	jsonNull := tc.MakeRequest(http.MethodPost, "/v1/resources/massEdit", map[string]any{
		"Target": "filter", "Filter": "name=whatever", "ExpectedCount": nil,
		"TagsOp": "add", "TagIds": []uint{tag.ID},
	})
	assert.Equal(t, http.StatusBadRequest, jsonNull.Code, jsonNull.Body.String())
	assert.Contains(t, jsonNull.Body.String(), "expected count is required")

	// Omitted: refused.
	omitted := tc.MakeRequest(http.MethodPost, "/v1/resources/massEdit", map[string]any{
		"Target": "filter", "Filter": "name=whatever", "TagsOp": "add", "TagIds": []uint{tag.ID},
	})
	assert.Equal(t, http.StatusBadRequest, omitted.Code, omitted.Body.String())
	assert.Contains(t, omitted.Body.String(), "expected count is required")
}

func body(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	b, err := io.ReadAll(rr.Body)
	if err != nil {
		return ""
	}
	return string(b)
}
