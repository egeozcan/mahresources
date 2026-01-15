package application_context

import (
	"fmt"
	"mahresources/constants"
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
	EntityTypeNoteType     = "noteType"
)

var allEntityTypes = []string{
	EntityTypeResource, EntityTypeNote, EntityTypeGroup,
	EntityTypeTag, EntityTypeCategory, EntityTypeQuery,
	EntityTypeRelationType, EntityTypeNoteType,
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

	typesToSearch := getTypesToSearch(query.Types)

	var wg sync.WaitGroup
	resultsChan := make(chan []query_models.SearchResultItem, len(typesToSearch))

	for _, entityType := range typesToSearch {
		wg.Add(1)
		go func(et string) {
			defer wg.Done()
			results := ctx.searchEntityType(et, searchTerm, query.Limit)
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

	ctx.db.Preload("Category").
		Where("name "+likeOp+" ? OR description "+likeOp+" ?", pattern, pattern).
		Limit(limit).
		Find(&groups)

	results := make([]query_models.SearchResultItem, 0, len(groups))
	for _, g := range groups {
		extra := make(map[string]string)
		if g.Category != nil {
			extra["category"] = g.Category.Name
		}
		results = append(results, query_models.SearchResultItem{
			ID:          g.ID,
			Type:        EntityTypeGroup,
			Name:        g.Name,
			Description: truncateDescription(g.Description, 100),
			Score:       calculateRelevanceScore(g.Name, g.Description, searchTerm),
			URL:         fmt.Sprintf("/group?id=%d", g.ID),
			Extra:       extra,
		})
	}
	return results
}

func (ctx *MahresourcesContext) searchNotes(searchTerm string, limit int) []query_models.SearchResultItem {
	var notes []models.Note
	likeOp := ctx.getLikeOperator()
	pattern := "%" + searchTerm + "%"

	ctx.db.Preload("NoteType").
		Where("name "+likeOp+" ? OR description "+likeOp+" ?", pattern, pattern).
		Limit(limit).
		Find(&notes)

	results := make([]query_models.SearchResultItem, 0, len(notes))
	for _, n := range notes {
		extra := make(map[string]string)
		if n.NoteType != nil {
			extra["noteType"] = n.NoteType.Name
		}
		results = append(results, query_models.SearchResultItem{
			ID:          n.ID,
			Type:        EntityTypeNote,
			Name:        n.Name,
			Description: truncateDescription(n.Description, 100),
			Score:       calculateRelevanceScore(n.Name, n.Description, searchTerm),
			URL:         fmt.Sprintf("/note?id=%d", n.ID),
			Extra:       extra,
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
