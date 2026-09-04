package search

import (
	"fmt"
	"mahresources/constants"
	"mahresources/fts"
	"mahresources/models"
	"mahresources/models/query_models"
	"sort"
	"strings"
	"sync"

	"gorm.io/gorm"
)

// Entity type constants
const (
	EntityTypeResource         = "resource"
	EntityTypeNote             = "note"
	EntityTypeGroup            = "group"
	EntityTypeTag              = "tag"
	EntityTypeCategory         = "category"
	EntityTypeQuery            = "query"
	EntityTypeRelationType     = "relationType"
	EntityTypeNoteType         = "noteType"
	EntityTypeResourceCategory = "resourceCategory"
	EntityTypeMRQLQuery        = "mrqlQuery"
)

// searchCacheLimit is how many rows a cacheable search collects before it stops.
// It is also the ceiling the reported Total saturates at, which is why
// GlobalSearchResponse carries TotalCapped: without it a caller cannot tell
// "exactly 50 matches" from "at least 50" (WS6, finding 32).
const searchCacheLimit = 50

// InvalidateSearchCacheByType removes cached search results that contain the specified entity type.
// This should be called after creating, updating, or deleting entities to ensure search results are fresh.
// Note: Even without explicit invalidation, the cache has a 60-second TTL for eventual consistency.
func (ctx *Service) InvalidateSearchCacheByType(entityType string) {
	if ctx.cache != nil {
		ctx.cache.InvalidateByType(entityType)
	}
}

// ClearSearchCache removes all cached search results
func (ctx *Service) ClearSearchCache() {
	if ctx.cache != nil {
		ctx.cache.Clear()
	}
}

var allEntityTypes = []string{
	EntityTypeResource, EntityTypeNote, EntityTypeGroup,
	EntityTypeTag, EntityTypeCategory, EntityTypeQuery,
	EntityTypeRelationType, EntityTypeNoteType, EntityTypeResourceCategory,
	EntityTypeMRQLQuery,
}

// newFTSProvider builds the provider for the configured dialect.
func (ctx *Service) newFTSProvider() fts.FTSProvider {
	if ctx.dbType == constants.DbTypePosgres {
		return fts.NewPostgresFTS()
	}
	return fts.NewSQLiteFTS()
}

// UseExistingFTS enables reads against an FTS schema that was initialized and
// validated earlier. Unlike InitFTS it performs no DDL or index rebuild.
func (ctx *Service) UseExistingFTS() {
	ctx.setFTS(ctx.newFTSProvider(), true)
}

// InitFTS initializes the FTS provider based on the database type.
func (ctx *opCtx) InitFTS() error {
	provider := ctx.newFTSProvider()

	if err := provider.Setup(ctx.db); err != nil {
		ctx.setFTS(nil, false)
		return err
	}

	ctx.setFTS(provider, true)
	return nil
}

// ftsAvailable reports whether full-text search is initialized and usable.
// Callers outside this file read FTS state through it rather than touching the
// field, so the state can move behind a service without touching them.
func (ctx *Service) ftsAvailable() bool {
	return ctx.ftsSnapshot().enabled
}

// GlobalSearch performs a unified search across all entity types
func (ctx *opCtx) GlobalSearch(query *query_models.GlobalSearchQuery) (*query_models.GlobalSearchResponse, error) {
	if query.Limit <= 0 {
		query.Limit = 20
	} else if query.Limit > 50 {
		query.Limit = 50
	}

	searchTerm := strings.TrimSpace(query.Query)
	if searchTerm == "" {
		return &query_models.GlobalSearchResponse{
			Query:   "",
			Total:   0,
			Results: []query_models.SearchResultItem{},
		}, nil
	}

	// RBAC: the result cache is keyed on the term alone and shared process-wide,
	// so it must be bypassed for group-limited principals to avoid leaking
	// (or being poisoned by) results from another scope. Their per-type queries
	// are still scoped by the GORM callbacks (search uses ctx.db).
	cacheable := !ctx.restricted()

	// Check server-side cache first (only for default type searches to keep cache key simple)
	if cacheable && ctx.cache != nil && len(query.Types) == 0 {
		cacheKey := strings.ToLower(searchTerm)
		if cached, ok := ctx.cache.Get(cacheKey); ok {
			// Total reflects all cached results; apply limit only for the returned slice
			total := len(cached)
			results := cached
			if len(results) > query.Limit {
				results = results[:query.Limit]
			}
			return &query_models.GlobalSearchResponse{
				Query: searchTerm,
				Total: total,
				// The cache holds at most searchCacheLimit rows, so a cached
				// total is a floor for exactly the same reason a fresh one is.
				TotalCapped: total >= searchCacheLimit,
				Results:     results,
			}, nil
		}
	}

	// Use a higher limit for caching to support subsequent queries with different limits
	// Only use cacheLimit when we're going to cache (default type searches)
	searchLimit := query.Limit
	shouldCache := cacheable && ctx.cache != nil && len(query.Types) == 0
	if shouldCache {
		searchLimit = searchCacheLimit // Cache up to searchCacheLimit results
	}

	// Parse the search query to detect prefix/fuzzy modes
	parsedQuery := fts.ParseSearchQuery(searchTerm)

	// After sanitization, the search term may be empty (e.g., query was only
	// special characters like quotes or angle brackets). Return no results
	// rather than matching everything.
	if parsedQuery.Term == "" {
		return &query_models.GlobalSearchResponse{
			Query:   searchTerm,
			Total:   0,
			Results: []query_models.SearchResultItem{},
		}, nil
	}

	typesToSearch := getTypesToSearch(query.Types)

	var wg sync.WaitGroup
	resultsChan := make(chan []query_models.SearchResultItem, len(typesToSearch))

	for _, entityType := range typesToSearch {
		wg.Add(1)
		go func(et string) {
			defer wg.Done()
			var results []query_models.SearchResultItem
			if ctx.ftsAvailable() {
				results = ctx.searchEntityTypeFTS(et, parsedQuery, searchLimit)
			} else {
				// Fallback to LIKE-based search if FTS is not available
				results = ctx.searchEntityType(et, searchTerm, searchLimit)
			}
			resultsChan <- results
		}(entityType)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	allResults := make([]query_models.SearchResultItem, 0)
	for results := range resultsChan {
		allResults = append(allResults, results...)
	}

	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Score > allResults[j].Score
	})

	// Trim to cache limit for caching
	if len(allResults) > searchLimit {
		allResults = allResults[:searchLimit]
	}

	// Cache results before applying user's limit
	if shouldCache {
		ctx.cache.Set(strings.ToLower(searchTerm), allResults)
	}

	// Total reflects all results found; apply limit only for the returned slice
	total := len(allResults)
	if len(allResults) > query.Limit {
		allResults = allResults[:query.Limit]
	}

	return &query_models.GlobalSearchResponse{
		Query: searchTerm,
		Total: total,
		// total is computed after the trim above, so it saturates at
		// searchLimit. Saying so is what lets the UI render "50+" rather than
		// present the ceiling as an exact count (WS6, finding 32).
		TotalCapped: total >= searchLimit,
		Results:     allResults,
	}, nil
}

func getTypesToSearch(requestedTypes []string) []string {
	if len(requestedTypes) == 0 {
		return allEntityTypes
	}

	typeSet := make(map[string]bool)
	for _, t := range allEntityTypes {
		typeSet[t] = true
	}

	var filtered []string
	for _, t := range requestedTypes {
		if typeSet[t] {
			filtered = append(filtered, t)
		}
	}

	if len(filtered) == 0 {
		return allEntityTypes
	}
	return filtered
}

// searchable is a constraint for models that can appear in search results.
type searchable interface {
	models.Group | models.Note | models.Resource | models.Tag |
		models.Category | models.Query | models.GroupRelationType |
		models.NoteType | models.ResourceCategory | models.SavedMRQLQuery
}

// searchEntityInfo holds the metadata needed to search a specific entity type.
type searchEntityInfo struct {
	entityType string
	urlFormat  string
	// extraLikeCols are additional columns to include in LIKE searches beyond name+description
	extraLikeCols []string
}

var entitySearchInfo = map[string]searchEntityInfo{
	EntityTypeGroup:            {entityType: EntityTypeGroup, urlFormat: "/group?id=%d"},
	EntityTypeNote:             {entityType: EntityTypeNote, urlFormat: "/note?id=%d"},
	EntityTypeResource:         {entityType: EntityTypeResource, urlFormat: "/resource?id=%d", extraLikeCols: []string{"original_name"}},
	EntityTypeTag:              {entityType: EntityTypeTag, urlFormat: "/tag?id=%d"},
	EntityTypeCategory:         {entityType: EntityTypeCategory, urlFormat: "/category?id=%d"},
	EntityTypeQuery:            {entityType: EntityTypeQuery, urlFormat: "/query?id=%d"},
	EntityTypeRelationType:     {entityType: EntityTypeRelationType, urlFormat: "/relationType?id=%d"},
	EntityTypeNoteType:         {entityType: EntityTypeNoteType, urlFormat: "/noteType?id=%d"},
	EntityTypeResourceCategory: {entityType: EntityTypeResourceCategory, urlFormat: "/resourceCategory?id=%d"},
	EntityTypeMRQLQuery:        {entityType: EntityTypeMRQLQuery, urlFormat: "/mrql?saved=%d"},
}

func (ctx *opCtx) searchEntityType(entityType, searchTerm string, limit int) []query_models.SearchResultItem {
	switch entityType {
	case EntityTypeGroup:
		return searchEntitiesLike[models.Group](ctx, entityType, searchTerm, limit)
	case EntityTypeNote:
		return searchEntitiesLike[models.Note](ctx, entityType, searchTerm, limit)
	case EntityTypeResource:
		return searchEntitiesLike[models.Resource](ctx, entityType, searchTerm, limit)
	case EntityTypeTag:
		return searchEntitiesLike[models.Tag](ctx, entityType, searchTerm, limit)
	case EntityTypeCategory:
		return searchEntitiesLike[models.Category](ctx, entityType, searchTerm, limit)
	case EntityTypeQuery:
		return searchEntitiesLike[models.Query](ctx, entityType, searchTerm, limit)
	case EntityTypeRelationType:
		return searchEntitiesLike[models.GroupRelationType](ctx, entityType, searchTerm, limit)
	case EntityTypeNoteType:
		return searchEntitiesLike[models.NoteType](ctx, entityType, searchTerm, limit)
	case EntityTypeResourceCategory:
		return searchEntitiesLike[models.ResourceCategory](ctx, entityType, searchTerm, limit)
	case EntityTypeMRQLQuery:
		return searchEntitiesLike[models.SavedMRQLQuery](ctx, entityType, searchTerm, limit)
	}
	return nil
}

func (ctx *opCtx) searchEntityTypeFTS(entityType string, query fts.ParsedQuery, limit int) []query_models.SearchResultItem {
	switch entityType {
	case EntityTypeResource:
		return searchEntitiesFTS[models.Resource](ctx, entityType, query, limit)
	case EntityTypeNote:
		return searchEntitiesFTS[models.Note](ctx, entityType, query, limit)
	case EntityTypeGroup:
		return searchEntitiesFTS[models.Group](ctx, entityType, query, limit)
	case EntityTypeTag:
		return searchEntitiesFTS[models.Tag](ctx, entityType, query, limit)
	case EntityTypeCategory:
		return searchEntitiesFTS[models.Category](ctx, entityType, query, limit)
	case EntityTypeQuery:
		return searchEntitiesFTS[models.Query](ctx, entityType, query, limit)
	case EntityTypeRelationType:
		return searchEntitiesFTS[models.GroupRelationType](ctx, entityType, query, limit)
	case EntityTypeNoteType:
		return searchEntitiesFTS[models.NoteType](ctx, entityType, query, limit)
	case EntityTypeResourceCategory:
		return searchEntitiesFTS[models.ResourceCategory](ctx, entityType, query, limit)
	case EntityTypeMRQLQuery:
		// Saved MRQL queries are not FTS-indexed; LIKE is fine at their
		// cardinality. Use RawTerm (hyphens preserved) so a hyphenated name
		// still substring-matches — query.Term collapses hyphens to spaces for
		// FTS and would not LIKE-match the stored value.
		term := query.RawTerm
		if term == "" {
			term = query.Term
		}
		return searchEntitiesLike[models.SavedMRQLQuery](ctx, entityType, term, limit)
	}
	return nil
}

func calculateRelevanceScore(name, description, searchTerm string, extraFields ...string) int {
	nameLower := strings.ToLower(name)
	termLower := strings.ToLower(searchTerm)

	if nameLower == termLower {
		return 100
	}
	if strings.HasPrefix(nameLower, termLower) {
		return 80
	}
	if strings.Contains(nameLower, termLower) {
		return 60
	}
	// Check extra fields (e.g., original_name) before description
	for _, extra := range extraFields {
		if extra != "" {
			extraLower := strings.ToLower(extra)
			if extraLower == termLower {
				return 90 // exact match on extra field
			}
			if strings.Contains(extraLower, termLower) {
				return 55 // substring match on extra field
			}
		}
	}
	if strings.Contains(strings.ToLower(description), termLower) {
		return 40
	}
	return 20
}

func truncateDescription(desc string, maxLen int) string {
	runes := []rune(desc)
	if len(runes) <= maxLen {
		return desc
	}
	return string(runes[:maxLen-3]) + "..."
}

// entityFields extracts the common search fields (id, name, description) from any searchable model.
func entityFields(v any) (id uint, name, description string) {
	switch e := v.(type) {
	case models.Group:
		return e.ID, e.Name, e.Description
	case models.Note:
		return e.ID, e.Name, e.Description
	case models.Resource:
		return e.ID, e.Name, e.Description
	case models.Tag:
		return e.ID, e.Name, e.Description
	case models.Category:
		return e.ID, e.Name, e.Description
	case models.Query:
		return e.ID, e.Name, e.Description
	case models.GroupRelationType:
		return e.ID, e.Name, e.Description
	case models.NoteType:
		return e.ID, e.Name, e.Description
	case models.ResourceCategory:
		return e.ID, e.Name, e.Description
	case models.SavedMRQLQuery:
		return e.ID, e.Name, e.Description
	}
	return 0, "", ""
}

// entityExtra returns additional metadata for a search result (e.g., contentType for resources).
func entityExtra(v any) map[string]string {
	if r, ok := v.(models.Resource); ok && r.ContentType != "" {
		return map[string]string{"contentType": r.ContentType}
	}
	return nil
}

// entityDisplayType preserves the storage kind for routing while exposing the
// taxonomy name people actually work with in search (for example PM Task or PM
// Epic). Empty means the client should use its generic type label.
func entityDisplayType(v any) string {
	switch e := v.(type) {
	case models.Note:
		if e.NoteType != nil {
			return e.NoteType.Name
		}
	case models.Group:
		if e.Category != nil {
			return e.Category.Name
		}
	case models.Resource:
		if e.ResourceCategory != nil {
			return e.ResourceCategory.Name
		}
	}
	return ""
}

func preloadSearchDisplayType(db *gorm.DB, entityType string) *gorm.DB {
	switch entityType {
	case EntityTypeNote:
		return db.Preload("NoteType")
	case EntityTypeGroup:
		return db.Preload("Category")
	case EntityTypeResource:
		return db.Preload("ResourceCategory")
	default:
		return db
	}
}

// entityExtraText returns additional searchable text fields for relevance scoring.
// For resources, this includes the original_name so that matches on the original
// filename are scored appropriately (instead of falling through to the minimum score).
func entityExtraText(v any) string {
	if r, ok := v.(models.Resource); ok {
		return r.OriginalName
	}
	return ""
}

// escapeLikeWildcards escapes SQL LIKE wildcard characters so they match literally.
func escapeLikeWildcards(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// searchEntitiesLike performs a LIKE-based search for any searchable entity type.
func searchEntitiesLike[T searchable](ctx *opCtx, entityType, searchTerm string, limit int) []query_models.SearchResultItem {
	info := entitySearchInfo[entityType]
	likeOp := ctx.LikeOperator()
	escaped := escapeLikeWildcards(searchTerm)
	pattern := "%" + escaped + "%"
	likeEscape := " ESCAPE '\\'"

	// BH-005a: SQLite's default LIKE is case-insensitive for ASCII *only*.
	// Non-ASCII characters (e.g. Ä, ş) are byte-compared, so "Spätzle" doesn't
	// match "SPÄTZLE". Wrap both column and pattern in LOWER() on SQLite; the
	// Postgres path already uses ILIKE and is case-folded end-to-end.
	usingSQLite := ctx.dbType != constants.DbTypePosgres
	buildClause := func(col string) string {
		if usingSQLite {
			return "LOWER(" + col + ") " + likeOp + " LOWER(?)" + likeEscape
		}
		return col + " " + likeOp + " ?" + likeEscape
	}

	// Build WHERE clause: always search name and description, plus any extra columns
	whereParts := []string{
		buildClause("name"),
		buildClause("description"),
	}
	args := []any{pattern, pattern}
	for _, col := range info.extraLikeCols {
		whereParts = append(whereParts, buildClause(col))
		args = append(args, pattern)
	}

	var entities []T
	preloadSearchDisplayType(ctx.db, entityType).
		Where(strings.Join(whereParts, " OR "), args...).
		Limit(limit).
		Find(&entities)

	results := make([]query_models.SearchResultItem, 0, len(entities))
	for _, e := range entities {
		id, name, description := entityFields(e)
		results = append(results, query_models.SearchResultItem{
			ID:          id,
			Type:        info.entityType,
			DisplayType: entityDisplayType(e),
			Name:        name,
			Description: truncateDescription(description, 100),
			Score:       calculateRelevanceScore(name, description, searchTerm, entityExtraText(e)),
			URL:         fmt.Sprintf(info.urlFormat, id),
			Extra:       entityExtra(e),
		})
	}
	return results
}

// searchEntitiesFTS performs an FTS-based search for any searchable entity type.
func searchEntitiesFTS[T searchable](ctx *opCtx, entityType string, query fts.ParsedQuery, limit int) []query_models.SearchResultItem {
	config := fts.GetEntityConfig(entityType)
	if config == nil {
		return nil
	}

	// Load provider and enabled together. Reading ftsAvailable() at the fan-out
	// and the provider here are two separate observations, so this one must
	// tolerate FTS having been torn down in between rather than assume non-nil.
	state := ctx.ftsSnapshot()
	if state.provider == nil {
		return nil
	}

	info := entitySearchInfo[entityType]

	var entities []T
	preloadSearchDisplayType(ctx.db.Model(new(T)), entityType).
		Scopes(state.provider.BuildSearchScope(config.TableName, config.Columns, query)).
		Limit(limit).
		Find(&entities)

	results := make([]query_models.SearchResultItem, 0, len(entities))
	for i, e := range entities {
		id, name, description := entityFields(e)
		score := 100 - i
		if score < 1 {
			score = 1
		}
		results = append(results, query_models.SearchResultItem{
			ID:          id,
			Type:        info.entityType,
			DisplayType: entityDisplayType(e),
			Name:        name,
			Description: truncateDescription(description, 100),
			Score:       score,
			URL:         fmt.Sprintf(info.urlFormat, id),
			Extra:       entityExtra(e),
		})
	}
	return results
}
