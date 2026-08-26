package template_context_providers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/flosch/pongo2/v4"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers/template_entities"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func computeAssetVersion() string {
	h := sha256.New()
	for _, path := range []string{
		"public/index.css",
		"public/tailwind.css",
		"public/jsonTable.css",
		"public/dist/main.js",
	} {
		if data, err := os.ReadFile(path); err == nil {
			h.Write(data)
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}

var AssetVersion = computeAssetVersion()

func DocsURL(baseURL, slug string) string {
	base := strings.TrimRight(baseURL, "/")
	if base == "" {
		return ""
	}
	path := strings.TrimLeft(slug, "/")
	if path == "" {
		return base
	}
	return base + "/" + path
}

var baseTemplateContext = pongo2.Context{
	"assetVersion": AssetVersion,
	"title":        "mahresources",
	"menu": []template_entities.Entry{
		{
			Name: "Dashboard",
			Url:  "/dashboard",
		},
		{
			Name: "Notes",
			Url:  "/notes",
		},
		{
			Name: "Resources",
			Url:  "/resources",
		},
		{
			Name: "Tags",
			Url:  "/tags",
		},
		{
			Name: "Groups",
			Url:  "/groups",
		},
		{
			Name: "MRQL",
			Url:  "/mrql",
		},
		// Not in the admin menu, for the same reason /downloads is not an admin
		// page: every role owns Resource Reductions and each principal sees only
		// its own, so putting it behind the editor-gated dropdown would hide a
		// plain user's own pending work from them.
		{
			Name: "Reductions",
			Url:  "/reductions",
		},
	},
	"adminMenu": []template_entities.Entry{
		{
			Name: "Overview",
			Url:  "/admin/overview",
		},
		{
			Name: "Queries",
			Url:  "/queries",
		},
		{
			Name: "Categories",
			Url:  "/categories",
		},
		{
			Name: "Resource Categories",
			Url:  "/resourceCategories",
		},
		{
			Name: "Relations",
			Url:  "/relations",
		},
		{
			Name: "Relation Types",
			Url:  "/relationTypes",
		},
		{
			Name: "Note Types",
			Url:  "/noteTypes",
		},
		{
			Name: "Template Partials",
			Url:  "/templatePartials",
		},
		{
			Name: "Export",
			Url:  "/admin/export",
		},
		{
			Name: "Import",
			Url:  "/admin/import",
		},
		{
			Name: "Shares",
			Url:  "/admin/shares",
		},
		{
			Name: "Settings",
			Url:  "/admin/settings",
		},
		{
			Name: "Downloads",
			Url:  "/downloads",
		},
		{
			Name: "Logs",
			Url:  "/logs",
		},
	},
	"partial": func(name string) string { return "/partials/" + name + ".tpl" },
}

// navSectionByFirstSegment maps the first path segment to the nav entry that owns
// it.
//
// WS10 findings 116 and 121: the nav highlight was `menuEntry.Url == path`, an
// exact match, so every detail page highlighted nothing at all — /resource?id=85,
// /note?id=61 and /group?id=68 all measured an empty active-link list, and a reader
// had no indication of where they were.
//
// A table rather than a rule, because the app's singular/plural convention is not
// mechanical (`/resourceCategory` → `/resourceCategories`, `/query` → `/queries`)
// and guessing wrong lights the wrong entry, which is worse than lighting none. A
// segment that is absent from the table simply marks nothing, which is the previous
// behaviour — so a new page is never mis-attributed, it is only unmarked until it is
// listed here.
var navSectionByFirstSegment = map[string]string{
	"dashboard": "/dashboard",

	"note":  "/notes",
	"notes": "/notes",

	"resource":  "/resources",
	"resources": "/resources",

	"group":  "/groups",
	"groups": "/groups",

	"tag":  "/tags",
	"tags": "/tags",

	"mrql": "/mrql",

	"reduction":  "/reductions",
	"reductions": "/reductions",

	"query":   "/queries",
	"queries": "/queries",

	"category":   "/categories",
	"categories": "/categories",

	"resourceCategory":   "/resourceCategories",
	"resourceCategories": "/resourceCategories",

	"relation":  "/relations",
	"relations": "/relations",

	"relationType":  "/relationTypes",
	"relationTypes": "/relationTypes",

	"noteType":  "/noteTypes",
	"noteTypes": "/noteTypes",

	"templatePartial":  "/templatePartials",
	"templatePartials": "/templatePartials",

	"log":  "/logs",
	"logs": "/logs",

	"downloads": "/downloads",
}

// adminNavSections are the /admin/* nav entries, matched by prefix rather than by
// first segment (every one of them starts "admin").
var adminNavSections = []string{
	"/admin/overview",
	"/admin/export",
	"/admin/import",
	"/admin/shares",
	"/admin/settings",
	"/admin/users",
}

// activeNavURL returns the nav entry URL the given request path belongs to, or ""
// when it belongs to none (e.g. /account, a plugin page, an error page).
func activeNavURL(path string) string {
	for _, section := range adminNavSections {
		if path == section || strings.HasPrefix(path, section+"/") {
			return section
		}
	}

	first := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 2)[0]
	return navSectionByFirstSegment[first]
}

// formCancelURL is where a create or edit form's Cancel goes: the entity's own
// page when editing, its list when creating, "" everywhere else.
//
// Finding 129: no create or edit form in the app had a Cancel, a Back or a
// breadcrumb, so abandoning an edit meant the browser's Back button and a
// keyboard user tabbing off the end of the form found nothing. It is derived
// from the URL rather than set by each of the twelve providers because the
// answer *is* a property of the URL — /X/new and /X/edit?id=N — and
// navSectionByFirstSegment already knows every entity's list path, including
// the four whose plural is not mechanical (query→queries,
// resourceCategory→resourceCategories, noteType→noteTypes, category→categories).
func formCancelURL(request *http.Request) string {
	segments := strings.Split(strings.Trim(request.URL.Path, "/"), "/")
	if len(segments) != 2 {
		return ""
	}
	entity, action := segments[0], segments[1]
	if action != "new" && action != "edit" {
		return ""
	}
	if _, known := navSectionByFirstSegment[entity]; !known {
		return ""
	}
	// gorilla/schema decodes the id case-insensitively, so both spellings reach
	// the same page and both have to reach the same Cancel target.
	id := request.URL.Query().Get("id")
	if id == "" {
		id = request.URL.Query().Get("ID")
	}
	if isDecimal(id) {
		return "/" + entity + "?id=" + id
	}
	return navSectionByFirstSegment[entity]
}

// isDecimal reports whether s is a non-empty run of ASCII digits. An id that is
// anything else is not an id, and must not be reflected into an href.
func isDecimal(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

// normalizeQueryValues ensures URL query parameter keys are accessible with
// PascalCase names in templates. gorilla/schema decodes case-insensitively,
// but templates access queryValues via case-sensitive Go map lookups
// (e.g., queryValues.Name.0). This function adds PascalCase copies of
// lowercase-initial keys so both "name" and "Name" work as map keys.
func normalizeQueryValues(values url.Values) url.Values {
	result := make(url.Values)
	for key, vals := range values {
		result[key] = vals
	}
	for key, vals := range values {
		if len(key) > 0 && key[0] >= 'a' && key[0] <= 'z' {
			canonical := strings.ToUpper(key[:1]) + key[1:]
			if _, exists := result[canonical]; !exists {
				result[canonical] = vals
			}
		}
	}
	return result
}

var StaticTemplateCtx = func(request *http.Request) pongo2.Context {
	currentId := 0

	context := pongo2.Context{
		"queryValues": normalizeQueryValues(request.URL.Query()),
		"path":        request.URL.Path,
		"url":         request.URL.String(),
		"withQuery":   getWithQuery(request),
		"hasQuery":    getHasQuery(request),
		"stringId":    stringId,
		// WS6: which branch of partials/listEmpty.tpl a zero-result list shows.
		// Computed here rather than per entity because it is a property of the
		// URL, and every list template extends this context.
		"hasActiveFilter": hasActiveFilter(request),
		"clearFiltersUrl": clearFiltersURL(request),
		// WS10 findings 116/121: which nav entry the current URL belongs to. See
		// activeNavURL — a detail page has to keep its section lit, so this is a
		// section match and not `path`.
		"activeNavUrl": activeNavURL(request.URL.Path),
		"activeNavIsExact": func() bool {
			return activeNavURL(request.URL.Path) == request.URL.Path
		}(),
		// WS14 finding 129: where partials/form/createFormSubmit.tpl's Cancel
		// link points. Empty on every page that is not a create/edit form, and
		// the partial renders no link then.
		"cancelUrl": formCancelURL(request),
		"getNextId": func(elName string) string {
			currentId += 1
			return fmt.Sprintf("input_%v_%v", elName, currentId)
		},
		"dereference": dereference,
	}

	if errMessage := request.URL.Query().Get("Error"); errMessage != "" {
		context.Update(pongo2.Context{"errorMessage": errMessage})
	}

	return context.Update(baseTemplateContext)
}

func getHasQuery(request *http.Request) func(name string, value string) bool {
	q := request.URL.Query()

	return func(name, value string) bool {
		if q.Get(name) == "" {
			return false
		}

		if existingValue, ok := q[name]; ok {
			return contains(existingValue, value)
		}

		return true
	}
}

func createSortCols(standardCols []SortColumn, currentSortVals []string) []SortColumn {
	if len(currentSortVals) == 0 {
		return standardCols
	}

	result := make([]SortColumn, len(standardCols))
	copy(result, standardCols)

	// Add any custom sort columns from the current values
	for _, sortVal := range currentSortVals {
		if strings.TrimSpace(sortVal) == "" {
			continue
		}

		currentSort := strings.Split(sortVal, " ")[0]
		found := false

		for _, col := range result {
			if col.Value == currentSort {
				found = true
				break
			}
		}

		if !found {
			// Prepend custom column
			result = append([]SortColumn{
				{
					Name:  fmt.Sprintf("Custom (%v)", currentSort),
					Value: currentSort,
				},
			}, result...)
		}
	}

	return result
}

func stringId(id any) string {
	if u, ok := id.(uint); ok {
		return strconv.Itoa(int(u))
	}
	if u, ok := id.(*uint); ok {
		if u == nil {
			return ""
		}
		return strconv.Itoa(int(*u))
	}
	return ""
}

func getWithQuery(request *http.Request) func(name, value string, resetPage bool) string {
	return func(name, value string, resetPage bool) string {
		parsedBaseUrl := &url.URL{}
		*parsedBaseUrl = *request.URL
		q := parsedBaseUrl.Query()

		if resetPage {
			q.Del("page")
		}

		if q.Get(name) == "" {
			q.Set(name, value)
		} else if existingValue, ok := q[name]; ok && !contains(existingValue, value) {
			q[name] = append(existingValue, value)
		} else {
			q[name] = http_utils.RemoveValue(q[name], value)
		}

		parsedBaseUrl.RawQuery = q.Encode()
		// we don't want to rewrite pointing to a partial
		parsedBaseUrl.Path = strings.TrimSuffix(parsedBaseUrl.Path, ".body")

		return parsedBaseUrl.String()
	}
}

func getURLWithNewPath(url *url.URL, path string) url.URL {
	newURL := *url
	newURL.Path = path

	return newURL
}

// getResultsPerPage reads an optional pageSize query parameter, clamped to [1, 200].
// Falls back to defaultPerPage if not provided.
func getResultsPerPage(request *http.Request, defaultPerPage int) int {
	if customPageSize := http_utils.GetIntQueryParameter(request, "pageSize", 0); customPageSize > 0 {
		if customPageSize > 200 {
			customPageSize = 200
		}
		return int(customPageSize)
	}
	return defaultPerPage
}

func getPathExtensionOptions(url *url.URL, options *[]*SelectOption) *[]*SelectOption {
	for _, option := range *options {
		if strings.HasSuffix(url.Path, option.Link) {
			(*option).Active = true
		}
		urlWithNewPath := getURLWithNewPath(url, option.Link)
		// Filters carry across a view change; the page number must not. Views
		// use different page sizes (Simple renders 4x the rows), so "page 2"
		// denotes a different slice in each one and carrying it produced a
		// blank dead end with no explanation. Switching views lands on page 1.
		if q := urlWithNewPath.Query(); q.Has("page") {
			q.Del("page")
			urlWithNewPath.RawQuery = q.Encode()
		}
		(*option).Link = urlWithNewPath.String()
	}

	return options
}

func dereference(v interface{}) interface{} {
	switch v := v.(type) {
	case *uint:
		if v == nil {
			return nil
		}
		return *v
	case *string:
		if v == nil {
			return nil
		}
		return *v
	case *time.Time:
		if v == nil {
			return nil
		}
		return *v
	default:
		return v
	}
}
