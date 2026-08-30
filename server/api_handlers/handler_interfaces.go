package api_handlers

import (
	"context"
	"encoding/json"
	"time"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/mrql"
	"mahresources/plugin_system"
	"mahresources/server/template_handlers/template_filters"
)

// The capability each remaining handler group needs from the business layer.
//
// Most of this package already takes contracts/ interfaces; these cover the
// handlers that still took *application_context.MahresourcesContext outright.
// They live here rather than in contracts/ for the reason GroupImporter below
// already documents: contracts/ may import only models/ and constants/, and
// these methods return application_context-owned types (MRQLResult, ServerStats,
// RuntimeSettings, UserInput, ...) or plugin_system types.
//
// Compile-time proof that MahresourcesContext satisfies every one of these.
var (
	_ MRQLAPIContext            = (*application_context.MahresourcesContext)(nil)
	_ MRQLExportContext         = (*application_context.MahresourcesContext)(nil)
	_ PluginAPIContext          = (*application_context.MahresourcesContext)(nil)
	_ TimelineContext           = (*application_context.MahresourcesContext)(nil)
	_ UserAdminContext          = (*application_context.MahresourcesContext)(nil)
	_ AccountContext            = (*application_context.MahresourcesContext)(nil)
	_ AdminStatsContext         = (*application_context.MahresourcesContext)(nil)
	_ SettingsContext           = (*application_context.MahresourcesContext)(nil)
	_ TemplateGenerationContext = (*application_context.MahresourcesContext)(nil)
	_ TemplatePreviewContext    = (*application_context.MahresourcesContext)(nil)
	_ PluginManagerProvider     = (*application_context.MahresourcesContext)(nil)
	_ DeferredRenderContext     = (*application_context.MahresourcesContext)(nil)
	_ UserSettingsContext       = (*application_context.MahresourcesContext)(nil)
	_ PluginActionRunner        = (*application_context.MahresourcesContext)(nil)
)

// PluginManagerProvider is the plugin registry accessor. Several handlers need
// nothing else from the context.
type PluginManagerProvider interface {
	PluginManager() *plugin_system.PluginManager
}

// ShortcodeLintContext serves /v1/shortcodes/lint. Beyond the plugin registry
// (for the plugin shortcode catalogue) it reads the template-partial *names*, so
// a [partial] reference to a partial that does not exist can be reported
// (finding 155).
type ShortcodeLintContext interface {
	PluginManagerProvider
	partialNameResolver
}

// MRQLAPIContext serves the /v1/mrql surface: execution, validation,
// completion, explain, generation, and the saved-query CRUD.
type MRQLAPIContext interface {
	// PluginAllowsScopedPrincipals reports whether a group-limited caller may
	// reach this plugin's own surfaces. An operator decision, per plugin, so a
	// seam that renders plugin code needs the name to ask about.
	PluginAllowsScopedPrincipals(pluginName string) bool
	PluginManagerProvider
	template_filters.QueryExecutorContext
	template_filters.PartialResolverContext
	Principal() *auth.Principal
	ExecuteMRQLParsed(reqCtx context.Context, parsed *mrql.Query, limit, page int) (*application_context.MRQLResult, error)
	ExecuteMRQLGrouped(reqCtx context.Context, parsed *mrql.Query) (*application_context.MRQLGroupedResult, error)
	ExplainMRQLWithOptions(reqCtx context.Context, parsed *mrql.Query, explainOptions application_context.MRQLExplainOptions) (*application_context.MRQLExplainResult, error)
	ValidateMRQLWithParams(queryStr string) (bool, []map[string]any, []string)
	ValidateMRQLFilter(entity mrql.EntityType, queryStr string) (bool, []map[string]any)
	CompleteMRQL(queryStr string, cursor int) []mrql.Suggestion
	CompleteMRQLFilter(entity mrql.EntityType, queryStr string, cursor int) []mrql.Suggestion
	LoadMRQLRenderData(reqCtx context.Context, resourceCategoryIDs, noteTypeIDs, categoryIDs, scopeGroupIDs []uint) (*application_context.MRQLRenderData, error)
	MRQLGenerator() application_context.MRQLGenerator
	MRQLGenerationRateLimiter() *application_context.MRQLGenerationRateLimiter
	MRQLQueryTimeout() time.Duration
	MRQLPageQueryBudget() int
	DeferredSigningKey() []byte
	GetSavedMRQLQueries(offset, limit int) ([]models.SavedMRQLQuery, error)
	GetSavedMRQLQuery(id uint) (*models.SavedMRQLQuery, error)
	GetSavedMRQLQueryByName(name string) (*models.SavedMRQLQuery, error)
	UpdateSavedMRQLQuery(id uint, name, query, description string) (*models.SavedMRQLQuery, error)
	CreateSavedMRQLQuery(name, query, description string) (*models.SavedMRQLQuery, error)
	DeleteSavedMRQLQuery(id uint) error
}

// savedMRQLLookup resolves a saved query by id or name. Shared by the MRQL API
// and export handlers, so it is narrower than either.
type savedMRQLLookup interface {
	GetSavedMRQLQuery(id uint) (*models.SavedMRQLQuery, error)
	GetSavedMRQLQueryByName(name string) (*models.SavedMRQLQuery, error)
}

// mrqlRenderContext is what building a render context for inline [mrql]
// shortcodes needs: the executor and partial resolver it installs, plus the
// budget, timeout and signing key it reads.
type mrqlRenderContext interface {
	template_filters.QueryExecutorContext
	template_filters.PartialResolverContext
	DeferredSigningKey() []byte
	MRQLPageQueryBudget() int
	MRQLQueryTimeout() time.Duration
}

// templateRenderContext is what rendering a category template requires,
// independent of which handler is doing it. The preview, NL-generation and
// deferred-reveal handlers all render, so all three embed it.
type templateRenderContext interface {
	PluginManagerProvider
	template_filters.MetaScopeResolver
	mrqlRenderContext
}

// MRQLExportContext serves the CSV/JSON export routes, which have their own
// bounds checks and execution paths.
type MRQLExportContext interface {
	savedMRQLLookup
	ExecuteMRQLParsedExport(reqCtx context.Context, parsed *mrql.Query, limit, page int) (*application_context.MRQLResult, error)
	ExecuteMRQLGroupedExport(reqCtx context.Context, parsed *mrql.Query) (*application_context.MRQLGroupedResult, error)
	ValidateMRQLFlatExportBounds(q *mrql.Query, limit, page int) error
	ValidateMRQLGroupedExportBounds(q *mrql.Query) error
}

// PluginAPIContext serves the plugin admin and host-function endpoints.
type PluginAPIContext interface {
	PluginManagerProvider
	GetPluginStates() ([]models.PluginState, error)
	SetPluginEnabled(pluginName string, enabled bool) error
	SetPluginScopedAccess(pluginName string, allowed bool) error
	SavePluginSettings(pluginName string, values map[string]any) ([]plugin_system.ValidationError, error)
	PluginKVPurge(pluginName string) error
	PluginSchedulesFor(pluginName string) ([]models.PluginSchedule, error)
	RunPluginScheduleNow(pluginName, scheduleID string) error
	PluginScheduledDownloadsFor(pluginName string) ([]models.ScheduledDownload, error)
	CancelScheduledDownload(id uint) (bool, error)
	GetNote(id uint) (*models.Note, error)
	GetBlock(id uint) (*models.NoteBlock, error)
}

// TimelineContext serves the per-entity timeline count endpoints.
type TimelineContext interface {
	GetResourceTimelineCounts(query *query_models.ResourceSearchQuery, boundaries []application_context.BucketBoundary) ([]models.TimelineBucket, error)
	GetNoteTimelineCounts(query *query_models.NoteQuery, boundaries []application_context.BucketBoundary) ([]models.TimelineBucket, error)
	GetGroupTimelineCounts(query *query_models.GroupQuery, boundaries []application_context.BucketBoundary) ([]models.TimelineBucket, error)
	GetTagTimelineCounts(query *query_models.TagQuery, boundaries []application_context.BucketBoundary) ([]models.TimelineBucket, error)
	GetCategoryTimelineCounts(query *query_models.CategoryQuery, boundaries []application_context.BucketBoundary) ([]models.TimelineBucket, error)
	GetQueryTimelineCounts(query *query_models.QueryQuery, boundaries []application_context.BucketBoundary) ([]models.TimelineBucket, error)
}

// UserAdminContext serves /v1/users and /admin/users.
type UserAdminContext interface {
	GetUsers(offset, limit int) ([]models.User, error)
	GetUser(id uint) (*models.User, error)
	CreateUser(input *application_context.UserInput) (*models.User, error)
	UpdateUser(id uint, update *application_context.UserUpdate) (*models.User, error)
	DeleteUser(id uint) error
}

// AccountContext serves the self-service account endpoints: password change and
// personal API tokens.
type AccountContext interface {
	ChangeOwnPassword(userID uint, currentPassword, newPassword string, keepSessionTokenHash *string) error
	SessionCookieName() string
	CreateApiToken(userID uint, name string, expiresAt *time.Time) (string, *models.ApiToken, error)
	ListApiTokens(userID uint) ([]models.ApiToken, error)
	RevokeApiToken(id, userID uint) error
}

// AdminStatsContext serves the admin dashboard's statistics and maintenance
// actions.
type AdminStatsContext interface {
	GetServerStats() (*application_context.ServerStats, error)
	GetDataStats() (*application_context.DataStats, error)
	GetExpensiveStats() (*application_context.ExpensiveStats, error)
	RecomputeSimilarities() (string, error)
	RetryFailedHashes() (int64, error)
}

// SettingsContext serves the runtime-settings admin API.
type SettingsContext interface {
	Settings() *application_context.RuntimeSettings
}

// TemplateGenerationContext serves the natural-language category-template
// generation endpoints.
type TemplateGenerationContext interface {
	TemplatePreviewContext
	TemplateGenerator() application_context.TemplateGenerator
	TemplateGenerationRateLimiter() *application_context.MRQLGenerationRateLimiter
	GetGroups(offset, maxResults int, query *query_models.GroupQuery) ([]models.Group, error)
	GetNotes(offset, maxResults int, query *query_models.NoteQuery) ([]models.Note, error)
	GetResources(offset, maxResults int, query *query_models.ResourceSearchQuery) ([]models.Resource, error)
	GetTemplatePartials(query *query_models.TemplatePartialQuery, offset, maxResults int) ([]models.TemplatePartial, error)
}

// TemplatePreviewContext serves the category-template live preview, which
// renders a real entity through a candidate template.
type TemplatePreviewContext interface {
	// PluginAllowsScopedPrincipals reports whether a group-limited caller may
	// reach this plugin's own surfaces. An operator decision, per plugin, so a
	// seam that renders plugin code needs the name to ask about.
	PluginAllowsScopedPrincipals(pluginName string) bool
	templateRenderContext
	GetGroup(id uint) (*models.Group, error)
	GetNote(id uint) (*models.Note, error)
	GetResource(id uint) (*models.Resource, error)
	GetCategory(id uint) (*models.Category, error)
	GetNoteType(id uint) (*models.NoteType, error)
	GetResourceCategory(id uint) (*models.ResourceCategory, error)
}

// DeferredRenderContext serves the [lazy]/[details] deferred-render endpoint,
// which opens a signed token and renders its payload.
type DeferredRenderContext interface {
	// PluginAllowsScopedPrincipals reports whether a group-limited caller may
	// reach this plugin's own surfaces. An operator decision, per plugin, so a
	// seam that renders plugin code needs the name to ask about.
	PluginAllowsScopedPrincipals(pluginName string) bool
	TemplatePreviewContext
}

// UserSettingsContext serves the per-user settings endpoints. WithRequest is
// inherited from contracts.RequestContextSetter and returns any, so handlers
// assert the result back to this interface.
type UserSettingsContext interface {
	contracts.RequestContextSetter
	GetUserSettings() (map[string]json.RawMessage, error)
	SetUserSetting(key string, value json.RawMessage) error
	DeleteUserSetting(key string) error
}
