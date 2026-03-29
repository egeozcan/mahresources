package template_context_providers

import (
	"log"
	"net/http"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
)

func MRQLContextProvider(ctx *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		baseContext := StaticTemplateCtx(request)

		savedQueries, err := ctx.GetSavedMRQLQueries(0, 0) // all, for sidebar display
		if err != nil {
			log.Printf("mrql: failed to load saved queries: %v", err)
		}

		return pongo2.Context{
			"pageTitle":    "MRQL Query",
			"hideSidebar":  true,
			"savedQueries": savedQueries,
		}.Update(baseContext)
	}
}
