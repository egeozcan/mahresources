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
	"strconv"
)

func AlbumListContextProvider(context *context.MahresourcesContext) func(request *http.Request) pongo2.Context {
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

		tags, err := context.GetTagsByName("", 0)

		if err != nil {
			fmt.Println(err)

			return baseContext
		}

		tagList := models.TagList(*tags)
		tagsDisplay := template_entities.GenerateRelationsDisplay(query.Tags, tagList.ToNamedEntities(), request.URL.String(), true, "tags")

		people, _ := context.GetPeopleWithIds(query.People)
		peopleList := models.PersonList(*people)
		peopleDisplay := template_entities.GenerateRelationsDisplay(query.People, peopleList.ToNamedEntities(), request.URL.String(), true, "people")

		return pongo2.Context{
			"pageTitle":  "Albums",
			"albums":     albums,
			"people":     peopleDisplay,
			"pagination": pagination,
			"tags":       tagsDisplay,
			"action": template_entities.Entry{
				Name: "Create",
				Url:  "/album/new",
			},
		}.Update(baseContext)
	}
}

func AlbumCreateContextProvider(context *context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		return pongo2.Context{
			"pageTitle": "Create Album",
		}.Update(StaticTemplateCtx(request))
	}
}

func AlbumContextProvider(context *context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query http_query.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := StaticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return baseContext
		}

		album, err := context.GetAlbum(query.ID)

		if err != nil {
			fmt.Println(err)

			return baseContext
		}

		return pongo2.Context{
			"pageTitle": "Albums",
			"album":     album,
			"action": template_entities.Entry{
				Name: "Edit",
				Url:  "/album/edit?id=" + strconv.Itoa(int(query.ID)),
			},
		}.Update(baseContext)
	}
}
