package template_context_providers

import (
	"encoding/json"
	"fmt"
	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/interfaces"
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
			return addErrContext(err, baseContext)
		}

		groups, err := context.GetGroups(int(offset), constants.MaxResultsPerPage, &query)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		groupsCount, err := context.GetGroupsCount(&query)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), groupsCount, constants.MaxResultsPerPage, int(page))

		if err != nil {
			return addErrContext(err, baseContext)
		}

		tags, err := context.GetTagsWithIds(&query.Tags, 0)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		notes, err := context.GetNotesWithIds(&query.Notes)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		resources, err := context.GetResourcesWithIds(&query.Resources)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		categories, err := context.GetCategoriesWithIds(&query.Categories, 0)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		groupsSelection, err := context.GetGroupsWithIds(&query.Groups)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		owners, err := context.GetGroupsWithIds(&[]uint{query.OwnerId})

		if err != nil {
			return addErrContext(err, baseContext)
		}

		popularTags, err := context.GetPopularGroupTags(&query)

		if err != nil {
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
			"popularTags":     popularTags,
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
			"displayOptions": getPathExtensionOptions(request.URL, &[]*SelectOption{
				{Title: "List", Link: "/groups"},
				{Title: "Text", Link: "/groups/text"},
				{Title: "Tree", Link: "/group/tree"},
			}),
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

		if group.CategoryId != nil {
			tplContext["category"] = []*models.Category{group.Category}
		}

		if group.Owner != nil {
			tplContext["owner"] = []*models.Group{group.Owner}
		}

		return tplContext
	}
}

func GroupContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return groupContextProviderImpl(context)
}

func groupContextProviderImpl(context interfaces.GroupReader) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		group, err := context.GetGroup(query.ID)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		parents, err := context.FindParentsOfGroup(group.ID)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		breadcrumbEls := make([]template_entities.Entry, len(parents))

		for i, m := range parents {
			breadcrumbEls[i] = template_entities.Entry{
				Name: m.Name,
				ID:   m.ID,
				Url:  fmt.Sprintf("/group?id=%v", m.ID),
			}
		}

		var prefix string

		if group.Category != nil {
			prefix = group.Category.Name
		} else {
			prefix = "Uncategorized"
		}

		return pongo2.Context{
			"pageTitle": group.GetName(),
			"prefix":    prefix,
			"group":     group,
			"action": template_entities.Entry{
				Name: "Edit",
				Url:  "/group/edit?id=" + strconv.Itoa(int(query.ID)),
			},
			"deleteAction": template_entities.Entry{
				Name: "Delete",
				Url:  fmt.Sprintf("/v1/group/delete?Id=%v", group.ID),
			},
			"mainEntity":     group,
			"mainEntityType": "group",
			"breadcrumb": pongo2.Context{
				"HomeName": "Groups",
				"HomeUrl":  "groups",
				"Entries":  breadcrumbEls,
			},
		}.Update(baseContext)
	}
}

func GroupTreeContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		baseContext := staticTemplateCtx(request)

		rootID := http_utils.GetUIntQueryParameter(request, "root", 0)
		containingID := http_utils.GetUIntQueryParameter(request, "containing", 0)

		tplContext := pongo2.Context{
			"pageTitle": "Group Tree",
		}

		// If containing= is set, find the root ancestor and build a highlighted path
		var highlightedPath []uint
		if containingID > 0 {
			parents, err := context.FindParentsOfGroup(containingID)
			if err != nil {
				return addErrContext(err, baseContext).Update(tplContext)
			}

			if len(parents) > 0 {
				// parents is ordered [root, ..., parent, self]
				rootID = parents[0].ID
				highlightedPath = make([]uint, len(parents))
				for i, p := range parents {
					highlightedPath[i] = p.ID
				}
			}
		}

		// No root specified: show list of root groups
		if rootID == 0 {
			roots, err := context.GetGroupTreeRoots(50)
			if err != nil {
				return addErrContext(err, baseContext).Update(tplContext)
			}

			tplContext["roots"] = roots
			return tplContext.Update(baseContext)
		}

		// Fetch initial tree (3 levels deep)
		treeRows, err := context.GetGroupTreeDown(rootID, 3, 50)
		if err != nil {
			return addErrContext(err, baseContext).Update(tplContext)
		}

		treeRowsJSON, err := json.Marshal(treeRows)
		if err != nil {
			return addErrContext(err, baseContext).Update(tplContext)
		}

		highlightedPathJSON, err := json.Marshal(highlightedPath)
		if err != nil {
			return addErrContext(err, baseContext).Update(tplContext)
		}

		// Get root group name for the page title
		rootGroup, err := context.GetGroup(rootID)
		if err != nil {
			return addErrContext(err, baseContext).Update(tplContext)
		}

		tplContext["pageTitle"] = "Tree: " + rootGroup.GetName()
		tplContext["rootId"] = rootID
		tplContext["containingId"] = containingID
		tplContext["treeRowsJSON"] = string(treeRowsJSON)
		tplContext["highlightedPathJSON"] = string(highlightedPathJSON)
		tplContext["rootGroup"] = rootGroup
		tplContext["breadcrumb"] = pongo2.Context{
			"HomeName": "Groups",
			"HomeUrl":  "groups",
			"Entries": []template_entities.Entry{
				{Name: rootGroup.GetName(), ID: rootGroup.ID, Url: fmt.Sprintf("/group?id=%v", rootGroup.ID)},
			},
		}

		return tplContext.Update(baseContext)
	}
}
