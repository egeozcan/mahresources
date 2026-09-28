package server

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"mahresources/application_context"
	"mahresources/download_queue"
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
// state the legacy projection reads as that status, taken from the projection
// itself (application_context.LegacyDownloadStatusStates): so "pending" includes
// scheduled work, "paused" includes blocked work, which a Resume lifts as it
// lifts a pause, and "failed" includes interrupted work.
func legacyDownloadsLocation(values url.Values) string {
	query := make(url.Values)
	query.Add("kind", "remote-download")
	query.Add("kind", "deferred-download")
	// The Job Center's default, written out so the translated address is one the
	// page shows a list under rather than one it redirects again.
	query.Set("dismissed", "false")
	for _, value := range values["Status"] {
		status := download_queue.JobStatus(strings.ToLower(strings.TrimSpace(value)))
		for _, state := range application_context.LegacyDownloadStatusStates(status) {
			if !slices.Contains(query["state"], string(state)) {
				query.Add("state", string(state))
			}
		}
	}
	if value := strings.TrimSpace(values.Get("URL")); value != "" {
		query.Set("search", legacyDownloadSearchTerm(value))
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

// legacyDownloadSearchTerm carries a legacy URL filter to a Job Center search.
// A Job keeps no URL, only its summary's host and the file its path names
// (application_context.DownloadFileNameInURL), so a whole URL is searched for by
// that file, or by its host when its path names none. Anything else, the part
// of a URL the legacy box was usually given, is searched for as typed.
func legacyDownloadSearchTerm(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return value
	}
	if file := application_context.DownloadFileNameInURL(value); file != "" {
		return file
	}
	return parsed.Host
}

// canonicalJobTimeBound carries one legacy date bound. A bare date stays a date:
// the Job Center reads it as the whole local day at either end, so an old link's
// "from that day to that day" still lists that day rather than an empty instant.
func canonicalJobTimeBound(value string) string {
	if _, err := time.Parse("2006-01-02", value); err == nil {
		return value
	}
	if _, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return value
	}
	return ""
}
