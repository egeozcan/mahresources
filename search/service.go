// Package search owns global search: the cross-entity query, the LIKE and FTS
// backends, and the process-wide result cache.
//
// Like groupio, it is stateless with respect to the database handle. Every
// query-issuing entry point takes the caller's *gorm.DB rather than capturing
// one, because search is confined by the GORM scope callbacks that read the
// handle's context. A captured handle would return another tenant's rows.
// See docs/plans/2026-07-28-application-context-decomposition.md section 2.
//
// What Service does own is the state that must be shared rather than copied:
// the result cache and the FTS provider. Duplicating either per call fails
// quietly rather than loudly, which is why both have dedicated tests in
// application_context.
package search

import (
	"sync/atomic"

	"gorm.io/gorm"

	"mahresources/constants"
	"mahresources/fts"
	"mahresources/models/query_models"
)

// Service holds the process-lifetime search state. One per process, shared by
// every derived context.
type Service struct {
	cache  *SearchCache
	dbType string

	// state is swapped atomically by InitFTS and UseExistingFTS. Before the
	// extraction these were two plain fields, and every derived context held its
	// own copy; sharing one Service makes the reads genuinely concurrent, since
	// GlobalSearch fans out over goroutines that consult FTS state.
	//
	// It is one snapshot rather than two atomics on purpose: provider and
	// enabled must be observed together, or a reader can see enabled == true
	// with provider still nil and dereference it.
	state atomic.Pointer[ftsState]
}

// ftsState is the FTS configuration as a single immutable value.
type ftsState struct {
	provider fts.FTSProvider
	enabled  bool
}

// ftsSnapshot returns the current state, zero-valued before initialization.
func (s *Service) ftsSnapshot() ftsState {
	if st := s.state.Load(); st != nil {
		return *st
	}
	return ftsState{}
}

// setFTS publishes a new state. Startup only in production, but safe to call
// at any time.
func (s *Service) setFTS(provider fts.FTSProvider, enabled bool) {
	s.state.Store(&ftsState{provider: provider, enabled: enabled})
}

// NewService builds the search service. dbType selects the FTS provider and the
// LIKE operator; it comes from config and is never mutated at runtime.
func NewService(cache *SearchCache, dbType string) *Service {
	return &Service{cache: cache, dbType: dbType}
}

// PrincipalScope reports whether the calling principal is confined to a group
// subtree.
//
// The result cache is keyed on the search term alone and shared process-wide,
// so it must be bypassed entirely, for reads and writes, whenever the caller is
// confined: otherwise a confined caller is served another scope's results, or
// writes its truncated result set where an unconfined caller will read it.
type PrincipalScope interface {
	IsRestricted() bool
}

// Deps is the per-call input. Rebuild it for every call so a principal-derived
// handle is picked up rather than the handle current when Service was built.
type Deps struct {
	DB    *gorm.DB
	Scope PrincipalScope
}

// opCtx binds the shared Service to one call's Deps, for the duration of that
// call only. Field writes through the embedded *Service (InitFTS) reach the
// shared state, which is what keeps FTS initialization visible to every derived
// context.
type opCtx struct {
	*Service
	db    *gorm.DB
	scope PrincipalScope
}

func (s *Service) op(d Deps) *opCtx {
	return &opCtx{Service: s, db: d.DB, scope: d.Scope}
}

// restricted reports whether the caller is confined. A missing resolver is
// treated as confined, so the failure mode is a skipped cache rather than a
// cross-scope leak.
func (ctx *opCtx) restricted() bool {
	return ctx.scope == nil || ctx.scope.IsRestricted()
}

// --- Public entry points ---

// GlobalSearch performs a unified search across all entity types.
func (s *Service) GlobalSearch(d Deps, query *query_models.GlobalSearchQuery) (*query_models.GlobalSearchResponse, error) {
	return s.op(d).GlobalSearch(query)
}

// InitFTS initializes the FTS provider and, on success, enables FTS for every
// context sharing this Service. Startup only.
func (s *Service) InitFTS(d Deps) error {
	return s.op(d).InitFTS()
}

// FTSEnabled reports whether full-text search is initialized and usable.
func (s *Service) FTSEnabled() bool {
	return s.ftsAvailable()
}

// LikeOperator exposes the dialect-appropriate LIKE operator.
func (s *Service) LikeOperator() string {
	if s.dbType == constants.DbTypePosgres {
		return "ILIKE"
	}
	return "LIKE"
}
