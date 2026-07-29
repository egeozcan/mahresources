package application_context

import (
	"mahresources/models/query_models"
	"mahresources/search"
)

// Facade over the search package. It exists so that moving global search out of
// this package changed no call site: the entity-type constants keep their names
// and the five methods keep their signatures.

// Entity type constants, re-declared so the 13 files in this package that use
// them (and the 31 InvalidateSearchCacheByType call sites across 12 of them)
// need no edit. They are untyped string constants, so these are the same values,
// not merely equal ones.
const (
	EntityTypeResource         = search.EntityTypeResource
	EntityTypeNote             = search.EntityTypeNote
	EntityTypeGroup            = search.EntityTypeGroup
	EntityTypeTag              = search.EntityTypeTag
	EntityTypeCategory         = search.EntityTypeCategory
	EntityTypeQuery            = search.EntityTypeQuery
	EntityTypeRelationType     = search.EntityTypeRelationType
	EntityTypeNoteType         = search.EntityTypeNoteType
	EntityTypeResourceCategory = search.EntityTypeResourceCategory
	EntityTypeMRQLQuery        = search.EntityTypeMRQLQuery
)

// searchScope adapts MahresourcesContext to search.PrincipalScope. It reads the
// principal of THIS context, so a derived copy reports its own confinement.
type searchScope struct{ ctx *MahresourcesContext }

// IsRestricted collapses principalForcedScope's two flags: a principal that is
// confined to a subtree, and one that must be confined but could not be
// resolved, both have to bypass the shared result cache.
func (s searchScope) IsRestricted() bool {
	_, forced, deny := s.ctx.principalForcedScope()
	return forced || deny
}

// searchDeps rebuilds per call so it always carries THIS context's db — the
// subtree-scoped handle on a derived copy. Never cache it. Search is confined
// entirely by the GORM callbacks reading that handle, so a stale one returns
// another tenant's rows.
func (ctx *MahresourcesContext) searchDeps() search.Deps {
	return search.Deps{DB: ctx.db, Scope: searchScope{ctx: ctx}}
}

// GlobalSearch performs a unified search across all entity types.
func (ctx *MahresourcesContext) GlobalSearch(query *query_models.GlobalSearchQuery) (*query_models.GlobalSearchResponse, error) {
	return ctx.search.GlobalSearch(ctx.searchDeps(), query)
}

// InitFTS initializes the FTS provider based on the database type.
func (ctx *MahresourcesContext) InitFTS() error {
	return ctx.search.InitFTS(ctx.searchDeps())
}

// UseExistingFTS enables reads against an FTS schema that was initialized and
// validated earlier. Unlike InitFTS it performs no DDL or index rebuild.
func (ctx *MahresourcesContext) UseExistingFTS() {
	ctx.search.UseExistingFTS()
}

// InvalidateSearchCacheByType removes cached search results that contain the
// specified entity type. Called after create/update/delete so the next search
// sees fresh data; the cache also has a 60-second TTL as a backstop.
func (ctx *MahresourcesContext) InvalidateSearchCacheByType(entityType string) {
	ctx.search.InvalidateSearchCacheByType(entityType)
}

// ClearSearchCache removes all cached search results.
func (ctx *MahresourcesContext) ClearSearchCache() {
	ctx.search.ClearSearchCache()
}

// ftsAvailable reports whether full-text search is initialized and usable.
// Read by admin_context and mrql_context.
func (ctx *MahresourcesContext) ftsAvailable() bool {
	return ctx.search.FTSEnabled()
}
