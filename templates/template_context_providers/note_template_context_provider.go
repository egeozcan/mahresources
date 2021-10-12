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

func NoteListContextProvider(context *context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		offset := (page - 1) * constants.MaxResults
		var query http_query.NoteQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := StaticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		notes, err := context.GetNotes(int(offset), constants.MaxResults, &query)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		noteCount, err := context.GetNoteCount(&query)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), noteCount, constants.MaxResults, int(page))

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

		groups, _ := context.GetGroupsWithIds(query.Groups)
		groupsList := models.GroupList(*groups)
		groupsDisplay := template_entities.GenerateRelationsDisplay(query.Groups, groupsList.ToNamedEntities(), request.URL.String(), true, "groups")

		return pongo2.Context{
			"pageTitle":  "Notes",
			"notes":      notes,
			"groups":     groupsDisplay,
			"pagination": pagination,
			"tags":       tagsDisplay,
			"action": template_entities.Entry{
				Name: "Create",
				Url:  "/note/new",
			},
		}.Update(baseContext)
	}
}

func NoteCreateContextProvider(context *context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		tplContext := pongo2.Context{
			"pageTitle": "Create Note",
		}.Update(StaticTemplateCtx(request))

		var query http_query.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())

		if err != nil || query.ID == 0 {
			return tplContext
		}

		note, err := context.GetNote(query.ID)

		if err != nil {
			return addErrContext(err, tplContext)
		}

		tagIDs := make([]uint, len(note.Tags))

		for i, tag := range note.Tags {
			tagIDs[i] = tag.ID
		}

		tags := note.Tags

		groupsIDs := make([]uint, len(note.Groups))

		for i, group := range note.Groups {
			groupsIDs[i] = group.ID
		}

		groups := note.Groups

		tagList := models.TagList(tags)
		tagsDisplay := template_entities.GenerateRelationsDisplay(tagIDs, tagList.ToNamedEntities(), request.URL.String(), true, "tags")

		groupsList := models.GroupList(groups)
		groupsDisplay := template_entities.GenerateRelationsDisplay(groupsIDs, groupsList.ToNamedEntities(), request.URL.String(), true, "groups")

		tplContext["note"] = note
		tplContext["pageTitle"] = "Edit Note"
		tplContext["tags"] = tagsDisplay.SelectedRelations
		tplContext["groups"] = groupsDisplay.SelectedRelations

		if note.OwnerId != 0 {
			ownerEntity, err := context.GetGroup(note.OwnerId)

			if err == nil {
				owner := &template_entities.DisplayedRelation{
					Name:   ownerEntity.GetName(),
					Link:   "",
					Active: false,
					ID:     note.OwnerId,
				}

				tplContext["owner"] = []*template_entities.DisplayedRelation{owner}
			}
		}

		return tplContext
	}
}

func NoteContextProvider(context *context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query http_query.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := StaticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		note, err := context.GetNote(query.ID)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle": "Note: " + note.GetName(),
			"note":      note,
			"action": template_entities.Entry{
				Name: "Edit",
				Url:  "/note/edit?id=" + strconv.Itoa(int(query.ID)),
			},
		}.Update(baseContext)
	}
}
