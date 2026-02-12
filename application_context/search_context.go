package application_context

import (
	"fmt"
	"mahresources/constants"
	"mahresources/fts"
	"mahresources/models"
	"mahresources/models/query_models"
	"sort"
	"strings"
	"sync"
)

// Entity type constants
const (
	EntityTypeResource     = "resource"
	EntityTypeNote         = "note"
	EntityTypeGroup        = "group"
	EntityTypeTag          = "tag"
	EntityTypeCategory     = "category"
	EntityTypeQuery        = "query"
	EntityTypeRelationType = "relationType"
	EntityTypeNoteType             = "noteType"
	EntityTypeResourceCategory     = "resourceCategory"
)

// InvalidateSearchCacheByType removes cached search results that contain the specified entity type.
// This should be called after creating, updating, or deleting entities to ensure search results are fresh.
// Note: Even without explicit invalidation, the cache has a 60-second TTL for eventual consistency.
func (ctx *MahresourcesContext) InvalidateSearchCacheByType(entityType string) {
	if ctx.searchCache != nil {
		ctx.searchCache.InvalidateByType(entityType)
	}
}

// ClearSearchCache removes all cached search results
func (ctx *MahresourcesContext) ClearSearchCache() {
	if ctx.searchCache != nil {
		ctx.searchCache.Clear()
	}
}

var allEntityTypes = []string{
	EntityTypeResource, EntityTypeNote, EntityTypeGroup,
	EntityTypeTag, EntityTypeCategory, EntityTypeQuery,
	EntityTypeRelationType, EntityTypeNoteType, EntityTypeResourceCategory,
}

// InitFTS initializes the FTS provider based on the database type
func (ctx *MahresourcesContext) InitFTS() error {
	if ctx.Config.DbType == constants.DbTypePosgres {
		ctx.ftsProvider = fts.NewPostgresFTS()
	} else {
		ctx.ftsProvider = fts.NewSQLiteFTS()
	}

	if err := ctx.ftsProvider.Setup(ctx.db); err != nil {
		ctx.ftsProvider = nil
		ctx.ftsEnabled = false
		return err
	}

	ctx.ftsEnabled = true
	return nil
}

// GlobalSearch performs a unified search across all entity types
func (ctx *MahresourcesContext) GlobalSearch(query *query_models.GlobalSearchQuery) (*query_models.GlobalSearchResponse, error) {
	if query.Limit <= 0 || query.Limit > 50 {
		query.Limit = 20
	}

	searchTerm := strings.TrimSpace(query.Query)
	if searchTerm == "" {
		return &query_models.GlobalSearchResponse{
			Query:   "",
			Total:   0,
			Results: []query_models.SearchResultItem{},
		}, nil
	}

	// Check server-side cache first (only for default type searches to keep cache key simple)
	if ctx.searchCache != nil && len(query.Types) == 0 {
		cacheKey := strings.ToLower(searchTerm)
		if cached, ok := ctx.searchCache.Get(cacheKey); ok {
			// Apply limit to cached results
			results := cached
			if len(results) > query.Limit {
				results = results[:query.Limit]
			}
			return &query_models.GlobalSearchResponse{
				Query:   searchTerm,
				Total:   len(results),
				Results: results,
			}, nil
		}
	}

	// Use a higher limit for caching to support subsequent queries with different limits
	// Only use cacheLimit when we're going to cache (default type searches)
	searchLimit := query.Limit
	shouldCache := ctx.searchCache != nil && len(query.Types) == 0
	if shouldCache {
		searchLimit = 50 // Cache up to 50 results
	}

	// Parse the search query to detect prefix/fuzzy modes
	parsedQuery := fts.ParseSearchQuery(searchTerm)
	typesToSearch := getTypesToSearch(query.Types)

	var wg sync.WaitGroup
	resultsChan := make(chan []query_models.SearchResultItem, len(typesToSearch))

	for _, entityType := range typesToSearch {
		wg.Add(1)
		go func(et string) {
			defer wg.Done()
			var results []query_models.SearchResultItem
			if ctx.ftsEnabled {
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

	var allResults []query_models.SearchResultItem
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
		ctx.searchCache.Set(strings.ToLower(searchTerm), allResults)
	}

	// Apply user's requested limit for response
	if len(allResults) > query.Limit {
		allResults = allResults[:query.Limit]
	}

	return &query_models.GlobalSearchResponse{
		Query:   searchTerm,
		Total:   len(allResults),
		Results: allResults,
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
		models.NoteType | models.ResourceCategory
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
}

func (ctx *MahresourcesContext) searchEntityType(entityType, searchTerm string, limit int) []query_models.SearchResultItem {
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
	}
	return nil
}

func (ctx *MahresourcesContext) searchEntityTypeFTS(entityType string, query fts.ParsedQuery, limit int) []query_models.SearchResultItem {
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
	}
	return nil
}

func (ctx *MahresourcesContext) getLikeOperator() string {
	if ctx.Config.DbType == constants.DbTypePosgres {
		return "ILIKE"
	}
	return "LIKE"
}

func calculateRelevanceScore(name, description, searchTerm string) int {
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
	if strings.Contains(strings.ToLower(description), termLower) {
		return 40
	}
	return 20
}

func truncateDescription(desc string, maxLen int) string {
	if len(desc) <= maxLen {
		return desc
	}
	return desc[:maxLen-3] + "..."
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

// searchEntitiesLike performs a LIKE-based search for any searchable entity type.
func searchEntitiesLike[T searchable](ctx *MahresourcesContext, entityType, searchTerm string, limit int) []query_models.SearchResultItem {
	info := entitySearchInfo[entityType]
	likeOp := ctx.getLikeOperator()
	pattern := "%" + searchTerm + "%"

	// Build WHERE clause: always search name and description, plus any extra columns
	whereParts := []string{
		"name " + likeOp + " ?",
		"description " + likeOp + " ?",
	}
	args := []any{pattern, pattern}
	for _, col := range info.extraLikeCols {
		whereParts = append(whereParts, col+" "+likeOp+" ?")
		args = append(args, pattern)
	}

	var entities []T
	ctx.db.
		Where(strings.Join(whereParts, " OR "), args...).
		Limit(limit).
		Find(&entities)

	results := make([]query_models.SearchResultItem, 0, len(entities))
	for _, e := range entities {
		id, name, description := entityFields(e)
		results = append(results, query_models.SearchResultItem{
			ID:          id,
			Type:        info.entityType,
			Name:        name,
			Description: truncateDescription(description, 100),
			Score:       calculateRelevanceScore(name, description, searchTerm),
			URL:         fmt.Sprintf(info.urlFormat, id),
			Extra:       entityExtra(e),
		})
	}
	return results
}

// searchEntitiesFTS performs an FTS-based search for any searchable entity type.
func searchEntitiesFTS[T searchable](ctx *MahresourcesContext, entityType string, query fts.ParsedQuery, limit int) []query_models.SearchResultItem {
	config := fts.GetEntityConfig(entityType)
	if config == nil {
		return nil
	}

	info := entitySearchInfo[entityType]

	var entities []T
	ctx.db.Model(new(T)).
		Scopes(ctx.ftsProvider.BuildSearchScope(config.TableName, config.Columns, query)).
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
			Name:        name,
			Description: truncateDescription(description, 100),
			Score:       score,
			URL:         fmt.Sprintf(info.urlFormat, id),
			Extra:       entityExtra(e),
		})
	}
	return results
}
