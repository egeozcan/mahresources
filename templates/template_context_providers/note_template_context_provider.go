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

		tags, err := context.GetTagsWithIds(&query.Tags, 0)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		groups, err := context.GetGroupsWithIds(&query.Groups)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle":  "Notes",
			"notes":      notes,
			"groups":     groups,
			"pagination": pagination,
			"tags":       tags,
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

		groupsIDs := make([]uint, len(note.Groups))

		for i, group := range note.Groups {
			groupsIDs[i] = group.ID
		}

		tplContext["note"] = note
		tplContext["pageTitle"] = "Edit Note"
		tplContext["tags"] = &note.Tags
		tplContext["groups"] = &note.Groups

		if note.OwnerId != 0 {
			ownerEntity, err := context.GetGroup(note.OwnerId)

			if err == nil {
				tplContext["owner"] = []*models.Group{ownerEntity}
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
