package template_context_providers

import (
	"github.com/flosch/pongo2/v4"
	"mahresources/templates/template_entities"
	"net/http"
)

var BaseTemplateContext = pongo2.Context{
	"title": "mahresources",
	"menu": []template_entities.Entry{
		template_entities.Entry{
			Name: "Albums",
			Url:  "/albums",
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
			Name: "People",
			Url:  "/people",
		},
	},
}

var StaticTemplateCtx = func(request *http.Request) pongo2.Context {
	return pongo2.Context{
		"queryValues": request.URL.Query(),
		"path":        request.URL.Path,
	}.Update(BaseTemplateContext)
}
