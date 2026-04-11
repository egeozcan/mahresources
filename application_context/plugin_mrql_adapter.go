package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"mahresources/models"
	"mahresources/mrql"
	"mahresources/plugin_system"
)

// pluginMRQLAdapter implements plugin_system.MRQLExecutor using MahresourcesContext.
type pluginMRQLAdapter struct {
	ctx *MahresourcesContext
}

func (a *pluginMRQLAdapter) ExecuteMRQL(reqCtx context.Context, query string, opts plugin_system.MRQLExecOptions) (*plugin_system.MRQLResult, error) {
	parsed, err := mrql.Parse(query)
	if err != nil {
		return nil, err
	}
	if err := mrql.Validate(parsed); err != nil {
		return nil, err
	}

	if opts.Limit > 0 {
		parsed.Limit = opts.Limit
	}

	entityType := mrql.ExtractEntityType(parsed)
	if entityType == mrql.EntityUnspecified {
		return nil, fmt.Errorf("MRQL query must specify an entity type (e.g. type=resource)")
	}
	parsed.EntityType = entityType

	// GROUP BY path
	if parsed.GroupBy != nil {
		if opts.Buckets > 0 {
			parsed.BucketLimit = opts.Buckets
		}
		grouped, err := a.ctx.ExecuteMRQLGroupedWithScope(reqCtx, parsed, opts.ScopeID)
		if err != nil {
			return nil, err
		}
		return a.convertGrouped(grouped), nil
	}

	// Flat path
	translateOpts := mrql.TranslateOptions{}
	result, err := a.ctx.ExecuteSingleEntityWithScope(reqCtx, parsed, entityType, translateOpts, opts.ScopeID)
	if err != nil {
		return nil, err
	}
	return a.convertFlat(result), nil
}

func (a *pluginMRQLAdapter) convertFlat(result *MRQLResult) *plugin_system.MRQLResult {
	pr := &plugin_system.MRQLResult{
		EntityType: result.EntityType,
		Mode:       "flat",
	}
	for _, r := range result.Resources {
		pr.Items = append(pr.Items, mrqlResourceToMap(&r))
	}
	for _, n := range result.Notes {
		pr.Items = append(pr.Items, mrqlNoteToMap(&n))
	}
	for _, g := range result.Groups {
		pr.Items = append(pr.Items, mrqlGroupToMap(&g))
	}
	return pr
}

func (a *pluginMRQLAdapter) convertGrouped(result *MRQLGroupedResult) *plugin_system.MRQLResult {
	pr := &plugin_system.MRQLResult{
		EntityType: result.EntityType,
	}
	if result.Mode == "aggregated" {
		pr.Mode = "aggregated"
		pr.Rows = result.Rows
		return pr
	}
	pr.Mode = "bucketed"
	for _, bucket := range result.Groups {
		group := plugin_system.MRQLResultGroup{Key: bucket.Key}
		switch items := bucket.Items.(type) {
		case []models.Resource:
			for i := range items {
				group.Items = append(group.Items, mrqlResourceToMap(&items[i]))
			}
		case []models.Note:
			for i := range items {
				group.Items = append(group.Items, mrqlNoteToMap(&items[i]))
			}
		case []models.Group:
			for i := range items {
				group.Items = append(group.Items, mrqlGroupToMap(&items[i]))
			}
		}
		pr.Groups = append(pr.Groups, group)
	}
	return pr
}

// mrqlResourceToMap converts a Resource to a map with lowercase/camelCase keys
// matching MRQL field naming conventions.
func mrqlResourceToMap(r *models.Resource) map[string]any {
	m := map[string]any{
		"id":           float64(r.ID),
		"name":         r.Name,
		"description":  r.Description,
		"contentType":  r.ContentType,
		"fileSize":     float64(r.FileSize),
		"width":        float64(r.Width),
		"height":       float64(r.Height),
		"originalName": r.OriginalName,
		"hash":         r.Hash,
		"category":     r.Category,
		"entity_type":  "resource",
		"createdAt":    r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"updatedAt":    r.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if r.OwnerId != nil {
		m["ownerId"] = float64(*r.OwnerId)
	}
	if len(r.Meta) > 0 {
		var meta map[string]any
		if err := json.Unmarshal(r.Meta, &meta); err == nil {
			m["meta"] = meta
		}
	}
	return m
}

// mrqlNoteToMap converts a Note to a map with lowercase/camelCase keys.
func mrqlNoteToMap(n *models.Note) map[string]any {
	m := map[string]any{
		"id":          float64(n.ID),
		"name":        n.Name,
		"description": n.Description,
		"entity_type": "note",
		"createdAt":   n.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"updatedAt":   n.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if n.OwnerId != nil {
		m["ownerId"] = float64(*n.OwnerId)
	}
	if n.StartDate != nil {
		m["startDate"] = n.StartDate.Format("2006-01-02T15:04:05Z07:00")
	}
	if n.EndDate != nil {
		m["endDate"] = n.EndDate.Format("2006-01-02T15:04:05Z07:00")
	}
	if len(n.Meta) > 0 {
		var meta map[string]any
		if err := json.Unmarshal(n.Meta, &meta); err == nil {
			m["meta"] = meta
		}
	}
	return m
}

// mrqlGroupToMap converts a Group to a map with lowercase/camelCase keys.
func mrqlGroupToMap(g *models.Group) map[string]any {
	m := map[string]any{
		"id":          float64(g.ID),
		"name":        g.Name,
		"description": g.Description,
		"entity_type": "group",
		"createdAt":   g.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"updatedAt":   g.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if g.OwnerId != nil {
		m["ownerId"] = float64(*g.OwnerId)
	}
	if g.URL != nil {
		u := url.URL(*g.URL)
		m["url"] = u.String()
	}
	if len(g.Meta) > 0 {
		var meta map[string]any
		if err := json.Unmarshal(g.Meta, &meta); err == nil {
			m["meta"] = meta
		}
	}
	return m
}

