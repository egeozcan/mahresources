package template_context_providers

import (
	"fmt"
	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers/template_entities"
	"net/http"
	"strconv"
)

func NoteListContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		offset := (page - 1) * constants.MaxResultsPerPage
		var query query_models.NoteQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		notes, err := context.GetNotes(int(offset), constants.MaxResultsPerPage, &query)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		noteCount, err := context.GetNoteCount(&query)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), noteCount, constants.MaxResultsPerPage, int(page))

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

		owners, err := context.GetGroupsWithIds(&[]uint{query.OwnerId})

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		noteTypes, err := context.GetNoteTypesWithIds([]uint{query.NoteTypeId})

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle":   "Notes",
			"notes":       notes,
			"groups":      groups,
			"owners":      owners,
			"pagination":  pagination,
			"tags":        tags,
			"noteTypes":   noteTypes,
			"parsedQuery": query,
			"action": template_entities.Entry{
				Name: "Create",
				Url:  "/note/new",
			},
			"sortValues": createSortCols([]SortColumn{
				{Name: "Created", Value: "created_at"},
				{Name: "Name", Value: "name"},
				{Name: "Updated", Value: "updated_at"},
			}, query.SortBy),
		}.Update(baseContext)
	}
}

func NoteCreateContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		tplContext := pongo2.Context{
			"pageTitle": "Create Note",
		}.Update(staticTemplateCtx(request))

		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())

		if err != nil || query.ID == 0 {
			var noteQuery query_models.NoteQuery
			err := decoder.Decode(&noteQuery, request.URL.Query())

			if err == nil {
				tplContext["note"] = noteQuery

				groups, _ := context.GetGroupsWithIds(&noteQuery.Groups)
				tags, _ := context.GetTagsWithIds(&noteQuery.Tags, 0)

				if noteQuery.OwnerId != 0 {
					owner, _ := context.GetGroup(noteQuery.OwnerId)
					tplContext["owner"] = []*models.Group{owner}
				}

				tplContext["groups"] = groups
				tplContext["tags"] = tags
			}

			return tplContext
		}

		note, err := context.GetNote(query.ID)

		if err != nil {
			return addErrContext(err, tplContext)
		}

		tplContext["note"] = note
		tplContext["pageTitle"] = "Edit Note"
		tplContext["tags"] = &note.Tags
		tplContext["groups"] = &note.Groups

		if note.OwnerId != nil {
			ownerEntity, err := context.GetGroup(*note.OwnerId)

			if err == nil {
				tplContext["owner"] = []*models.Group{ownerEntity}
			}
		}

		if note.NoteTypeId != nil {
			noteType, err := context.GetNoteType(*note.NoteTypeId)
			if err == nil {
				tplContext["noteType"] = []*models.NoteType{noteType}
			}
		}

		return tplContext
	}
}

func NoteContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			fmt.Println("error parsing query", err)

			return addErrContext(err, baseContext)
		}

		note, err := context.GetNote(query.ID)

		if err != nil {
			fmt.Println("error getting the note", err)

			return addErrContext(err, baseContext)
		}

		// Determine share server base URL if sharing is enabled
		shareEnabled := context.Config.SharePort != ""
		shareBaseUrl := ""
		if shareEnabled {
			shareBaseUrl = fmt.Sprintf("http://%s:%s", context.Config.ShareBindAddress, context.Config.SharePort)
		}

		return pongo2.Context{
			"pageTitle": "Note: " + note.GetName(),
			"note":      note,
			"action": template_entities.Entry{
				Name: "Edit",
				Url:  "/note/edit?id=" + strconv.Itoa(int(query.ID)),
			},
			"deleteAction": template_entities.Entry{
				Name: "Delete",
				Url:  fmt.Sprintf("/v1/note/delete?Id=%v", note.ID),
			},
			"mainEntity":     note,
			"mainEntityType": "note",
			"shareEnabled":   shareEnabled,
			"shareBaseUrl":   shareBaseUrl,
		}.Update(baseContext)
	}
}

func NoteTypeListContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		offset := (page - 1) * constants.MaxResultsPerPage
		var query query_models.NoteTypeQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		noteTypes, err := context.GetNoteTypes(&query, int(offset), constants.MaxResultsPerPage)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		noteTypesCount, err := context.GetNoteTypesCount(&query)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), noteTypesCount, constants.MaxResultsPerPage, int(page))

		if err != nil {
			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle":  "Note Types",
			"noteTypes":  noteTypes,
			"pagination": pagination,
			"action": template_entities.Entry{
				Name: "Add",
				Url:  "/noteType/new",
			},
		}.Update(baseContext)
	}
}

func NoteTypeCreateContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		tplContext := pongo2.Context{
			"pageTitle": "Create Note Type",
		}.Update(staticTemplateCtx(request))

		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())

		if err != nil || query.ID == 0 {
			return tplContext
		}

		noteType, err := context.GetNoteType(query.ID)

		if err != nil {
			return tplContext
		}

		tplContext["pageTitle"] = "Edit Note Type"
		tplContext["noteType"] = noteType

		return tplContext
	}
}

func NoteTypeContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		noteType, err := context.GetNoteType(query.ID)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle": "Note Type " + noteType.Name,
			"noteType":  noteType,
			"action": template_entities.Entry{
				Name: "Edit",
				Url:  "/noteType/edit?id=" + strconv.Itoa(int(query.ID)),
			},
			"deleteAction": template_entities.Entry{
				Name: "Delete",
				Url:  fmt.Sprintf("/v1/note/noteType/delete?Id=%v", noteType.ID),
			},
			"mainEntity":     noteType,
			"mainEntityType": "noteType",
		}.Update(baseContext)
	}
}
