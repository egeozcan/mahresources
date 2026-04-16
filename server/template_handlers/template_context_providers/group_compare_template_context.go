package template_context_providers

import (
	"net/http"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/interfaces"
)

func GroupCompareContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return groupCompareContextProviderImpl(context)
}

func groupCompareContextProviderImpl(context interfaces.GroupComparer) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		baseContext := StaticTemplateCtx(request)

		var query query_models.CrossGroupCompareQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			return addErrContext(err, baseContext)
		}

		if query.Group1ID == 0 {
			return baseContext.Update(pongo2.Context{
				"pageTitle":    "Compare Groups",
				"errorMessage": "Group 1 ID (g1) is required",
				"query":        query,
			})
		}

		if query.Group2ID == 0 {
			query.Group2ID = query.Group1ID
		}

		comparison, err := context.CompareGroupsCross(query.Group1ID, query.Group2ID)
		if err != nil {
			return addErrContext(err, baseContext)
		}

		return baseContext.Update(pongo2.Context{
			"pageTitle":    "Compare Groups",
			"comparison":   comparison,
			"query":        query,
			"group1Picker": buildGroupComparePicker(comparison.Group1),
			"group2Picker": buildGroupComparePicker(comparison.Group2),
			"label1":       "Left",
			"label2":       "Right",
		})
	}
}

func buildGroupComparePicker(group *models.Group) map[string]any {
	if group == nil {
		return map[string]any{}
	}

	item := map[string]any{
		"ID":   group.ID,
		"Name": group.Name,
	}
	if group.Category != nil {
		item["Category"] = map[string]any{
			"Name": group.Category.Name,
		}
	}
	return item
}
