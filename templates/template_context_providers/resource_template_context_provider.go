package template_context_providers

import (
	"fmt"
	"github.com/flosch/pongo2/v4"
	"mahresources/constants"
	"mahresources/context"
	"mahresources/http_utils"
	"mahresources/http_utils/http_query"
	"mahresources/templates/template_entities"
	"net/http"
)

func ResourceListContextProvider(context *context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		offset := (page - 1) * constants.MaxResults
		var query http_query.ResourceQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := StaticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return baseContext
		}

		resources, err := context.GetResources(int(offset), constants.MaxResults, &query)

		if err != nil {
			fmt.Println(err)

			return baseContext
		}

		resourceCount, err := context.GetResourceCount(&query)

		if err != nil {
			fmt.Println(err)

			return baseContext
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), resourceCount, constants.MaxResults, int(page))

		if err != nil {
			fmt.Println(err)

			return baseContext
		}

		tags, err := context.GetTagsForResources()

		if err != nil {
			fmt.Println(err)

			return baseContext
		}

		tagsDisplay := template_entities.GenerateTagsSelection(query.Tags, tags, request.URL.String(), true, "tags")

		return pongo2.Context{
			"pageTitle":  "Resources",
			"resources":  resources,
			"pagination": pagination,
			"tags":       tagsDisplay,
			"action": template_entities.Entry{
				Name: "Create",
				Url:  "/resource/new",
			},
		}.Update(baseContext)
	}
}

func ResourceCreateContextProvider(context *context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		return pongo2.Context{
			"pageTitle": "Create Resource",
		}.Update(StaticTemplateCtx(request))
	}
}