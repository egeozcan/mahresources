package template_context_providers

import (
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/flosch/pongo2/v4"
)

// WS6, findings 54 and 68/77 — every list view's zero-result state.
//
// The empty state has two branches: a filter matched nothing (offer a way out
// of the filter) or the list is genuinely empty (offer to create something).
// Telling them apart needs to know whether the request carries a filter at all,
// which is a property of the URL and not of any one entity — so it is computed
// once in StaticTemplateCtx and read by templates/partials/listEmpty.tpl.

// nonFilterParams are the query parameters that steer presentation rather than
// narrow the result set. Everything else on a list URL is a filter.
//
// "page" is here because paging is not filtering; "SortBy" because ordering is
// not filtering; "Error" because StaticTemplateCtx injects it itself from a
// redirect. Anything not listed counts as a filter, which is the safe default:
// a new filter field that nobody adds here still produces the honest
// "match these filters" wording rather than the wrong "nothing here yet".
var nonFilterParams = map[string]bool{
	"page":     true,
	"Page":     true,
	"sortBy":   true,
	"SortBy":   true,
	"Error":    true,
	"error":    true,
	"redirect": true,
}

// hasActiveFilter reports whether the request narrows its list at all. An empty
// value does not count — a list page reached from a submitted-but-blank filter
// form carries "?Name=&Description=" and is not filtered by any useful reading.
func hasActiveFilter(request *http.Request) bool {
	for key, values := range request.URL.Query() {
		if nonFilterParams[key] {
			continue
		}
		for _, v := range values {
			if strings.TrimSpace(v) != "" {
				return true
			}
		}
	}
	return false
}

// clearFiltersURL is the same list view with every filter dropped. Sort order is
// deliberately kept — it is not what emptied the list, and silently resetting it
// would surprise a reader who set it. Paging is dropped, because page 5 of an
// unfiltered list is not where "clear filters" should land anyone.
func clearFiltersURL(request *http.Request) string {
	cleared := url.Values{}
	for key, values := range request.URL.Query() {
		if key == "SortBy" || key == "sortBy" {
			cleared[key] = values
		}
	}

	out := &url.URL{Path: request.URL.Path}
	out.RawQuery = cleared.Encode()
	return out.String()
}

// WS6, findings 68 and 146 — an out-of-range ?page=.
//
// /resources?page=99 on a two-page list rendered a blank content column under a
// pagination footer that still offered pages 1 and 2, and whose "Previous" link
// pointed at page 98 — equally blank, and equally happy to offer page 97. The
// reader could walk backwards 97 times before seeing anything.
//
// outOfRangePageRedirect returns a context carrying the _redirect signal that
// RenderTemplate acts on (render_template.go:44), or nil when the page is fine.
// It is called at each of the twelve GeneratePagination sites, which is where
// the count and the page size are both already in hand.
func outOfRangePageRedirect(request *http.Request, numResults int64, pageSize int, currentPage int64) pongo2.Context {
	// A zero-result list has no "last valid page" to send anyone to. Redirecting
	// it would either loop or land on a page that answers exactly the same way;
	// the empty state is the correct answer there. This is also what keeps a
	// filter that matches nothing from bouncing the reader off their own URL.
	if numResults <= 0 || pageSize <= 0 {
		return nil
	}

	// Only a page navigation gets redirected. render_template.go acts on
	// _redirect *before* it looks at the JSON branch, so without this the
	// documented "add .json for JSON" contract would start answering 302 to
	// /resources.json?page=99 instead of 200 with an empty array. The .body
	// fragment fetches are excluded for the same reason: a morph target that
	// redirects would splice another page's rows in under the current URL.
	if p := request.URL.Path; strings.HasSuffix(p, ".json") || strings.HasSuffix(p, ".body") {
		return nil
	}
	if accept := request.Header.Get("Accept"); strings.Contains(accept, "application/json") &&
		!strings.Contains(accept, "text/html") {
		return nil
	}

	lastPage := int64(math.Ceil(float64(numResults) / float64(pageSize)))
	if currentPage <= lastPage {
		return nil
	}

	target := &url.URL{}
	*target = *request.URL
	q := target.Query()
	q.Set("page", strconv.FormatInt(lastPage, 10))
	target.RawQuery = q.Encode()

	// RequestURI, not String: the absolute form would let a proxied Host header
	// steer the redirect off-site.
	return pongo2.Context{"_redirect": target.RequestURI()}
}
