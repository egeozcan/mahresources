package template_context_providers

import (
	"net/http"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/models/query_models"
)

const dashboardItemsPerSection = 6
const dashboardActivityLimit = 20

func DashboardContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		baseContext := staticTemplateCtx(request)

		recentResourcesQuery := &query_models.ResourceSearchQuery{
			SortBy: []string{"created_at desc"},
		}
		recentResources, _ := context.GetResources(0, dashboardItemsPerSection, recentResourcesQuery)

		recentNotesQuery := &query_models.NoteQuery{
			SortBy: []string{"created_at desc"},
		}
		recentNotes, _ := context.GetNotes(0, dashboardItemsPerSection, recentNotesQuery)

		recentGroupsQuery := &query_models.GroupQuery{
			SortBy: []string{"created_at desc"},
		}
		recentGroups, _ := context.GetGroups(0, dashboardItemsPerSection, recentGroupsQuery)

		recentTagsQuery := &query_models.TagQuery{
			SortBy: []string{"created_at desc"},
		}
		recentTags, _ := context.GetTags(0, dashboardItemsPerSection, recentTagsQuery)

		activityFeed, _ := context.GetRecentActivity(dashboardActivityLimit)

		return pongo2.Context{
			"pageTitle":       "Dashboard",
			"recentResources": recentResources,
			"recentNotes":     recentNotes,
			"recentGroups":    recentGroups,
			"recentTags":      recentTags,
			"activityFeed":    activityFeed,
		}.Update(baseContext)
	}
}
