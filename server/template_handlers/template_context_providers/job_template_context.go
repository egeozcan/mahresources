package template_context_providers

import (
	"net/http"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
)

// JobCenterCutoverEnabled is the single release gate for the canonical Job UI.
const JobCenterCutoverEnabled = true

func JobCenterListContextProvider(_ *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		return pongo2.Context{
			"pageTitle":               "Job Center",
			"hideSidebar":             true,
			"jobCenterCutoverEnabled": JobCenterCutoverEnabled,
		}.Update(StaticTemplateCtx(request))
	}
}

func JobDetailContextProvider(_ *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		return pongo2.Context{
			"pageTitle":               "Job detail",
			"hideSidebar":             true,
			"jobCenterCutoverEnabled": JobCenterCutoverEnabled,
		}.Update(StaticTemplateCtx(request))
	}
}
