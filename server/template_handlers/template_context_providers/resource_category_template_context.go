package template_context_providers

import (
	"fmt"
	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers/template_entities"
	"net/http"
	"strconv"
)

func ResourceCategoryListContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		offset := (page - 1) * constants.MaxResultsPerPage
		var query query_models.ResourceCategoryQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		resourceCategories, err := context.GetResourceCategories(int(offset), constants.MaxResultsPerPage, &query)

		if err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		resourceCategoriesCount, err := context.GetResourceCategoriesCount(&query)

		if err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), resourceCategoriesCount, constants.MaxResultsPerPage, int(page))

		if err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle":          "Resource Categories",
			"resourceCategories": resourceCategories,
			"pagination":         pagination,
			"action": template_entities.Entry{
				Name: "Add",
				Url:  "/resourceCategory/new",
			},
		}.Update(baseContext)
	}
}

func ResourceCategoryCreateContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		tplContext := pongo2.Context{
			"pageTitle": "Create Resource Category",
		}.Update(staticTemplateCtx(request))

		var query query_models.EntityIdQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			return addErrContext(err, tplContext)
		}

		if query.ID == 0 {
			return tplContext
		}

		resourceCategory, err := context.GetResourceCategory(query.ID)

		if err != nil {
			return tplContext
		}

		tplContext["pageTitle"] = "Edit Resource Category"
		tplContext["resourceCategory"] = resourceCategory

		return tplContext
	}
}

func ResourceCategoryContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		resourceCategory, err := context.GetResourceCategory(query.ID)

		if err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		// Fetch resources in this category with pagination
		resourcePage := http_utils.GetIntQueryParameter(request, "resourcePage", 1)
		resourceOffset := (resourcePage - 1) * constants.MaxResultsPerPage
		resourceQuery := &query_models.ResourceSearchQuery{
			ResourceCategoryId: query.ID,
		}

		resources, err := context.GetResources(int(resourceOffset), constants.MaxResultsPerPage, resourceQuery)
		if err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle":        "Resource Category: " + resourceCategory.Name,
			"resourceCategory": resourceCategory,
			"resources":        resources,
			"action": template_entities.Entry{
				Name: "Edit",
				Url:  "/resourceCategory/edit?id=" + strconv.Itoa(int(query.ID)),
			},
			"deleteAction": template_entities.Entry{
				Name: "Delete",
				Url:  "/v1/resourceCategory/delete",
				ID:   resourceCategory.ID,
			},
			"mainEntity":     resourceCategory,
			"mainEntityType": "resourceCategory",
		}.Update(baseContext)
	}
}
