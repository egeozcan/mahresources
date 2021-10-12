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

func ResourceListContextProvider(context *context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		offset := (page - 1) * constants.MaxResults
		var query http_query.ResourceQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := StaticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		resources, err := context.GetResources(int(offset), constants.MaxResults, &query)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		resourceCount, err := context.GetResourceCount(&query)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), resourceCount, constants.MaxResults, int(page))

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		tags, err := context.GetTagsByName("", 0)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
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
		tplContext := pongo2.Context{
			"pageTitle": "Create Resource",
		}.Update(StaticTemplateCtx(request))

		var query http_query.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())

		if err != nil {
			return tplContext
		}

		resource, err := context.GetResource(query.ID)

		if err != nil {
			return addErrContext(err, tplContext)
		}

		tagIDs := make([]uint, len(resource.Tags))

		for i, tag := range resource.Tags {
			tagIDs[i] = tag.ID
		}

		tagList := models.TagList(resource.Tags)
		tagsDisplay := template_entities.GenerateRelationsDisplay(tagIDs, tagList.ToNamedEntities(), request.URL.String(), true, "tags")

		peopleIDs := make([]uint, len(resource.People))

		for i, person := range resource.People {
			peopleIDs[i] = person.ID
		}

		peopleList := models.PersonList(resource.People)
		peopleDisplay := template_entities.GenerateRelationsDisplay(peopleIDs, peopleList.ToNamedEntities(), request.URL.String(), true, "people")

		albumIDs := make([]uint, len(resource.Albums))

		for i, album := range resource.Albums {
			albumIDs[i] = album.ID
		}

		albumList := models.AlbumList(resource.Albums)
		albumDisplay := template_entities.GenerateRelationsDisplay(albumIDs, albumList.ToNamedEntities(), request.URL.String(), true, "albums")

		if resource.OwnerId != 0 {
			ownerEntity, err := context.GetPerson(resource.OwnerId)

			if err == nil {
				owner := &template_entities.DisplayedRelation{
					Name:   ownerEntity.GetName(),
					Link:   "",
					Active: false,
					ID:     resource.OwnerId,
				}

				tplContext["owner"] = []*template_entities.DisplayedRelation{owner}
			}
		}

		tplContext["resource"] = resource
		tplContext["pageTitle"] = "Edit Resource"
		tplContext["tags"] = tagsDisplay.SelectedRelations
		tplContext["people"] = peopleDisplay.SelectedRelations
		tplContext["albums"] = albumDisplay.SelectedRelations

		return tplContext
	}
}

func ResourceContextProvider(context *context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query http_query.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := StaticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		resource, err := context.GetResource(query.ID)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle": "Resource " + resource.Name,
			"resource":  resource,
			"action": template_entities.Entry{
				Name: "Edit",
				Url:  "/resource/edit?id=" + strconv.Itoa(int(query.ID)),
			},
		}.Update(baseContext)
	}
}
