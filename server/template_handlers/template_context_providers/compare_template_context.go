package template_context_providers

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/models"
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
				"query":        query,
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

		// Redirect to appropriate versions if versions are missing and comparing different resources.
		// For cross-resource comparison, use the current version (matching CurrentVersionID)
		// rather than the latest version number, since transferred merge versions may have
		// higher numbers but represent different files.
		if query.Resource1ID != query.Resource2ID && (query.Version1 == 0 || query.Version2 == 0) {
			v1 := query.Version1
			v2 := query.Version2

			if v1 == 0 {
				v1 = currentVersionNumber(resource1, versions1)
			}
			if v2 == 0 {
				v2 = currentVersionNumber(resource2, versions2)
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
		var comparison *models.VersionComparison
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

		// Determine if merge is available (cross-resource, both at current versions)
		crossResource := query.Resource1ID != query.Resource2ID
		canMerge := false
		if crossResource {
			cv1 := currentVersionNumber(resource1, versions1)
			cv2 := currentVersionNumber(resource2, versions2)
			canMerge = cv1 > 0 && cv2 > 0 && query.Version1 == cv1 && query.Version2 == cv2
		}

		// Compute side labels: version-aware for same resource, Left/Right for cross-resource
		label1, label2 := "Left", "Right"
		if !crossResource && query.Version1 > 0 && query.Version2 > 0 {
			if query.Version1 == query.Version2 {
				label1 = fmt.Sprintf("v%d", query.Version1)
				label2 = fmt.Sprintf("v%d", query.Version2)
			} else {
				curVer := currentVersionNumber(resource1, versions1)
				v1IsCurrent := query.Version1 == curVer
				v2IsCurrent := query.Version2 == curVer

				switch {
				case v1IsCurrent:
					label1 = "Current"
					label2 = fmt.Sprintf("v%d", query.Version2)
				case v2IsCurrent:
					label1 = fmt.Sprintf("v%d", query.Version1)
					label2 = "Current"
				default:
					if query.Version1 > query.Version2 {
						label1 = "Newer"
						label2 = "Older"
					} else {
						label1 = "Older"
						label2 = "Newer"
					}
				}
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
			"crossResource":   crossResource,
			"canMerge":        canMerge,
			"label1":          label1,
			"label2":          label2,
		})
	}
}

// currentVersionNumber returns the version number that matches the resource's
// CurrentVersionID. Falls back to the latest version number if CurrentVersionID
// is not set or not found in the versions list.
func currentVersionNumber(resource *models.Resource, versions []models.ResourceVersion) int {
	if resource.CurrentVersionID != nil {
		for _, v := range versions {
			if v.ID == *resource.CurrentVersionID {
				return v.VersionNumber
			}
		}
	}
	if len(versions) > 0 {
		return versions[0].VersionNumber
	}
	return 0
}
