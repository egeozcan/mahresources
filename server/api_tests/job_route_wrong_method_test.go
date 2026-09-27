package api_tests

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/spf13/afero"

	"mahresources/server"
)

// A GET of a legacy Job route that only accepts POST is a wrong method, answered as
// every other wrong method on the API is, and never read as a Job id by the
// canonical /v1/jobs/{id} route ("job not found").
func TestAGetOfAPostOnlyJobRouteIsAnsweredAsAWrongMethod(t *testing.T) {
	tc := SetupTestEnv(t)
	installJobControlPlane(t, tc)
	router := server.BuildPrimaryRouter(tc.AppCtx, afero.NewMemMapFs(), map[string]string{})

	var postOnly []string
	_ = router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		template, err := route.GetPathTemplate()
		if err != nil || !strings.HasPrefix(template, "/v1/jobs/") || strings.ContainsAny(template, "{}") {
			return nil
		}
		if strings.Contains(strings.TrimPrefix(template, "/v1/jobs/"), "/") {
			return nil
		}
		methods, err := route.GetMethods()
		if err != nil {
			return nil
		}
		for _, method := range methods {
			if method == http.MethodGet {
				return nil
			}
		}
		postOnly = append(postOnly, template)
		return nil
	})
	if len(postOnly) < 5 {
		t.Fatalf("found only %v; the walk no longer sees the legacy Job routes", postOnly)
	}

	for _, path := range postOnly {
		rec := doReq(tc, http.MethodGet, path, map[string]string{"Accept": "application/json"}, nil, nil)
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "no such endpoint") {
			t.Errorf("GET %s answered %d %s, want the API's wrong-method answer", path, rec.Code, strings.TrimSpace(rec.Body.String()))
		}
	}
}
