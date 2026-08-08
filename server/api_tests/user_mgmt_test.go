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
)

func TestUserAdminAPI(t *testing.T) {
	tc := setupAuthEnv(t)
	admin := roleBearer(t, tc, models.RoleAdmin)
	adminH := map[string]string{"Accept": "application/json", "Authorization": admin, "Content-Type": "application/json"}

	// Create a user.
	create := doReq(tc, http.MethodPost, "/v1/users", adminH, nil,
		strings.NewReader(`{"username":"newuser","password":"password1","role":"editor"}`))
	if create.Code != http.StatusOK {
		t.Fatalf("admin create user should be 200, got %d (%s)", create.Code, create.Body.String())
	}
	if strings.Contains(create.Body.String(), "passwordHash") || strings.Contains(create.Body.String(), "\"password1\"") {
		t.Fatalf("create response must not leak the password/hash: %s", create.Body.String())
	}

	// List users includes it.
	list := doReq(tc, http.MethodGet, "/v1/users", adminH, nil, nil).Body.String()
	if !strings.Contains(list, "newuser") {
		t.Fatalf("user list should contain newuser, got: %s", list)
	}

	// Duplicate username conflicts.
	dup := doReq(tc, http.MethodPost, "/v1/users", adminH, nil,
		strings.NewReader(`{"username":"newuser","password":"password1","role":"user"}`))
	if dup.Code != http.StatusConflict {
		t.Fatalf("duplicate username should be 409, got %d", dup.Code)
	}

	// Guest without a scope group is rejected (400).
	badGuest := doReq(tc, http.MethodPost, "/v1/users", adminH, nil,
		strings.NewReader(`{"username":"g","password":"pw","role":"guest"}`))
	if badGuest.Code != http.StatusBadRequest {
		t.Fatalf("guest without scope group should be 400, got %d", badGuest.Code)
	}
}

func TestUserAdminAPIResponseUsesLiveUserJSONShape(t *testing.T) {
	tc := setupAuthEnv(t)
	admin := roleBearer(t, tc, models.RoleAdmin)
	response := doReq(tc, http.MethodPost, "/v1/users", map[string]string{
		"Accept": "application/json", "Authorization": admin, "Content-Type": "application/json",
	}, nil, strings.NewReader(`{"username":"response-shape","password":"password1","role":"editor"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("create user status=%d body=%s", response.Code, response.Body.String())
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if body["ID"] == nil {
		t.Fatalf("live user response must contain uppercase ID: %s", response.Body.String())
	}
	if body["id"] != nil {
		t.Fatalf("live user response unexpectedly contains lowercase id: %s", response.Body.String())
	}
	for _, field := range []string{"username", "displayName", "role", "disabled"} {
		if body[field] == nil {
			t.Errorf("live user response missing %s: %s", field, response.Body.String())
		}
	}
	for _, secret := range []string{"password", "passwordHash"} {
		if body[secret] != nil {
			t.Errorf("live user response leaks %s: %s", secret, response.Body.String())
		}
	}
}

func TestUserAdminAPI_NonAdminForbidden(t *testing.T) {
	tc := setupAuthEnv(t)
	editor := roleBearer(t, tc, models.RoleEditor)
	h := map[string]string{"Accept": "application/json", "Authorization": editor, "Content-Type": "application/json"}

	resp := doReq(tc, http.MethodPost, "/v1/users", h, nil,
		strings.NewReader(`{"username":"x","password":"pw","role":"user"}`))
	if resp.Code != http.StatusForbidden {
		t.Fatalf("non-admin creating a user should be 403, got %d", resp.Code)
	}
	if list := doReq(tc, http.MethodGet, "/v1/users", h, nil, nil); list.Code != http.StatusForbidden {
		t.Fatalf("non-admin listing users should be 403, got %d", list.Code)
	}
}

func TestAccountSelfService(t *testing.T) {
	tc := setupAuthEnv(t)
	// Create an editor and authenticate as them via a session cookie.
	if _, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "selfuser", Password: "origpw12", Role: models.RoleEditor,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	login := doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json"}, nil,
		strings.NewReader(`{"username":"selfuser","password":"origpw12"}`))
	cookie := sessionCookie(t, login)
	jsonCookie := []*http.Cookie{cookie}
	// Cookie-authenticated state-changing requests must carry the CSRF token, as
	// a real browser does (token published in the page meta tag / echoed by JS).
	csrf := csrfFor(t, tc, cookie)
	h := map[string]string{"Accept": "application/json", "Content-Type": "application/json", "X-CSRF-Token": csrf}

	// Mint an API token for self.
	mint := doReq(tc, http.MethodPost, "/v1/account/tokens", h, jsonCookie, strings.NewReader(`{"name":"cli"}`))
	if mint.Code != http.StatusOK {
		t.Fatalf("mint token should be 200, got %d (%s)", mint.Code, mint.Body.String())
	}
	if !strings.Contains(mint.Body.String(), `"token"`) {
		t.Fatalf("mint response should contain the raw token once: %s", mint.Body.String())
	}

	// The minted token authenticates API calls.
	rawToken := extractJSONString(mint.Body.String(), "token")
	withTok := doReq(tc, http.MethodGet, "/v1/notes",
		map[string]string{"Accept": "application/json", "Authorization": "Bearer " + rawToken}, nil, nil)
	if withTok.Code != http.StatusOK {
		t.Fatalf("minted token should authenticate, got %d", withTok.Code)
	}

	// Change own password (wrong current → 401).
	wrong := doReq(tc, http.MethodPost, "/v1/account/password", h, jsonCookie,
		strings.NewReader(`{"currentPassword":"nope","newPassword":"newpw123"}`))
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("wrong current password should be 401, got %d", wrong.Code)
	}
	ok := doReq(tc, http.MethodPost, "/v1/account/password", h, jsonCookie,
		strings.NewReader(`{"currentPassword":"origpw12","newPassword":"newpw123"}`))
	if ok.Code != http.StatusOK {
		t.Fatalf("password change should be 200, got %d (%s)", ok.Code, ok.Body.String())
	}
	// New password authenticates; old does not.
	if _, err := tc.AppCtx.AuthenticateUser("selfuser", "newpw123"); err != nil {
		t.Fatalf("new password should work: %v", err)
	}
	if _, err := tc.AppCtx.AuthenticateUser("selfuser", "origpw12"); err == nil {
		t.Fatalf("old password should no longer work")
	}
}

func TestAdminUserCreateRoleRequiresExplicitSelection(t *testing.T) {
	tc := SetupTestEnv(t)
	body := tc.requestWithAccept(http.MethodGet, "/admin/users", browserAccept, "").Body.String()

	if !strings.Contains(body, `<select id="u-role" name="role" required`) {
		t.Fatalf("create role select must be required")
	}
	if !strings.Contains(body, `<option value="" disabled selected>Select a role</option>`) {
		t.Fatalf("create role select must start with a disabled selected placeholder")
	}
	for _, role := range models.ValidRoles {
		if strings.Contains(body, fmt.Sprintf(`value="%s" selected`, role)) {
			t.Errorf("role %q must not be selected on first load", role)
		}
	}
}

func TestAdminUserDisabledReplayUsesSubmittedIntent(t *testing.T) {
	tc := setupAuthEnv(t)
	login := doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json"}, nil,
		strings.NewReader(`{"username":"admin","password":"adminpw1"}`))
	cookie := sessionCookie(t, login)
	headers := map[string]string{
		"Accept": "text/html", "Content-Type": "application/x-www-form-urlencoded",
		"X-CSRF-Token": csrfFor(t, tc, cookie),
	}

	admin, err := tc.AppCtx.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("load admin: %v", err)
	}

	// Stored enabled, submitted checked. Demoting the last admin forces replay.
	enableToDisabled := url.Values{
		"id": {fmt.Sprint(admin.ID)}, "username": {admin.Username},
		"role": {string(models.RoleEditor)}, "disabled": {"true"}, "disabledPresent": {"true"},
	}
	res := doReq(tc, http.MethodPost, "/v1/user", headers, []*http.Cookie{cookie}, strings.NewReader(enableToDisabled.Encode()))
	page := doReq(tc, http.MethodGet, res.Header().Get("Location"), map[string]string{"Accept": "text/html"}, []*http.Cookie{cookie}, nil)
	if !strings.Contains(page.Body.String(), `id="ue-disabled" name="disabled" type="checkbox" value="true"`) ||
		!strings.Contains(page.Body.String(), `data-disabled-replayed="checked"`) {
		t.Fatalf("enabled→checked rejected save did not replay checked intent")
	}

	// Stored disabled, submitted unchecked. A duplicate username forces replay.
	target, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "disabled-replay", Password: "password1", Role: models.RoleEditor, Disabled: true,
	})
	if err != nil {
		t.Fatalf("create disabled target: %v", err)
	}
	disabledToEnabled := url.Values{
		"id": {fmt.Sprint(target.ID)}, "username": {admin.Username},
		"role": {string(models.RoleEditor)}, "disabledPresent": {"true"},
	}
	res = doReq(tc, http.MethodPost, "/v1/user", headers, []*http.Cookie{cookie}, strings.NewReader(disabledToEnabled.Encode()))
	page = doReq(tc, http.MethodGet, res.Header().Get("Location"), map[string]string{"Accept": "text/html"}, []*http.Cookie{cookie}, nil)
	if !strings.Contains(page.Body.String(), `data-disabled-replayed="unchecked"`) {
		t.Fatalf("disabled→unchecked rejected save did not replay unchecked intent")
	}
}

func TestAdminUserRejectedEditPreservesExplicitEmptyFields(t *testing.T) {
	tc := setupAuthEnv(t)
	login := doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json"}, nil,
		strings.NewReader(`{"username":"admin","password":"adminpw1"}`))
	cookie := sessionCookie(t, login)
	target, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "empty-replay", DisplayName: "Stored Name", Password: "password1", Role: models.RoleUser,
	})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	form := url.Values{
		"id": {fmt.Sprint(target.ID)}, "username": {"admin"}, "displayName": {""},
		"role": {string(models.RoleUser)}, "scopeGroupId": {""}, "disabledPresent": {"true"},
	}
	res := doReq(tc, http.MethodPost, "/v1/user", map[string]string{
		"Accept": "text/html", "Content-Type": "application/x-www-form-urlencoded",
		"X-CSRF-Token": csrfFor(t, tc, cookie),
	}, []*http.Cookie{cookie}, strings.NewReader(form.Encode()))
	if res.Code != http.StatusFound {
		t.Fatalf("rejected edit status=%d body=%s", res.Code, res.Body.String())
	}
	page := doReq(tc, http.MethodGet, res.Header().Get("Location"), map[string]string{"Accept": "text/html"}, []*http.Cookie{cookie}, nil)
	body := page.Body.String()
	for field, marker := range map[string]string{
		"displayName":  `id="ue-display" name="displayName" type="text"`,
		"scopeGroupId": `id="ue-scope" name="scopeGroupId" type="number" min="1"`,
	} {
		start := strings.Index(body, marker)
		if start < 0 || !strings.Contains(body[start:min(start+250, len(body))], `value=""`) {
			t.Fatalf("explicit empty %s was replaced by stored value", field)
		}
	}
}

func TestAdminUserPasswordPolicyIsExactAndSecretFree(t *testing.T) {
	tc := SetupTestEnv(t)
	body := tc.requestWithAccept(http.MethodGet, "/admin/users", browserAccept, "").Body.String()
	if !strings.Contains(body, "8 Unicode code points") || !strings.Contains(body, "72 UTF-8 bytes") {
		t.Fatalf("password help must state both exact policy units")
	}
	if strings.Contains(body, "adminpw1") || strings.Contains(body, "passwordHash") {
		t.Fatalf("password policy help leaked credential material")
	}
}

func TestAdminAndAccountPagesRender(t *testing.T) {
	tc := setupAuthEnv(t)
	login := doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json"}, nil,
		strings.NewReader(`{"username":"admin","password":"adminpw1"}`))
	cookie := sessionCookie(t, login)
	htmlH := map[string]string{"Accept": "text/html"}

	users := doReq(tc, http.MethodGet, "/admin/users", htmlH, []*http.Cookie{cookie}, nil)
	if users.Code != http.StatusOK {
		t.Fatalf("/admin/users should render for admin, got %d", users.Code)
	}
	if !strings.Contains(users.Body.String(), "Create user") {
		t.Fatalf("/admin/users should show the create form")
	}

	account := doReq(tc, http.MethodGet, "/account", htmlH, []*http.Cookie{cookie}, nil)
	if account.Code != http.StatusOK {
		t.Fatalf("/account should render, got %d", account.Code)
	}
	if !strings.Contains(account.Body.String(), "API tokens") {
		t.Fatalf("/account should show the API tokens section")
	}
}

// extractJSONString pulls a top-level string field value out of a small JSON
// object body (test helper, not a general parser).
func extractJSONString(body, key string) string {
	marker := `"` + key + `":"`
	i := strings.Index(body, marker)
	if i < 0 {
		return ""
	}
	rest := body[i+len(marker):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// Product decision 107: the user edit page.
//
// Before it, /admin/users offered exactly one per-row action — Delete — so
// changing a role, resetting a password or disabling an account all meant
// destroying it first, which also destroys its tokens and sessions and nulls
// CreatedByUserId across fifteen tables.
func TestAdminUserEditPage_RendersPrefilledForTheAdmin(t *testing.T) {
	tc := setupAuthEnv(t)
	login := doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json"}, nil,
		strings.NewReader(`{"username":"admin","password":"adminpw1"}`))
	cookie := sessionCookie(t, login)
	htmlH := map[string]string{"Accept": "text/html"}

	admin, err := tc.AppCtx.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("load the admin: %v", err)
	}

	// The list page offers the way in.
	list := doReq(tc, http.MethodGet, "/admin/users", htmlH, []*http.Cookie{cookie}, nil)
	if list.Code != http.StatusOK {
		t.Fatalf("/admin/users should render for admin, got %d", list.Code)
	}
	editHref := fmt.Sprintf(`/admin/users/edit?id=%d`, admin.ID)
	if !strings.Contains(list.Body.String(), editHref) {
		t.Errorf("decision 107: /admin/users has no link to %s, so the only per-row action is still Delete", editHref)
	}

	page := doReq(tc, http.MethodGet, editHref, htmlH, []*http.Cookie{cookie}, nil)
	if page.Code != http.StatusOK {
		t.Fatalf("%s should render for admin, got %d", editHref, page.Code)
	}
	body := page.Body.String()

	// Prefilled so the full edit form submits the intended account state. The
	// explicit marker distinguishes its unchecked Disabled box from a partial
	// form client that omitted the property and expects it preserved.
	for _, want := range []string{
		`name="id" value=` + fmt.Sprintf(`"%d"`, admin.ID),
		`value="admin"`,
		`name="role"`,
		`name="disabled"`,
		`name="disabledPresent" value="true"`,
		`name="scopeGroupId"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the edit form is missing %s.\nbody: %s", want, body)
		}
	}
	// The password field exists but is not required: blank means unchanged.
	if !strings.Contains(body, `id="ue-password"`) {
		t.Errorf("the edit form has no password field, so resetting a password still needs a delete")
	}
	if strings.Contains(body, `id="ue-password" name="password" type="password" required`) {
		t.Errorf("the password field is required, but UpdateUser treats blank as unchanged — requiring it forces a reset on every save")
	}
	// Finding 129: an edit form with no way out.
	if !strings.Contains(body, `data-testid="form-cancel"`) {
		t.Errorf("finding 129: the edit form has no Cancel. formCancelURL only answers for two-segment paths, so this three-segment one has to set cancelUrl itself")
	}
}

// The ErrLastAdmin 409 has to *surface*, not look like a success and not throw the
// admin at a full-page error that discards what they typed.
func TestAdminUserEdit_LastAdminConflictComesBackToTheForm(t *testing.T) {
	tc := setupAuthEnv(t)
	login := doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json"}, nil,
		strings.NewReader(`{"username":"admin","password":"adminpw1"}`))
	cookie := sessionCookie(t, login)

	admin, err := tc.AppCtx.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("load the admin: %v", err)
	}

	form := url.Values{}
	form.Set("id", fmt.Sprintf("%d", admin.ID))
	form.Set("disabledPresent", "true")
	form.Set("username", "admin")
	form.Set("displayName", "Kept Value")
	form.Set("password", "supersecret1")
	form.Set("role", string(models.RoleEditor)) // the demotion the invariant forbids

	res := doReq(tc, http.MethodPost, "/v1/user", map[string]string{
		"Accept":       "text/html",
		"Content-Type": "application/x-www-form-urlencoded",
		"X-CSRF-Token": csrfFor(t, tc, cookie),
	}, []*http.Cookie{cookie}, strings.NewReader(form.Encode()))

	if res.Code != http.StatusFound {
		t.Fatalf("a rejected HTML form save should redirect back to the form, got %d. HandleError renders a full-page error document and loses every typed value.\nbody: %s",
			res.Code, res.Body.String())
	}
	loc := res.Header().Get("Location")
	if !strings.HasPrefix(loc, "/admin/users/edit?id=") {
		t.Errorf("the rejection should come back to the edit form, got Location %q", loc)
	}
	if !strings.Contains(loc, "error=") {
		t.Errorf("the redirect carries no error param, so the page has nothing to render: %q", loc)
	}
	if !strings.Contains(loc, "cannot+remove+the+last+enabled+administrator") {
		t.Errorf("the redirect should say what was refused, got %q", loc)
	}
	if !strings.Contains(loc, "Kept+Value") {
		t.Errorf("the typed display name was not carried back, so the admin has to retype the form: %q", loc)
	}
	if strings.Contains(loc, "supersecret1") {
		t.Errorf("the password was echoed into the URL: %q", loc)
	}
	// Only the id, once.
	if strings.Count(loc, "id=") != 1 {
		t.Errorf("the id appears more than once in %q", loc)
	}

	// The account is unchanged, so the conflict really was a refusal.
	after, err := tc.AppCtx.GetUser(admin.ID)
	if err != nil {
		t.Fatalf("reload the admin: %v", err)
	}
	if after.Role != models.RoleAdmin {
		t.Errorf("the sole admin was demoted to %q despite the guard", after.Role)
	}

	// The control: the JSON contract still answers 409 rather than redirecting.
	js := doReq(tc, http.MethodPost, "/v1/user", map[string]string{
		"Accept":       "application/json",
		"Content-Type": "application/json",
		"X-CSRF-Token": csrfFor(t, tc, cookie),
	}, []*http.Cookie{cookie},
		strings.NewReader(fmt.Sprintf(`{"id":%d,"username":"admin","role":"editor"}`, admin.ID)))
	if js.Code != http.StatusConflict {
		t.Errorf("the JSON path should still be 409, got %d: %s", js.Code, js.Body.String())
	}
}

// A save the invariant allows must actually go through, or the tests above would
// pass against a page that can never save anything.
func TestAdminUserEdit_AnAllowedSaveGoesThrough(t *testing.T) {
	tc := setupAuthEnv(t)
	login := doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json"}, nil,
		strings.NewReader(`{"username":"admin","password":"adminpw1"}`))
	cookie := sessionCookie(t, login)

	admin, err := tc.AppCtx.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("load the admin: %v", err)
	}

	form := url.Values{}
	form.Set("id", fmt.Sprintf("%d", admin.ID))
	form.Set("disabledPresent", "true")
	form.Set("username", "admin")
	form.Set("displayName", "Renamed Admin")
	form.Set("password", "") // blank means unchanged
	form.Set("role", string(models.RoleAdmin))

	res := doReq(tc, http.MethodPost, "/v1/user", map[string]string{
		"Accept":       "text/html",
		"Content-Type": "application/x-www-form-urlencoded",
		"X-CSRF-Token": csrfFor(t, tc, cookie),
	}, []*http.Cookie{cookie}, strings.NewReader(form.Encode()))
	// 303, not the 302 the rejection path uses: RedirectIfHTMLAccepted turns a
	// successful POST into a GET with See Other, while HandleFormErrorWithStatus
	// keeps Found. The difference is deliberate on both sides.
	if res.Code != http.StatusSeeOther {
		t.Fatalf("an accepted save should redirect, got %d: %s", res.Code, res.Body.String())
	}
	if loc := res.Header().Get("Location"); loc != "/admin/users" {
		t.Errorf("an accepted save should return to the list, got %q", loc)
	}

	after, err := tc.AppCtx.GetUser(admin.ID)
	if err != nil {
		t.Fatalf("reload the admin: %v", err)
	}
	if after.DisplayName != "Renamed Admin" {
		t.Errorf("the display name was not saved, got %q", after.DisplayName)
	}
	// Blank password left it alone, which is the whole reason the field is optional.
	if _, err := tc.AppCtx.AuthenticateUser("admin", "adminpw1"); err != nil {
		t.Errorf("a blank password field changed the password: %v", err)
	}
}

// A user id that does not exist is a 404, and a missing one is a 400 — the same
// contract every other entity page keeps (entity-not-found-returns-404.spec.ts).
//
// Both need `addMessageErrContext`, not a bare `errorMessage`: the status code is
// what makes render_template render error.tpl rather than the page, so setting only
// the message answers 200 with an alert above an empty edit form. And GetUser
// translates gorm.ErrRecordNotFound into ErrUserNotFound, whose text is "user not
// found" — addErrContext keys on the literal "record not found", so without the
// explicit branch a missing account is a 500.
func TestAdminUserEdit_MissingAndUnknownIdsAreNot500(t *testing.T) {
	tc := setupAuthEnv(t)
	login := doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json"}, nil,
		strings.NewReader(`{"username":"admin","password":"adminpw1"}`))
	cookie := sessionCookie(t, login)
	htmlH := map[string]string{"Accept": "text/html"}

	for _, tc2 := range []struct {
		path string
		want int
	}{
		{"/admin/users/edit?id=99999", http.StatusNotFound},
		{"/admin/users/edit", http.StatusBadRequest},
	} {
		res := doReq(tc, http.MethodGet, tc2.path, htmlH, []*http.Cookie{cookie}, nil)
		if res.Code != tc2.want {
			t.Errorf("GET %s = %d, want %d.\nbody: %s", tc2.path, res.Code, tc2.want, truncate(res.Body.String()))
		}
		// The error page, not the edit form with a nil user in it.
		if strings.Contains(res.Body.String(), `data-testid="user-edit-form"`) {
			t.Errorf("GET %s rendered the edit form instead of the error page", tc2.path)
		}
	}

	// The control: a real id still renders the form, so the assertions above are not
	// passing because the page stopped working.
	admin, err := tc.AppCtx.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("load the admin: %v", err)
	}
	ok := doReq(tc, http.MethodGet, fmt.Sprintf("/admin/users/edit?id=%d", admin.ID), htmlH, []*http.Cookie{cookie}, nil)
	if ok.Code != http.StatusOK || !strings.Contains(ok.Body.String(), `data-testid="user-edit-form"`) {
		t.Fatalf("a real id should render the form, got %d — this test measured nothing", ok.Code)
	}
}
