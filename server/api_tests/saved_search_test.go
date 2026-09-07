package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/models"
)

const savedSearchEndpoint = "/v1/account/saved-searches"

func savedSearchRequest(tc *TestContext, bearer, method, suffix, body string) *httptest.ResponseRecorder {
	headers := map[string]string{"Accept": "application/json", "Content-Type": "application/json"}
	if bearer != "" {
		headers["Authorization"] = bearer
	}
	return doReq(tc, method, savedSearchEndpoint+suffix, headers, nil, strings.NewReader(body))
}

func createSavedSearch(t *testing.T, tc *TestContext, bearer, name, target string) models.SavedSearch {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": name, "url": target})
	rr := savedSearchRequest(tc, bearer, http.MethodPost, "", string(body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}
	var search models.SavedSearch
	if err := json.Unmarshal(rr.Body.Bytes(), &search); err != nil {
		t.Fatal(err)
	}
	return search
}

func testSavedSearchCRUD(t *testing.T, tc *TestContext) {
	t.Helper()
	root, err := tc.AppCtx.EnsureRootAdmin()
	if err != nil {
		t.Fatal(err)
	}
	search := createSavedSearch(t, tc, "", "  Zebra  ", "/resources/details?Name=needle&page=3&SortBy=Name&SortBy=CreatedAt")
	if search.Name != "Zebra" || search.Family != "resources" || search.Layout != "Details" || strings.Contains(search.URL, "page=") {
		t.Fatalf("incorrect saved search: %+v", search)
	}
	createSavedSearch(t, tc, "", "Alpha", "/resources/simple")
	createSavedSearch(t, tc, "", "Alpha", "/resources") // names need not be unique
	createSavedSearch(t, tc, "", "Other list", "/notes")
	rr := savedSearchRequest(tc, "", http.MethodGet, "?family=resources", "")
	var searches []models.SavedSearch
	if err := json.Unmarshal(rr.Body.Bytes(), &searches); err != nil || rr.Code != 200 || len(searches) != 3 || searches[0].Name != "Alpha" || searches[2].Name != "Zebra" {
		t.Fatalf("list: %d %s (%v)", rr.Code, rr.Body.String(), err)
	}
	suffix := fmt.Sprintf("/%d", search.ID)
	rr = savedSearchRequest(tc, "", http.MethodPatch, suffix, `{"name":"Renamed"}`)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "needle") {
		t.Fatalf("rename lost URL: %d %s", rr.Code, rr.Body.String())
	}
	rr = savedSearchRequest(tc, "", http.MethodPatch, suffix, `{"url":"/resources/timeline?timelineMode=updated&timelineGranularity=week&timelineAnchor=2026-09-01"}`)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "Renamed") || !strings.Contains(rr.Body.String(), "Timeline") {
		t.Fatalf("replace: %d %s", rr.Code, rr.Body.String())
	}
	var stored models.SavedSearch
	if err := tc.DB.First(&stored, search.ID).Error; err != nil || stored.UserId != root.ID || !strings.Contains(stored.URL, "timelineAnchor=2026-09-01") {
		t.Fatalf("persisted record: %+v %v", stored, err)
	}
	if rr := savedSearchRequest(tc, "", http.MethodDelete, suffix, ""); rr.Code != 204 {
		t.Fatalf("delete: %d %s", rr.Code, rr.Body.String())
	}
	if rr := savedSearchRequest(tc, "", http.MethodPatch, suffix, `{"name":"Gone"}`); rr.Code != 404 {
		t.Fatalf("deleted search can be changed: %d", rr.Code)
	}
}

func TestSavedSearchCRUD(t *testing.T) {
	testSavedSearchCRUD(t, SetupTestEnv(t))
}

func TestSavedSearchPersonalPermissionsAndCleanup(t *testing.T) {
	tc := setupAuthEnv(t)
	_, alice := userWithBearer(t, tc, "search-alice", models.RoleUser)
	bob := roleBearer(t, tc, models.RoleGuest)
	var bobUser models.User
	if err := tc.DB.Where("role = ?", models.RoleGuest).First(&bobUser).Error; err != nil {
		t.Fatal(err)
	}
	bobID := bobUser.ID
	search := createSavedSearch(t, tc, alice, "Private", "/groups?Name=private")
	suffix := fmt.Sprintf("/%d", search.ID)
	for _, method := range []string{http.MethodPatch, http.MethodDelete} {
		if rr := savedSearchRequest(tc, bob, method, suffix, `{"name":"Stolen"}`); rr.Code != 404 {
			t.Fatalf("cross-user %s: %d %s", method, rr.Code, rr.Body.String())
		}
	}
	if rr := savedSearchRequest(tc, bob, http.MethodGet, "", ""); rr.Code != 200 || strings.TrimSpace(rr.Body.String()) != "[]" {
		t.Fatalf("leaked personal search: %d %s", rr.Code, rr.Body.String())
	}
	guestSearch := createSavedSearch(t, tc, bob, "Guest search", "/notes")
	guestSuffix := fmt.Sprintf("/%d", guestSearch.ID)
	if rr := savedSearchRequest(tc, bob, http.MethodPatch, guestSuffix, `{"name":"Guest rename"}`); rr.Code != 200 {
		t.Fatalf("guest rename: %d %s", rr.Code, rr.Body.String())
	}
	if rr := savedSearchRequest(tc, bob, http.MethodDelete, guestSuffix, ""); rr.Code != 204 {
		t.Fatalf("guest delete: %d %s", rr.Code, rr.Body.String())
	}
	createSavedSearch(t, tc, bob, "Clean up", "/notes")
	if err := tc.AppCtx.DeleteUser(bobID); err != nil {
		t.Fatal(err)
	}
	var count int64
	tc.DB.Model(&models.SavedSearch{}).Where("user_id = ?", bobID).Count(&count)
	if count != 0 {
		t.Fatal("user deletion left saved searches behind")
	}
	if rr := savedSearchRequest(tc, alice, http.MethodGet, "", ""); rr.Code != 200 || !strings.Contains(rr.Body.String(), "Private") {
		t.Fatalf("other user's searches removed: %d %s", rr.Code, rr.Body.String())
	}
}

func TestSavedSearchValidation(t *testing.T) {
	tc := SetupTestEnv(t)
	if _, err := tc.AppCtx.EnsureRootAdmin(); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{}`, `null`, `{"name":"","url":"/notes"}`, `{"name":"   ","url":"/notes"}`,
		`{"name":"x","url":"https://example.com/notes"}`, `{"name":"x","url":"/note?id=1"}`,
		`{"name":"x","url":"/notes","userId":5}`, `{"name":"x","url":"/notes"} {}`,
		`{"name":"` + strings.Repeat("a", 201) + `","url":"/notes"}`,
		`{"name":"x","url":"/notes?Name=` + strings.Repeat("a", 65536) + `"}`,
	} {
		if rr := savedSearchRequest(tc, "", http.MethodPost, "", body); rr.Code != 400 {
			t.Errorf("invalid body accepted: %d %.150s", rr.Code, body)
		}
	}
	search := createSavedSearch(t, tc, "", "Valid", "/notes")
	for _, body := range []string{`{}`, `{"name":null}`, `{"url":"/resources"}`} {
		if rr := savedSearchRequest(tc, "", http.MethodPatch, fmt.Sprintf("/%d", search.ID), body); rr.Code != 400 {
			t.Errorf("invalid patch: %d %s", rr.Code, rr.Body.String())
		}
	}
	for _, suffix := range []string{"/0", "/-1", "/nope", "/18446744073709551616"} {
		if rr := savedSearchRequest(tc, "", http.MethodDelete, suffix, ""); rr.Code != 400 {
			t.Errorf("invalid id %s: %d", suffix, rr.Code)
		}
	}
	if rr := savedSearchRequest(tc, "", http.MethodGet, "?family=unknown", ""); rr.Code != 400 {
		t.Errorf("invalid family: %d", rr.Code)
	}
}

func TestSavedSearchSessionCSRF(t *testing.T) {
	tc := setupAuthEnv(t)
	cookie, token := loginCookieAndCSRF(t, tc)
	body := `{"name":"Session search","url":"/notes"}`
	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodDelete} {
		path := savedSearchEndpoint
		if method != http.MethodPost {
			path += "/1"
		}
		rr := doReq(tc, method, path, map[string]string{"Accept": "application/json", "Content-Type": "application/json"}, []*http.Cookie{cookie}, strings.NewReader(body))
		if rr.Code != http.StatusForbidden {
			t.Fatalf("%s without CSRF: %d %s", method, rr.Code, rr.Body.String())
		}
	}
	rr := doReq(tc, http.MethodPost, savedSearchEndpoint, map[string]string{
		"Accept": "application/json", "Content-Type": "application/json", "X-CSRF-Token": token,
	}, []*http.Cookie{cookie}, strings.NewReader(body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create with CSRF: %d %s", rr.Code, rr.Body.String())
	}
}
