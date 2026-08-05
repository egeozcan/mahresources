//go:build json1 && fts5

package api_tests

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/spf13/afero"

	"mahresources/server"
)

// Phase 3 guard for finding 47 of the 2026-07-29 UI bug hunt (docs/todo.md).
//
// `GET /v1/note/block/types` answered in a different order on every request,
// because models/block_types/registry.go ranged over a Go map and Go randomises
// map iteration deliberately. The visible symptom was a keyboard user in the
// block picker: the roving tabindex walked a list whose contents moved under it,
// so ArrowDown selected a different type each time the menu opened.
//
// bughunt_ws8_test.go pins that one endpoint. This is the generalisation the
// plan asked for: **every** parameterless GET under /v1/ must answer the same
// bytes for the same request, so a new catalogue endpoint assembled from a map
// is caught the day it is added rather than the day someone notices a menu
// reshuffling.
//
// The sweep discovers its own routes from the router, so it does not go stale.

// nonDeterministicEndpoints enumerates the parameterless GETs whose response is
// legitimately allowed to differ between two identical requests. Each entry MUST
// say why.
var nonDeterministicEndpoints = map[string]string{
	// Server-sent events: the handler holds the connection open and streams, so
	// ServeHTTP never returns and a comparison would hang rather than fail.
	"/v1/download/events": "SSE stream; the handler never returns",
	"/v1/jobs/events":     "SSE stream; the handler never returns",

	// Reports live process measurements — uptime, allocation counters, goroutine
	// count — which change between two calls by design.
	"/v1/admin/server-stats": "reports live process metrics (uptime, memory, goroutines)",
}

// TestParameterlessGETsAreByteStable calls every parameterless GET under /v1/
// twenty times and requires one distinct response body.
func TestParameterlessGETsAreByteStable(t *testing.T) {
	tc := SetupTestEnv(t)
	// Several endpoints in this sweep fan out over goroutines (/v1/admin/data-stats
	// runs fifteen count queries concurrently, /v1/search one per entity type),
	// which forces the pool open. That used to be fatal: the harness DSN was
	// `cache=private`, where every new pool connection is a separate empty
	// database, so those endpoints answered 500 "no such table" a random number of
	// times out of twenty and this guard failed on the harness rather than on the
	// product. The DSN is `cache=shared` now, so that is no longer what the pin is
	// for. It stays because unpinning seventeen tests at once is its own change and
	// deserves its own measurement (shared-cache SQLite has a locking hazard
	// setupAuthEnv spells out), and because twenty byte-identical responses are
	// easier to reason about on one connection anyway.
	if sqlDB, err := tc.DB.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}

	paths := parameterlessV1GETs(t, tc)
	if len(paths) < 40 {
		t.Fatalf("route walk found only %d parameterless /v1/ GETs; expected dozens. "+
			"The sweep is not looking at the endpoints it claims to guard.", len(paths))
	}

	// Every skip entry must still name a live route, or it is a hole in the
	// guard that looks like a documented exception.
	live := map[string]bool{}
	for _, p := range paths {
		live[p] = true
	}
	for path, reason := range nonDeterministicEndpoints {
		if !live[path] {
			t.Errorf("nonDeterministicEndpoints[%q] (%s) is not a registered parameterless "+
				"/v1/ GET any more. Remove it.", path, reason)
		}
	}

	swept := 0
	for _, path := range paths {
		if _, skip := nonDeterministicEndpoints[path]; skip {
			continue
		}
		swept++
		t.Run(path, func(t *testing.T) {
			bodies := map[string]int{}
			var status int
			for range 20 {
				req, _ := http.NewRequest(http.MethodGet, path, nil)
				req.Header.Set("Accept", "application/json")
				rr := httptest.NewRecorder()
				tc.Router.ServeHTTP(rr, req)
				status = rr.Code
				bodies[rr.Body.String()]++
			}
			if len(bodies) == 1 {
				return
			}
			// Report the shortest two, so the message shows the difference
			// rather than two walls of JSON.
			variants := make([]string, 0, len(bodies))
			for b := range bodies {
				variants = append(variants, b)
			}
			sort.Slice(variants, func(i, j int) bool { return len(variants[i]) < len(variants[j]) })
			shown := variants
			if len(shown) > 2 {
				shown = shown[:2]
			}
			t.Errorf("GET %s (status %d) answered %d distinct bodies across 20 identical requests.\n"+
				"Ranging a Go map randomises iteration order; sort before encoding. If the\n"+
				"variation is legitimate, add the route to nonDeterministicEndpoints with a\n"+
				"reason.\nfirst:  %s\nsecond: %s",
				path, status, len(bodies), truncateBody(shown[0], 400), truncateBody(shown[len(shown)-1], 400))
		})
	}

	if swept < 40 {
		t.Errorf("only %d endpoints were swept; the skip list has eaten the guard", swept)
	}
}

// parameterlessV1GETs walks the real router and returns every GET route under
// /v1/ whose path has no {variable} segments, so it can be requested as written.
func parameterlessV1GETs(t *testing.T, tc *TestContext) []string {
	t.Helper()

	// The router is built only to be walked; nothing is served through it.
	router := server.BuildPrimaryRouter(tc.AppCtx, afero.NewMemMapFs(), map[string]string{})

	seen := map[string]bool{}
	var out []string
	err := router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		path, perr := route.GetPathTemplate()
		if perr != nil || !strings.HasPrefix(path, "/v1/") || strings.Contains(path, "{") {
			return nil
		}
		methods, _ := route.GetMethods()
		isGet := len(methods) == 0
		for _, m := range methods {
			if m == http.MethodGet {
				isGet = true
			}
		}
		if !isGet || seen[path] {
			return nil
		}
		seen[path] = true
		out = append(out, path)
		return nil
	})
	if err != nil {
		t.Fatalf("router walk: %v", err)
	}
	sort.Strings(out)
	return out
}

func truncateBody(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
