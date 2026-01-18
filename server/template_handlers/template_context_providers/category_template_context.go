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

func CategoryListContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		offset := (page - 1) * constants.MaxResultsPerPage
		var query query_models.CategoryQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		categories, err := context.GetCategories(int(offset), constants.MaxResultsPerPage, &query)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		categoriesCount, err := context.GetCategoriesCount(&query)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), categoriesCount, constants.MaxResultsPerPage, int(page))

		if err != nil {
			fmt.Println(err)

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
		}.Update(baseContext)
	}
}

func CategoryCreateContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		tplContext := pongo2.Context{
			"pageTitle": "Create Category",
		}.Update(staticTemplateCtx(request))

		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())

		category, err := context.GetCategory(query.ID)

		if err != nil {
			return tplContext
		}

		tplContext["pageTitle"] = "Edit Category"
		tplContext["category"] = category

		return tplContext
	}
}

func CategoryContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		category, err := context.GetCategory(query.ID)

		if err != nil {
			fmt.Println(err)

			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle": "Category " + category.Name,
			"category":  category,
			"action": template_entities.Entry{
				Name: "Edit",
				Url:  "/category/edit?id=" + strconv.Itoa(int(query.ID)),
			},
			"deleteAction": template_entities.Entry{
				Name: "Delete",
				Url:  "/v1/category/delete",
				ID:   category.ID,
			},
			"mainEntity":     category,
			"mainEntityType": "category",
		}.Update(baseContext)
	}
}
