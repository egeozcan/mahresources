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

		tags, err := context.GetTagsWithIds(&query.Tags, 0)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		notes, _ := context.GetNotesWithIds(&query.Notes)
		groups, _ := context.GetGroupsWithIds(&query.Groups)

		return pongo2.Context{
			"pageTitle":  "Resources",
			"resources":  resources,
			"pagination": pagination,
			"tags":       tags,
			"notes":      notes,
			"groups":     groups,
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

		if resource.OwnerId != 0 {
			ownerEntity, err := context.GetGroup(resource.OwnerId)

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
