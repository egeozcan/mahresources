package api_tests

import (
	"net/http"
	"strings"
	"testing"
)

func TestAutocompleterIgnoresRedirectParam(t *testing.T) {
	for _, accept := range []string{"", "application/json"} {
		tc := SetupTestEnv(t)
		hdr := map[string]string{}
		if accept != "" {
			hdr["Accept"] = accept
		}
		rr := doReq(tc, http.MethodGet, "/partials/autocompleter?profile=single&id=x&_redirect=https://evil.example/x", hdr, nil, nil)
		if rr.Code != http.StatusOK || rr.Header().Get("Location") != "" {
			t.Errorf("accept=%q: %d %q", accept, rr.Code, rr.Header().Get("Location"))
		}
	}
}

func TestAutocompleterAuthEnabledNoRedirect(t *testing.T) {
	tc := setupAuthEnv(t)
	login := doReq(tc, http.MethodPost, "/login",
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, nil,
		strings.NewReader("username=admin&password=adminpw1&next=%2Fpartials%2Fautocompleter%3F_redirect%3Dhttps%3A%2F%2Fevil.example%2Fx"))
	cookie := sessionCookie(t, login)
	loc := login.Header().Get("Location")
	rr := doReq(tc, http.MethodGet, loc, nil, []*http.Cookie{cookie}, nil)
	if rr.Code != http.StatusOK || rr.Header().Get("Location") != "" {
		t.Errorf("post-login bounce: %d %q", rr.Code, rr.Header().Get("Location"))
	}
}

func TestAutocompleterNoRawJSInjection(t *testing.T) {
	tc := SetupTestEnv(t)
	rr := doReq(tc, http.MethodGet, "/partials/autocompleter?profile=tagEditor&entityId=alert(1)&parameters=alert(2)&onChange=alert(3)&excludeIds=alert(4)&addUrl=x'&removeUrl=y", nil, nil, nil)
	for _, s := range []string{"alert(1)", "alert(2)", "alert(3)", "alert(4)"} {
		if strings.Contains(rr.Body.String(), s) {
			t.Errorf("body contains %s", s)
		}
	}
}

func TestAutocompleterNonRegression(t *testing.T) {
	tc := SetupTestEnv(t)
	rr := doReq(tc, http.MethodGet, "/partials/autocompleter?profile=tag&usage=group&selectedItems=[]&title=Edit%20tags&id=t1&elName=editedId", nil, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("code %d", rr.Code)
	}
	for _, s := range []string{`data-selector-profile="tag"`, `data-selector-field="editedId"`, `id="t1"`, `usage: 'group'`} {
		if !strings.Contains(rr.Body.String(), s) {
			t.Errorf("missing %s", s)
		}
	}
	if rr := doReq(tc, http.MethodGet, "/partials/autocompleter?profile=single&id=x&partial=x", nil, nil, nil); rr.Code != http.StatusOK {
		t.Errorf("partial override: %d", rr.Code)
	}
}
