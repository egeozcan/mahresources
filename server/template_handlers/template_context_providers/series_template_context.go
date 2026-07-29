package template_context_providers

import (
	"net/http"

	"github.com/flosch/pongo2/v4"
	"mahresources/contracts"
	"mahresources/models/query_models"
	"mahresources/server/template_handlers/template_entities"
)

func SeriesContextProvider(context contracts.SeriesReader) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := StaticTemplateCtx(request)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		// Finding 111: /series is a detail route with no index page, so a bare
		// /series used to look up series 0 and answer "record not found", which
		// says nothing about the missing id. There is no series list to offer, so
		// the message has to carry the explanation itself.
		if query.ID == 0 {
			return addMessageErrContext(
				"A series id is required — open a series from one of its resources.",
				http.StatusNotFound, baseContext)
		}

		series, err := context.GetSeries(query.ID)
		if err != nil {
			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle": "Series: " + series.Name,
			"prefix":    "Series",
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
