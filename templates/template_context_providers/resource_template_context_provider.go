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

		notes, _ := context.GetNotesWithIds(query.Notes)
		noteList := models.NoteList(*notes)
		notesDisplay := template_entities.GenerateRelationsDisplay(query.Notes, noteList.ToNamedEntities(), request.URL.String(), true, "notes")

		groups, _ := context.GetGroupsWithIds(query.Groups)
		groupsList := models.GroupList(*groups)
		groupsDisplay := template_entities.GenerateRelationsDisplay(query.Groups, groupsList.ToNamedEntities(), request.URL.String(), true, "groups")

		return pongo2.Context{
			"pageTitle":  "Resources",
			"resources":  resources,
			"pagination": pagination,
			"tags":       tagsDisplay,
			"notes":      notesDisplay,
			"groups":     groupsDisplay,
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

		if err != nil || query.ID == 0 {
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

		groupsIDs := make([]uint, len(resource.Groups))

		for i, group := range resource.Groups {
			groupsIDs[i] = group.ID
		}

		groupsList := models.GroupList(resource.Groups)
		groupsDisplay := template_entities.GenerateRelationsDisplay(groupsIDs, groupsList.ToNamedEntities(), request.URL.String(), true, "groups")

		noteIDs := make([]uint, len(resource.Notes))

		for i, note := range resource.Notes {
			noteIDs[i] = note.ID
		}

		noteList := models.NoteList(resource.Notes)
		noteDisplay := template_entities.GenerateRelationsDisplay(noteIDs, noteList.ToNamedEntities(), request.URL.String(), true, "notes")

		if resource.OwnerId != 0 {
			ownerEntity, err := context.GetGroup(resource.OwnerId)

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
		tplContext["groups"] = groupsDisplay.SelectedRelations
		tplContext["notes"] = noteDisplay.SelectedRelations

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
