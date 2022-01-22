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

func GroupsListContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		offset := (page - 1) * constants.MaxResultsPerPage
		var query query_models.GroupQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		groups, err := context.GetGroups(int(offset), constants.MaxResultsPerPage, &query)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		groupsCount, err := context.GetGroupsCount(&query)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), groupsCount, constants.MaxResultsPerPage, int(page))

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		tags, err := context.GetTagsWithIds(&query.Tags, 0)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}
		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		notes, err := context.GetNotesWithIds(&query.Notes)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		resources, err := context.GetResourcesWithIds(&query.Resources)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		categories, err := context.GetCategoriesWithIds(&query.Categories, 0)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		groupsSelection, err := context.GetGroupsWithIds(&query.Groups)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		owners, err := context.GetGroupsWithIds(&[]uint{query.OwnerId})

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle":       "Groups",
			"groups":          groups,
			"owners":          owners,
			"groupsSelection": groupsSelection,
			"categories":      categories,
			"pagination":      pagination,
			"tags":            tags,
			"notes":           notes,
			"resources":       resources,
			"parsedQuery":     query,
			"action": template_entities.Entry{
				Name: "Add",
				Url:  "/group/new",
			},
			"sortValues": createSortCols([]SortColumn{
				{Name: "Created", Value: "created_at"},
				{Name: "Name", Value: "name"},
				{Name: "Updated", Value: "updated_at"},
			}, query.SortBy),
		}.Update(baseContext)
	}
}

func GroupCreateContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		tplContext := pongo2.Context{
			"pageTitle": "Add New Group",
		}.Update(staticTemplateCtx(request))

		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())

		if err != nil || query.ID == 0 {
			var groupQuery query_models.GroupQuery
			err := decoder.Decode(&groupQuery, request.URL.Query())

			if err == nil {
				tplContext["group"] = groupQuery

				tags, _ := context.GetTagsWithIds(&groupQuery.Tags, 0)
				groups, _ := context.GetGroupsWithIds(&groupQuery.Groups)

				if groupQuery.CategoryId != 0 {
					category, _ := context.GetCategoriesWithIds(&[]uint{groupQuery.CategoryId}, 0)
					tplContext["category"] = category
				}

				if groupQuery.OwnerId != 0 {
					owner, _ := context.GetGroup(groupQuery.OwnerId)
					tplContext["owner"] = []*models.Group{owner}
				}

				tplContext["tags"] = tags
				tplContext["groups"] = groups
			}

			return tplContext
		}

		group, err := context.GetGroup(query.ID)

		if err != nil {
			return addErrContext(err, tplContext)
		}

		tplContext["group"] = group
		tplContext["pageTitle"] = "Edit Group"
		tplContext["tags"] = &group.Tags
		tplContext["groups"] = &group.RelatedGroups

		if group.Owner != nil {
			tplContext["owner"] = []*models.Group{group.Owner}
		}

		return tplContext
	}
}

func GroupContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		group, err := context.GetGroup(query.ID)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		parents, err := context.FindParentsOfGroup(group.ID)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		breadcrumbEls := make([]template_entities.Entry, len(*parents))

		for i, m := range *parents {
			breadcrumbEls[i] = template_entities.Entry{
				Name: m.Name,
				ID:   m.ID,
				Url:  fmt.Sprintf("/group?id=%v", m.ID),
			}
		}

		return pongo2.Context{
			"pageTitle": group.GetName(),
			"prefix":    group.Category.Name,
			"group":     group,
			"action": template_entities.Entry{
				Name: "Edit",
				Url:  "/group/edit?id=" + strconv.Itoa(int(query.ID)),
			},
			"deleteAction": template_entities.Entry{
				Name: "Delete",
				Url:  fmt.Sprintf("/v1/group/delete?Id=%v", group.ID),
			},
			"mainEntity": group,
			"breadcrumb": pongo2.Context{
				"HomeName": "Groups",
				"HomeUrl":  "groups",
				"Entries":  breadcrumbEls,
			},
		}.Update(baseContext)
	}
}
