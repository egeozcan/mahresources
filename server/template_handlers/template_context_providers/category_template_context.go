package template_context_providers

import (
	"fmt"

	"github.com/flosch/pongo2/v4"
	"mahresources/constants"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers/template_entities"
	"net/http"
	"strconv"
)

func CategoryListContextProvider(context CategoryPageContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetPageParameter(request)
		resultsPerPage := getResultsPerPage(request, constants.MaxResultsPerPage)
		offset := (page - 1) * int64(resultsPerPage)
		var query query_models.CategoryQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := StaticTemplateCtx(request)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		categories, err := context.GetCategories(int(offset), resultsPerPage, &query)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		categoriesCount, err := context.GetCategoriesCount(&query)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		// WS6 68/146: an out-of-range ?page= redirects to the last real page
		// instead of rendering blank under a footer that offers pages it has.
		if redirect := outOfRangePageRedirect(request, categoriesCount, resultsPerPage, page); redirect != nil {
			return redirect
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), categoriesCount, resultsPerPage, int(page))

		if err != nil {
			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle":  "Categories",
			"categories": categories,
			"pagination": pagination,
			"action": template_entities.Entry{
				Name: "Add",
				Url:  "/category/new",
			},
			"sortValues": createSortCols([]SortColumn{
				{Name: "Created", Value: "created_at"},
				{Name: "Name", Value: "name"},
				{Name: "Updated", Value: "updated_at"},
			}, query.SortBy),
			"displayOptions": getPathExtensionOptions(request.URL, &[]*SelectOption{
				{Title: "List", Link: "/categories"},
				{Title: "Timeline", Link: "/categories/timeline"},
			}),
		}.Update(baseContext)
	}
}

func CategoryTimelineContextProvider(context CategoryPageContext) func(request *http.Request) pongo2.Context {
	listProvider := CategoryListContextProvider(context)
	return func(request *http.Request) pongo2.Context {
		ctx := listProvider(request)
		ctx["pageTitle"] = "Categories - Timeline"
		return ctx
	}
}

func CategoryCreateContextProvider(context CategoryPageContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		tplContext := pongo2.Context{
			"pageTitle": "Create Category",
		}.Update(StaticTemplateCtx(request))

		var query query_models.EntityIdQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			return addErrContext(err, tplContext)
		}

		if query.ID == 0 {
			return tplContext
		}

		category, err := context.GetCategory(query.ID)

		if err != nil {
			return addErrContext(err, tplContext)
		}

		tplContext["pageTitle"] = "Edit Category"
		tplContext["category"] = category

		return tplContext
	}
}

func CategoryContextProvider(context CategoryPageContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := StaticTemplateCtx(request)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		category, err := context.GetCategory(query.ID)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		// Finding 98: a category still in use was deleted behind the generic
		// "Are you sure you want to delete?" and its groups silently fell back to
		// Uncategorized. A count query rather than len(category.Groups), which is
		// a preloaded association capped at pageLimit.
		groupsInUse, err := context.GetGroupsCount(&query_models.GroupQuery{CategoryId: category.ID})
		if err != nil {
			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle":   "Category: " + category.Name,
			"prefix":      "Category",
			"category":    category,
			"groupsInUse": groupsInUse,
			"action": template_entities.Entry{
				Name: "Edit",
				Url:  "/category/edit?id=" + strconv.Itoa(int(query.ID)),
			},
			"deleteAction": template_entities.Entry{
				Name:    "Delete",
				Url:     "/v1/category/delete",
				ID:      category.ID,
				Confirm: categoryDeleteConfirm(category.Name, groupsInUse),
			},
			"mainEntity":     category,
			"mainEntityType": "category",
		}.Update(baseContext)
	}
}

// categoryDeleteConfirm states what deleting a category destroys. Finding 98:
// the groups keep existing, they just lose their category, and nothing said so
// at any point — before or after. Singular/plural because "1 groups" reads as a
// bug in the very message that is meant to be trusted.
func categoryDeleteConfirm(name string, groups int64) string {
	if groups == 0 {
		return fmt.Sprintf("Delete category '%s'?", name)
	}
	noun := "groups"
	if groups == 1 {
		noun = "group"
	}
	return fmt.Sprintf("Delete category '%s'? %d %s will become Uncategorized.", name, groups, noun)
}
