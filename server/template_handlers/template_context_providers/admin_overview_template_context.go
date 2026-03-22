package template_context_providers

import (
	"net/http"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
)

func AdminOverviewContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		baseContext := staticTemplateCtx(request)
		return pongo2.Context{
			"pageTitle":   "Admin Overview",
			"hideSidebar": true,
		}.Update(baseContext)
	}
}
