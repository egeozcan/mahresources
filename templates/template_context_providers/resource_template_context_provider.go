package template_context_providers

import (
	"fmt"
	"github.com/flosch/pongo2/v4"
	"mahresources/constants"
	"mahresources/context"
	"mahresources/http_query"
	"mahresources/http_utils"
	"mahresources/models"
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

		tags, err := context.GetTags("", 0)

		if err != nil {
			fmt.Println(err)

			return baseContext
		}

		tagList := models.TagList(*tags)
		tagsDisplay := template_entities.GenerateRelationsDisplay(query.Tags, tagList.ToNamedEntities(), request.URL.String(), true, "tags")

		albums, _ := context.GetAlbumsWithIds(query.Albums)
		albumList := models.AlbumList(*albums)
		albumsDisplay := template_entities.GenerateRelationsDisplay(query.Albums, albumList.ToNamedEntities(), request.URL.String(), true, "albums")

		people, _ := context.GetPeopleWithIds(query.People)
		peopleList := models.PersonList(*people)
		peopleDisplay := template_entities.GenerateRelationsDisplay(query.People, peopleList.ToNamedEntities(), request.URL.String(), true, "people")

		return pongo2.Context{
			"pageTitle":  "Resources",
			"resources":  resources,
			"pagination": pagination,
			"tags":       tagsDisplay,
			"albums":     albumsDisplay,
			"people":     peopleDisplay,
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
