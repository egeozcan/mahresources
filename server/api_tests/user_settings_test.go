package api_tests

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/models"
)

// putSetting PUTs a single per-user setting. A blank bearer omits the Authorization
// header (used for the auth-off path). valueJSON is the raw JSON value; it is wrapped
// in the {"value": ...} envelope the handler expects.
func putSetting(tc *TestContext, bearer, key, valueJSON string) *httptest.ResponseRecorder {
	body := `{"value":` + valueJSON + `}`
	h := map[string]string{"Content-Type": "application/json", "Accept": "application/json"}
	if bearer != "" {
		h["Authorization"] = bearer
	}
	return doReq(tc, http.MethodPut, "/v1/account/settings/"+key, h, nil, strings.NewReader(body))
}

func getSettings(t *testing.T, tc *TestContext, bearer string) map[string]json.RawMessage {
	t.Helper()
	h := map[string]string{"Accept": "application/json"}
	if bearer != "" {
		h["Authorization"] = bearer
	}
	rr := doReq(tc, http.MethodGet, "/v1/account/settings", h, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET settings: want 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode settings: %v (%s)", err, rr.Body.String())
	}
	return out
}

// Auth-off (the default deployment): the request runs as the implicit root admin, so
// settings persist under root and a plain round-trip works with no CSRF/auth.
func TestUserSettings_AuthOff_RootRoundTrip(t *testing.T) {
	tc := SetupTestEnv(t)
	// Mirror the main.go boot sequence so the actor cache resolves to root.
	if _, err := tc.AppCtx.EnsureRootAdmin(); err != nil {
		t.Fatalf("ensure root admin: %v", err)
	}

	quickTags := `{"version":3,"quickSlots":[[null]],"recentTags":[null],"flowMode":true}`
	if rr := putSetting(tc, "", "quickTags", quickTags); rr.Code != http.StatusOK {
		t.Fatalf("PUT: want 200, got %d (%s)", rr.Code, rr.Body.String())
	}

	got := getSettings(t, tc, "")
	raw, ok := got["quickTags"]
	if !ok {
		t.Fatalf("quickTags missing from %v", got)
	}
	if !jsonEqual(t, raw, quickTags) {
		t.Fatalf("round-trip mismatch: got %s want %s", raw, quickTags)
	}

	// It landed under a real (root) owner, not user 0.
	var count int64
	tc.DB.Model(&models.UserSetting{}).Where("key = ? AND user_id > 0", "quickTags").Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 root-owned quickTags row, got %d", count)
	}

	// DELETE removes it.
	if rr := doReq(tc, http.MethodDelete, "/v1/account/settings/quickTags",
		map[string]string{"Accept": "application/json"}, nil, nil); rr.Code != http.StatusOK {
		t.Fatalf("DELETE: want 200, got %d (%s)", rr.Code, rr.Body.String())
	}
	if got := getSettings(t, tc, ""); len(got) != 0 {
		t.Fatalf("expected no settings after delete, got %v", got)
	}
}

// Auth-on: each user sees only their own settings. Bearer tokens are CSRF-exempt.
func TestUserSettings_AuthOn_PerUserIsolation(t *testing.T) {
	tc := setupAuthEnv(t)
	aID, aBearer := userWithBearer(t, tc, "alice", models.RoleUser)
	_, bBearer := userWithBearer(t, tc, "bob", models.RoleUser)

	aliceVal := `{"showDescriptions":true}`
	if rr := putSetting(tc, aBearer, "uiSettings", aliceVal); rr.Code != http.StatusOK {
		t.Fatalf("alice PUT: want 200, got %d (%s)", rr.Code, rr.Body.String())
	}

	// Alice sees her setting.
	if got := getSettings(t, tc, aBearer); !jsonEqual(t, got["uiSettings"], aliceVal) {
		t.Fatalf("alice should see her setting, got %v", got)
	}
	// Bob sees nothing (isolation).
	if got := getSettings(t, tc, bBearer); len(got) != 0 {
		t.Fatalf("bob should see no settings, got %v", got)
	}
	// The row is owned by alice.
	var owner uint
	tc.DB.Model(&models.UserSetting{}).Where("key = ?", "uiSettings").Select("user_id").Scan(&owner)
	if owner != aID {
		t.Fatalf("uiSettings owner: want alice(%d), got %d", aID, owner)
	}
}

// Validation: empty value, oversize value, and an over-long key all 400.
func TestUserSettings_AuthOn_Validation(t *testing.T) {
	tc := setupAuthEnv(t)
	_, bearer := userWithBearer(t, tc, "val", models.RoleUser)

	// Missing/empty value → 400.
	if rr := doReq(tc, http.MethodPut, "/v1/account/settings/x",
		map[string]string{"Content-Type": "application/json", "Authorization": bearer},
		nil, strings.NewReader(`{}`)); rr.Code != http.StatusBadRequest {
		t.Fatalf("empty value: want 400, got %d (%s)", rr.Code, rr.Body.String())
	}

	// Oversize value (> 256KB) → 400.
	big := `"` + strings.Repeat("a", 300*1024) + `"`
	if rr := putSetting(tc, bearer, "big", big); rr.Code != http.StatusBadRequest {
		t.Fatalf("oversize value: want 400, got %d", rr.Code)
	}

	// Over-long key (> 128 chars) → 400.
	longKey := strings.Repeat("k", 200)
	if rr := putSetting(tc, bearer, longKey, `1`); rr.Code != http.StatusBadRequest {
		t.Fatalf("long key: want 400, got %d (%s)", rr.Code, rr.Body.String())
	}
}

// The per-user key cap rejects a brand-new key past the limit but always allows
// updating an existing one. Exercised at the context layer to avoid 200 HTTP calls.
func TestUserSettings_KeyCountCap(t *testing.T) {
	tc := setupAuthEnv(t)
	uID, _ := userWithBearer(t, tc, "capuser", models.RoleUser)

	ctx := tc.AppCtx.WithPrincipal(&auth.Principal{UserID: uID})
	for i := 0; i < application_context.MaxUserSettingKeysPerUser; i++ {
		key := "k" + strconv.Itoa(i)
		if err := ctx.SetUserSetting(key, json.RawMessage(`1`)); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	// A brand-new key past the cap is rejected.
	if err := ctx.SetUserSetting("overflow", json.RawMessage(`1`)); err == nil {
		t.Fatalf("expected key-cap rejection, got nil")
	}
	// Updating an existing key at the cap still works.
	if err := ctx.SetUserSetting("k0", json.RawMessage(`2`)); err != nil {
		t.Fatalf("update at cap should succeed, got %v", err)
	}
}

// The cap is enforced atomically: with one slot left, many concurrent new-key writes
// must yield exactly one success (not one-per-racing-request). Most meaningful under
// Postgres (true concurrency); under the single-connection SQLite harness the writes
// serialize, which also satisfies the invariant.
func TestUserSettings_KeyCapAtomicUnderConcurrency(t *testing.T) {
	tc := setupAuthEnv(t)
	uID, _ := userWithBearer(t, tc, "capconc", models.RoleUser)
	ctx := tc.AppCtx.WithPrincipal(&auth.Principal{UserID: uID})

	// Fill to exactly one below the cap.
	for i := 0; i < application_context.MaxUserSettingKeysPerUser-1; i++ {
		if err := ctx.SetUserSetting("k"+strconv.Itoa(i), json.RawMessage(`1`)); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	const N = 8
	var wg sync.WaitGroup
	errs := make([]error, N)
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			errs[idx] = ctx.SetUserSetting("c"+strconv.Itoa(idx), json.RawMessage(`1`))
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, e := range errs {
		switch {
		case e == nil:
			successes++
		case errors.Is(e, application_context.ErrTooManySettings):
		default:
			t.Fatalf("unexpected error: %v", e)
		}
	}
	if successes != 1 {
		t.Fatalf("want exactly 1 success at the cap boundary, got %d", successes)
	}
	var total int64
	tc.DB.Model(&models.UserSetting{}).Where("user_id = ?", uID).Count(&total)
	if total != int64(application_context.MaxUserSettingKeysPerUser) {
		t.Fatalf("total keys = %d, want %d (cap must not be exceeded)", total, application_context.MaxUserSettingKeysPerUser)
	}
}

func TestUserSettings_DeleteMissingIsNoop(t *testing.T) {
	tc := setupAuthEnv(t)
	_, bearer := userWithBearer(t, tc, "del", models.RoleUser)
	if rr := doReq(tc, http.MethodDelete, "/v1/account/settings/nope",
		map[string]string{"Accept": "application/json", "Authorization": bearer}, nil, nil); rr.Code != http.StatusOK {
		t.Fatalf("delete missing: want 200, got %d (%s)", rr.Code, rr.Body.String())
	}
}

func jsonEqual(t *testing.T, raw json.RawMessage, want string) bool {
	t.Helper()
	var a, b interface{}
	if err := json.Unmarshal(raw, &a); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(want), &b); err != nil {
		t.Fatalf("bad want json: %v", err)
	}
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}
