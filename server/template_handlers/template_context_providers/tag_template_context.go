package template_context_providers

import (
	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers/template_entities"
	"net/http"
	"strconv"
)

func TagListContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetPageParameter(request)
		resultsPerPage := getResultsPerPage(request, constants.MaxResultsPerPage)
		offset := (page - 1) * int64(resultsPerPage)
		var query query_models.TagQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := StaticTemplateCtx(request)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		tags, err := context.GetTags(int(offset), resultsPerPage, &query)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		tagsCount, err := context.GetTagsCount(&query)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), tagsCount, resultsPerPage, int(page))

		if err != nil {
			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle":  "Tags",
			"tags":       tags,
			"pagination": pagination,
			"action": template_entities.Entry{
				Name: "Add",
				Url:  "/tag/new",
			},
			"sortValues": createSortCols([]SortColumn{
				{Name: "Created", Value: "created_at"},
				{Name: "Name", Value: "name"},
				{Name: "Updated", Value: "updated_at"},
			}, query.SortBy),
			"displayOptions": getPathExtensionOptions(request.URL, &[]*SelectOption{
				{Title: "List", Link: "/tags"},
				{Title: "Timeline", Link: "/tags/timeline"},
			}),
		}.Update(baseContext)
	}
}

func TagTimelineContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	listProvider := TagListContextProvider(context)
	return func(request *http.Request) pongo2.Context {
		ctx := listProvider(request)
		ctx["pageTitle"] = "Tags - Timeline"
		return ctx
	}
}

func TagCreateContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		tplContext := pongo2.Context{
			"pageTitle": "Create Tag",
		}.Update(StaticTemplateCtx(request))

		var query query_models.EntityIdQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			return addErrContext(err, tplContext)
		}

		if query.ID == 0 {
			return tplContext
		}

		tag, err := context.GetTag(query.ID)

		if err != nil {
			return addErrContext(err, tplContext)
		}

		tplContext["pageTitle"] = "Edit Tag"
		tplContext["tag"] = tag

		return tplContext
	}
}

func TagContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := StaticTemplateCtx(request)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		tag, err := context.GetTag(query.ID)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle": "Tag: " + tag.Name,
			"prefix":    "Tag",
			"tag":       tag,
			"action": template_entities.Entry{
				Name: "Edit",
				Url:  "/tag/edit?id=" + strconv.Itoa(int(query.ID)),
			},
			"deleteAction": template_entities.Entry{
				Name: "Delete",
				Url:  "/v1/tag/delete",
				ID:   tag.ID,
			},
			"mainEntity":     tag,
			"mainEntityType": "tag",
		}.Update(baseContext)
	}
}
