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
		tplContext := pongo2.Context{
			"pageTitle": "Create Album",
		}.Update(StaticTemplateCtx(request))

		var query http_query.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())

		if err != nil {
			return tplContext
		}

		album, err := context.GetAlbum(query.ID)

		if err != nil {
			return tplContext
		}

		tagIDs := make([]uint, len(album.Tags))

		for i, tag := range album.Tags {
			tagIDs[i] = tag.ID
		}

		tags := album.Tags

		peopleIDs := make([]uint, len(album.People))

		for i, person := range album.People {
			peopleIDs[i] = person.ID
		}

		people := album.People

		tagList := models.TagList(tags)
		tagsDisplay := template_entities.GenerateRelationsDisplay(tagIDs, tagList.ToNamedEntities(), request.URL.String(), true, "tags")

		peopleList := models.PersonList(people)
		peopleDisplay := template_entities.GenerateRelationsDisplay(peopleIDs, peopleList.ToNamedEntities(), request.URL.String(), true, "people")

		tplContext["album"] = album
		tplContext["pageTitle"] = "Edit Album"
		tplContext["tags"] = tagsDisplay.SelectedRelations
		tplContext["people"] = peopleDisplay.SelectedRelations

		if album.OwnerId != 0 {
			ownerEntity, err := context.GetPerson(album.OwnerId)

			if err == nil {
				owner := &template_entities.DisplayedRelation{
					Name:   ownerEntity.GetName(),
					Link:   "",
					Active: false,
					ID:     album.OwnerId,
				}

				tplContext["owner"] = []*template_entities.DisplayedRelation{owner}
			}
		}

		return tplContext
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
			"pageTitle": "Album: " + album.GetName(),
			"album":     album,
			"action": template_entities.Entry{
				Name: "Edit",
				Url:  "/album/edit?id=" + strconv.Itoa(int(query.ID)),
			},
		}.Update(baseContext)
	}
}
