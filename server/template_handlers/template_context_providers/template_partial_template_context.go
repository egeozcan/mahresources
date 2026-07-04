package template_context_providers

import (
	"net/http"
	"strconv"

	"github.com/flosch/pongo2/v4"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers/template_entities"
)

func TemplatePartialListContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetPageParameter(request)
		offset := (page - 1) * constants.MaxResultsPerPage
		var query query_models.TemplatePartialQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := StaticTemplateCtx(request)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		partials, err := context.GetTemplatePartials(&query, int(offset), constants.MaxResultsPerPage)
		if err != nil {
			return addErrContext(err, baseContext)
		}

		partialsCount, err := context.GetTemplatePartialsCount(&query)
		if err != nil {
			return addErrContext(err, baseContext)
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), partialsCount, constants.MaxResultsPerPage, int(page))
		if err != nil {
			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle":  "Template Partials",
			"partials":   partials,
			"pagination": pagination,
			"action": template_entities.Entry{
				Name: "Add",
				Url:  "/templatePartial/new",
			},
		}.Update(baseContext)
	}
}

func TemplatePartialCreateContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		tplContext := pongo2.Context{
			"pageTitle": "Create Template Partial",
		}.Update(StaticTemplateCtx(request))

		var query query_models.EntityIdQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			return addErrContext(err, tplContext)
		}

		if query.ID == 0 {
			return tplContext
		}

		partial, err := context.GetTemplatePartial(query.ID)
		if err != nil {
			return addErrContext(err, tplContext)
		}

		tplContext["pageTitle"] = "Edit Template Partial"
		tplContext["templatePartial"] = partial

		return tplContext
	}
}

func TemplatePartialContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := StaticTemplateCtx(request)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		partial, err := context.GetTemplatePartial(query.ID)
		if err != nil {
			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle":       "Template Partial: " + partial.Name,
			"prefix":          "Template Partial",
			"templatePartial": partial,
			"action": template_entities.Entry{
				Name: "Edit",
				Url:  "/templatePartial/edit?id=" + strconv.Itoa(int(query.ID)),
			},
			"deleteAction": template_entities.Entry{
				Name: "Delete",
				Url:  "/v1/templatePartial/delete?id=" + strconv.Itoa(int(query.ID)),
			},
		}.Update(baseContext)
	}
}
