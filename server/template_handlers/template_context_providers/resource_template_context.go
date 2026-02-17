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
	"strings"
)

func ResourceListContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		var resultsPerPage = constants.MaxResultsPerPage

		simpleMode := strings.HasSuffix(request.URL.Path, "/simple")

		if simpleMode {
			resultsPerPage = constants.MaxResultsPerPage * 4
		}

		// Allow custom pageSize via URL parameter (bounded between 1 and 200)
		if customPageSize := http_utils.GetIntQueryParameter(request, "pageSize", 0); customPageSize > 0 {
			if customPageSize > 200 {
				customPageSize = 200
			}
			resultsPerPage = int(customPageSize)
		}

		offset := (page - 1) * int64(resultsPerPage)
		var query query_models.ResourceSearchQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		resources, err := context.GetResources(int(offset), resultsPerPage, &query)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		resourceCount, err := context.GetResourceCount(&query)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), resourceCount, resultsPerPage, int(page))

		if err != nil {
			return addErrContext(err, baseContext)
		}

		tags, err := context.GetTagsWithIds(&query.Tags, 0)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		notes, _ := context.GetNotesWithIds(&query.Notes)
		groups, _ := context.GetGroupsWithIds(&query.Groups)

		owner := make([]*models.Group, 0)
		if query.OwnerId != 0 {
			ownerEntity, err := context.GetGroup(query.OwnerId)

			if err == nil {
				owner = []*models.Group{ownerEntity}
			}
		}

		var selectedResourceCategory []*models.ResourceCategory
		if query.ResourceCategoryId != 0 {
			rc, err := context.GetResourceCategory(query.ResourceCategoryId)
			if err == nil {
				selectedResourceCategory = []*models.ResourceCategory{rc}
			}
		}

		popularTags, err := context.GetPopularResourceTags(&query)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle":                "Resources",
			"resources":                resources,
			"pagination":               pagination,
			"tags":                     tags,
			"popularTags":              popularTags,
			"notes":                    notes,
			"owner":                    owner,
			"groups":                   groups,
			"selectedResourceCategory": selectedResourceCategory,
			"parsedQuery":              query,
			"simpleMode":               simpleMode,
			"action": template_entities.Entry{
				Name: "Create",
				Url:  "/resource/new",
			},
			"sortValues": createSortCols([]SortColumn{
				{Name: "Created", Value: "created_at"},
				{Name: "Name", Value: "name"},
				{Name: "Updated", Value: "updated_at"},
				{Name: "Size", Value: "file_size"},
			}, query.SortBy),
			"displayOptions": getPathExtensionOptions(request.URL, &[]*SelectOption{
				{Title: "Thumbnails", Link: "/resources"},
				{Title: "Details", Link: "/resources/details"},
				{Title: "Simple", Link: "/resources/simple"},
			}),
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
			var resourceTpl query_models.ResourceSearchQuery
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

		if resource.ResourceCategoryId != nil {
			resourceCategory, err := context.GetResourceCategory(*resource.ResourceCategoryId)
			if err == nil {
				tplContext["resourceCategories"] = &[]*models.ResourceCategory{resourceCategory}
			}
		}

		if resource.SeriesID != nil && resource.Series != nil {
			tplContext["series"] = []*models.Series{resource.Series}
		}

		return tplContext
	}
}

func ResourceContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query query_models.EntityIdQuery

		baseContext := staticTemplateCtx(request)

		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			return addErrContext(err, baseContext)
		}

		similarResources, err := context.GetSimilarResources(query.ID)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		resource, err := context.GetResource(query.ID)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		versions, _ := context.GetVersions(resource.ID)

		var seriesSiblings []*models.Resource
		if resource.SeriesID != nil {
			seriesSiblings, _ = context.GetSeriesSiblings(resource.ID, *resource.SeriesID)
		}

		result := pongo2.Context{
			"pageTitle":        "Resource " + resource.Name,
			"resource":         resource,
			"versions":         versions,
			"seriesSiblings":   seriesSiblings,
			"similarResources": similarResources,
			"action": template_entities.Entry{
				Name: "Edit",
				Url:  "/resource/edit?id=" + strconv.Itoa(int(query.ID)),
			},
			"deleteAction": template_entities.Entry{
				Name: "Delete",
				Url:  "/v1/resource/delete",
				ID:   resource.ID,
			},
			"isImage":        resource.IsImage(),
			"mainEntity":     resource,
			"mainEntityType": "resource",
		}

		if resource.OwnerId != nil {
			parents, err := context.FindParentsOfGroup(*resource.OwnerId)

			if err != nil {
				return addErrContext(err, baseContext)
			}

			breadcrumbEls := make([]template_entities.Entry, len(parents)+1)

			for i, m := range parents {
				breadcrumbEls[i] = template_entities.Entry{
					Name: m.Name,
					ID:   m.ID,
					Url:  fmt.Sprintf("/group?id=%v", m.ID),
				}
			}

			breadcrumbEls[len(parents)] = template_entities.Entry{
				Name: resource.Name,
				ID:   resource.ID,
				Url:  fmt.Sprintf("/resource?id=%v", resource.ID),
			}

			result["breadcrumb"] = pongo2.Context{
				"HomeName": "Groups",
				"HomeUrl":  "groups",
				"Entries":  breadcrumbEls,
			}
		}

		return result.Update(baseContext)
	}
}
