package template_context_providers

import (
	"net/http"

	"github.com/flosch/pongo2/v4"

	"mahresources/application_context"
)

// AdminImportContextProvider returns the Pongo2 context for /admin/import.
func AdminImportContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		baseContext := StaticTemplateCtx(request)
		return pongo2.Context{
			"pageTitle":   "Import Groups",
			"hideSidebar": true,
		}.Update(baseContext)
	}
}
