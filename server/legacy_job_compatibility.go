package server

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"mahresources/server/template_handlers/template_context_providers"
)

// The redirect and the new page use the same release gate. A partial flip would
// redirect /downloads to a page that the router has not mounted yet.
const canonicalJobUICutoverComplete = template_context_providers.JobCenterCutoverEnabled

const (
	legacyJobDeprecation = "@1790121600"
	legacyJobSunset      = "Wed, 24 Mar 2027 00:00:00 GMT"
	legacyJobSuccessor   = "</v1/jobs>; rel=\"successor-version\""
)

func addLegacyJobHeaders(header http.Header) {
	header.Set("Deprecation", legacyJobDeprecation)
	header.Set("Sunset", legacyJobSunset)
	header.Add("Link", legacyJobSuccessor)
}

func legacyJobHandler(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		addLegacyJobHeaders(w.Header())
		handler(w, r)
	}
}

// legacyDownloadsLocation carries filters the Job Center can represent to its
// canonical query vocabulary. Filters with no equivalent are intentionally
// omitted rather than rewritten into a query with different meaning.
//
// Both download Kinds are listed: a download scheduled for later was a download on
// the page this address used to show, and a legacy status names every canonical
// state the legacy projection reads as that status (downloadStatusFromState): so
// "pending" includes scheduled work, and "paused" includes blocked work, which a
// Resume lifts as it lifts a pause.
func legacyDownloadsLocation(values url.Values) string {
	query := make(url.Values)
	query.Add("kind", "remote-download")
	query.Add("kind", "deferred-download")
	// The Job Center's default, written out so the translated address is one the
	// page shows a list under rather than one it redirects again.
	query.Set("dismissed", "false")
	for _, value := range values["Status"] {
		states := map[string][]string{
			"pending":     {"queued", "scheduled"},
			"downloading": {"running"},
			"processing":  {"running"},
			"paused":      {"paused", "blocked"},
			"completed":   {"succeeded"},
			"failed":      {"failed"},
			"cancelled":   {"cancelled"},
		}[strings.ToLower(strings.TrimSpace(value))]
		for _, state := range states {
			if !slices.Contains(query["state"], state) {
				query.Add("state", state)
			}
		}
	}
	if value := strings.TrimSpace(values.Get("URL")); value != "" {
		query.Set("search", value)
	}
	for old, canonical := range map[string]string{
		"CreatedAfter":  "acceptedAfter",
		"CreatedBefore": "acceptedBefore",
	} {
		if value := strings.TrimSpace(values.Get(old)); value != "" {
			if bound := canonicalJobTimeBound(value); bound != "" {
				query.Set(canonical, bound)
			}
		}
	}
	return "/jobs?" + query.Encode()
}

func canonicalJobTimeBound(value string) string {
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		return parsed.UTC().Format(time.RFC3339)
	}
	if _, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return value
	}
	return ""
}
