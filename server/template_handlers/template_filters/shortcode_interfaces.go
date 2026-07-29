package template_filters

import (
	"context"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/mrql"
)

// The capability each shortcode helper needs from the business layer.
//
// These three entry points are called from api_handlers, whose handlers now
// take narrow interfaces. A handler cannot pass its interface value to a
// function demanding *MahresourcesContext, so these had to narrow too for the
// api_handlers migration to reach the MRQL and deferred-render handlers.
//
// Only the three exported Build* entry points are migrated; the rest of this
// package still takes the concrete type.

var (
	_ PartialResolverContext = (*application_context.MahresourcesContext)(nil)
	_ QueryExecutorContext   = (*application_context.MahresourcesContext)(nil)
	_ MetaScopeResolver      = (*application_context.MahresourcesContext)(nil)
)

// PartialResolverContext resolves [partial] references by name.
type PartialResolverContext interface {
	GetTemplatePartialByName(name string) (*models.TemplatePartial, error)
}

// QueryExecutorContext runs the inline [mrql] shortcode: scope resolution,
// execution, the per-page query budget, and the logger the budget warning uses.
type QueryExecutorContext interface {
	ResolveMRQLScope(q *mrql.Query) (uint, error)
	ExecuteMRQLScoped(reqCtx context.Context, parsed *mrql.Query, scopeGroupID uint) (*application_context.MRQLResult, error)
	ExecuteMRQLGroupedWithScope(reqCtx context.Context, parsed *mrql.Query, scopeID uint) (*application_context.MRQLGroupedResult, error)
	CountMRQLScoped(reqCtx context.Context, parsed *mrql.Query, scopeGroupID uint) (int64, error)
	GetSavedMRQLQueryByName(name string) (*models.SavedMRQLQuery, error)
	LoadMRQLRenderData(reqCtx context.Context, resourceCategoryIDs, noteTypeIDs, categoryIDs, scopeGroupIDs []uint) (*application_context.MRQLRenderData, error)
	MRQLPageQueryBudget() int
	Logger() *application_context.Logger
}

// MetaScopeResolver supplies the group-scope walk and the signing key the
// [meta], [lazy] and [details] shortcodes need.
type MetaScopeResolver interface {
	DeferredSigningKey() []byte
	ResolveParentScopeID(groupID uint) uint
	ResolveRootScopeID(groupID uint) uint
}

// budgetLogger is the sliver logPageQueryBudgetExceeded needs.
type budgetLogger interface {
	Logger() *application_context.Logger
}

// mrqlShortcodeRunner is the sliver executeMRQLForShortcode needs: scope
// resolution plus the three execution shapes and saved-query lookup.
type mrqlShortcodeRunner interface {
	ResolveMRQLScope(q *mrql.Query) (uint, error)
	ExecuteMRQLScoped(reqCtx context.Context, parsed *mrql.Query, scopeGroupID uint) (*application_context.MRQLResult, error)
	ExecuteMRQLGroupedWithScope(reqCtx context.Context, parsed *mrql.Query, scopeID uint) (*application_context.MRQLGroupedResult, error)
	CountMRQLScoped(reqCtx context.Context, parsed *mrql.Query, scopeGroupID uint) (int64, error)
	GetSavedMRQLQueryByName(name string) (*models.SavedMRQLQuery, error)
	LoadMRQLRenderData(reqCtx context.Context, resourceCategoryIDs, noteTypeIDs, categoryIDs, scopeGroupIDs []uint) (*application_context.MRQLRenderData, error)
}
