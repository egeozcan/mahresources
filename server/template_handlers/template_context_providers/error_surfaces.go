package template_context_providers

import (
	"fmt"
	"net/url"
	"strings"
)

// RecoveryLink is one "where to go next" entry on an error page. Error pages
// carry a list of them so a reader who hits a 404 or a rejected form has an
// in-app way out instead of the browser's back button.
type RecoveryLink struct {
	Name string
	Url  string
}

// DashboardRecovery is the link every error page ends with.
var DashboardRecovery = RecoveryLink{Name: "Go to the dashboard", Url: "/dashboard"}

// entitySurface describes one template route family for error purposes: what the
// page displays, and where a reader who could not get it should be sent.
type entitySurface struct {
	// article + noun read as a sentence: "That note doesn't exist…".
	noun    string
	list    string // list page path; empty when the entity has no index page
	listing string // the list page's label
}

// entitySurfaces maps the first segment of a template route to the entity behind
// it. Keyed on the route rather than on the model because one provider serves
// several routes (/note, /note/edit, /note/text) and several models share one
// provider, and because the route is what the reader typed.
var entitySurfaces = map[string]entitySurface{
	"note":             {"note", "/notes", "Notes"},
	"notes":            {"note", "/notes", "Notes"},
	"noteType":         {"note type", "/noteTypes", "Note types"},
	"noteTypes":        {"note type", "/noteTypes", "Note types"},
	"resource":         {"resource", "/resources", "Resources"},
	"resources":        {"resource", "/resources", "Resources"},
	"resourceCategory": {"resource category", "/resourceCategories", "Resource categories"},
	"group":            {"group", "/groups", "Groups"},
	"groups":           {"group", "/groups", "Groups"},
	"tag":              {"tag", "/tags", "Tags"},
	"tags":             {"tag", "/tags", "Tags"},
	"category":         {"category", "/categories", "Categories"},
	"categories":       {"category", "/categories", "Categories"},
	"query":            {"query", "/queries", "Queries"},
	"queries":          {"query", "/queries", "Queries"},
	"relation":         {"relation", "/relations", "Relations"},
	"relations":        {"relation", "/relations", "Relations"},
	"relationType":     {"relation type", "/relationTypes", "Relation types"},
	"relationTypes":    {"relation type", "/relationTypes", "Relation types"},
	"templatePartial":  {"template partial", "/templatePartials", "Template partials"},
	"templatePartials": {"template partial", "/templatePartials", "Template partials"},
	"log":              {"log entry", "/logs", "Logs"},
	"logs":             {"log entry", "/logs", "Logs"},
	// A series has a detail page but no index page (finding 111), so the reader
	// is sent to the resources that carry them.
	"series": {"series", "/resources", "Resources"},
}

// surfaceForPath resolves the entity behind a request path. Returns ok=false for
// routes that display no single entity (/dashboard, /admin/*, /mrql).
func surfaceForPath(path string) (entitySurface, bool) {
	trimmed := strings.TrimPrefix(path, "/")
	if idx := strings.Index(trimmed, "/"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	trimmed = strings.TrimSuffix(trimmed, ".json")
	trimmed = strings.TrimSuffix(trimmed, ".body")
	surface, ok := entitySurfaces[trimmed]
	return surface, ok
}

// requestPathFromContext recovers the request path from the context a provider
// built. StaticTemplateCtx puts the full request URL under "url" for every
// template route, which is what makes addErrContext path-aware without changing
// its signature at 157 call sites. Providers that build a context by hand simply
// get the generic message.
func requestPathFromContext(ctx map[string]any) string {
	raw, _ := ctx["url"].(string)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Path
}

// notFoundMessage turns GORM's "record not found" into something a reader can
// act on. Findings 119/132/135: the ORM string was rendered verbatim as the page
// body, so a missing note read "record not found".
func (s entitySurface) notFoundMessage() string {
	return fmt.Sprintf("That %s doesn't exist, or it has been deleted.", s.noun)
}

// recoveryLinks returns where to send a reader who could not reach this entity.
func (s entitySurface) recoveryLinks() []RecoveryLink {
	links := make([]RecoveryLink, 0, 2)
	if s.list != "" {
		links = append(links, RecoveryLink{Name: "Back to " + s.listing, Url: s.list})
	}
	return append(links, DashboardRecovery)
}

// RecoveryLinksForRequest builds the recovery list for an error raised outside a
// template provider — an API rejection a browser is about to display. The form
// that was rejected names where it came from in its ?redirect= parameter (every
// merge and Add Tags form does), and the Referer covers the rest, so the first
// link is usually the page the reader was actually on.
func RecoveryLinksForRequest(requestURL *url.URL, referer string) []RecoveryLink {
	links := make([]RecoveryLink, 0, 3)
	seen := map[string]bool{}

	add := func(name, target string) {
		if target == "" || seen[target] {
			return
		}
		seen[target] = true
		links = append(links, RecoveryLink{Name: name, Url: target})
	}

	if requestURL != nil {
		add("Back to the page you came from", safeLocalPath(requestURL.Query().Get("redirect")))
	}
	add("Back to the page you came from", safeLocalPath(refererPath(referer)))

	if requestURL != nil {
		// /v1/tags/merge -> Tags. An API path names the entity it acts on, so a
		// rejected write can still offer that entity's list.
		if surface, ok := surfaceForPath(strings.TrimPrefix(requestURL.Path, "/v1")); ok {
			add("Back to "+surface.listing, surface.list)
		}
	}

	links = append(links, DashboardRecovery)
	return links
}

// safeLocalPath accepts only same-site absolute paths, so a crafted ?redirect=
// cannot turn an error page into an open redirect surface.
func safeLocalPath(raw string) string {
	if raw == "" || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" {
		return ""
	}
	return raw
}

// refererPath reduces a Referer header to its path and query, discarding the
// origin. The result is checked by safeLocalPath before use.
func refererPath(referer string) string {
	if referer == "" {
		return ""
	}
	parsed, err := url.Parse(referer)
	if err != nil || parsed.Path == "" {
		return ""
	}
	out := parsed.Path
	if parsed.RawQuery != "" {
		out += "?" + parsed.RawQuery
	}
	return out
}
