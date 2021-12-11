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

func ResourceListContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		offset := (page - 1) * constants.MaxResultsPerPage
		var query query_models.ResourceQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		resources, err := context.GetResources(int(offset), constants.MaxResultsPerPage, &query)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		resourceCount, err := context.GetResourceCount(&query)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), resourceCount, constants.MaxResultsPerPage, int(page))

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		tags, err := context.GetTagsWithIds(&query.Tags, 0)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		notes, _ := context.GetNotesWithIds(&query.Notes)
		groups, _ := context.GetGroupsWithIds(&query.Groups)

		return pongo2.Context{
			"pageTitle":   "Resources",
			"resources":   resources,
			"pagination":  pagination,
			"tags":        tags,
			"notes":       notes,
			"groups":      groups,
			"parsedQuery": query,
			"action": template_entities.Entry{
				Name: "Create",
				Url:  "/resource/new",
			},
		}.Update(baseContext)
	}
}

func ResourceCreateContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		tplContext := pongo2.Context{
			"pageTitle": "Create Resource",
		}.Update(staticTemplateCtx(request))

		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())

		if err != nil || query.ID == 0 {
			var resourceTpl query_models.ResourceQuery
			err := decoder.Decode(&resourceTpl, request.URL.Query())

			if err == nil {
				tplContext["resource"] = resourceTpl

				groups, _ := context.GetGroupsWithIds(&resourceTpl.Groups)
				tags, _ := context.GetTagsWithIds(&resourceTpl.Tags, 0)
				notes, _ := context.GetNotesWithIds(&resourceTpl.Notes)

				if resourceTpl.OwnerId != 0 {
					owner, _ := context.GetGroup(resourceTpl.OwnerId)
					tplContext["owner"] = []*models.Group{owner}
				}

				tplContext["groups"] = groups
				tplContext["tags"] = tags
				tplContext["notes"] = notes
			}

			return tplContext
		}

		resource, err := context.GetResource(query.ID)

		if err != nil {
			return addErrContext(err, tplContext)
		}

		if resource.OwnerId != nil {
			ownerEntity, err := context.GetGroup(*resource.OwnerId)

			if err == nil {
				tplContext["owner"] = []*models.Group{ownerEntity}
			}
		}

		tplContext["resource"] = resource
		tplContext["pageTitle"] = "Edit Resource"
		tplContext["tags"] = &resource.Tags
		tplContext["groups"] = &resource.Groups
		tplContext["notes"] = &resource.Notes

		return tplContext
	}
}

func ResourceContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

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
			"deleteAction": template_entities.Entry{
				Name: "Delete",
				Url:  "/v1/resource/delete",
				ID:   resource.ID,
			},
		}.Update(baseContext)
	}
}
