package template_context_providers

import (
	"log"
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
		recentResources, resErr := context.GetResources(0, dashboardItemsPerSection, recentResourcesQuery)
		if resErr != nil {
			log.Printf("dashboard: failed to load resources: %v", resErr)
		}

		recentNotesQuery := &query_models.NoteQuery{
			SortBy: []string{"created_at desc"},
		}
		recentNotes, notesErr := context.GetNotes(0, dashboardItemsPerSection, recentNotesQuery)
		if notesErr != nil {
			log.Printf("dashboard: failed to load notes: %v", notesErr)
		}

		recentGroupsQuery := &query_models.GroupQuery{
			SortBy: []string{"created_at desc"},
		}
		recentGroups, groupsErr := context.GetGroups(0, dashboardItemsPerSection, recentGroupsQuery)
		if groupsErr != nil {
			log.Printf("dashboard: failed to load groups: %v", groupsErr)
		}

		recentTagsQuery := &query_models.TagQuery{
			SortBy: []string{"created_at desc"},
		}
		recentTags, tagsErr := context.GetTags(0, dashboardItemsPerSection, recentTagsQuery)
		if tagsErr != nil {
			log.Printf("dashboard: failed to load tags: %v", tagsErr)
		}

		activityFeed, actErr := context.GetRecentActivity(dashboardActivityLimit)
		if actErr != nil {
			log.Printf("dashboard: failed to load activity feed: %v", actErr)
		}

		// Surface error to template only if all queries failed
		if resErr != nil && notesErr != nil && groupsErr != nil && tagsErr != nil && actErr != nil {
			return addErrContext(resErr, baseContext)
		}

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
