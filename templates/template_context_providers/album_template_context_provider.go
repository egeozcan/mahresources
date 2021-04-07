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

func AlbumContextProvider(context *context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		offset := (page - 1) * constants.MaxResults
		var query http_query.AlbumQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := StaticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return baseContext
		}

		albums, err := context.GetAlbums(int(offset), constants.MaxResults, &query)

		if err != nil {
			fmt.Println(err)

			return baseContext
		}

		albumCount, err := context.GetAlbumCount(&query)

		if err != nil {
			fmt.Println(err)

			return baseContext
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), albumCount, constants.MaxResults, int(page))

		if err != nil {
			fmt.Println(err)

			return baseContext
		}

		tags, err := context.GetTagsForAlbums()

		if err != nil {
			fmt.Println(err)

			return baseContext
		}

		tagsDisplay := template_entities.GenerateTagsSelection(query.Tags, tags, request.URL.String(), true, "tags")

		return pongo2.Context{
			"pageTitle":  "Albums",
			"albums":     albums,
			"pagination": pagination,
			"tags":       tagsDisplay,
			"action": template_entities.Entry{
				Name: "Create",
				Url:  "/album/new",
			},
			"search": template_entities.Search{
				QueryParamName: "Name",
				Text:           "Search for an album",
			},
		}.Update(baseContext)
	}
}

func CreateAlbumContextProvider(context *context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		return pongo2.Context{
			"pageTitle": "Create Album",
		}.Update(StaticTemplateCtx(request))
	}
}
