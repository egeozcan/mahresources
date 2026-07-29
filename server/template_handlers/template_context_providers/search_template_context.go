package template_context_providers

import (
	"net/http"

	"github.com/flosch/pongo2/v4"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
)

// WS6, finding 32 — the full search-results page.
//
// The Cmd+K dialog fetched /v1/search?limit=15, showed fifteen rows, and said
// nothing about the rest; /search itself answered 404, so "see them all" had
// nowhere to go. This provider backs the destination.
//
// Deliberately no pagination: the search service caps its own result set at 50
// rows (search.searchCacheLimit) and reports that through TotalCapped, so a
// second page would be empty by construction. The page states the cap instead
// of pretending to page past it.

// searchPageLimit is what the page asks for. It matches the service ceiling, so
// the page shows everything the service is willing to return.
const searchPageLimit = 50

// searchTypeLabels mirrors the dialog's own labels (src/components/globalSearch.js)
// so the same entity reads the same way in both places.
var searchTypeLabels = map[string]string{
	"resource":         "Resources",
	"note":             "Notes",
	"group":            "Groups",
	"tag":              "Tags",
	"category":         "Categories",
	"resourceCategory": "Resource Categories",
	"query":            "Queries",
	"relationType":     "Relation Types",
	"noteType":         "Note Types",
	"mrqlQuery":        "Saved Queries",
}

// searchTypeOrder fixes the section order. Ranging the map would give pongo2 a
// different order on every render — the same defect finding 47 was about.
var searchTypeOrder = []string{
	"resource", "note", "group", "tag", "category",
	"resourceCategory", "query", "mrqlQuery", "noteType", "relationType",
}

// SearchResultGroup is one section of the page: an entity type and its hits.
type SearchResultGroup struct {
	Type  string
	Label string
	Items []query_models.SearchResultItem
}

func SearchPageContextProvider(context SearchPageContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		baseContext := StaticTemplateCtx(request)
		term := http_utils.GetQueryParameter(request, "q", "")

		baseContext.Update(pongo2.Context{
			"pageTitle":   "Search",
			"searchQuery": term,
			"groups":      []SearchResultGroup{},
			"total":       0,
			"totalCapped": false,
		})

		if term == "" {
			return baseContext
		}

		response, err := context.GlobalSearch(&query_models.GlobalSearchQuery{
			Query: term,
			Limit: searchPageLimit,
		})
		if err != nil {
			return addErrContext(err, baseContext)
		}

		byType := make(map[string][]query_models.SearchResultItem, len(searchTypeOrder))
		for _, item := range response.Results {
			byType[item.Type] = append(byType[item.Type], item)
		}

		groups := make([]SearchResultGroup, 0, len(byType))
		for _, t := range searchTypeOrder {
			if items := byType[t]; len(items) > 0 {
				label := searchTypeLabels[t]
				if label == "" {
					label = t
				}
				groups = append(groups, SearchResultGroup{Type: t, Label: label, Items: items})
			}
		}

		baseContext.Update(pongo2.Context{
			"groups":      groups,
			"total":       response.Total,
			"totalCapped": response.TotalCapped,
		})

		return baseContext
	}
}
