package template_context_providers

import (
	"net/http"

	"github.com/flosch/pongo2/v4"
)

func AdminOverviewContextProvider(_ any) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		baseContext := StaticTemplateCtx(request)
		return pongo2.Context{
			"pageTitle":   "Admin Overview",
			"hideSidebar": true,
		}.Update(baseContext)
	}
}
