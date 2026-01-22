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

// ftsProvider is the active FTS provider (nil if FTS is not initialized)
var ftsProvider fts.FTSProvider

// ftsEnabled indicates whether FTS is available
var ftsEnabled bool

// Entity type constants
const (
	EntityTypeResource     = "resource"
	EntityTypeNote         = "note"
	EntityTypeGroup        = "group"
	EntityTypeTag          = "tag"
	EntityTypeCategory     = "category"
	EntityTypeQuery        = "query"
	EntityTypeRelationType = "relationType"
	EntityTypeNoteType     = "noteType"
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
	EntityTypeRelationType, EntityTypeNoteType,
}

// InitFTS initializes the FTS provider based on the database type
func (ctx *MahresourcesContext) InitFTS() error {
	if ctx.Config.DbType == constants.DbTypePosgres {
		ftsProvider = fts.NewPostgresFTS()
	} else {
		ftsProvider = fts.NewSQLiteFTS()
	}

	if err := ftsProvider.Setup(ctx.db); err != nil {
		ftsProvider = nil
		ftsEnabled = false
		return err
	}

	ftsEnabled = true
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
			if ftsEnabled {
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
		ctx.searchCache.Set(searchTerm, allResults)
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

func (ctx *MahresourcesContext) searchEntityType(entityType, searchTerm string, limit int) []query_models.SearchResultItem {
	switch entityType {
	case EntityTypeGroup:
		return ctx.searchGroups(searchTerm, limit)
	case EntityTypeNote:
		return ctx.searchNotes(searchTerm, limit)
	case EntityTypeResource:
		return ctx.searchResources(searchTerm, limit)
	case EntityTypeTag:
		return ctx.searchTags(searchTerm, limit)
	case EntityTypeCategory:
		return ctx.searchCategories(searchTerm, limit)
	case EntityTypeQuery:
		return ctx.searchQueries(searchTerm, limit)
	case EntityTypeRelationType:
		return ctx.searchRelationTypes(searchTerm, limit)
	case EntityTypeNoteType:
		return ctx.searchNoteTypes(searchTerm, limit)
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

func (ctx *MahresourcesContext) searchGroups(searchTerm string, limit int) []query_models.SearchResultItem {
	var groups []models.Group
	likeOp := ctx.getLikeOperator()
	pattern := "%" + searchTerm + "%"

	ctx.db.
		Where("name "+likeOp+" ? OR description "+likeOp+" ?", pattern, pattern).
		Limit(limit).
		Find(&groups)

	results := make([]query_models.SearchResultItem, 0, len(groups))
	for _, g := range groups {
		results = append(results, query_models.SearchResultItem{
			ID:          g.ID,
			Type:        EntityTypeGroup,
			Name:        g.Name,
			Description: truncateDescription(g.Description, 100),
			Score:       calculateRelevanceScore(g.Name, g.Description, searchTerm),
			URL:         fmt.Sprintf("/group?id=%d", g.ID),
		})
	}
	return results
}

func (ctx *MahresourcesContext) searchNotes(searchTerm string, limit int) []query_models.SearchResultItem {
	var notes []models.Note
	likeOp := ctx.getLikeOperator()
	pattern := "%" + searchTerm + "%"

	ctx.db.
		Where("name "+likeOp+" ? OR description "+likeOp+" ?", pattern, pattern).
		Limit(limit).
		Find(&notes)

	results := make([]query_models.SearchResultItem, 0, len(notes))
	for _, n := range notes {
		results = append(results, query_models.SearchResultItem{
			ID:          n.ID,
			Type:        EntityTypeNote,
			Name:        n.Name,
			Description: truncateDescription(n.Description, 100),
			Score:       calculateRelevanceScore(n.Name, n.Description, searchTerm),
			URL:         fmt.Sprintf("/note?id=%d", n.ID),
		})
	}
	return results
}

func (ctx *MahresourcesContext) searchResources(searchTerm string, limit int) []query_models.SearchResultItem {
	var resources []models.Resource
	likeOp := ctx.getLikeOperator()
	pattern := "%" + searchTerm + "%"

	ctx.db.
		Where("name "+likeOp+" ? OR description "+likeOp+" ? OR original_name "+likeOp+" ?",
			pattern, pattern, pattern).
		Limit(limit).
		Find(&resources)

	results := make([]query_models.SearchResultItem, 0, len(resources))
	for _, r := range resources {
		extra := make(map[string]string)
		if r.ContentType != "" {
			extra["contentType"] = r.ContentType
		}
		results = append(results, query_models.SearchResultItem{
			ID:          r.ID,
			Type:        EntityTypeResource,
			Name:        r.Name,
			Description: truncateDescription(r.Description, 100),
			Score:       calculateRelevanceScore(r.Name, r.Description, searchTerm),
			URL:         fmt.Sprintf("/resource?id=%d", r.ID),
			Extra:       extra,
		})
	}
	return results
}

func (ctx *MahresourcesContext) searchTags(searchTerm string, limit int) []query_models.SearchResultItem {
	var tags []models.Tag
	likeOp := ctx.getLikeOperator()
	pattern := "%" + searchTerm + "%"

	ctx.db.
		Where("name "+likeOp+" ? OR description "+likeOp+" ?", pattern, pattern).
		Limit(limit).
		Find(&tags)

	results := make([]query_models.SearchResultItem, 0, len(tags))
	for _, t := range tags {
		results = append(results, query_models.SearchResultItem{
			ID:          t.ID,
			Type:        EntityTypeTag,
			Name:        t.Name,
			Description: truncateDescription(t.Description, 100),
			Score:       calculateRelevanceScore(t.Name, t.Description, searchTerm),
			URL:         fmt.Sprintf("/tag?id=%d", t.ID),
		})
	}
	return results
}

func (ctx *MahresourcesContext) searchCategories(searchTerm string, limit int) []query_models.SearchResultItem {
	var categories []models.Category
	likeOp := ctx.getLikeOperator()
	pattern := "%" + searchTerm + "%"

	ctx.db.
		Where("name "+likeOp+" ? OR description "+likeOp+" ?", pattern, pattern).
		Limit(limit).
		Find(&categories)

	results := make([]query_models.SearchResultItem, 0, len(categories))
	for _, c := range categories {
		results = append(results, query_models.SearchResultItem{
			ID:          c.ID,
			Type:        EntityTypeCategory,
			Name:        c.Name,
			Description: truncateDescription(c.Description, 100),
			Score:       calculateRelevanceScore(c.Name, c.Description, searchTerm),
			URL:         fmt.Sprintf("/category?id=%d", c.ID),
		})
	}
	return results
}

func (ctx *MahresourcesContext) searchQueries(searchTerm string, limit int) []query_models.SearchResultItem {
	var queries []models.Query
	likeOp := ctx.getLikeOperator()
	pattern := "%" + searchTerm + "%"

	ctx.db.
		Where("name "+likeOp+" ? OR description "+likeOp+" ?", pattern, pattern).
		Limit(limit).
		Find(&queries)

	results := make([]query_models.SearchResultItem, 0, len(queries))
	for _, q := range queries {
		results = append(results, query_models.SearchResultItem{
			ID:          q.ID,
			Type:        EntityTypeQuery,
			Name:        q.Name,
			Description: truncateDescription(q.Description, 100),
			Score:       calculateRelevanceScore(q.Name, q.Description, searchTerm),
			URL:         fmt.Sprintf("/query?id=%d", q.ID),
		})
	}
	return results
}

func (ctx *MahresourcesContext) searchRelationTypes(searchTerm string, limit int) []query_models.SearchResultItem {
	var relationTypes []models.GroupRelationType
	likeOp := ctx.getLikeOperator()
	pattern := "%" + searchTerm + "%"

	ctx.db.
		Where("name "+likeOp+" ? OR description "+likeOp+" ?", pattern, pattern).
		Limit(limit).
		Find(&relationTypes)

	results := make([]query_models.SearchResultItem, 0, len(relationTypes))
	for _, rt := range relationTypes {
		results = append(results, query_models.SearchResultItem{
			ID:          rt.ID,
			Type:        EntityTypeRelationType,
			Name:        rt.Name,
			Description: truncateDescription(rt.Description, 100),
			Score:       calculateRelevanceScore(rt.Name, rt.Description, searchTerm),
			URL:         fmt.Sprintf("/relationType?id=%d", rt.ID),
		})
	}
	return results
}

func (ctx *MahresourcesContext) searchNoteTypes(searchTerm string, limit int) []query_models.SearchResultItem {
	var noteTypes []models.NoteType
	likeOp := ctx.getLikeOperator()
	pattern := "%" + searchTerm + "%"

	ctx.db.
		Where("name "+likeOp+" ? OR description "+likeOp+" ?", pattern, pattern).
		Limit(limit).
		Find(&noteTypes)

	results := make([]query_models.SearchResultItem, 0, len(noteTypes))
	for _, nt := range noteTypes {
		results = append(results, query_models.SearchResultItem{
			ID:          nt.ID,
			Type:        EntityTypeNoteType,
			Name:        nt.Name,
			Description: truncateDescription(nt.Description, 100),
			Score:       calculateRelevanceScore(nt.Name, nt.Description, searchTerm),
			URL:         fmt.Sprintf("/noteType?id=%d", nt.ID),
		})
	}
	return results
}

// =============================================================================
// FTS-based search functions
// =============================================================================

func (ctx *MahresourcesContext) searchEntityTypeFTS(entityType string, query fts.ParsedQuery, limit int) []query_models.SearchResultItem {
	switch entityType {
	case EntityTypeResource:
		return ctx.searchResourcesFTS(query, limit)
	case EntityTypeNote:
		return ctx.searchNotesFTS(query, limit)
	case EntityTypeGroup:
		return ctx.searchGroupsFTS(query, limit)
	case EntityTypeTag:
		return ctx.searchTagsFTS(query, limit)
	case EntityTypeCategory:
		return ctx.searchCategoriesFTS(query, limit)
	case EntityTypeQuery:
		return ctx.searchQueriesFTS(query, limit)
	case EntityTypeRelationType:
		return ctx.searchRelationTypesFTS(query, limit)
	case EntityTypeNoteType:
		return ctx.searchNoteTypesFTS(query, limit)
	}
	return nil
}

func (ctx *MahresourcesContext) searchResourcesFTS(query fts.ParsedQuery, limit int) []query_models.SearchResultItem {
	config := fts.GetEntityConfig(EntityTypeResource)
	if config == nil {
		return nil
	}

	var resources []models.Resource
	db := ctx.db.Model(&models.Resource{}).
		Scopes(ftsProvider.BuildSearchScope(config.TableName, config.Columns, query)).
		Limit(limit)

	db.Find(&resources)

	results := make([]query_models.SearchResultItem, 0, len(resources))
	for i, r := range resources {
		extra := make(map[string]string)
		if r.ContentType != "" {
			extra["contentType"] = r.ContentType
		}
		// Use position-based scoring since results are ordered by relevance
		score := 100 - i
		if score < 1 {
			score = 1
		}
		results = append(results, query_models.SearchResultItem{
			ID:          r.ID,
			Type:        EntityTypeResource,
			Name:        r.Name,
			Description: truncateDescription(r.Description, 100),
			Score:       score,
			URL:         fmt.Sprintf("/resource?id=%d", r.ID),
			Extra:       extra,
		})
	}
	return results
}

func (ctx *MahresourcesContext) searchNotesFTS(query fts.ParsedQuery, limit int) []query_models.SearchResultItem {
	config := fts.GetEntityConfig(EntityTypeNote)
	if config == nil {
		return nil
	}

	var notes []models.Note
	db := ctx.db.Model(&models.Note{}).
		Scopes(ftsProvider.BuildSearchScope(config.TableName, config.Columns, query)).
		Limit(limit)

	db.Find(&notes)

	results := make([]query_models.SearchResultItem, 0, len(notes))
	for i, n := range notes {
		score := 100 - i
		if score < 1 {
			score = 1
		}
		results = append(results, query_models.SearchResultItem{
			ID:          n.ID,
			Type:        EntityTypeNote,
			Name:        n.Name,
			Description: truncateDescription(n.Description, 100),
			Score:       score,
			URL:         fmt.Sprintf("/note?id=%d", n.ID),
		})
	}
	return results
}

func (ctx *MahresourcesContext) searchGroupsFTS(query fts.ParsedQuery, limit int) []query_models.SearchResultItem {
	config := fts.GetEntityConfig(EntityTypeGroup)
	if config == nil {
		return nil
	}

	var groups []models.Group
	db := ctx.db.Model(&models.Group{}).
		Scopes(ftsProvider.BuildSearchScope(config.TableName, config.Columns, query)).
		Limit(limit)

	db.Find(&groups)

	results := make([]query_models.SearchResultItem, 0, len(groups))
	for i, g := range groups {
		score := 100 - i
		if score < 1 {
			score = 1
		}
		results = append(results, query_models.SearchResultItem{
			ID:          g.ID,
			Type:        EntityTypeGroup,
			Name:        g.Name,
			Description: truncateDescription(g.Description, 100),
			Score:       score,
			URL:         fmt.Sprintf("/group?id=%d", g.ID),
		})
	}
	return results
}

func (ctx *MahresourcesContext) searchTagsFTS(query fts.ParsedQuery, limit int) []query_models.SearchResultItem {
	config := fts.GetEntityConfig(EntityTypeTag)
	if config == nil {
		return nil
	}

	var tags []models.Tag
	db := ctx.db.Model(&models.Tag{}).
		Scopes(ftsProvider.BuildSearchScope(config.TableName, config.Columns, query)).
		Limit(limit)

	db.Find(&tags)

	results := make([]query_models.SearchResultItem, 0, len(tags))
	for i, t := range tags {
		score := 100 - i
		if score < 1 {
			score = 1
		}
		results = append(results, query_models.SearchResultItem{
			ID:          t.ID,
			Type:        EntityTypeTag,
			Name:        t.Name,
			Description: truncateDescription(t.Description, 100),
			Score:       score,
			URL:         fmt.Sprintf("/tag?id=%d", t.ID),
		})
	}
	return results
}

func (ctx *MahresourcesContext) searchCategoriesFTS(query fts.ParsedQuery, limit int) []query_models.SearchResultItem {
	config := fts.GetEntityConfig(EntityTypeCategory)
	if config == nil {
		return nil
	}

	var categories []models.Category
	db := ctx.db.Model(&models.Category{}).
		Scopes(ftsProvider.BuildSearchScope(config.TableName, config.Columns, query)).
		Limit(limit)

	db.Find(&categories)

	results := make([]query_models.SearchResultItem, 0, len(categories))
	for i, c := range categories {
		score := 100 - i
		if score < 1 {
			score = 1
		}
		results = append(results, query_models.SearchResultItem{
			ID:          c.ID,
			Type:        EntityTypeCategory,
			Name:        c.Name,
			Description: truncateDescription(c.Description, 100),
			Score:       score,
			URL:         fmt.Sprintf("/category?id=%d", c.ID),
		})
	}
	return results
}

func (ctx *MahresourcesContext) searchQueriesFTS(query fts.ParsedQuery, limit int) []query_models.SearchResultItem {
	config := fts.GetEntityConfig(EntityTypeQuery)
	if config == nil {
		return nil
	}

	var queries []models.Query
	db := ctx.db.Model(&models.Query{}).
		Scopes(ftsProvider.BuildSearchScope(config.TableName, config.Columns, query)).
		Limit(limit)

	db.Find(&queries)

	results := make([]query_models.SearchResultItem, 0, len(queries))
	for i, q := range queries {
		score := 100 - i
		if score < 1 {
			score = 1
		}
		results = append(results, query_models.SearchResultItem{
			ID:          q.ID,
			Type:        EntityTypeQuery,
			Name:        q.Name,
			Description: truncateDescription(q.Description, 100),
			Score:       score,
			URL:         fmt.Sprintf("/query?id=%d", q.ID),
		})
	}
	return results
}

func (ctx *MahresourcesContext) searchRelationTypesFTS(query fts.ParsedQuery, limit int) []query_models.SearchResultItem {
	config := fts.GetEntityConfig(EntityTypeRelationType)
	if config == nil {
		return nil
	}

	var relationTypes []models.GroupRelationType
	db := ctx.db.Model(&models.GroupRelationType{}).
		Scopes(ftsProvider.BuildSearchScope(config.TableName, config.Columns, query)).
		Limit(limit)

	db.Find(&relationTypes)

	results := make([]query_models.SearchResultItem, 0, len(relationTypes))
	for i, rt := range relationTypes {
		score := 100 - i
		if score < 1 {
			score = 1
		}
		results = append(results, query_models.SearchResultItem{
			ID:          rt.ID,
			Type:        EntityTypeRelationType,
			Name:        rt.Name,
			Description: truncateDescription(rt.Description, 100),
			Score:       score,
			URL:         fmt.Sprintf("/relationType?id=%d", rt.ID),
		})
	}
	return results
}

func (ctx *MahresourcesContext) searchNoteTypesFTS(query fts.ParsedQuery, limit int) []query_models.SearchResultItem {
	config := fts.GetEntityConfig(EntityTypeNoteType)
	if config == nil {
		return nil
	}

	var noteTypes []models.NoteType
	db := ctx.db.Model(&models.NoteType{}).
		Scopes(ftsProvider.BuildSearchScope(config.TableName, config.Columns, query)).
		Limit(limit)

	db.Find(&noteTypes)

	results := make([]query_models.SearchResultItem, 0, len(noteTypes))
	for i, nt := range noteTypes {
		score := 100 - i
		if score < 1 {
			score = 1
		}
		results = append(results, query_models.SearchResultItem{
			ID:          nt.ID,
			Type:        EntityTypeNoteType,
			Name:        nt.Name,
			Description: truncateDescription(nt.Description, 100),
			Score:       score,
			URL:         fmt.Sprintf("/noteType?id=%d", nt.ID),
		})
	}
	return results
}
