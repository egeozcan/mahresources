package template_context_providers

import (
	"github.com/flosch/pongo2/v4"
	"mahresources/models"
	"mahresources/models/query_models"
	"net/http"
	"strconv"
)

// TemplateRevisionsContextProvider serves the History page of one Template
// Carrier type. The page lists revisions and diffs them by calling the
// carrier's revision API, so this only resolves the carrier and names the URLs.
func TemplateRevisionsContextProvider(carrierType string) func(TemplateRevisionsPageContext) func(request *http.Request) pongo2.Context {
	return func(context TemplateRevisionsPageContext) func(request *http.Request) pongo2.Context {
		return func(request *http.Request) pongo2.Context {
			baseContext := StaticTemplateCtx(request)
			var query query_models.EntityIdQuery
			if err := decoder.Decode(&query, request.URL.Query()); err != nil {
				return addErrContext(err, baseContext)
			}

			var (
				name, noun, pagePrefix, apiPrefix string
				err                               error
			)
			switch carrierType {
			case models.TemplateCarrierCategory:
				var c *models.Category
				if c, err = context.GetCategory(query.ID); err == nil {
					name = c.Name
				}
				noun, pagePrefix, apiPrefix = "Category", "/category", "/v1/category"
			case models.TemplateCarrierResourceCategory:
				var c *models.ResourceCategory
				if c, err = context.GetResourceCategory(query.ID); err == nil {
					name = c.Name
				}
				noun, pagePrefix, apiPrefix = "Resource Category", "/resourceCategory", "/v1/resourceCategory"
			case models.TemplateCarrierNoteType:
				var n *models.NoteType
				if n, err = context.GetNoteType(query.ID); err == nil {
					name = n.Name
				}
				noun, pagePrefix, apiPrefix = "Note Type", "/noteType", "/v1/noteType"
			}
			if err != nil {
				return addErrContext(err, baseContext)
			}

			id := strconv.Itoa(int(query.ID))
			return pongo2.Context{
				"pageTitle":    "Template history: " + name,
				"headingTitle": name,
				"carrierType":  carrierType,
				"carrierNoun":  noun,
				"carrierName":  name,
				"carrierID":    query.ID,
				"apiPrefix":    apiPrefix,
				"displayUrl":   pagePrefix + "?id=" + id,
				"editUrl":      pagePrefix + "/edit?id=" + id,
				"prefix":       noun,
			}.Update(baseContext)
		}
	}
}
