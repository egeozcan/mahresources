package template_context_providers

import (
	"fmt"
	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers/template_entities"
	"net/http"
	"strconv"
)

func QueryListContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		offset := (page - 1) * constants.MaxResultsPerPage
		var query query_models.QueryQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		queries, err := context.GetQueries(int(offset), constants.MaxResultsPerPage, &query)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		queriesCount, err := context.GetQueriesCount(&query)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), queriesCount, constants.MaxResultsPerPage, int(page))

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle":  "Queries",
			"queries":    queries,
			"pagination": pagination,
			"action": template_entities.Entry{
				Name: "Add",
				Url:  "/query/new",
			},
		}.Update(baseContext)
	}
}

func QueryCreateContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		tplContext := pongo2.Context{
			"pageTitle": "Create Query",
		}.Update(staticTemplateCtx(request))

		var entityId query_models.EntityIdQuery
		err := decoder.Decode(&entityId, request.URL.Query())

		query, err := context.GetQuery(entityId.ID)

		if err != nil {
			return tplContext
		}

		tplContext["pageTitle"] = "Edit Query"
		tplContext["query"] = query

		return tplContext
	}
}

func QueryContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var entityId query_models.EntityIdQuery
		err := decoder.Decode(&entityId, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		query, err := context.GetQuery(entityId.ID)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle": "Query " + query.Name,
			"query":     query,
			"action": template_entities.Entry{
				Name: "Edit",
				Url:  "/query/edit?id=" + strconv.Itoa(int(query.ID)),
			},
			"deleteAction": template_entities.Entry{
				Name: "Delete",
				Url:  fmt.Sprintf("/v1/query/delete?Id=%v", query.ID),
			},
		}.Update(baseContext)
	}
}
