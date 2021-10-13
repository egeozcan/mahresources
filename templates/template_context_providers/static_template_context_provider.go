package template_context_providers

import (
	"github.com/flosch/pongo2/v4"
	"mahresources/http_utils"
	"mahresources/templates/template_entities"
	"net/http"
	"net/url"
	"strconv"
)

var BaseTemplateContext = pongo2.Context{
	"title": "mahresources",
	"menu": []template_entities.Entry{
		template_entities.Entry{
			Name: "Notes",
			Url:  "/notes",
		},
		template_entities.Entry{
			Name: "Resources",
			Url:  "/resources",
		},
		template_entities.Entry{
			Name: "Tags",
			Url:  "/tags",
		},
		template_entities.Entry{
			Name: "Groups",
			Url:  "/groups",
		},
	},
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

var StaticTemplateCtx = func(request *http.Request) pongo2.Context {
	return pongo2.Context{
		"queryValues": request.URL.Query(),
		"path":        request.URL.Path,
		"withQuery": func(name, value string, resetPage bool) string {
			parsedBaseUrl, _ := url.Parse(request.URL.String())
			q := request.URL.Query()

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

			return parsedBaseUrl.String()
		},
		"hasQuery": func(name, value string) bool {
			q := request.URL.Query()

			if q.Get(name) == "" {
				return false
			}

			if existingValue, ok := q[name]; ok {
				return contains(existingValue, value)
			}

			return true
		},
		"stringId": func(id uint) string {
			return strconv.Itoa(int(id))
		},
	}.Update(BaseTemplateContext)
}
