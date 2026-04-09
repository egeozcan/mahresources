package template_filters

import (
	"context"
	"encoding/json"
	"fmt"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/mrql"
	"mahresources/shortcodes"
)

// BuildQueryExecutor creates a QueryExecutor callback that uses the application
// context to execute MRQL queries. It preloads categories on result entities to
// extract CustomMRQLResult templates.
func BuildQueryExecutor(appCtx *application_context.MahresourcesContext) shortcodes.QueryExecutor {
	return func(reqCtx context.Context, query string, savedName string, limit int, buckets int) (*shortcodes.QueryResult, error) {
		return executeMRQLForShortcode(reqCtx, appCtx, query, savedName, limit, buckets)
	}
}

// executeMRQLForShortcode runs an MRQL query and converts the result into shortcode types.
// It detects GROUP BY queries and routes them through ExecuteMRQLGrouped.
func executeMRQLForShortcode(reqCtx context.Context, appCtx *application_context.MahresourcesContext, query string, savedName string, limit int, buckets int) (*shortcodes.QueryResult, error) {
	// Resolve saved query name to query string
	actualQuery := query
	if savedName != "" && query == "" {
		saved, err := appCtx.GetSavedMRQLQueryByName(savedName)
		if err != nil {
			return nil, err
		}
		actualQuery = saved.Query
	}

	// Parse to detect GROUP BY
	parsed, err := mrql.Parse(actualQuery)
	if err != nil {
		return nil, err
	}
	if err := mrql.Validate(parsed); err != nil {
		return nil, err
	}

	// Apply limit override
	if limit > 0 {
		parsed.Limit = limit
	}

	// GROUP BY queries use the grouped execution path
	if parsed.GroupBy != nil {
		entityType := mrql.ExtractEntityType(parsed)
		if entityType == mrql.EntityUnspecified {
			return nil, fmt.Errorf("GROUP BY requires an explicit entity type")
		}
		parsed.EntityType = entityType

		if buckets > 0 {
			parsed.BucketLimit = buckets
		}

		grouped, err := appCtx.ExecuteMRQLGrouped(reqCtx, parsed)
		if err != nil {
			return nil, err
		}
		return convertGroupedResultItems(grouped, appCtx), nil
	}

	// Non-grouped: flat query
	result, err := appCtx.ExecuteMRQL(reqCtx, actualQuery, limit, 0)
	if err != nil {
		return nil, err
	}

	qr := &shortcodes.QueryResult{
		EntityType: result.EntityType,
		Mode:       "flat",
	}
	qr.Items = convertResultItems(result, appCtx)

	return qr, nil
}

// convertResultItems converts MRQLResult entities into QueryResultItems with
// category information preloaded for CustomMRQLResult templates.
func convertResultItems(result *application_context.MRQLResult, appCtx *application_context.MahresourcesContext) []shortcodes.QueryResultItem {
	var items []shortcodes.QueryResultItem

	for i := range result.Resources {
		r := &result.Resources[i]
		// Preload category if not already loaded
		if r.ResourceCategory == nil && r.ResourceCategoryId > 0 {
			cat, err := appCtx.GetResourceCategory(r.ResourceCategoryId)
			if err == nil {
				r.ResourceCategory = cat
			}
		}
		item := shortcodes.QueryResultItem{
			EntityType: "resource",
			EntityID:   r.ID,
			Entity:     r,
			Meta:       json.RawMessage(r.Meta),
		}
		if r.ResourceCategory != nil {
			item.MetaSchema = r.ResourceCategory.MetaSchema
			item.CustomMRQLResult = r.ResourceCategory.CustomMRQLResult
		}
		items = append(items, item)
	}

	for i := range result.Notes {
		n := &result.Notes[i]
		if n.NoteType == nil && n.NoteTypeId != nil && *n.NoteTypeId > 0 {
			nt, err := appCtx.GetNoteType(*n.NoteTypeId)
			if err == nil {
				n.NoteType = nt
			}
		}
		item := shortcodes.QueryResultItem{
			EntityType: "note",
			EntityID:   n.ID,
			Entity:     n,
			Meta:       json.RawMessage(n.Meta),
		}
		if n.NoteType != nil {
			item.MetaSchema = n.NoteType.MetaSchema
			item.CustomMRQLResult = n.NoteType.CustomMRQLResult
		}
		items = append(items, item)
	}

	for i := range result.Groups {
		g := &result.Groups[i]
		if g.Category == nil && g.CategoryId != nil && *g.CategoryId > 0 {
			cat, err := appCtx.GetCategory(*g.CategoryId)
			if err == nil {
				g.Category = cat
			}
		}
		item := shortcodes.QueryResultItem{
			EntityType: "group",
			EntityID:   g.ID,
			Entity:     g,
			Meta:       json.RawMessage(g.Meta),
		}
		if g.Category != nil {
			item.MetaSchema = g.Category.MetaSchema
			item.CustomMRQLResult = g.Category.CustomMRQLResult
		}
		items = append(items, item)
	}

	return items
}

// convertGroupedResultItems converts MRQLGroupedResult into QueryResult.
func convertGroupedResultItems(result *application_context.MRQLGroupedResult, appCtx *application_context.MahresourcesContext) *shortcodes.QueryResult {
	qr := &shortcodes.QueryResult{
		EntityType: result.EntityType,
	}

	if result.Mode == "aggregated" {
		qr.Mode = "aggregated"
		qr.Rows = result.Rows
		return qr
	}

	// Bucketed mode
	qr.Mode = "bucketed"
	for _, bucket := range result.Groups {
		group := shortcodes.QueryResultGroup{
			Key: bucket.Key,
		}
		// Convert bucket items — they are typed entities
		switch items := bucket.Items.(type) {
		case []models.Resource:
			for i := range items {
				r := &items[i]
				if r.ResourceCategory == nil && r.ResourceCategoryId > 0 {
					cat, _ := appCtx.GetResourceCategory(r.ResourceCategoryId)
					if cat != nil {
						r.ResourceCategory = cat
					}
				}
				item := shortcodes.QueryResultItem{
					EntityType: "resource",
					EntityID:   r.ID,
					Entity:     r,
					Meta:       json.RawMessage(r.Meta),
				}
				if r.ResourceCategory != nil {
					item.MetaSchema = r.ResourceCategory.MetaSchema
					item.CustomMRQLResult = r.ResourceCategory.CustomMRQLResult
				}
				group.Items = append(group.Items, item)
			}
		case []models.Note:
			for i := range items {
				n := &items[i]
				if n.NoteType == nil && n.NoteTypeId != nil && *n.NoteTypeId > 0 {
					nt, _ := appCtx.GetNoteType(*n.NoteTypeId)
					if nt != nil {
						n.NoteType = nt
					}
				}
				item := shortcodes.QueryResultItem{
					EntityType: "note",
					EntityID:   n.ID,
					Entity:     n,
					Meta:       json.RawMessage(n.Meta),
				}
				if n.NoteType != nil {
					item.MetaSchema = n.NoteType.MetaSchema
					item.CustomMRQLResult = n.NoteType.CustomMRQLResult
				}
				group.Items = append(group.Items, item)
			}
		case []models.Group:
			for i := range items {
				g := &items[i]
				if g.Category == nil && g.CategoryId != nil && *g.CategoryId > 0 {
					cat, _ := appCtx.GetCategory(*g.CategoryId)
					if cat != nil {
						g.Category = cat
					}
				}
				item := shortcodes.QueryResultItem{
					EntityType: "group",
					EntityID:   g.ID,
					Entity:     g,
					Meta:       json.RawMessage(g.Meta),
				}
				if g.Category != nil {
					item.MetaSchema = g.Category.MetaSchema
					item.CustomMRQLResult = g.Category.CustomMRQLResult
				}
				group.Items = append(group.Items, item)
			}
		}
		qr.Groups = append(qr.Groups, group)
	}

	return qr
}
