package template_context_providers

import (
	"mahresources/application_context"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/mrql"
	"mahresources/plugin_system"
)

// The capability each template page needs from the business layer, one
// interface per page area.
//
// Before this, all 61 providers took *application_context.MahresourcesContext
// and could reach any of its 329 exported methods. Now each declares its own
// surface, so what a page touches is visible in its signature and the compiler
// enforces it.
//
// These live here rather than in contracts/ for the same reason
// api_handlers.GroupImporter does: contracts/ may import only models/ and
// constants/ (enforced by internal/arch), and several of these methods return
// application_context-owned types — RuntimeSettings, PopularTag, ActivityEntry,
// MRQLFilterError — or plugin_system.PluginManager. Moving those DTOs into
// contracts/ would let most of these move too; that is a separate change.
//
// Existing contracts/ interfaces are embedded wherever one already describes
// the capability, rather than restating its methods.

// Compile-time proof that MahresourcesContext satisfies every one of these.
// The generic adapter in routes.go type-asserts on them at request time; these
// assertions are what make that assertion unreachable.
var (
	_ AccountPageContext          = (*application_context.MahresourcesContext)(nil)
	_ AdminSettingsPageContext    = (*application_context.MahresourcesContext)(nil)
	_ AdminSharesPageContext      = (*application_context.MahresourcesContext)(nil)
	_ CategoryPageContext         = (*application_context.MahresourcesContext)(nil)
	_ ComparePageContext          = (*application_context.MahresourcesContext)(nil)
	_ DashboardPageContext        = (*application_context.MahresourcesContext)(nil)
	_ GroupComparePageContext     = (*application_context.MahresourcesContext)(nil)
	_ GroupPageContext            = (*application_context.MahresourcesContext)(nil)
	_ HoverCardPageContext        = (*application_context.MahresourcesContext)(nil)
	_ MRQLPageContext             = (*application_context.MahresourcesContext)(nil)
	_ NotePageContext             = (*application_context.MahresourcesContext)(nil)
	_ PluginManagePageContext     = (*application_context.MahresourcesContext)(nil)
	_ QueryPageContext            = (*application_context.MahresourcesContext)(nil)
	_ RelationPageContext         = (*application_context.MahresourcesContext)(nil)
	_ ResourceCategoryPageContext = (*application_context.MahresourcesContext)(nil)
	_ ResourcePageContext         = (*application_context.MahresourcesContext)(nil)
	_ SearchPageContext           = (*application_context.MahresourcesContext)(nil)
	_ TagPageContext              = (*application_context.MahresourcesContext)(nil)
	_ TemplatePartialPageContext  = (*application_context.MahresourcesContext)(nil)
	_ contracts.LogEntryReader    = (*application_context.MahresourcesContext)(nil)
	_ contracts.SeriesReader      = (*application_context.MahresourcesContext)(nil)
)

// AccountPageContext serves /admin/users and /account.
type AccountPageContext interface {
	GetUsers(offset, limit int) ([]models.User, error)
	ListApiTokens(userID uint) ([]models.ApiToken, error)
}

// AdminSettingsPageContext serves /admin/settings. Configuration is the whole
// config because the page renders a reference table of boot-only fields.
type AdminSettingsPageContext interface {
	Settings() *application_context.RuntimeSettings
	Configuration() *application_context.MahresourcesConfig
	// ShareServerFailed distinguishes "share port 8383" from "share port 8383 and
	// nothing is listening on it" (finding 51).
	ShareServerFailed() bool
}

// AdminSharesPageContext serves /admin/shares.
type AdminSharesPageContext interface {
	GetSharedNotes() ([]models.Note, error)
	Settings() *application_context.RuntimeSettings
}

// CategoryPageContext serves the category list, timeline, create and display pages.
type CategoryPageContext interface {
	contracts.CategoryReader
	GetCategoriesCount(query *query_models.CategoryQuery) (int64, error)
	GetCategory(id uint) (*models.Category, error)
}

// ComparePageContext serves /resource/compare.
type ComparePageContext interface {
	contracts.VersionReader
	GetResource(id uint) (*models.Resource, error)
	CompareVersionsCross(r1ID uint, v1Num int, r2ID uint, v2Num int) (*models.VersionComparison, error)
}

// DashboardPageContext serves /dashboard.
type DashboardPageContext interface {
	GetGroups(offset, maxResults int, query *query_models.GroupQuery) ([]models.Group, error)
	GetNotes(offset, maxResults int, query *query_models.NoteQuery) ([]models.Note, error)
	GetResources(offset, maxResults int, query *query_models.ResourceSearchQuery) ([]models.Resource, error)
	GetTags(offset, maxResults int, query *query_models.TagQuery) ([]models.Tag, error)
	GetRecentActivity(limit int) ([]application_context.ActivityEntry, error)
}

// GroupComparePageContext serves /group/compare.
type GroupComparePageContext interface {
	CompareGroupsCross(g1ID uint, g2ID uint) (*models.GroupComparison, error)
}

// GroupPageContext serves the group list, timeline, create, display and tree pages.
type GroupPageContext interface {
	contracts.GroupReader
	contracts.GroupTreeReader
	GetGroupsCount(query *query_models.GroupQuery) (int64, error)
	GetPopularGroupTags(query *query_models.GroupQuery) ([]application_context.PopularTag, error)
	CheckMRQLFilter(entity mrql.EntityType, expr string) *application_context.MRQLFilterError
	selectionHydrator
}

// HoverCardPageContext serves the entity hover cards.
type HoverCardPageContext interface {
	GetGroup(id uint) (*models.Group, error)
	GetNote(id uint) (*models.Note, error)
	GetResource(id uint) (*models.Resource, error)
}

// MRQLPageContext serves /mrql.
type MRQLPageContext interface {
	GetSavedMRQLQueries(offset, limit int) ([]models.SavedMRQLQuery, error)
}

// NotePageContext serves the note and note-type pages.
type NotePageContext interface {
	contracts.NoteReader
	contracts.NoteTypeReader
	GetGroup(id uint) (*models.Group, error)
	GetNoteCount(query *query_models.NoteQuery) (int64, error)
	GetNoteType(id uint) (*models.NoteType, error)
	GetNoteTypesCount(query *query_models.NoteTypeQuery) (int64, error)
	GetNoteTypesWithIds(ids []uint) ([]models.NoteType, error)
	GetPopularNoteTags(query *query_models.NoteQuery) ([]application_context.PopularTag, error)
	CheckMRQLFilter(entity mrql.EntityType, expr string) *application_context.MRQLFilterError
	Settings() *application_context.RuntimeSettings
	ShareEnabled() bool
	// ShareConfigured is "an operator asked for sharing", where ShareEnabled is
	// "sharing works". They differ exactly when the share server failed to bind
	// (finding 51), which is the case the note sidebar has to explain.
	ShareConfigured() bool
	selectionHydrator
}

// PluginManagePageContext serves /admin/plugins.
type PluginManagePageContext interface {
	GetPluginStates() ([]models.PluginState, error)
	PluginManager() *plugin_system.PluginManager
}

// QueryPageContext serves the saved-query pages.
type QueryPageContext interface {
	contracts.QueryReader
	GetQueriesCount(queryQ *query_models.QueryQuery) (int64, error)
	IsReadOnlyDBEnforced() bool
	DatabaseType() string
}

// SearchPageContext serves /search, the full results page behind the Cmd+K
// dialog's "see all" row (WS6, finding 32).
type SearchPageContext interface {
	contracts.GlobalSearcher
}

// RelationPageContext serves the relation and relation-type pages.
type RelationPageContext interface {
	GetGroup(id uint) (*models.Group, error)
	GetRelation(id uint) (*models.GroupRelation, error)
	GetRelations(offset int, maxResults int, query *query_models.GroupRelationshipQuery) ([]*models.GroupRelation, error)
	GetRelationsCount(query *query_models.GroupRelationshipQuery) (int64, error)
	GetRelationType(id uint) (*models.GroupRelationType, error)
	GetRelationTypes(offset int, maxResults int, query *query_models.RelationshipTypeQuery) ([]*models.GroupRelationType, error)
	GetRelationTypesCount(query *query_models.RelationshipTypeQuery) (int64, error)
	GetRelationTypesWithIds(ids *[]uint) ([]*models.GroupRelationType, error)
	GetGroupsWithIds(ids *[]uint) ([]*models.Group, error)
	GetCategoriesWithIds(ids *[]uint, limit int) ([]models.Category, error)
}

// ResourceCategoryPageContext serves the resource-category pages.
type ResourceCategoryPageContext interface {
	contracts.ResourceCategoryReader
	GetResourceCategoriesCount(query *query_models.ResourceCategoryQuery) (int64, error)
	GetResourceCategory(id uint) (*models.ResourceCategory, error)
	GetResources(offset, maxResults int, query *query_models.ResourceSearchQuery) ([]models.Resource, error)
	GetResourceCount(query *query_models.ResourceSearchQuery) (int64, error)
	IsDefaultResourceCategory(id uint) bool
}

// ResourcePageContext serves the resource list, timeline, create and display pages.
type ResourcePageContext interface {
	contracts.ResourceReader
	contracts.VersionReader
	contracts.SeriesSiblingReader
	GetGroup(id uint) (*models.Group, error)
	FindParentsOfGroup(id uint) ([]models.Group, error)
	GetResourceCount(query *query_models.ResourceSearchQuery) (int64, error)
	GetResourceCategory(id uint) (*models.ResourceCategory, error)
	GetSimilarResources(id uint) ([]*models.Resource, error)
	GetPopularResourceTags(query *query_models.ResourceSearchQuery) ([]application_context.PopularTag, error)
	ProbeVideoDuration(resourceId uint) (float64, error)
	CheckMRQLFilter(entity mrql.EntityType, expr string) *application_context.MRQLFilterError
	AltFileSystems() map[string]string
	selectionHydrator
}

// TagPageContext serves the tag list, timeline, create and display pages.
type TagPageContext interface {
	contracts.TagsReader
	GetTag(id uint) (*models.Tag, error)
	GetTagsCount(query *query_models.TagQuery) (int64, error)
}

// TemplatePartialPageContext serves the template-partial pages.
type TemplatePartialPageContext interface {
	contracts.TemplatePartialReader
	GetTemplatePartial(id uint) (*models.TemplatePartial, error)
	GetTemplatePartialsCount(query *query_models.TemplatePartialQuery) (int64, error)
}

// selectionHydrator loads the entities a create/edit form has pre-selected, so
// the pickers can render them without a round trip. Every entity form needs
// some subset, so it is shared rather than restated.
type selectionHydrator interface {
	GetGroupsWithIds(ids *[]uint) ([]*models.Group, error)
	GetTagsWithIds(ids *[]uint, limit int) ([]models.Tag, error)
	GetNotesWithIds(ids *[]uint) ([]*models.Note, error)
	GetResourcesWithIds(ids *[]uint) ([]*models.Resource, error)
	GetCategoriesWithIds(ids *[]uint, limit int) ([]models.Category, error)
}
