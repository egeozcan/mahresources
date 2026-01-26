package template_context_providers

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/models/query_models"
	"net/http"
)

func CompareContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		baseContext := staticTemplateCtx(request)

		var query query_models.CrossVersionCompareQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			return addErrContext(err, baseContext)
		}

		// Validate required params
		if query.Resource1ID == 0 {
			return baseContext.Update(pongo2.Context{
				"pageTitle":    "Compare Versions",
				"errorMessage": "Resource 1 ID (r1) is required",
			})
		}

		// Default r2 to r1 if not provided
		if query.Resource2ID == 0 {
			query.Resource2ID = query.Resource1ID
		}

		// Get resource 1 and its versions for the picker
		resource1, err := context.GetResource(query.Resource1ID)
		if err != nil {
			return addErrContext(err, baseContext)
		}
		versions1, _ := context.GetVersions(query.Resource1ID)

		// Get resource 2 and its versions
		resource2, err := context.GetResource(query.Resource2ID)
		if err != nil {
			return addErrContext(err, baseContext)
		}
		versions2, _ := context.GetVersions(query.Resource2ID)

		// Redirect to latest versions if versions are missing and comparing different resources
		if query.Resource1ID != query.Resource2ID && (query.Version1 == 0 || query.Version2 == 0) {
			v1 := query.Version1
			v2 := query.Version2

			if v1 == 0 && len(versions1) > 0 {
				v1 = versions1[0].VersionNumber
			}
			if v2 == 0 && len(versions2) > 0 {
				v2 = versions2[0].VersionNumber
			}

			// Only redirect if we found both versions
			if v1 > 0 && v2 > 0 {
				redirectURL := fmt.Sprintf("/resource/compare?%s", url.Values{
					"r1": {fmt.Sprintf("%d", query.Resource1ID)},
					"v1": {fmt.Sprintf("%d", v1)},
					"r2": {fmt.Sprintf("%d", query.Resource2ID)},
					"v2": {fmt.Sprintf("%d", v2)},
				}.Encode())
				return baseContext.Update(pongo2.Context{
					"_redirect": redirectURL,
				})
			}
		}

		// Perform comparison if both versions specified
		var comparison *application_context.VersionComparison
		if query.Version1 > 0 && query.Version2 > 0 {
			comparison, err = context.CompareVersionsCross(
				query.Resource1ID, query.Version1,
				query.Resource2ID, query.Version2,
			)
			if err != nil {
				return addErrContext(err, baseContext)
			}
		}

		// Determine content type category for UI rendering
		contentCategory := "binary"
		if comparison != nil && comparison.Version1 != nil {
			ct := comparison.Version1.ContentType
			if strings.HasPrefix(ct, "image/") {
				contentCategory = "image"
			} else if strings.HasPrefix(ct, "text/") || ct == "application/json" || ct == "application/xml" {
				contentCategory = "text"
			} else if ct == "application/pdf" {
				contentCategory = "pdf"
			}
		}

		return baseContext.Update(pongo2.Context{
			"pageTitle":       "Compare Versions",
			"resource1":       resource1,
			"resource2":       resource2,
			"versions1":       versions1,
			"versions2":       versions2,
			"comparison":      comparison,
			"query":           query,
			"contentCategory": contentCategory,
			"crossResource":   query.Resource1ID != query.Resource2ID,
		})
	}
}
