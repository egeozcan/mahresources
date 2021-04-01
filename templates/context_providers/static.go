package context_providers

import (
	"github.com/flosch/pongo2/v4"
	"mahresources/templates/menu"
	"net/http"
)

var BaseTemplateContext = pongo2.Context{
	"title": "mahresources",
	"menu": []menu.Entry{
		menu.Entry{
			Name: "Albums",
			Url:  "/albums",
		},
		menu.Entry{
			Name: "Resources",
			Url:  "/resources",
		},
		menu.Entry{
			Name: "Tags",
			Url:  "/tags",
		},
		menu.Entry{
			Name: "People",
			Url:  "/people",
		},
	},
}

var StaticTemplateCtx = func(request *http.Request) pongo2.Context { return BaseTemplateContext }
