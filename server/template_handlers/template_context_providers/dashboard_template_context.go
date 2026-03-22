package template_context_providers

import (
	"log"
	"net/http"
	"sync"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
)

const dashboardItemsPerSection = 6
const dashboardActivityLimit = 20

func DashboardContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		baseContext := staticTemplateCtx(request)

		var (
			recentResources []models.Resource
			recentNotes     []models.Note
			recentGroups    []models.Group
			recentTags      []models.Tag
			activityFeed    []application_context.ActivityEntry

			resErr, notesErr, groupsErr, tagsErr, actErr error
		)

		var wg sync.WaitGroup
		wg.Add(5)

		go func() {
			defer wg.Done()
			recentResources, resErr = context.GetResources(0, dashboardItemsPerSection, &query_models.ResourceSearchQuery{
				SortBy: []string{"created_at desc"},
			})
			if resErr != nil {
				log.Printf("dashboard: failed to load resources: %v", resErr)
			}
		}()

		go func() {
			defer wg.Done()
			recentNotes, notesErr = context.GetNotes(0, dashboardItemsPerSection, &query_models.NoteQuery{
				SortBy: []string{"created_at desc"},
			})
			if notesErr != nil {
				log.Printf("dashboard: failed to load notes: %v", notesErr)
			}
		}()

		go func() {
			defer wg.Done()
			recentGroups, groupsErr = context.GetGroups(0, dashboardItemsPerSection, &query_models.GroupQuery{
				SortBy: []string{"created_at desc"},
			})
			if groupsErr != nil {
				log.Printf("dashboard: failed to load groups: %v", groupsErr)
			}
		}()

		go func() {
			defer wg.Done()
			recentTags, tagsErr = context.GetTags(0, dashboardItemsPerSection, &query_models.TagQuery{
				SortBy: []string{"created_at desc"},
			})
			if tagsErr != nil {
				log.Printf("dashboard: failed to load tags: %v", tagsErr)
			}
		}()

		go func() {
			defer wg.Done()
			activityFeed, actErr = context.GetRecentActivity(dashboardActivityLimit)
			if actErr != nil {
				log.Printf("dashboard: failed to load activity feed: %v", actErr)
			}
		}()

		wg.Wait()

		// Surface error to template only if all queries failed
		if resErr != nil && notesErr != nil && groupsErr != nil && tagsErr != nil && actErr != nil {
			return addErrContext(resErr, baseContext)
		}

		return pongo2.Context{
			"pageTitle":       "Dashboard",
			"hideSidebar": true,
			"recentResources": recentResources,
			"recentNotes":     recentNotes,
			"recentGroups":    recentGroups,
			"recentTags":      recentTags,
			"activityFeed":    activityFeed,
		}.Update(baseContext)
	}
}
