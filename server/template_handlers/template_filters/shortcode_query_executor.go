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
		qr := convertGroupedResultItems(grouped, appCtx)
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
	qr.Items = convertResultItems(result, appCtx)

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
			item.CustomCSS = r.ResourceCategory.CustomCSS
			item.CategoryID = r.ResourceCategory.ID
		}
		// Populate scope fields
		if r.OwnerId != nil && *r.OwnerId > 0 {
			item.ScopeGroupID = *r.OwnerId
			item.ParentGroupID = appCtx.ResolveParentScopeID(*r.OwnerId)
			item.RootGroupID = appCtx.ResolveRootScopeID(*r.OwnerId)
		} else {
			item.ScopeGroupID = mrql.UnresolvedScopeSentinel
			item.ParentGroupID = mrql.UnresolvedScopeSentinel
			item.RootGroupID = mrql.UnresolvedScopeSentinel
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
			item.CustomCSS = n.NoteType.CustomCSS
			item.CategoryID = n.NoteType.ID
		}
		// Populate scope fields
		if n.OwnerId != nil && *n.OwnerId > 0 {
			item.ScopeGroupID = *n.OwnerId
			item.ParentGroupID = appCtx.ResolveParentScopeID(*n.OwnerId)
			item.RootGroupID = appCtx.ResolveRootScopeID(*n.OwnerId)
		} else {
			item.ScopeGroupID = mrql.UnresolvedScopeSentinel
			item.ParentGroupID = mrql.UnresolvedScopeSentinel
			item.RootGroupID = mrql.UnresolvedScopeSentinel
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
			item.CustomCSS = g.Category.CustomCSS
			item.CategoryID = g.Category.ID
		}
		// Groups are their own scope
		item.ScopeGroupID = g.ID
		if g.OwnerId != nil && *g.OwnerId > 0 {
			item.ParentGroupID = *g.OwnerId
		} else {
			item.ParentGroupID = mrql.UnresolvedScopeSentinel
		}
		item.RootGroupID = appCtx.ResolveRootScopeID(g.ID)
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
				if r.OwnerId != nil && *r.OwnerId > 0 {
					item.ScopeGroupID = *r.OwnerId
					item.ParentGroupID = appCtx.ResolveParentScopeID(*r.OwnerId)
					item.RootGroupID = appCtx.ResolveRootScopeID(*r.OwnerId)
				} else {
					item.ScopeGroupID = mrql.UnresolvedScopeSentinel
					item.ParentGroupID = mrql.UnresolvedScopeSentinel
					item.RootGroupID = mrql.UnresolvedScopeSentinel
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
				if n.OwnerId != nil && *n.OwnerId > 0 {
					item.ScopeGroupID = *n.OwnerId
					item.ParentGroupID = appCtx.ResolveParentScopeID(*n.OwnerId)
					item.RootGroupID = appCtx.ResolveRootScopeID(*n.OwnerId)
				} else {
					item.ScopeGroupID = mrql.UnresolvedScopeSentinel
					item.ParentGroupID = mrql.UnresolvedScopeSentinel
					item.RootGroupID = mrql.UnresolvedScopeSentinel
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
				item.ScopeGroupID = g.ID
				if g.OwnerId != nil && *g.OwnerId > 0 {
					item.ParentGroupID = *g.OwnerId
				} else {
					item.ParentGroupID = mrql.UnresolvedScopeSentinel
				}
				item.RootGroupID = appCtx.ResolveRootScopeID(g.ID)
				group.Items = append(group.Items, item)
			}
		}
		qr.Groups = append(qr.Groups, group)
	}

	return qr
}
