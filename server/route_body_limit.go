package server

import (
	"net/http"
	"strings"

	"mahresources/server/api_handlers"
)

// Route-specific request body limits (design D14b). They are applied before any
// middleware reads the body, in particular before the CSRF check parses a form
// (csrfTokenFromRequest), so a cookie-authenticated form cannot make the server
// buffer more than the route's limit just to find the token is missing. The global
// JSON limit is off by default (MaxJSONBodySize), and these do not depend on it.
const (
	// reductionBodyLimit covers every /v1/reduction* route: ids and keys, capped
	// elementwise (500, 10,000) after a bounded decode.
	reductionBodyLimit = 1 << 20
	// bulkResourceDeleteBodyLimit covers /v1/resources/delete: at most 10,000 ids.
	bulkResourceDeleteBodyLimit = 1 << 20
	// resourceMergeBodyLimit covers /v1/resources/merge: a Winner and at most 199 Losers.
	resourceMergeBodyLimit = 256 << 10
)

// routeBodyLimit is the byte limit for a request path, or 0 for none.
func routeBodyLimit(path string) int64 {
	path = strings.TrimSuffix(strings.TrimSuffix(path, ".json"), ".body")
	switch {
	case strings.HasPrefix(path, "/v1/reduction"):
		return reductionBodyLimit
	case path == "/v1/resources/delete":
		return bulkResourceDeleteBodyLimit
	case path == "/v1/resources/merge":
		return resourceMergeBodyLimit
	}
	return 0
}

// withRouteBodyLimit wraps the body of the routes that have a limit. It is the
// outermost handler, so nothing reads the body first.
func withRouteBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if limit := routeBodyLimit(r.URL.Path); limit > 0 && r.Body != nil && r.Method != http.MethodGet {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			r = api_handlers.MarkBodyBounded(r)
		}
		next.ServeHTTP(w, r)
	})
}
