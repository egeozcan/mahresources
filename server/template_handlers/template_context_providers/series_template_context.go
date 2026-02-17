package template_context_providers

import (
	"net/http"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/models/query_models"
	"mahresources/server/template_handlers/template_entities"
)

func SeriesContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		series, err := context.GetSeries(query.ID)
		if err != nil {
			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle": "Series: " + series.Name,
			"series":    series,
			"deleteAction": template_entities.Entry{
				Name: "Delete",
				Url:  "/v1/series/delete",
				ID:   series.ID,
			},
			"mainEntity":     series,
			"mainEntityType": "series",
		}.Update(baseContext)
	}
}
