package template_context_providers

import (
	"net/http"

	"github.com/flosch/pongo2/v4"

	"mahresources/application_context"
)

// AdminExportContextProvider returns the Pongo2 context for /admin/export.
// Pre-selected group IDs can be passed via the ?groups=ID,ID query string
// (used by the groups-list bulk-selection redirect in Task 17).
func AdminExportContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		baseContext := StaticTemplateCtx(request)
		preselect := request.URL.Query().Get("groups")
		return pongo2.Context{
			"pageTitle":           "Export Groups",
			"hideSidebar":         true,
			"preselectedGroupIds": preselect,
		}.Update(baseContext)
	}
}
