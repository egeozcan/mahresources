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
//
// When a per-page QueryBudget is attached to the request context (list-page
// renders wire one), the executor serves identical queries from the budget's
// result cache (free) and charges each cache miss against the budget. Once the
// budget is spent it refuses further misses with a budget error — rendered as
// the standard MRQL error box — and logs a single warning per page.
func BuildQueryExecutor(appCtx *application_context.MahresourcesContext) shortcodes.QueryExecutor {
	base := func(reqCtx context.Context, query string, opts shortcodes.QueryOptions) (*shortcodes.QueryResult, error) {
		return executeMRQLForShortcode(reqCtx, appCtx, query, opts)
	}
	return shortcodes.BudgetedExecutor(base, func(limit int) {
		logPageQueryBudgetExceeded(appCtx, limit)
	})
}

// pageQueryBudget returns the configured per-page inline-MRQL query budget,
// or 0 (disabled) when no application context is available.
func pageQueryBudget(appCtx *application_context.MahresourcesContext) int {
	if appCtx == nil {
		return 0
	}
	return appCtx.MRQLPageQueryBudget()
}

// logPageQueryBudgetExceeded records one warning per page render when the inline
// MRQL query budget is hit, reviewable at /logs (entity type "mrql").
func logPageQueryBudgetExceeded(appCtx *application_context.MahresourcesContext, limit int) {
	if appCtx == nil {
		return
	}
	appCtx.Logger().Warning(
		models.LogActionSystem,
		"mrql",
		nil,
		"",
		fmt.Sprintf("inline MRQL query budget exceeded (%d per page); some [mrql] shortcodes were not executed. Refine templates or raise -mrql-page-query-budget.", limit),
		map[string]interface{}{"budget": limit},
	)
}

// executeMRQLForShortcode runs an MRQL query and converts the result into shortcode types.
// It detects GROUP BY queries and routes them through ExecuteMRQLGrouped.
func executeMRQLForShortcode(reqCtx context.Context, appCtx *application_context.MahresourcesContext, query string, opts shortcodes.QueryOptions) (*shortcodes.QueryResult, error) {
	// Resolve saved query name to query string
	actualQuery := query
	var savedID uint
	if opts.SavedName != "" && query == "" {
		saved, err := appCtx.GetSavedMRQLQueryByName(opts.SavedName)
		if err != nil {
			return nil, err
		}
		actualQuery = saved.Query
		savedID = saved.ID
	}

	// Parse to detect GROUP BY
	parsed, err := mrql.Parse(actualQuery)
	if err != nil {
		return nil, err
	}
	// Bind $name placeholders (from param-<name> shortcode attrs) before validation.
	if err := mrql.BindParams(parsed, shortcodeParamsToAny(opts.Params)); err != nil {
		return nil, err
	}
	if err := mrql.Validate(parsed); err != nil {
		return nil, err
	}

	// Apply limit override
	if opts.Limit > 0 {
		parsed.Limit = opts.Limit
	}

	// Explicit SCOPE in the query text takes precedence over the shortcode scope
	// attr. When it does, the scope is already reproduced by the query text, so a
	// "view all" link must not append a second SCOPE clause.
	scopeGroupID := opts.ScopeGroupID
	explicitScope := parsed.Scope != nil
	if explicitScope {
		resolvedID, err := appCtx.ResolveMRQLScope(parsed)
		if err != nil {
			return nil, err
		}
		scopeGroupID = resolvedID
	}

	// View-all link fields. Prefer the saved deep-link (/mrql?saved=<id>) so the
	// saved-query identity survives — but only when there is no applied scope to
	// carry: ?saved=<id> opens the query globally, so a scoped saved query must
	// fall back to an inline ?q= link with the scope baked into the query text.
	// Inline queries get the scope spliced at the grammatically correct position
	// (SCOPE must precede GROUP BY / ORDER BY / LIMIT / OFFSET), which a naive
	// append would break.
	linkQuery := actualQuery
	linkSavedID := savedID
	if !explicitScope && scopeGroupID != 0 && scopeGroupID != mrql.UnresolvedScopeSentinel {
		linkQuery = mrql.InsertScopeClause(actualQuery, scopeGroupID)
		linkSavedID = 0
	}

	// GROUP BY queries use the grouped execution path
	if parsed.GroupBy != nil {
		entityType := mrql.ExtractEntityType(parsed)
		if entityType == mrql.EntityUnspecified {
			return nil, fmt.Errorf("GROUP BY requires an explicit entity type")
		}
		parsed.EntityType = entityType

		if opts.Buckets > 0 {
			parsed.BucketLimit = opts.Buckets
		}

		grouped, err := appCtx.ExecuteMRQLGroupedWithScope(reqCtx, parsed, scopeGroupID)
		if err != nil {
			return nil, err
		}
		qr, err := convertGroupedResultItems(reqCtx, grouped, appCtx)
		if err != nil {
			return nil, err
		}
		qr.EffectiveQuery = linkQuery
		qr.SavedID = linkSavedID
		return qr, nil
	}

	// Non-grouped: flat query using scoped execution
	result, err := appCtx.ExecuteMRQLScoped(reqCtx, parsed, scopeGroupID)
	if err != nil {
		return nil, err
	}

	qr := &shortcodes.QueryResult{
		EntityType:     result.EntityType,
		Mode:           "flat",
		EffectiveQuery: linkQuery,
		SavedID:        linkSavedID,
	}
	qr.Items, err = convertResultItems(reqCtx, result, appCtx)
	if err != nil {
		return nil, err
	}

	// A true total (ignoring limit) is only needed when the template references
	// {total}; it runs a second COUNT query over the same WHERE/scope.
	if opts.WantTotal {
		total, err := appCtx.CountMRQLScoped(reqCtx, parsed, scopeGroupID)
		if err != nil {
			return nil, err
		}
		qr.Total = &total
	}

	return qr, nil
}

// shortcodeParamsToAny converts a string-valued shortcode params map into the
// map[string]any that mrql.BindParams expects (values are lenient-coerced).
func shortcodeParamsToAny(params map[string]string) map[string]any {
	if len(params) == 0 {
		return nil
	}
	out := make(map[string]any, len(params))
	for k, v := range params {
		out[k] = v
	}
	return out
}

type mrqlRenderIDs struct {
	resourceCategories []uint
	noteTypes          []uint
	categories         []uint
	scopeGroups        []uint
}

func collectResultRenderIDs(result *application_context.MRQLResult, ids *mrqlRenderIDs) {
	for i := range result.Resources {
		r := &result.Resources[i]
		if r.ResourceCategory == nil {
			ids.resourceCategories = append(ids.resourceCategories, r.ResourceCategoryId)
		}
		if r.OwnerId != nil {
			ids.scopeGroups = append(ids.scopeGroups, *r.OwnerId)
		}
	}
	for i := range result.Notes {
		n := &result.Notes[i]
		if n.NoteType == nil && n.NoteTypeId != nil {
			ids.noteTypes = append(ids.noteTypes, *n.NoteTypeId)
		}
		if n.OwnerId != nil {
			ids.scopeGroups = append(ids.scopeGroups, *n.OwnerId)
		}
	}
	for i := range result.Groups {
		g := &result.Groups[i]
		if g.Category == nil && g.CategoryId != nil {
			ids.categories = append(ids.categories, *g.CategoryId)
		}
		ids.scopeGroups = append(ids.scopeGroups, g.ID)
	}
}

func loadResultRenderData(reqCtx context.Context, appCtx *application_context.MahresourcesContext, ids mrqlRenderIDs) (*application_context.MRQLRenderData, error) {
	return appCtx.LoadMRQLRenderData(reqCtx, ids.resourceCategories, ids.noteTypes, ids.categories, ids.scopeGroups)
}

// convertResultItems converts MRQLResult entities into QueryResultItems using
// batch-loaded scalar carriers and hierarchy data.
func convertResultItems(reqCtx context.Context, result *application_context.MRQLResult, appCtx *application_context.MahresourcesContext) ([]shortcodes.QueryResultItem, error) {
	var ids mrqlRenderIDs
	collectResultRenderIDs(result, &ids)
	data, err := loadResultRenderData(reqCtx, appCtx, ids)
	if err != nil {
		return nil, err
	}
	return convertResultItemsWithData(reqCtx, result, data)
}

func convertResultItemsWithData(reqCtx context.Context, result *application_context.MRQLResult, data *application_context.MRQLRenderData) ([]shortcodes.QueryResultItem, error) {
	items := make([]shortcodes.QueryResultItem, 0, len(result.Resources)+len(result.Notes)+len(result.Groups))
	sentinel := mrql.UnresolvedScopeSentinel

	for i := range result.Resources {
		if err := reqCtx.Err(); err != nil {
			return nil, err
		}
		r := &result.Resources[i]
		if r.ResourceCategory == nil {
			r.ResourceCategory = data.ResourceCategories[r.ResourceCategoryId]
		}
		item := shortcodes.QueryResultItem{EntityType: "resource", EntityID: r.ID, Entity: r, Meta: json.RawMessage(r.Meta)}
		if r.ResourceCategory != nil {
			item.MetaSchema = r.ResourceCategory.MetaSchema
			item.CustomMRQLResult = r.ResourceCategory.CustomMRQLResult
			item.CustomCSS = r.ResourceCategory.CustomCSS
			item.CategoryID = r.ResourceCategory.ID
		}
		item.ScopeGroupID, item.ParentGroupID, item.RootGroupID = sentinel, sentinel, sentinel
		if r.OwnerId != nil && *r.OwnerId > 0 {
			item.ScopeGroupID = *r.OwnerId
			if scope, ok := data.Scopes[*r.OwnerId]; ok {
				item.ParentGroupID, item.RootGroupID = scope.ParentGroupID, scope.RootGroupID
			}
		}
		items = append(items, item)
	}

	for i := range result.Notes {
		if err := reqCtx.Err(); err != nil {
			return nil, err
		}
		n := &result.Notes[i]
		if n.NoteType == nil && n.NoteTypeId != nil {
			n.NoteType = data.NoteTypes[*n.NoteTypeId]
		}
		item := shortcodes.QueryResultItem{EntityType: "note", EntityID: n.ID, Entity: n, Meta: json.RawMessage(n.Meta)}
		if n.NoteType != nil {
			item.MetaSchema = n.NoteType.MetaSchema
			item.CustomMRQLResult = n.NoteType.CustomMRQLResult
			item.CustomCSS = n.NoteType.CustomCSS
			item.CategoryID = n.NoteType.ID
		}
		item.ScopeGroupID, item.ParentGroupID, item.RootGroupID = sentinel, sentinel, sentinel
		if n.OwnerId != nil && *n.OwnerId > 0 {
			item.ScopeGroupID = *n.OwnerId
			if scope, ok := data.Scopes[*n.OwnerId]; ok {
				item.ParentGroupID, item.RootGroupID = scope.ParentGroupID, scope.RootGroupID
			}
		}
		items = append(items, item)
	}

	for i := range result.Groups {
		if err := reqCtx.Err(); err != nil {
			return nil, err
		}
		g := &result.Groups[i]
		if g.Category == nil && g.CategoryId != nil {
			g.Category = data.Categories[*g.CategoryId]
		}
		item := shortcodes.QueryResultItem{EntityType: "group", EntityID: g.ID, Entity: g, Meta: json.RawMessage(g.Meta), ScopeGroupID: g.ID, ParentGroupID: sentinel, RootGroupID: sentinel}
		if g.Category != nil {
			item.MetaSchema = g.Category.MetaSchema
			item.CustomMRQLResult = g.Category.CustomMRQLResult
			item.CustomCSS = g.Category.CustomCSS
			item.CategoryID = g.Category.ID
		}
		if g.OwnerId != nil && *g.OwnerId > 0 {
			item.ParentGroupID = *g.OwnerId
		}
		if scope, ok := data.Scopes[g.ID]; ok {
			item.RootGroupID = scope.RootGroupID
		}
		items = append(items, item)
	}
	return items, nil
}

// convertGroupedResultItems converts MRQLGroupedResult into QueryResult.
func convertGroupedResultItems(reqCtx context.Context, result *application_context.MRQLGroupedResult, appCtx *application_context.MahresourcesContext) (*shortcodes.QueryResult, error) {
	qr := &shortcodes.QueryResult{EntityType: result.EntityType}
	if result.Mode == "aggregated" {
		qr.Mode, qr.Rows = "aggregated", result.Rows
		return qr, nil
	}

	var ids mrqlRenderIDs
	for i := range result.Groups {
		switch items := result.Groups[i].Items.(type) {
		case []models.Resource:
			collectResultRenderIDs(&application_context.MRQLResult{Resources: items}, &ids)
		case []models.Note:
			collectResultRenderIDs(&application_context.MRQLResult{Notes: items}, &ids)
		case []models.Group:
			collectResultRenderIDs(&application_context.MRQLResult{Groups: items}, &ids)
		}
	}
	data, err := loadResultRenderData(reqCtx, appCtx, ids)
	if err != nil {
		return nil, err
	}

	qr.Mode = "bucketed"
	for i := range result.Groups {
		if err := reqCtx.Err(); err != nil {
			return nil, err
		}
		bucket := &result.Groups[i]
		group := shortcodes.QueryResultGroup{Key: bucket.Key}
		var flat application_context.MRQLResult
		switch items := bucket.Items.(type) {
		case []models.Resource:
			flat.Resources = items
		case []models.Note:
			flat.Notes = items
		case []models.Group:
			flat.Groups = items
		}
		group.Items, err = convertResultItemsWithData(reqCtx, &flat, data)
		if err != nil {
			return nil, err
		}
		qr.Groups = append(qr.Groups, group)
	}
	return qr, nil
}
