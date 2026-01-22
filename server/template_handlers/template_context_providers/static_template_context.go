package template_context_providers

import (
	"fmt"
	"github.com/flosch/pongo2/v4"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers/template_entities"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var baseTemplateContext = pongo2.Context{
	"title": "mahresources",
	"menu": []template_entities.Entry{
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
			Name: "Queries",
			Url:  "/queries",
		},
		{
			Name: "Categories",
			Url:  "/categories",
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
			Name: "Logs",
			Url:  "/logs",
		},
	},
	"partial": func(name string) string { return "/partials/" + name + ".tpl" },
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

var staticTemplateCtx = func(request *http.Request) pongo2.Context {
	currentId := 0

	context := pongo2.Context{
		"queryValues": request.URL.Query(),
		"path":        request.URL.Path,
		"url":         request.URL.String(),
		"withQuery":   getWithQuery(request),
		"hasQuery":    getHasQuery(request),
		"stringId":    stringId,
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

func getPathExtensionOptions(url *url.URL, options *[]*SelectOption) *[]*SelectOption {
	for _, option := range *options {
		if strings.HasSuffix(url.Path, option.Link) {
			(*option).Active = true
		}
		urlWithNewPath := getURLWithNewPath(url, option.Link)
		(*option).Link = urlWithNewPath.String()
	}

	return options
}

func dereference(v interface{}) interface{} {
	switch v.(type) {
	case *uint:
		return *v.(*uint)
	case *string:
		return *v.(*string)
	case *time.Time:
		return *v.(*time.Time)
	default:
		return v
	}
}
